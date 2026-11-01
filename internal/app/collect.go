package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// collectReviewLifetime is how long a confirmed collect review stays valid at least.
const collectReviewLifetime = 10 * time.Minute

// CollectContent keeps collected files on the controller until whoever
// asked copies them into a folder of their own.
type CollectContent interface {
	// Begin reserves a collection whose files go into a folder named label.
	Begin(label string) (string, error)
	// Store keeps the files of one computer of a collection; fill hands
	// over each file's pieces in order. When it fails, nothing of that
	// computer is kept.
	Store(collection, computer string, entries []classroomview.FileEntry, fill func(write func(index int, data []byte) error) error) error
	// Remove forgets a collection and its files.
	Remove(collection string)
}

// ErrCollectNoSpace means the controller lacks the free space to keep a
// computer's files.
var ErrCollectNoSpace = errors.New("the controller has too little free space")

// CollectManager copies the files of students' desktops to the controller.
// Like sending files, it changes nothing on the students' computers and
// does not take the administrative operation lock.
type CollectManager struct {
	source  LockSource
	content CollectContent
	now     func() time.Time
}

func NewCollectManager(source LockSource, content CollectContent) *CollectManager {
	return &CollectManager{source, content, time.Now}
}

// collectParallel bounds the computers whose files stream at the same time.
const collectParallel = 4

// collectFolderLayout names a collection's folder by its date and time.
const collectFolderLayout = "2006-01-02 15.04"

func (m *CollectManager) Plan(ctx context.Context, repository, requested, destination string) domain.CollectPlan {
	p := domain.CollectPlan{SchemaVersion: domain.SchemaVersion, Operation: "collect-plan", State: "blocked", Requested: requested, Destination: destination, Targets: []domain.CollectTarget{}, Issues: []domain.ValidationIssue{}}
	fail := func(message string) domain.CollectPlan {
		p.Message = message
		p.Issues = append(p.Issues, domain.ValidationIssue{Field: "collect", Message: message})
		return p
	}
	if destination == "" {
		return fail("Choose the folder the collected files go to.")
	}
	var err error
	p.Repository, err = filepath.Abs(repository)
	if err != nil {
		return fail(err.Error())
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
	p.Targets = make([]domain.CollectTarget, len(hosts))
	eachHost(hosts, lockParallel, func(index int, host domain.HostMeta) {
		p.Targets[index] = m.look(ctx, host)
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
	// The expiry sits on the shared review grid so Reviewed can rebuild it.
	p.ExpiresAt = reviewExpiry(m.now(), collectReviewLifetime)
	p.Message = fmt.Sprintf("The desktop files of %d of %d computers are copied to %s, one folder per computer. Hidden files and links are left out; nothing changes on the students' computers.", eligible, len(p.Targets), destination)
	p.ReviewToken = domain.CollectReviewToken(p)
	return p
}

// look tells whether a computer can be collected from and what its
// desktop holds now; the desktop is read again when the review is applied.
func (m *CollectManager) look(ctx context.Context, host domain.HostMeta) domain.CollectTarget {
	target := domain.CollectTarget{HostMeta: host}
	session, err := m.source.Connect(ctx, host)
	if err != nil {
		target.Detail = lockUnavailable(err)
		return target
	}
	defer session.Close()
	collector, ok := session.(classroomview.Collector)
	if !ok {
		target.Detail = collectOutdated
		return target
	}
	entries, skipped, err := collector.ListDesktop()
	if agentCode(err) == classroomview.CodeUnsupported {
		target.Detail = collectOutdated
		return target
	}
	target.Eligible = true
	defer func() { _ = collector.EndCollect() }()
	switch {
	case agentCode(err) == classroomview.CodeCollectTooMuch:
		target.Detail = collectTooMuch
	case err != nil:
		target.Detail = "The desktop could not be read now."
	case errors.Is(classroomview.ValidateCollectEntries(entries), classroomview.ErrCollectTooMuch):
		target.Detail = collectTooMuch
	case len(entries) == 0:
		target.Detail = collectNothing
	default:
		files, bytes := collectSize(entries)
		target.Detail = collectDescription(files, bytes, skipped)
	}
	return target
}

const (
	collectOutdated = "The classroom view on this computer cannot collect files yet; update the computer."
	collectTooMuch  = "Too much to collect: more than 2000 items or 500 MB on the desktop."
	collectNothing  = "Nothing on the desktop."
)

func (m *CollectManager) Apply(ctx context.Context, p domain.CollectPlan, token string) domain.CollectReport {
	r := domain.CollectReport{SchemaVersion: domain.SchemaVersion, Operation: "collect-apply", State: "blocked", Targets: []domain.CollectOutcome{}}
	if p.HasErrors() || token == "" || token != p.ReviewToken || token != domain.CollectReviewToken(p) || !m.now().Before(p.ExpiresAt) {
		r.Message = "Review expired or changed; create a fresh plan."
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
	folder := m.now().Local().Format(collectFolderLayout)
	collection, err := m.content.Begin(folder)
	if err != nil {
		r.Message = "The collected files cannot be kept on the controller: " + err.Error() + "."
		return r
	}
	outcomes := make([]domain.CollectOutcome, len(hosts))
	eachHost(hosts, collectParallel, func(index int, host domain.HostMeta) {
		outcome := domain.CollectOutcome{Name: host.Name, State: domain.CollectNotSent, Detail: "Unavailable in the reviewed plan; nothing was collected."}
		if p.Targets[index].Eligible {
			outcome = m.collect(ctx, host, collection)
		}
		outcomes[index] = outcome
	})
	collected, finished := 0, 0
	for _, outcome := range outcomes {
		switch outcome.State {
		case domain.CollectCollected:
			collected++
			finished++
		case domain.CollectEmpty:
			finished++
		}
	}
	r.Targets = outcomes
	r.State = "partial"
	if finished == len(outcomes) {
		r.State = "completed"
	}
	if collected == 0 {
		m.content.Remove(collection)
	} else {
		r.Save, r.Folder = collection, folder
	}
	r.Message = fmt.Sprintf("Collected from %d of %d selected computers.", collected, len(outcomes))
	return r
}

// collect copies one computer's desktop into the collection; when anything
// goes wrong, nothing of that computer is kept.
func (m *CollectManager) collect(ctx context.Context, host domain.HostMeta, collection string) domain.CollectOutcome {
	outcome := domain.CollectOutcome{Name: host.Name, State: domain.CollectNotSent}
	session, err := m.source.Connect(ctx, host)
	if err != nil {
		outcome.Detail = lockUnavailable(err)
		return outcome
	}
	defer session.Close()
	outcome.State = domain.CollectFailed
	collector, ok := session.(classroomview.Collector)
	if !ok {
		outcome.Detail = collectOutdated
		return outcome
	}
	entries, skipped, err := collector.ListDesktop()
	switch code := agentCode(err); {
	case code == classroomview.CodeUnsupported:
		outcome.Detail = collectOutdated
		return outcome
	case code == classroomview.CodeCollectTooMuch:
		outcome.State, outcome.Detail = domain.CollectTooMuch, collectTooMuch+" Nothing was collected."
		return outcome
	case err != nil:
		outcome.Detail = "The desktop could not be read; nothing was collected."
		return outcome
	}
	defer func() { _ = collector.EndCollect() }()
	if err := classroomview.ValidateCollectEntries(entries); err != nil {
		if errors.Is(err, classroomview.ErrCollectTooMuch) {
			outcome.State, outcome.Detail = domain.CollectTooMuch, collectTooMuch+" Nothing was collected."
		} else {
			outcome.Detail = "The computer sent file names that cannot be used; nothing was collected."
		}
		return outcome
	}
	if len(entries) == 0 {
		outcome.State, outcome.Detail = domain.CollectEmpty, collectNothing
		return outcome
	}
	err = m.content.Store(collection, host.Name, entries, func(write func(int, []byte) error) error {
		for index, entry := range entries {
			if entry.Dir || entry.Size == 0 {
				continue
			}
			if err := collector.ReadDesktopFile(index, entry.Size, func(data []byte) error { return write(index, data) }); err != nil {
				return err
			}
		}
		return nil
	})
	switch {
	case errors.Is(err, ErrCollectNoSpace):
		outcome.Detail = "The controller has too little free space for these files; nothing was collected. Ask the administrator to free disk space. Code: DISK-LOW."
		return outcome
	case agentCode(err) != "":
		outcome.Detail = "A file changed or could not be read while it was copied; nothing was collected. Try again."
		return outcome
	case err != nil:
		outcome.Detail = "The copy was interrupted; nothing was collected from this computer."
		return outcome
	}
	// The student's screen tells that the desktop was collected, folders
	// and empty files included; a review never reaches this point. A
	// computer that cannot show it keeps its collection.
	_ = session.Notice(classroomview.NoticeCollect)
	files, bytes := collectSize(entries)
	outcome.State, outcome.Items, outcome.Bytes = domain.CollectCollected, files, bytes
	outcome.Detail = collectDescription(files, bytes, skipped)
	return outcome
}

func agentCode(err error) string {
	var agentError classroomview.AgentError
	if errors.As(err, &agentError) {
		return agentError.Code
	}
	return ""
}

func collectSize(entries []classroomview.FileEntry) (int, int64) {
	files, bytes := 0, int64(0)
	for _, entry := range entries {
		if !entry.Dir {
			files++
			bytes += entry.Size
		}
	}
	return files, bytes
}

// collectDescription words what a desktop holds, such as "3 files (1.2 MB).".
func collectDescription(files int, bytes int64, skipped int) string {
	text := "Only empty folders."
	switch {
	case files == 1:
		text = "1 file (" + shareSize(bytes) + ")."
	case files > 1:
		text = fmt.Sprintf("%d files (%s).", files, shareSize(bytes))
	}
	switch {
	case skipped == 1:
		text += " 1 item with an unusual name is left out."
	case skipped > 1:
		text += fmt.Sprintf(" %d items with unusual names are left out.", skipped)
	}
	return text
}

// ReviewedCollect tells whether token confirms a review of the plan fresh, a
// plan built at now. A match returns fresh carrying that review's expiry and
// token, so an apply in a later process, or in a later grid step, still
// matches until the review expires.
func ReviewedCollect(now time.Time, fresh domain.CollectPlan, token string) (domain.CollectPlan, ReviewMatch) {
	if fresh.HasErrors() {
		return fresh, ReviewChanged
	}
	return matchReview(now, collectReviewLifetime, fresh, token, func(p domain.CollectPlan, expiry time.Time) domain.CollectPlan {
		p.ExpiresAt = expiry
		p.ReviewToken = domain.CollectReviewToken(p)
		return p
	}, func(p domain.CollectPlan) string { return p.ReviewToken })
}
