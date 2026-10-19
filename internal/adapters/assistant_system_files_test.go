package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/systemfiles"
)

func TestReadSystemFileUsesStrictFixedSSHAndRechecksTheAnswer(t *testing.T) {
	host := domain.HostMeta{Name: "pc01", IP: "192.0.2.1"}
	answer, _ := json.Marshal(systemfiles.Result{Path: "/boot/grub/grub.cfg", Kind: "file", Content: "menuentry \"NixOS\" {\n"})
	run := func(ctx context.Context, _ int, name string, args ...string) (string, string, error) {
		joined := strings.Join(args, " ")
		if name != "ssh" {
			t.Fatal(name)
		}
		for _, required := range []string{"-F /dev/null", "StrictHostKeyChecking=yes", "IdentityAgent=none", "root@192.0.2.1 nixorium-host-diagnostics file /boot/grub/grub.cfg"} {
			if !strings.Contains(joined, required) {
				t.Fatal(joined)
			}
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded")
		}
		return string(answer), "", nil
	}
	result, err := readSystemFile(t.Context(), host, false, "/boot/grub/grub.cfg", run)
	if err != nil || !strings.Contains(result.Content, "NixOS") {
		t.Fatal(result, err)
	}

	// The computer cannot answer for another path.
	other, _ := json.Marshal(systemfiles.Result{Path: "/etc/hosts", Kind: "file"})
	if _, err := readSystemFile(t.Context(), host, false, "/boot/grub/grub.cfg", func(context.Context, int, string, ...string) (string, string, error) {
		return string(other), "", nil
	}); err == nil {
		t.Fatal("an answer for another path was accepted")
	}
	// A refused path never reaches the computer.
	if _, err := readSystemFile(t.Context(), host, false, "/etc/shadow", func(context.Context, int, string, ...string) (string, string, error) {
		t.Fatal("refused path was sent")
		return "", "", nil
	}); !errors.Is(err, ErrSystemFileRefused) {
		t.Fatal(err)
	}
	for stderr, want := range map[string]error{
		"This path is not readable by the Assistant.": ErrSystemFileRefused,
		"No such file or directory.":                  ErrSystemFileMissing,
		"This account cannot read it.":                ErrSystemFileUnreadable,
		"Choose one supported diagnostics area.":      ErrSystemFileOldComputer,
	} {
		_, err := readSystemFile(t.Context(), host, false, "/etc/hosts", func(context.Context, int, string, ...string) (string, string, error) {
			return "", stderr, errors.New("exit status 3")
		})
		if !errors.Is(err, want) {
			t.Errorf("%q: %v", stderr, err)
		}
	}
}

func TestReadSystemFileNeedsTheNewConsent(t *testing.T) {
	w, store, repo := assistantWorkspaceFixture(t)
	calls := 0
	tools := AssistantWorkspaceTools{Store: store, SystemFiles: func(_ context.Context, repository, host, path string) string {
		calls++
		if repository != repo || host != "pc01" || path != "/etc/hosts" {
			t.Fatal(repository, host, path)
		}
		return `{"ok":true}`
	}}
	call := domain.AssistantToolCall{Name: "read_system_file", Arguments: []byte(`{"host":"pc01","path":"/etc/hosts"}`)}
	if err := os.WriteFile(filepath.Join(store.Directory, "consent.json"), []byte(`{"schemaVersion":1,"acceptedAt":"2026-10-10T00:00:00Z","textVersion":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := tools.Execute(t.Context(), w.Conversation, call); !strings.Contains(got, "consent_required") || calls != 0 {
		t.Fatal("the previous disclosure authorized system files", got, calls)
	}
	if err := store.state().WriteConsent(); err != nil {
		t.Fatal(err)
	}
	if got := tools.Execute(t.Context(), w.Conversation, call); got != `{"ok":true}` || calls != 1 {
		t.Fatal(got, calls)
	}
}
