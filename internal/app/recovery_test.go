package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeRecoverySource struct {
	pending      domain.PendingDeployment
	pendingFound bool
	pendingErr   error
	usb, reset   bool
	holder       domain.OperationHolder
	busy         bool
	settings     []domain.ValidationIssue
	drift        string
	pxeMode      string
}

// fakeRecorderSource also answers whether the pending record's writer lives.
type fakeRecorderSource struct {
	fakeRecoverySource
	running bool
}

func (s fakeRecorderSource) ProcessRunning(int, string) bool { return s.running }

func (s fakeRecoverySource) PendingDeployment() (domain.PendingDeployment, bool, error) {
	return s.pending, s.pendingFound, s.pendingErr
}
func (s fakeRecoverySource) RemoteReservationPresent() bool   { return s.usb }
func (s fakeRecoverySource) TemplateResetPending(string) bool { return s.reset }
func (s fakeRecoverySource) OperationLockHolder() (domain.OperationHolder, bool) {
	return s.holder, s.busy
}
func (s fakeRecoverySource) LabSettingsIssues(string) []domain.ValidationIssue { return s.settings }
func (s fakeRecoverySource) ControllerActivationDrift() string                 { return s.drift }
func (s fakeRecoverySource) ServiceState(_ context.Context, unit string) domain.ServiceState {
	if s.pxeMode == "recovery-required" && unit == PXENetworkUnit {
		return domain.ServiceState{Name: unit, Loaded: true, Active: true}
	}
	return domain.ServiceState{Name: unit, Loaded: true}
}

func TestRecoveryReportsEveryPersistentBlocker(t *testing.T) {
	started := time.Date(2026, 10, 1, 10, 2, 0, 0, time.UTC)
	source := fakeRecoverySource{
		pending:      domain.PendingDeployment{SchemaVersion: 1, Repository: "/srv/lab", Revision: strings.Repeat("a", 40), Targets: []domain.DeploymentTarget{{Name: "pc01"}, {Name: "pc02"}}, StartedAt: started},
		pendingFound: true, usb: true, reset: true,
		holder: domain.OperationHolder{Operation: "Update computers", User: "admin", StartedAt: started, Known: true}, busy: true,
		settings: []domain.ValidationIssue{{Field: "lab.retiredOption", Message: "unknown field"}},
		drift:    "the running system differs from the last applied configuration",
	}
	report := NewRecoveryInspector(source).Observe(context.Background(), "/srv/lab")
	if report.State != "attention" {
		t.Fatalf("state = %s", report.State)
	}
	want := map[string]string{
		domain.RecoveryDeploymentPending: "DEPLOY-PENDING", domain.RecoveryUSBReserved: "USB-RESERVED",
		domain.RecoveryResetPending: "RESET-PENDING", domain.RecoveryOperationBusy: "OP-BUSY",
		domain.RecoverySettingsInvalid: "SETTINGS-INVALID", domain.RecoveryControllerChanged: "CONTROLLER-NOT-APPLIED",
	}
	for kind, code := range want {
		condition, found := report.Condition(kind)
		if !found || condition.Next.Code != code || condition.Title == "" {
			t.Fatalf("%s = %+v, %v", kind, condition, found)
		}
	}
	pending, _ := report.Condition(domain.RecoveryDeploymentPending)
	if len(pending.Affects) != 2 || pending.Since == nil || !strings.Contains(pending.Detail, "pc01, pc02") {
		t.Fatalf("pending = %+v", pending)
	}
	if want := "Update of pc01, pc02 started " + started.Local().Format("2006-01-02 15:04") + "; how it ended is unknown."; pending.Detail != want {
		t.Fatalf("pending detail = %q, want %q", pending.Detail, want)
	}
	busy, _ := report.Condition(domain.RecoveryOperationBusy)
	if busy.Title != "Another Nixorium operation is still running" || !strings.Contains(busy.Detail, "Update computers, started by admin") {
		t.Fatalf("busy = %+v", busy)
	}

	clear := NewRecoveryInspector(fakeRecoverySource{}).Observe(context.Background(), "/srv/lab")
	if clear.State != "clear" || len(clear.Conditions) != 0 {
		t.Fatalf("clear = %+v", clear)
	}
	unsafe := NewRecoveryInspector(fakeRecoverySource{pendingFound: true, pendingErr: errors.New("not private")}).Observe(context.Background(), "/srv/lab")
	if condition, found := unsafe.Condition(domain.RecoveryDeploymentPending); !found || !strings.Contains(condition.Detail, "not private") {
		t.Fatalf("unsafe pending record hidden: %+v", unsafe)
	}
}

type failingMetaSource struct{ *fakeSource }

func (failingMetaSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return domain.LabMeta{}, errors.New("nix: error: attribute 'retiredOption' unexpected\nlong trace")
}

func TestDoctorStillReportsWhenTheLaboratoryCannotBeRead(t *testing.T) {
	inspector := NewInspector(failingMetaSource{readyFake()})
	report, err := inspector.Doctor(context.Background(), "/srv/lab", DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "errors" || len(report.Findings) < 3 {
		t.Fatalf("report = %+v", report)
	}
	first := report.Findings[0]
	if first.ID != "CONFIG-EVAL" || first.Level != domain.LevelError || strings.Contains(first.Evidence, "long trace") || first.Remediation == "" {
		t.Fatalf("CONFIG-EVAL = %+v", first)
	}
}

func TestRecoveryShowsARunningUpdateAsBusyNotInterrupted(t *testing.T) {
	pending := domain.PendingDeployment{SchemaVersion: 1, Revision: strings.Repeat("a", 40), Targets: []domain.DeploymentTarget{{Name: "pc01"}}, StartedAt: time.Now(), ControllerPID: 4242, ControllerPIDStart: "77"}
	holder := domain.OperationHolder{Operation: "Update computers", User: "admin", Known: true}
	for _, test := range []struct {
		name        string
		source      fakeRecorderSource
		interrupted bool
	}{
		{"recorder updating", fakeRecorderSource{fakeRecoverySource{pending: pending, pendingFound: true, holder: holder, busy: true}, true}, false},
		{"recorder gone", fakeRecorderSource{fakeRecoverySource{pending: pending, pendingFound: true}, false}, true},
		{"recorder alive without the lock", fakeRecorderSource{fakeRecoverySource{pending: pending, pendingFound: true}, true}, true},
		{"apply returned with unknown results", fakeRecorderSource{fakeRecoverySource{pending: func() domain.PendingDeployment {
			returned := pending
			returned.ApplyReturned = true
			return returned
		}(), pendingFound: true, holder: holder, busy: true}, true}, true},
		{"recorder unknown", fakeRecorderSource{fakeRecoverySource{pending: domain.PendingDeployment{Targets: pending.Targets}, pendingFound: true, holder: holder, busy: true}, true}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := NewRecoveryInspector(test.source).Observe(context.Background(), "/srv/lab")
			if _, found := report.Condition(domain.RecoveryDeploymentPending); found != test.interrupted {
				t.Fatalf("interrupted = %v, want %v: %+v", found, test.interrupted, report.Conditions)
			}
			if _, found := report.Condition(domain.RecoveryOperationBusy); found != test.source.busy {
				t.Fatalf("busy = %v, want %v", found, test.source.busy)
			}
		})
	}
}

type fakeQueuedRecoverySource struct {
	fakeRecoverySource
	waiting, attention int
}

func (s fakeQueuedRecoverySource) DeferredUpdateSummary(context.Context, string) (int, int) {
	return s.waiting, s.attention
}

func TestRecoveryDoesNotCallAQueuedUpdateThatNeedsReviewNothingToDo(t *testing.T) {
	for _, test := range []struct {
		name               string
		waiting, attention int
		code, detail       string
	}{
		{"only waiting", 2, 0, "UPDATES-QUEUED", "2 computers will update when switched on."},
		{"one stale entry", 0, 1, "UPDATES-NEED-REVIEW", "Queued updates: 1 computer needs attention. "},
		{"several need review", 0, 3, "UPDATES-NEED-REVIEW", "Queued updates: 3 computers need attention. "},
		{"mixed", 2, 1, "UPDATES-NEED-REVIEW", "Queued updates: 1 computer needs attention, 2 computers will update when switched on. "},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := NewRecoveryInspector(fakeQueuedRecoverySource{waiting: test.waiting, attention: test.attention}).Observe(context.Background(), "/srv/lab")
			if len(report.Conditions) != 1 {
				t.Fatalf("a queue is one condition, got %+v", report.Conditions)
			}
			condition, found := report.Condition(domain.RecoveryDeferredUpdates)
			if !found || condition.Next.Code != test.code || !strings.HasPrefix(condition.Detail, test.detail) {
				t.Fatalf("condition = %+v, %v; want %s %q", condition, found, test.code, test.detail)
			}
			if condition.Title != condition.Next.Title {
				t.Fatalf("title %q differs from the catalogue %q", condition.Title, condition.Next.Title)
			}
			if test.attention > 0 {
				if condition.Next.Action == "Nothing to do" || strings.Contains(condition.Title, "will update when") || !strings.Contains(condition.Detail, "review") {
					t.Fatalf("a queued update that needs review reads as harmless: %+v", condition)
				}
			} else if condition.Next.Action != "Nothing to do" {
				t.Fatalf("waiting entries lost their reassurance: %+v", condition.Next)
			}
		})
	}
	if report := NewRecoveryInspector(fakeQueuedRecoverySource{}).Observe(context.Background(), "/srv/lab"); len(report.Conditions) != 0 {
		t.Fatalf("an empty queue is not a condition: %+v", report.Conditions)
	}
}
