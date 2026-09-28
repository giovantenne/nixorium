package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func workspaceRepository(t *testing.T) string {
	t.Helper()
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", "{ outputs = { self }: {}; }\n")
	writeGitReviewFile(t, repository, "flake.lock", "{\"nodes\":{\"root\":{}},\"root\":\"root\",\"version\":7}\n")
	workspaceTestGit(t, repository, "add", "flake.nix", "flake.lock")
	workspaceTestGit(t, repository, "commit", "-qm", "flake fixture")
	return repository
}

func workspaceTestGit(t *testing.T, repository string, args ...string) string {
	t.Helper()
	output, err := workspaceGit(t.Context(), repository, args...)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func workspaceWriterFixture(t *testing.T, exists bool) (*os.File, domain.WorkspaceSnapshot, func() (domain.WorkspaceSnapshot, error)) {
	t.Helper()
	repository := workspaceRepository(t)
	if exists {
		writeGitReviewFile(t, repository, domain.WorkspaceFileName, `{"schemaVersion":1}`)
	}
	root, err := openWorkspaceRoot(repository, unix.LOCK_EX)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	inspect := func() (domain.WorkspaceSnapshot, error) {
		local, err := readWorkspaceLocalState(t.Context(), root)
		return domain.WorkspaceSnapshot{
			Revision: local.Revision, SourceFingerprint: workspaceDigest(local), PinFingerprint: local.Pin,
			BaseFingerprint: local.Base, BaseExists: local.BaseExists,
		}, err
	}
	expected, err := inspect()
	if err != nil {
		t.Fatal(err)
	}
	return root, expected, inspect
}

func assertWorkspaceDraftsRemoved(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".workspace-profile.") {
			t.Fatalf("draft remains: %s", entry.Name())
		}
	}
}

func TestWorkspaceWriterCreatesAndReplacesOnlyTheProfile(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "replace"}[exists], func(t *testing.T) {
			root, expected, inspect := workspaceWriterFixture(t, exists)
			before := readGitReviewFile(t, root.Name(), "module.nix")
			indexBefore, err := os.ReadFile(filepath.Join(root.Name(), ".git/index"))
			if err != nil {
				t.Fatal(err)
			}
			data := []byte("{\n  \"schemaVersion\": 1,\n  \"desktop\": {}\n}\n")
			if err := writeWorkspaceFile(t.Context(), root, expected, data, inspect, root.Sync); err != nil {
				t.Fatal(err)
			}
			if got := readGitReviewFile(t, root.Name(), domain.WorkspaceFileName); got != string(data) {
				t.Fatalf("saved: %q", got)
			}
			info, err := os.Stat(filepath.Join(root.Name(), domain.WorkspaceFileName))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("file mode: %v %v", info, err)
			}
			indexAfter, _ := os.ReadFile(filepath.Join(root.Name(), ".git/index"))
			if string(indexAfter) != string(indexBefore) || readGitReviewFile(t, root.Name(), "module.nix") != before {
				t.Fatal("unrelated Git/worktree content changed")
			}
			if workspaceTestGit(t, root.Name(), "ls-files", "--", domain.WorkspaceFileName) != "" {
				t.Fatal("writer staged the profile")
			}
			assertWorkspaceDraftsRemoved(t, root.Name())
		})
	}
}

func TestWorkspaceWriterRechecksBeforeReplacement(t *testing.T) {
	for _, phase := range []int{1, 2} {
		for _, change := range []string{"base", "pin", "source", "created", "removed", "mode"} {
			t.Run(string(rune('0'+phase))+"/"+change, func(t *testing.T) {
				root, expected, inspect := workspaceWriterFixture(t, change != "created")
				calls := 0
				recheck := func() (domain.WorkspaceSnapshot, error) {
					calls++
					if calls == phase {
						switch change {
						case "base", "created":
							writeGitReviewFile(t, root.Name(), domain.WorkspaceFileName, `{"schemaVersion":1,"browser":{}}`)
						case "pin":
							writeGitReviewFile(t, root.Name(), "flake.lock", "{}")
						case "removed":
							if err := os.Remove(filepath.Join(root.Name(), domain.WorkspaceFileName)); err != nil {
								t.Fatal(err)
							}
						case "mode":
							if err := os.Chmod(filepath.Join(root.Name(), domain.WorkspaceFileName), 0644); err != nil {
								t.Fatal(err)
							}
						}
					}
					got, err := inspect()
					if change == "source" && calls == phase {
						got.SourceFingerprint = workspaceDigest("changed")
					}
					return got, err
				}
				err := writeWorkspaceFile(t.Context(), root, expected, []byte(`{"schemaVersion":1,"desktop":{}}`), recheck, root.Sync)
				if !errors.Is(err, domain.ErrWorkspaceConflict) {
					t.Fatalf("drift accepted: %v", err)
				}
				assertWorkspaceDraftsRemoved(t, root.Name())
			})
		}
	}
}

func TestWorkspaceWriterReportsDurabilityAfterReplacement(t *testing.T) {
	root, expected, inspect := workspaceWriterFixture(t, false)
	data := []byte("{\"schemaVersion\":1}\n")
	err := writeWorkspaceFile(t.Context(), root, expected, data, inspect, func() error { return errors.New("injected directory sync failure") })
	if !errors.Is(err, domain.ErrWorkspaceDurability) || readGitReviewFile(t, root.Name(), domain.WorkspaceFileName) != string(data) {
		t.Fatalf("durability outcome: %v", err)
	}
	assertWorkspaceDraftsRemoved(t, root.Name())
}

func TestWorkspaceWriterCancelsBeforeReplacement(t *testing.T) {
	root, expected, inspect := workspaceWriterFixture(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	recheck := func() (domain.WorkspaceSnapshot, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return inspect()
	}
	err := writeWorkspaceFile(ctx, root, expected, []byte("changed"), recheck, root.Sync)
	if !errors.Is(err, context.Canceled) || readGitReviewFile(t, root.Name(), domain.WorkspaceFileName) != `{"schemaVersion":1}` {
		t.Fatalf("cancellation: %v", err)
	}
	assertWorkspaceDraftsRemoved(t, root.Name())
}

func TestWorkspaceRejectsSpecialInputsAndLinks(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "directory", "hardlink", "oversized", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			repository := workspaceRepository(t)
			path := filepath.Join(repository, domain.WorkspaceFileName)
			outside := filepath.Join(t.TempDir(), "sentinel")
			if err := os.WriteFile(outside, []byte(`{"schemaVersion":1}`), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(outside, path)
			case "fifo":
				err = unix.Mkfifo(path, 0600)
			case "directory":
				err = os.Mkdir(path, 0700)
			case "hardlink":
				err = os.Link(outside, path)
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat(" ", domain.WorkspaceMaxBytes+1)), 0600)
			case "invalid":
				err = os.WriteFile(path, []byte("null"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = (Local{}).InspectWorkspace(t.Context(), repository, domain.WorkspaceProfile{SchemaVersion: 1})
			if err == nil {
				t.Fatal("special input accepted")
			}
			if got, _ := os.ReadFile(outside); string(got) != `{"schemaVersion":1}` {
				t.Fatal("outside sentinel changed")
			}
		})
	}
}

func TestWorkspaceRootIsNoFollowAndLockIsNonblocking(t *testing.T) {
	root, _, _ := workspaceWriterFixture(t, true)
	if other, err := openWorkspaceRoot(root.Name(), unix.LOCK_SH); err == nil {
		other.Close()
		t.Fatal("concurrent reader bypassed writer lock")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root.Name(), link); err != nil {
		t.Fatal(err)
	}
	if other, err := openWorkspaceRoot(link, unix.LOCK_EX); err == nil {
		other.Close()
		t.Fatal("root symlink followed")
	}
	if err := os.Mkdir(filepath.Join(root.Name(), "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if other, err := openWorkspaceRoot(filepath.Join(link, "child"), unix.LOCK_EX); err == nil {
		other.Close()
		t.Fatal("ancestor symlink followed")
	}
	old := root.Name() + "-renamed"
	if err := os.Rename(root.Name(), old); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(old, root.Name()) })
	if err := os.Mkdir(root.Name(), 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(root.Name()) })
	if _, err := workspaceRootIdentity(root); !errors.Is(err, domain.ErrWorkspaceConflict) {
		t.Fatalf("root replacement accepted: %v", err)
	}
}

func TestWorkspaceRejectsTrackedKeysAndUntrackedLockBeforeNix(t *testing.T) {
	for _, kind := range []string{"key", "lock"} {
		t.Run(kind, func(t *testing.T) {
			repository := workspaceRepository(t)
			if kind == "key" {
				writeGitReviewFile(t, repository, "secret-key", "private-sentinel")
				workspaceTestGit(t, repository, "add", "-f", "secret-key")
			} else {
				workspaceTestGit(t, repository, "rm", "--cached", "flake.lock")
			}
			if _, err := (Local{}).InspectWorkspace(t.Context(), repository, domain.WorkspaceProfile{SchemaVersion: 1}); err == nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Fatalf("unsafe source: %v", err)
			}
		})
	}
}
