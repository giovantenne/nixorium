package adapters

import (
	"context"
	"encoding/json"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/hostdiagnostics"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssistantDiagnosticsUsesStrictFixedSSHAndValidatesResult(t *testing.T) {
	host := domain.HostMeta{Name: "pc01", IP: "192.0.2.1"}
	report := hostdiagnostics.Report{SchemaVersion: 1, Area: "home_reset", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Units: []hostdiagnostics.Unit{}, Journal: []hostdiagnostics.Entry{}, Space: []hostdiagnostics.Space{}, Unavailable: []string{"journal", "boot_id", "revision", "disk:/", "disk:/home", "disk:/nix/store", "service_state", "home_reset_pending"}}
	data, _ := json.Marshal(report)
	run := func(ctx context.Context, limit int, name string, args ...string) (string, string, error) {
		joined := strings.Join(args, " ")
		if name != "ssh" || limit != 32*1024 {
			t.Fatal(name, limit)
		}
		for _, required := range []string{"-F /dev/null", "StrictHostKeyChecking=yes", "UpdateHostKeys=no", "IdentityAgent=none", "root@192.0.2.1 nixorium-host-diagnostics home_reset"} {
			if !strings.Contains(joined, required) {
				t.Fatal(joined)
			}
		}
		for _, forbidden := range []string{"accept-new", "StrictHostKeyChecking=no", "bash", "journalctl", "systemctl"} {
			if strings.Contains(joined, forbidden) {
				t.Fatal(joined)
			}
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded")
		}
		return string(data), "", nil
	}
	if _, err := hostDiagnostics(t.Context(), host, false, "home_reset", run); err != nil {
		t.Fatal(err)
	}
	report.Controller = true
	data, _ = json.Marshal(report)
	if _, err := hostDiagnostics(t.Context(), host, false, "home_reset", run); err == nil {
		t.Fatal("foreign role accepted")
	}
}
func TestAssistantNewObservationToolsRequireNewConsentAndSavedRepository(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	calls := 0
	tools := AssistantWorkspaceTools{Store: store, Diagnostics: func(_ context.Context, repository, host, area string) string {
		calls++
		if repository != repo || host != "pc01" || area != "system" {
			t.Fatal(repository, host, area)
		}
		return `{"ok":true}`
	}}
	call := domain.AssistantToolCall{Name: "host_diagnostics", Arguments: []byte(`{"host":"pc01","area":"system"}`)}
	conversation := w.Conversation
	conversation.Repository = "/caller-cannot-redirect"
	if got := tools.Execute(t.Context(), conversation, call); !strings.Contains(got, "consent_required") || calls != 0 {
		t.Fatal(got, calls)
	}
	if err := os.WriteFile(filepath.Join(store.Directory, "consent.json"), []byte(`{"schemaVersion":1,"acceptedAt":"2026-10-10T00:00:00Z","textVersion":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := tools.Execute(t.Context(), conversation, call); !strings.Contains(got, "consent_required") || calls != 0 {
		t.Fatal("previous disclosure authorized diagnostics", got, calls)
	}
	if got := tools.Execute(t.Context(), conversation, domain.AssistantToolCall{Name: "git_history", Arguments: []byte(`{}`)}); !strings.Contains(got, "consent_required") {
		t.Fatal("previous disclosure authorized history", got)
	}
	if err := AssistantStateStore(store).WriteConsent(); err != nil {
		t.Fatal(err)
	}
	if got := tools.Execute(t.Context(), conversation, call); got != `{"ok":true}` || calls != 1 {
		t.Fatal(got, calls)
	}
}
