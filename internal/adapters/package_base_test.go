package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestPreparePackageBaseValidatesCandidatesAndNeverWritesDeployment(t *testing.T) {
	for _, mode := range []string{"valid", "channel-same-revision", "unrelated", "noop", "build-failed"} {
		t.Run(mode, func(t *testing.T) {
			s := packageBaseFixture()
			repo := newGitReviewRepository(t)
			writeGitReviewFile(t, repo, "flake.nix", string(s.FlakeContent))
			writeGitReviewFile(t, repo, "flake.lock", string(s.LockContent))
			if _, err := run(context.Background(), "git", "-C", repo, "add", "flake.nix", "flake.lock"); err != nil {
				t.Fatal(err)
			}
			if _, err := run(context.Background(), "git", "-C", repo, "commit", "-qm", "fixture"); err != nil {
				t.Fatal(err)
			}
			candidate := strings.Replace(string(s.LockContent), strings.Repeat("a", 40), strings.Repeat("d", 40), 1)
			target := "nixos-26.05"
			if mode == "channel-same-revision" {
				target = "nixos-26.11"
				candidate = strings.Replace(string(s.LockContent), "nixos-26.05", target, 1)
			}
			if mode == "unrelated" {
				candidate = strings.Replace(candidate, strings.Repeat("c", 40), strings.Repeat("e", 40), 1)
			}
			if mode == "noop" {
				candidate = string(s.LockContent)
			}
			bin := t.TempDir()
			logPath := filepath.Join(bin, "calls")
			candidatePath := filepath.Join(bin, "candidate.json")
			if err := os.WriteFile(candidatePath, []byte(candidate), 0600); err != nil {
				t.Fatal(err)
			}
			script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$NIXORIUM_TEST_BASE_LOG"
case " $* " in
  *" flake lock "*)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --output-lock-file ]; then cp "$NIXORIUM_TEST_BASE_CANDIDATE" "$2"; exit 0; fi
      shift
    done
    exit 2 ;;
  *"#labMeta "*) printf '%s\n' '{"schemaVersion":2,"controller":{"name":"pc99"},"clients":{"count":2,"hosts":[{"name":"pc01"},{"name":"pc02"}]}}' ;;
  *"#deploymentStatus "*) printf '%s\n' '{"ready":true,"issues":[]}' ;;
  *"#nixoriumUpdateTargets "*) printf '%s\n' '["pc99","pc01","pc02"]' ;;
  *" build "*) test "$NIXORIUM_TEST_BASE_MODE" != build-failed ;;
  *) exit 3 ;;
esac
`
			if err := os.WriteFile(filepath.Join(bin, "nix"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("NIXORIUM_TEST_BASE_LOG", logPath)
			t.Setenv("NIXORIUM_TEST_BASE_CANDIDATE", candidatePath)
			t.Setenv("NIXORIUM_TEST_BASE_MODE", mode)
			proposal, err := (PackageBase{}).PrepareUpdate(context.Background(), repo, target)
			if (err == nil) != (mode == "valid" || mode == "channel-same-revision") {
				t.Fatalf("proposal=%+v err=%v", proposal, err)
			}
			for name, want := range map[string][]byte{"flake.nix": s.FlakeContent, "flake.lock": s.LockContent} {
				got, err := os.ReadFile(filepath.Join(repo, name))
				if err != nil || string(got) != string(want) {
					t.Fatalf("planning modified %s", name)
				}
			}
			if mode == "valid" {
				log, err := os.ReadFile(logPath)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(log), "--update-input nixpkgs") || !strings.Contains(string(log), "#nixoriumOfflineCheck") || !strings.Contains(string(log), "#nixosConfigurations.pc02.config.system.build.toplevel") || proposal.PackageBase == nil || proposal.PackageBase.TargetRevision != strings.Repeat("d", 40) {
					t.Fatalf("%+v\n%s", proposal, log)
				}
			}
		})
	}
}

func packageBaseFixture() domain.UpdateInputSnapshot {
	return domain.UpdateInputSnapshot{
		HasLock:      true,
		FlakeContent: []byte("{\n  inputs.nixpkgs.url = \"github:NixOS/nixpkgs/nixos-26.05\";\n  inputs.nixorium.url = \"github:example/nixorium/v2.0.0\";\n  inputs.nixorium.inputs.nixpkgs.follows = \"nixpkgs\";\n}\n"),
		LockContent: []byte(`{"version":7,"root":"root","nodes":{
		"root":{"inputs":{"nixpkgs":"base","nixorium":"core","private":"private"}},
		"base":{"locked":{"type":"github","owner":"NixOS","repo":"nixpkgs","rev":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","narHash":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},"original":{"type":"github","owner":"NixOS","repo":"nixpkgs","ref":"nixos-26.05"}},
		"core":{"inputs":{"nixpkgs":["nixpkgs"]},"locked":{"rev":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		"private":{"inputs":{"child":"child"}},"child":{"locked":{"rev":"cccccccccccccccccccccccccccccccccccccccc"}}
		}}`),
	}
}

func TestPackageBaseInspectActualPinAndRender(t *testing.T) {
	s := packageBaseFixture()
	repo := t.TempDir()
	for name, content := range map[string][]byte{"flake.nix": s.FlakeContent, "flake.lock": s.LockContent} {
		if err := os.WriteFile(filepath.Join(repo, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := (PackageBase{}).InspectUpdateInput(repo)
	if err != nil || got.CurrentRef != "nixos-26.05" || got.CurrentRev != strings.Repeat("a", 40) {
		t.Fatalf("%+v: %v", got, err)
	}
	after, err := proposedPackageBaseFlake(got, "nixos-26.11")
	want := strings.Replace(string(s.FlakeContent), "nixos-26.05", "nixos-26.11", 1)
	if err != nil || string(after) != want {
		t.Fatalf("%s: %v", after, err)
	}
	for name, change := range map[string]func(*domain.UpdateInputSnapshot){
		"declaration-ref": func(s *domain.UpdateInputSnapshot) {
			s.FlakeContent = []byte(strings.ReplaceAll(string(s.FlakeContent), "nixos-26.05", "nixos-26.11"))
		},
		"locked-owner": func(s *domain.UpdateInputSnapshot) {
			s.LockContent = []byte(strings.Replace(string(s.LockContent), `"owner":"NixOS"`, `"owner":"other"`, 1))
		},
		"invalid-hash": func(s *domain.UpdateInputSnapshot) {
			s.LockContent = []byte(strings.ReplaceAll(string(s.LockContent), "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "sha256-invalid"))
		},
		"follows": func(s *domain.UpdateInputSnapshot) {
			s.LockContent = []byte(strings.ReplaceAll(string(s.LockContent), `["nixpkgs"]`, `"base"`))
		},
		"legacy":    func(s *domain.UpdateInputSnapshot) { s.FlakeContent = []byte("{}") },
		"ambiguous": func(s *domain.UpdateInputSnapshot) { s.FlakeContent = append(s.FlakeContent, s.FlakeContent...) },
	} {
		t.Run(name, func(t *testing.T) {
			broken := packageBaseFixture()
			change(&broken)
			if _, err := inspectPackageBase(broken); err == nil {
				t.Fatal("invalid pin accepted")
			}
		})
	}
}

func TestPackageBasePreservesEntireUnrelatedGraph(t *testing.T) {
	before := packageBaseFixture().LockContent
	after := []byte(strings.Replace(string(before), strings.Repeat("a", 40), strings.Repeat("d", 40), 1))
	if err := preserveOtherPackageBaseNodes(before, after); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []string{strings.Repeat("b", 40), strings.Repeat("c", 40)} {
		changed := []byte(strings.Replace(string(after), revision, strings.Repeat("e", 40), 1))
		if err := preserveOtherPackageBaseNodes(before, changed); err == nil {
			t.Fatal("unrelated input changed")
		}
	}
	var graph map[string]any
	if err := json.Unmarshal(after, &graph); err != nil {
		t.Fatal(err)
	}
	graph["nodes"].(map[string]any)["base"].(map[string]any)["inputs"] = map[string]any{"injected": "private"}
	changed, _ := json.Marshal(graph)
	if err := preserveOtherPackageBaseNodes(before, changed); err == nil {
		t.Fatal("base input graph changed")
	}
}
