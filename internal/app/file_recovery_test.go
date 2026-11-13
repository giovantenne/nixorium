package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeFileRecoverySource struct {
	meta     domain.LabMeta
	sessions []domain.FileRecoverySession
	requests []domain.FileRecoveryHelperRequest
	hosts    []string
	refuse   string
	away     error
}

func (source *fakeFileRecoverySource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return source.meta, nil
}

func (source *fakeFileRecoverySource) FileRecovery(_ context.Context, host domain.HostMeta, request domain.FileRecoveryHelperRequest) (domain.FileRecoveryHelperAnswer, error) {
	source.requests = append(source.requests, request)
	source.hosts = append(source.hosts, host.Name+"@"+host.IP)
	if source.away != nil {
		return domain.FileRecoveryHelperAnswer{}, source.away
	}
	switch request.Operation {
	case domain.FileRecoveryHelperSessions:
		return domain.FileRecoveryHelperAnswer{Sessions: source.sessions}, nil
	case domain.FileRecoveryHelperList:
		if source.refuse != "" {
			return domain.FileRecoveryHelperAnswer{Error: source.refuse, Code: domain.FileRecoveryCodeNotFound}, nil
		}
		return domain.FileRecoveryHelperAnswer{Listing: &domain.FileRecoveryListing{Session: request.Session, Path: request.Path, Entries: []domain.FileRecoveryEntry{{Name: "notes.txt", Size: 3}}}}, nil
	case domain.FileRecoveryHelperRestore:
		// Like the computer: the reviewed session must still have its number.
		for _, session := range source.sessions {
			if session.Number == request.Session && session.ID != request.SessionID {
				return domain.FileRecoveryHelperAnswer{Error: "The computer restarted since the review and its saved sessions moved. Choose the session again.", Code: domain.FileRecoveryCodeMoved}, nil
			}
		}
		return domain.FileRecoveryHelperAnswer{Restored: &domain.FileRecoveryRestored{Folder: "Desktop/Recovered files", Placed: request.Paths, Files: len(request.Paths), Bytes: 3}}, nil
	}
	return domain.FileRecoveryHelperAnswer{Error: "unknown"}, nil
}

func fileRecoveryLab() *fakeFileRecoverySource {
	source := &fakeFileRecoverySource{}
	source.meta.Controller.Name = "pc99"
	source.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}}
	saved := time.Date(2026, 11, 10, 8, 12, 0, 0, time.UTC)
	source.sessions = []domain.FileRecoverySession{{Number: 1, ID: "00000000-0000-0000-0000-000000000001", SavedAt: saved}, {Number: 2, ID: "00000000-0000-0000-0000-000000000002", SavedAt: saved.Add(-24 * time.Hour)}}
	return source
}

func TestFileRecoveryBrowseReviewAndApply(t *testing.T) {
	source := fileRecoveryLab()
	manager := NewFileRecoveryManager(source)
	ctx := context.Background()
	sessions, err := manager.Sessions(ctx, "/srv/lab", "pc02")
	if err != nil || len(sessions) != 2 || source.hosts[0] != "pc02@10.0.0.2" {
		t.Fatalf("sessions = %v, %v, %v", sessions, err, source.hosts)
	}
	listing, err := manager.List(ctx, "/srv/lab", "pc02", 2, "Desktop")
	if err != nil || listing.Path != "Desktop" || len(listing.Entries) != 1 {
		t.Fatalf("listing = %+v, %v", listing, err)
	}
	plan := manager.Plan(ctx, "/srv/lab", "pc02", 2, []string{"Desktop/notes.txt"})
	if plan.HasErrors() || plan.ReviewToken == "" || plan.Target.Name != "pc02" || plan.SessionID != source.sessions[1].ID || !plan.SavedAt.Equal(source.sessions[1].SavedAt) {
		t.Fatalf("plan = %+v", plan)
	}
	changed := plan
	changed.Paths = []string{"Desktop/other.txt"}
	if report := manager.Apply(ctx, changed, changed.ReviewToken); report.State != "blocked" {
		t.Fatalf("changed paths applied: %+v", report)
	}
	report := manager.Apply(ctx, plan, plan.ReviewToken)
	if report.HasErrors() || report.Restored == nil || report.Restored.Files != 1 || !strings.Contains(report.Message, "until the computer restarts") {
		t.Fatalf("report = %+v", report)
	}
	last := source.requests[len(source.requests)-1]
	if last.Operation != domain.FileRecoveryHelperRestore || last.Session != 2 || last.Paths[0] != "Desktop/notes.txt" {
		t.Fatalf("restore request = %+v", last)
	}
}

func TestFileRecoveryRefusesMovedSessionsAndBadChoices(t *testing.T) {
	source := fileRecoveryLab()
	manager := NewFileRecoveryManager(source)
	ctx := context.Background()
	plan := manager.Plan(ctx, "/srv/lab", "pc01", 1, []string{"Desktop"})
	// The computer restarted: the reviewed session is now number 2 and a
	// new session 1 has another identity.
	source.sessions = []domain.FileRecoverySession{{Number: 1, ID: "00000000-0000-0000-0000-000000000003", SavedAt: source.sessions[0].SavedAt.Add(48 * time.Hour)}, {Number: 2, ID: source.sessions[0].ID, SavedAt: source.sessions[0].SavedAt}}
	if report := manager.Apply(ctx, plan, plan.ReviewToken); !report.HasErrors() || report.Restored != nil || !strings.Contains(report.Message, "restarted") {
		t.Fatalf("moved session = %+v", report)
	}
	last := source.requests[len(source.requests)-1]
	if last.Operation != domain.FileRecoveryHelperRestore || last.SessionID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("the restore did not carry the reviewed session: %+v", last)
	}
	for _, test := range []struct {
		requested string
		session   int
		paths     []string
	}{
		{"pc99", 1, []string{"a"}},
		{"@lab", 1, []string{"a"}},
		{"pc01,pc02", 1, []string{"a"}},
		{"pc07", 1, []string{"a"}},
		{"pc01", 0, []string{"a"}},
		{"pc01", 5, []string{"a"}},
		{"pc01", 1, nil},
		{"pc01", 1, []string{"../etc"}},
		{"pc01", 1, []string{"Desktop", "Desktop/a"}},
		{"pc01", 1, []string{".ssh"}},
		{"pc01", 1, []string{"Desktop/.hidden"}},
	} {
		if plan := manager.Plan(ctx, "/srv/lab", test.requested, test.session, test.paths); !plan.HasErrors() || plan.ReviewToken != "" {
			t.Fatalf("%+v planned: %+v", test, plan)
		}
	}
	source.refuse = "This folder is not in the saved session."
	if _, err := manager.List(ctx, "/srv/lab", "pc01", 1, "Missing"); err == nil || err.Error() != source.refuse {
		t.Fatalf("refusal = %v", err)
	}
	source.away = errors.New("connection refused")
	if _, err := manager.Sessions(ctx, "/srv/lab", "pc01"); err == nil || !strings.Contains(err.Error(), "pc01 could not be reached") {
		t.Fatalf("away = %v", err)
	}
}

func TestFileRecoveryReviewExpires(t *testing.T) {
	source := fileRecoveryLab()
	manager := NewFileRecoveryManager(source)
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01", 1, []string{"Desktop"})
	now = plan.ExpiresAt
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.State != "blocked" {
		t.Fatalf("expired review applied: %+v", report)
	}
}
