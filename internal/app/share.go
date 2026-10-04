package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// ShareContent holds files prepared on the controller for sending.
type ShareContent interface {
	// Files lists a complete prepared transfer with each file's SHA-256.
	Files(transfer string) ([]domain.ShareFile, error)
	// Open reads the file at a position of the list.
	Open(transfer string, index int) (io.ReadCloser, error)
}

// ShareManager sends prepared files to students' desktops. Like a lock, it
// changes only the students' sessions, so it does not take the
// administrative operation lock.
type ShareManager struct {
	source  LockSource
	content ShareContent
	now     func() time.Time
}

func NewShareManager(source LockSource, content ShareContent) *ShareManager {
	return &ShareManager{source, content, time.Now}
}

// shareParallel bounds simultaneous sendings: each one streams the files.
const shareParallel = 4

func (m *ShareManager) Plan(ctx context.Context, repository, requested, transfer string) domain.SharePlan {
	p := domain.SharePlan{SchemaVersion: domain.SchemaVersion, Operation: "share-plan", State: "blocked", Requested: requested, Transfer: transfer, Files: []domain.ShareFile{}, Targets: []domain.ShareTarget{}, Issues: []domain.ValidationIssue{}}
	fail := func(message string) domain.SharePlan {
		p.Message = message
		p.Issues = append(p.Issues, domain.ValidationIssue{Field: "share", Message: message})
		return p
	}
	var err error
	p.Repository, err = filepath.Abs(repository)
	if err != nil {
		return fail(err.Error())
	}
	files, err := m.content.Files(transfer)
	if err != nil {
		return fail("The files to send are not ready; prepare them again.")
	}
	p.Files = files
	for _, file := range files {
		p.Bytes += file.Size
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		return fail(err.Error())
	}
	selected, normalized, issues := selectDeploymentTargets(meta.Clients.Hosts, requested)
	if len(issues) > 0 {
		return fail(issues[0])
	}
	p.Requested = normalized
	hosts := shutdownHostMeta(selected)
	for _, host := range hosts {
		if host.Name == meta.Controller.Name {
			return fail("The controller cannot be selected.")
		}
	}
	p.Targets = make([]domain.ShareTarget, len(hosts))
	eachHost(hosts, lockParallel, func(index int, host domain.HostMeta) {
		target := domain.ShareTarget{HostMeta: host}
		session, err := m.source.Connect(ctx, host)
		if err != nil {
			target.Detail = lockUnavailable(err)
		} else {
			target.Eligible = true
			_ = session.Close()
		}
		p.Targets[index] = target
	})
	eligible := 0
	for _, target := range p.Targets {
		if target.Eligible {
			eligible++
		}
	}
	if eligible == 0 {
		return fail("No selected computer has someone signed in with the classroom view. Check that the computers are on and updated.")
	}
	p.State = "ready"
	p.ExpiresAt = m.now().UTC().Truncate(5 * time.Minute).Add(5 * time.Minute)
	p.Message = fmt.Sprintf("%s (%s) go to the desktop of %d of %d computers. A restart empties the student's home.", shareCount(files), shareSize(p.Bytes), eligible, len(p.Targets))
	p.ReviewToken = domain.ShareReviewToken(p)
	return p
}

func (m *ShareManager) Apply(ctx context.Context, p domain.SharePlan, token string) domain.ShareReport {
	r := domain.ShareReport{SchemaVersion: domain.SchemaVersion, Operation: "share-apply", State: "blocked", Targets: []domain.ShareOutcome{}}
	if p.HasErrors() || token == "" || token != p.ReviewToken || token != domain.ShareReviewToken(p) || !m.now().Before(p.ExpiresAt) {
		r.Message = "Review expired or changed; create a fresh plan."
		return r
	}
	// The prepared copy must still be exactly the reviewed files.
	files, err := m.content.Files(p.Transfer)
	if err != nil || !sameShareFiles(files, p.Files) {
		r.Message = "The prepared files changed or expired; prepare them again."
		return r
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		r.Message = err.Error()
		return r
	}
	identities := map[string]string{}
	for _, host := range meta.Clients.Hosts {
		identities[host.Name] = host.IP
	}
	hosts := make([]domain.HostMeta, len(p.Targets))
	for index, target := range p.Targets {
		if target.Name == meta.Controller.Name || identities[target.Name] != target.IP {
			r.Message = "Client inventory changed; review again."
			return r
		}
		hosts[index] = target.HostMeta
	}
	entries := make([]classroomview.FileEntry, len(files))
	for index, file := range files {
		entries[index] = classroomview.FileEntry{Path: file.Path, Size: file.Size, Dir: file.Dir}
	}
	outcomes := make([]domain.ShareOutcome, len(hosts))
	eachHost(hosts, shareParallel, func(index int, host domain.HostMeta) {
		outcome := domain.ShareOutcome{Name: host.Name, State: "not-sent", Detail: "Unavailable in the reviewed plan; nothing was sent."}
		if p.Targets[index].Eligible {
			outcome = m.send(ctx, host, p.Transfer, entries)
		}
		outcomes[index] = outcome
	})
	delivered := 0
	for _, outcome := range outcomes {
		if outcome.State == "delivered" {
			delivered++
		}
	}
	r.Targets = outcomes
	r.State = "partial"
	if delivered == len(outcomes) {
		r.State = "completed"
	}
	r.Message = fmt.Sprintf("Delivered to %d of %d selected computers.", delivered, len(outcomes))
	return r
}

func (m *ShareManager) send(ctx context.Context, host domain.HostMeta, transfer string, entries []classroomview.FileEntry) domain.ShareOutcome {
	outcome := domain.ShareOutcome{Name: host.Name, State: "not-sent"}
	session, err := m.source.Connect(ctx, host)
	if err != nil {
		outcome.Detail = lockUnavailable(err)
		return outcome
	}
	defer session.Close()
	placed, err := session.SendFiles(entries, func(index int) (io.ReadCloser, error) {
		return m.content.Open(transfer, index)
	})
	var agentError classroomview.AgentError
	switch {
	case err == nil:
		outcome.State = "delivered"
		outcome.Detail = "On the desktop: " + strings.Join(placed, ", ") + "."
	case errors.As(err, &agentError):
		outcome.State = "failed"
		outcome.Detail = "The computer refused the files; nothing was placed on its desktop."
	default:
		// The files are placed only at the very end, so an interruption
		// usually leaves the desktop unchanged, but this cannot be confirmed.
		outcome.State = "unconfirmed"
		outcome.Detail = "The sending was interrupted; check the desktop before sending again."
	}
	return outcome
}

func sameShareFiles(left, right []domain.ShareFile) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func shareCount(files []domain.ShareFile) string {
	count := 0
	for _, file := range files {
		if !strings.Contains(file.Path, "/") {
			count++
		}
	}
	if count == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", count)
}

func shareSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", bytes)
}

func eachHost(hosts []domain.HostMeta, parallel int, work func(int, domain.HostMeta)) {
	var group sync.WaitGroup
	slots := make(chan struct{}, parallel)
	for index, host := range hosts {
		group.Add(1)
		slots <- struct{}{}
		go func() {
			defer func() { <-slots; group.Done() }()
			work(index, host)
		}()
	}
	group.Wait()
}
