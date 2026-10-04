package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeLockSession struct {
	source *fakeLockSource
	name   string
}

func (session fakeLockSession) Thumbnail(int, int64) (classroomview.Message, error) {
	return classroomview.Message{}, errors.New("unused")
}
func (session fakeLockSession) Input([]classroomview.InputEvent, bool) error { return nil }
func (session fakeLockSession) Close() error                                 { return nil }
func (session fakeLockSession) ShowFrame([]byte) error                       { return nil }
func (session fakeLockSession) StopBroadcast() error                         { return nil }
func (session fakeLockSession) SendFiles([]classroomview.FileEntry, func(int) (io.ReadCloser, error)) ([]string, error) {
	return nil, nil
}
func (session fakeLockSession) Locked() bool {
	session.source.mutex.Lock()
	defer session.source.mutex.Unlock()
	return session.source.locked[session.name]
}
func (session fakeLockSession) SetLocked(locked bool) (bool, error) {
	session.source.mutex.Lock()
	defer session.source.mutex.Unlock()
	if session.source.silent[session.name] {
		return false, errors.New("timeout")
	}
	session.source.locked[session.name] = locked
	return locked, nil
}

type fakeLockSource struct {
	mutex   sync.Mutex
	meta    domain.LabMeta
	away    map[string]error
	silent  map[string]bool
	locked  map[string]bool
	connect int
}

func (source *fakeLockSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return source.meta, nil
}

func (source *fakeLockSource) Connect(_ context.Context, host domain.HostMeta) (classroomview.Session, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.connect++
	if err := source.away[host.Name]; err != nil {
		return nil, err
	}
	return fakeLockSession{source, host.Name}, nil
}

func lockLab() *fakeLockSource {
	meta := domain.LabMeta{}
	meta.Controller.Name = "pc99"
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}, {Name: "pc03", IP: "10.0.0.3"}}
	return &fakeLockSource{meta: meta, away: map[string]error{}, silent: map[string]bool{}, locked: map[string]bool{}}
}

func TestLockPlanAndApply(t *testing.T) {
	source := lockLab()
	source.away["pc02"] = classroomview.ErrUnreachable
	source.away["pc03"] = classroomview.AgentError{Code: classroomview.CodeNoSession}
	manager := NewLockManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02,pc03", domain.LockOn)
	if plan.HasErrors() || plan.ReviewToken == "" || len(plan.Targets) != 3 || !plan.Targets[0].Eligible || plan.Targets[1].Eligible || plan.Targets[2].Detail != "Nobody is signed in." {
		t.Fatalf("plan = %+v", plan)
	}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if report.State != "partial" || report.Targets[0].State != "verified" || report.Targets[1].State != "not-sent" || !source.locked["pc01"] {
		t.Fatalf("report = %+v", report)
	}
	unlock := manager.Plan(context.Background(), "/srv/lab", "pc01", domain.LockOff)
	if !unlock.Targets[0].Locked {
		t.Fatalf("the plan did not observe the lock: %+v", unlock)
	}
	if report := manager.Apply(context.Background(), unlock, unlock.ReviewToken); report.State != "completed" || source.locked["pc01"] {
		t.Fatalf("unlock = %+v", report)
	}
}

func TestLockApplyRefusesStaleReviews(t *testing.T) {
	source := lockLab()
	manager := NewLockManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01", domain.LockOn)
	changed := plan
	changed.Action = domain.LockOff
	for name, report := range map[string]domain.LockReport{
		"wrong token":    manager.Apply(context.Background(), plan, "sha256:other"),
		"changed action": manager.Apply(context.Background(), changed, plan.ReviewToken),
	} {
		if report.State != "blocked" {
			t.Fatalf("%s = %+v", name, report)
		}
	}
	manager.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.State != "blocked" || !strings.Contains(report.Message, "expired") {
		t.Fatalf("expired = %+v", report)
	}
	manager.now = time.Now
	source.meta.Clients.Hosts[0].IP = "10.0.0.9"
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.State != "blocked" || !strings.Contains(report.Message, "inventory") {
		t.Fatalf("inventory = %+v", report)
	}
	if source.locked["pc01"] {
		t.Fatal("a refused review locked a computer")
	}
}

func TestLockReportsUnconfirmedAndRefusesTheController(t *testing.T) {
	source := lockLab()
	source.silent["pc01"] = true
	manager := NewLockManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01", domain.LockOn)
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.Targets[0].State != "unconfirmed" {
		t.Fatalf("silent computer = %+v", report)
	}
	if plan := manager.Plan(context.Background(), "/srv/lab", "pc99", domain.LockOn); !plan.HasErrors() {
		t.Fatalf("controller plan = %+v", plan)
	}
	if plan := manager.Plan(context.Background(), "/srv/lab", "pc01", "freeze"); !plan.HasErrors() {
		t.Fatalf("unknown action = %+v", plan)
	}
}
