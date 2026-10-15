package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// fakeDesktop is what a computer's agent answers when collecting.
type fakeDesktop struct {
	entries []classroomview.FileEntry
	skipped int
	listErr error
	content map[int]string
	// extra is appended to a file's last piece, as a lying agent would.
	extra string
}

type collectSession struct {
	fakeLockSession
	lab *collectLab
}

func (session collectSession) ListDesktop() ([]classroomview.FileEntry, int, error) {
	lab := session.lab
	lab.mutex.Lock()
	lab.active++
	lab.peak = max(lab.peak, lab.active)
	desktop := lab.desktops[session.name]
	lab.mutex.Unlock()
	time.Sleep(5 * time.Millisecond)
	lab.mutex.Lock()
	lab.active--
	lab.mutex.Unlock()
	if desktop.listErr != nil {
		return nil, 0, desktop.listErr
	}
	return desktop.entries, desktop.skipped, nil
}

func (session collectSession) ReadDesktopFile(index int, size int64, piece func([]byte) error) error {
	session.lab.mutex.Lock()
	desktop := session.lab.desktops[session.name]
	session.lab.mutex.Unlock()
	data := desktop.content[index] + desktop.extra
	for len(data) > 0 {
		count := min(len(data), 2)
		if err := piece([]byte(data[:count])); err != nil {
			return err
		}
		data = data[count:]
	}
	return nil
}

func (session collectSession) EndCollect() error { return nil }

// collectLab is a laboratory whose computers answer with desktops; an
// outdated computer's session cannot collect.
type collectLab struct {
	*fakeLockSource
	mutex    sync.Mutex
	desktops map[string]fakeDesktop
	outdated map[string]bool
	active   int
	peak     int
}

func (lab *collectLab) Connect(ctx context.Context, host domain.HostMeta) (classroomview.Session, error) {
	session, err := lab.fakeLockSource.Connect(ctx, host)
	if err != nil {
		return nil, err
	}
	if lab.outdated[host.Name] {
		return session, nil
	}
	return collectSession{session.(fakeLockSession), lab}, nil
}

func newCollectLab(names ...string) *collectLab {
	source := lockLab()
	source.meta.Clients.Hosts = nil
	for index, name := range names {
		source.meta.Clients.Hosts = append(source.meta.Clients.Hosts, domain.HostMeta{Name: name, IP: fmt.Sprintf("10.0.0.%d", index+1)})
	}
	return &collectLab{fakeLockSource: source, desktops: map[string]fakeDesktop{}, outdated: map[string]bool{}}
}

// fakeCollectContent keeps stored files in memory, like the worker's
// store: a computer whose fill fails, or that is incomplete, keeps nothing.
type fakeCollectContent struct {
	mutex     sync.Mutex
	folder    string
	stored    map[string]map[int]string
	removed   bool
	noSpace   map[string]bool
	beginFail error
}

func (content *fakeCollectContent) Begin(folder string) (string, error) {
	if content.beginFail != nil {
		return "", content.beginFail
	}
	content.folder, content.stored = folder, map[string]map[int]string{}
	return "c1", nil
}

func (content *fakeCollectContent) Store(collection, computer string, entries []classroomview.FileEntry, fill func(func(int, []byte) error) error) error {
	if collection != "c1" {
		return errors.New("unknown collection")
	}
	if content.noSpace[computer] {
		return ErrCollectNoSpace
	}
	files := map[int]string{}
	err := fill(func(index int, data []byte) error {
		if entries[index].Dir || int64(len(files[index])+len(data)) > entries[index].Size {
			return errors.New("a file is larger than its listed size")
		}
		files[index] += string(data)
		return nil
	})
	if err != nil {
		return err
	}
	for index, entry := range entries {
		if !entry.Dir && int64(len(files[index])) != entry.Size {
			return errors.New("incomplete")
		}
	}
	content.mutex.Lock()
	content.stored[computer] = files
	content.mutex.Unlock()
	return nil
}

func (content *fakeCollectContent) Remove(string) { content.removed = true }

var homework = fakeDesktop{
	entries: []classroomview.FileEntry{{Path: "Homework", Dir: true}, {Path: "Homework/essay.odt", Size: 5}, {Path: "empty.txt"}, {Path: "notes.txt", Size: 3}},
	content: map[int]string{1: "essay", 3: "abc"},
	skipped: 1,
}

func TestCollectPlanReviewsTheComputers(t *testing.T) {
	lab := newCollectLab("pc01", "pc02", "pc03", "pc04", "pc05", "pc06")
	lab.desktops["pc01"] = homework
	lab.away["pc02"] = classroomview.ErrUnreachable
	lab.away["pc03"] = classroomview.AgentError{Code: classroomview.CodeNoSession}
	lab.outdated["pc04"] = true
	lab.desktops["pc05"] = fakeDesktop{listErr: classroomview.AgentError{Code: classroomview.CodeUnsupported}}
	lab.desktops["pc06"] = fakeDesktop{listErr: classroomview.AgentError{Code: classroomview.CodeCollectTooMuch}}
	manager := NewCollectManager(lab, &fakeCollectContent{})
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02,pc03,pc04,pc05,pc06", "/home/teacher/Homework")
	if plan.HasErrors() || plan.ReviewToken == "" || plan.Destination != "/home/teacher/Homework" || !strings.Contains(plan.Message, "2 of 6 computers are copied to /home/teacher/Homework") {
		t.Fatalf("plan = %+v", plan)
	}
	want := []struct {
		eligible bool
		detail   string
	}{
		{true, "3 files (8 bytes). 1 item with an unusual name is left out."},
		{false, ClassroomOffDetail},
		{false, "Nobody is signed in."},
		{false, collectOutdated},
		{false, collectOutdated},
		{true, collectTooMuch},
	}
	for index, target := range plan.Targets {
		if target.Eligible != want[index].eligible || target.Detail != want[index].detail {
			t.Fatalf("%s = %+v", target.Name, target)
		}
	}
	// What is on the desktops is read again at apply time: a change keeps
	// the review token.
	lab.desktops["pc01"] = fakeDesktop{}
	again := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02,pc03,pc04,pc05,pc06", "/home/teacher/Homework")
	if again.ReviewToken != plan.ReviewToken || again.Targets[0].Detail != collectNothing {
		t.Fatalf("again = %+v", again)
	}
	// The folder is part of the review.
	if other := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02,pc03,pc04,pc05,pc06", "/tmp/other"); other.ReviewToken == plan.ReviewToken {
		t.Fatal("another folder kept the review token")
	}
	for name, refused := range map[string]domain.CollectPlan{
		"no folder":     manager.Plan(context.Background(), "/srv/lab", "pc01", ""),
		"controller":    manager.Plan(context.Background(), "/srv/lab", "pc99", "/tmp"),
		"unknown":       manager.Plan(context.Background(), "/srv/lab", "pc42", "/tmp"),
		"none eligible": manager.Plan(context.Background(), "/srv/lab", "pc02,pc03", "/tmp"),
	} {
		if !refused.HasErrors() || refused.ReviewToken != "" {
			t.Fatalf("%s = %+v", name, refused)
		}
	}
}

func TestCollectApplyKeepsOnlyCompleteSafeDesktops(t *testing.T) {
	names := []string{"pc01", "pc02", "pc03", "pc04", "pc05", "pc06", "pc07", "pc08", "pc09"}
	lab := newCollectLab(names...)
	for _, name := range names {
		lab.desktops[name] = homework
	}
	content := &fakeCollectContent{noSpace: map[string]bool{}}
	manager := NewCollectManager(lab, content)
	manager.now = func() time.Time { return time.Date(2026, 10, 15, 10, 30, 12, 0, time.Local) }
	plan := manager.Plan(context.Background(), "/srv/lab", strings.Join(names, ","), "/home/teacher/Homework")
	if plan.HasErrors() {
		t.Fatalf("plan = %+v", plan)
	}
	// After the review the desktops change; each is read again.
	lab.peak = 0
	many := make([]classroomview.FileEntry, classroomview.MaxFileEntries+1)
	for index := range many {
		many[index] = classroomview.FileEntry{Path: fmt.Sprintf("f%d", index)}
	}
	lab.desktops["pc02"] = fakeDesktop{entries: []classroomview.FileEntry{}}
	lab.desktops["pc03"] = fakeDesktop{listErr: classroomview.AgentError{Code: classroomview.CodeCollectTooMuch}}
	lab.desktops["pc04"] = fakeDesktop{entries: many}
	lab.desktops["pc05"] = fakeDesktop{entries: []classroomview.FileEntry{{Path: "../../etc/cron.d/evil", Size: 1}}, content: map[int]string{0: "x"}}
	lab.desktops["pc06"] = fakeDesktop{entries: homework.entries, content: homework.content, extra: "more"}
	lab.away["pc07"] = classroomview.ErrUnreachable
	lab.desktops["pc08"] = fakeDesktop{entries: homework.entries, content: map[int]string{1: "essay", 3: "ab"}}
	content.noSpace["pc09"] = true
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	want := map[string]string{
		"pc01": domain.CollectCollected, "pc02": domain.CollectEmpty, "pc03": domain.CollectTooMuch,
		"pc04": domain.CollectTooMuch, "pc05": domain.CollectFailed, "pc06": domain.CollectFailed,
		"pc07": domain.CollectNotSent, "pc08": domain.CollectFailed, "pc09": domain.CollectFailed,
	}
	for _, outcome := range report.Targets {
		if outcome.State != want[outcome.Name] {
			t.Fatalf("%s = %+v", outcome.Name, outcome)
		}
	}
	if report.State != "partial" || report.Save != "c1" || report.Folder != "2026-10-15 10.30" || content.folder != "2026-10-15 10.30" || report.Message != "Collected from 1 of 9 selected computers." {
		t.Fatalf("report = %+v", report)
	}
	if report.Targets[0].Items != 3 || report.Targets[0].Bytes != 8 || report.Targets[7].Detail == "" || report.Targets[6].Detail != ClassroomOffDetail || !strings.Contains(report.Targets[8].Detail, "DISK-LOW") {
		t.Fatalf("outcomes = %+v", report.Targets)
	}
	// Nothing of a refused, oversized or interrupted computer is kept.
	if len(content.stored) != 1 || content.stored["pc01"][1] != "essay" || content.stored["pc01"][3] != "abc" {
		t.Fatalf("stored = %+v", content.stored)
	}
	if lab.peak > collectParallel {
		t.Fatalf("%d computers streamed at once", lab.peak)
	}
}

func TestCollectApplyRefusesStaleReviewsAndForgetsEmptyCollections(t *testing.T) {
	lab := newCollectLab("pc01", "pc02")
	lab.desktops["pc01"] = fakeDesktop{entries: []classroomview.FileEntry{}}
	lab.desktops["pc02"] = fakeDesktop{entries: []classroomview.FileEntry{}}
	content := &fakeCollectContent{}
	manager := NewCollectManager(lab, content)
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02", "/home/teacher")
	moved := plan
	moved.Destination = "/tmp"
	changed := newCollectLab("pc01", "pc03")
	for name, report := range map[string]domain.CollectReport{
		"wrong token":   manager.Apply(context.Background(), plan, "sha256:other"),
		"other folder":  manager.Apply(context.Background(), moved, plan.ReviewToken),
		"changed lab":   NewCollectManager(changed, content).Apply(context.Background(), plan, plan.ReviewToken),
		"expired":       (&CollectManager{lab, content, func() time.Time { return time.Now().Add(time.Hour) }}).Apply(context.Background(), plan, plan.ReviewToken),
		"store refused": NewCollectManager(lab, &fakeCollectContent{beginFail: errors.New("full")}).Apply(context.Background(), plan, plan.ReviewToken),
	} {
		if report.State != "blocked" || len(report.Targets) != 0 || report.Save != "" {
			t.Fatalf("%s = %+v", name, report)
		}
	}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if report.State != "completed" || report.Save != "" || !content.removed || report.Targets[0].State != domain.CollectEmpty {
		t.Fatalf("empty desktops = %+v", report)
	}
}
