package adapters

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The pair found on a live controller after updating from v3.3.1 to v3.8.0
// with Nixorium 3.8.0: the right commit is locked, but "original" still names
// the reference flake.nix had before the update.
const liveStaleFlake = "{\n  inputs.nixpkgs.url = \"github:NixOS/nixpkgs/nixos-26.05\";\n  inputs.nixorium.url = \"github:giovantenne/nixorium/v3.8.0\";\n  inputs.nixorium.inputs.nixpkgs.follows = \"nixpkgs\";\n}\n"

func liveLock(nixoriumRef string) string {
	return `{"root":"root","version":7,"nodes":{
"root":{"inputs":{"nixorium":"nixorium","nixpkgs":"nixpkgs"}},
"nixpkgs":{"locked":{"type":"github","owner":"NixOS","repo":"nixpkgs","rev":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","narHash":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="},"original":{"type":"github","owner":"NixOS","repo":"nixpkgs","ref":"nixos-26.05"}},
"nixorium":{"inputs":{"nixpkgs":["nixpkgs"]},"locked":{"type":"github","owner":"giovantenne","repo":"nixorium","rev":"6f5e7e5000000000000000000000000000000000"},"original":{"type":"github","owner":"giovantenne","repo":"nixorium","ref":"` + nixoriumRef + `"}}
}}
`
}

func TestFlakeLockOriginalMismatchesDetectsTheLiveControllerPair(t *testing.T) {
	mismatches, err := flakeLockOriginalMismatches([]byte(liveStaleFlake), []byte(liveLock("v3.3.1")))
	if err != nil || len(mismatches) != 1 || mismatches[0] != "nixorium: flake.nix declares github:giovantenne/nixorium/v3.8.0 but flake.lock records github:giovantenne/nixorium/v3.3.1" {
		t.Fatalf("mismatches = %q, err = %v", mismatches, err)
	}
	if mismatches, err := flakeLockOriginalMismatches([]byte(liveStaleFlake), []byte(liveLock("v3.8.0"))); err != nil || len(mismatches) != 0 {
		t.Fatalf("consistent pair reported %q, err = %v", mismatches, err)
	}
	staleBase := strings.Replace(liveLock("v3.8.0"), `"ref":"nixos-26.05"`, `"ref":"nixos-25.11"`, 1)
	if mismatches, _ := flakeLockOriginalMismatches([]byte(liveStaleFlake), []byte(staleBase)); len(mismatches) != 1 || !strings.HasPrefix(mismatches[0], "nixpkgs: ") {
		t.Fatalf("stale package base = %q", mismatches)
	}
	// Forms that managed updates never write are left to Nix.
	custom := strings.Replace(liveStaleFlake, "github:giovantenne/nixorium/v3.8.0", "git+https://example.org/nixorium?ref=v3.8.0", 1)
	if mismatches, _ := flakeLockOriginalMismatches([]byte(custom), []byte(liveLock("v3.3.1"))); len(mismatches) != 0 {
		t.Fatalf("custom source = %q", mismatches)
	}
}

func TestFlakeLockConsistencyAndInspectionReportTheRepair(t *testing.T) {
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "flake.nix"), []byte(liveStaleFlake), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "flake.lock"), []byte(liveLock("v3.3.1")), 0600); err != nil {
		t.Fatal(err)
	}
	err := (Local{}).FlakeLockConsistency(repository)
	if code, typed := domain.BlockedCodeOf(err); !typed || code != "PIN-OUTDATED" || !strings.Contains(err.Error(), "v3.3.1") || !strings.Contains(err.Error(), "Update Nixorium") || !strings.Contains(err.Error(), "nix flake lock") {
		t.Fatalf("consistency error = %v", err)
	}
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil || snapshot.CurrentRef != "v3.8.0" || !strings.Contains(snapshot.PinOutdated, "v3.3.1") {
		t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
	}
	if err := os.WriteFile(filepath.Join(repository, "flake.lock"), []byte(liveLock("v3.8.0")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (Local{}).FlakeLockConsistency(repository); err != nil {
		t.Fatalf("consistent deployment refused: %v", err)
	}
	if snapshot, _ := (Local{}).InspectUpdateInput(repository); snapshot.PinOutdated != "" {
		t.Fatalf("consistent deployment marked outdated: %q", snapshot.PinOutdated)
	}
}

func stagedUpdateRepository(t *testing.T, ref string) string {
	t.Helper()
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", strings.Replace(liveStaleFlake, "v3.8.0", ref, 1))
	writeGitReviewFile(t, repository, "flake.lock", liveLock(ref))
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.nix", "flake.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "deployment"); err != nil {
		t.Fatal(err)
	}
	return repository
}

// fakeUpdateNix answers like Nix for a framework update: the candidate lock
// records "original" from the flake.nix it locks, as real Nix does.
func fakeUpdateNix(t *testing.T) (string, string) {
	t.Helper()
	bin := t.TempDir()
	logPath := filepath.Join(bin, "calls")
	stages := t.TempDir()
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$NIXORIUM_TEST_NIX_LOG"
case " $* " in
` + fakeCandidateStoreCases + `
  *" flake lock "*)
    source=$(ls "$NIXORIUM_TEST_STAGES" | sort -n | tail -n 1)
    ref=$(sed -n 's|.*github:giovantenne/nixorium/\([^"]*\)".*|\1|p' "$NIXORIUM_TEST_STAGES/$source/flake.nix")
    if [ -n "${NIXORIUM_TEST_ORIGINAL_REF:-}" ]; then ref=$NIXORIUM_TEST_ORIGINAL_REF; fi
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --output-lock-file ]; then
        sed -e "s|6f5e7e5000000000000000000000000000000000|2222222222222222222222222222222222222222|" -e "s|\"ref\":\"v[^\"]*\"|\"ref\":\"$ref\"|" "$NIXORIUM_TEST_STAGES/$source/flake.lock" > "$2"
        exit 0
      fi
      shift
    done
    exit 2 ;;
  *"#labMeta "*) printf '%s\n' '{"schemaVersion":2,"deploymentMode":"controller","controller":{"name":"pc99"},"clients":{"count":0,"hosts":[]}}' ;;
  *"#deploymentStatus "*) printf '%s\n' '{"ready":false,"issues":[],"controller":{"ready":true,"issues":[],"requiresKeys":false}}' ;;
  *"#nixoriumValidateWorkspaceCandidate "*) printf 'true\n' ;;
  *"nixoriumWorkspace"*) printf '%s\n' '{"schemaVersion":1,"state":"prepared","managedFile":"workspace-profile.json","studentUser":"student","seed":"/nix/store/00000000000000000000000000000000-home","declared":{"schemaVersion":1},"effective":{"schemaVersion":1},"catalog":{"schemaVersion":1,"baseline":{"schemaVersion":1},"applications":[],"extensions":[]},"requiredPackages":[],"packages":[],"extensions":[],"targets":[{"name":"pc99","role":"controller"}]}' ;;
  *" build "*) exit 0 ;;
  *) exit 3 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "nix"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NIXORIUM_TEST_NIX_LOG", logPath)
	t.Setenv("NIXORIUM_TEST_STAGES", stages)
	return logPath, stages
}

func lockNodeOriginalRef(t *testing.T, lock []byte, input string) string {
	t.Helper()
	node, present, err := directRootInputNode(lock, input)
	if err != nil || !present {
		t.Fatalf("%s node: present=%t err=%v", input, present, err)
	}
	var decoded struct {
		Original struct{ Ref string } `json:"original"`
	}
	if err := json.Unmarshal(node, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.Original.Ref
}

func TestPrepareUpdateRecordsTheNewReferenceAsOriginal(t *testing.T) {
	repository := stagedUpdateRepository(t, "v3.3.1")
	logPath, stages := fakeUpdateNix(t)
	proposal, err := (Local{}).PrepareUpdate(context.Background(), repository, "v3.8.0")
	if err != nil {
		t.Fatal(err)
	}
	if ref := lockNodeOriginalRef(t, proposal.LockContent, "nixorium"); ref != "v3.8.0" {
		t.Fatalf("candidate original ref = %q, want v3.8.0", ref)
	}
	if mismatches, _ := flakeLockOriginalMismatches(proposal.FlakeContent, proposal.LockContent); len(mismatches) != 0 {
		t.Fatalf("proposed pair disagrees: %q", mismatches)
	}
	// The lock was computed from the proposed flake.nix, and the pair that
	// will be saved passed the Nix lock check before any evaluation.
	if source := readGitReviewFile(t, filepath.Join(stages, "0"), "flake.nix"); !strings.Contains(source, "nixorium/v3.8.0") {
		t.Fatalf("candidate lock source flake.nix = %q", source)
	}
	log, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	if len(lines) < 5 || !strings.Contains(lines[3], "flake lock --no-update-lock-file path:/nix/store/") || !strings.Contains(lines[4], "#labMeta") || strings.Contains(string(log), "--reference-lock-file") {
		t.Fatalf("Nix invocations:\n%s", log)
	}
	if !strings.Contains(proposal.Diff.Content, `"ref":"v3.8.0"`) {
		t.Fatalf("review diff does not show the corrected original:\n%s", proposal.Diff.Content)
	}
}

func TestPrepareUpdateRefusesACandidateLockThatDisagreesWithFlakeNix(t *testing.T) {
	for _, mode := range []string{"stale-original", "nix-check"} {
		t.Run(mode, func(t *testing.T) {
			repository := stagedUpdateRepository(t, "v3.3.1")
			logPath, _ := fakeUpdateNix(t)
			if mode == "stale-original" {
				// What Nixorium 3.8.0 produced: the old reference as original.
				t.Setenv("NIXORIUM_TEST_ORIGINAL_REF", "v3.3.1")
			} else {
				t.Setenv("NIXORIUM_TEST_LOCK_CHECK", "stale")
			}
			_, err := (Local{}).PrepareUpdate(context.Background(), repository, "v3.8.0")
			if err == nil || !strings.Contains(err.Error(), candidateLockMismatch) {
				t.Fatalf("error = %v", err)
			}
			log, _ := os.ReadFile(logPath)
			if strings.Contains(string(log), "#labMeta") || strings.Contains(string(log), " build ") {
				t.Fatalf("refused candidate was evaluated or built:\n%s", log)
			}
			if readGitReviewFile(t, repository, "flake.lock") != liveLock("v3.3.1") {
				t.Fatal("refused update changed flake.lock")
			}
		})
	}
}

func TestPrepareUpdateRepairsTheConfiguredReleaseAfterAnOlderUpdate(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", liveStaleFlake)
	writeGitReviewFile(t, repository, "flake.lock", liveLock("v3.3.1"))
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.nix", "flake.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "deployment"); err != nil {
		t.Fatal(err)
	}
	fakeUpdateNix(t)
	proposal, err := (Local{}).PrepareUpdate(context.Background(), repository, "v3.8.0")
	if err != nil {
		t.Fatal(err)
	}
	if string(proposal.FlakeContent) != liveStaleFlake || lockNodeOriginalRef(t, proposal.LockContent, "nixorium") != "v3.8.0" || strings.Contains(proposal.Diff.Content, "flake.nix") {
		t.Fatalf("repair proposal = %+v", proposal)
	}
}

func TestPreparePackageBaseRefusesAnOutdatedFrameworkRecord(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", liveStaleFlake)
	writeGitReviewFile(t, repository, "flake.lock", liveLock("v3.3.1"))
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.nix", "flake.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "deployment"); err != nil {
		t.Fatal(err)
	}
	logPath, _ := fakeUpdateNix(t)
	_, err := (PackageBase{}).PrepareUpdate(context.Background(), repository, "nixos-26.11")
	if err == nil || !strings.Contains(err.Error(), "flake.lock does not match flake.nix") || !strings.Contains(err.Error(), "Update Nixorium") {
		t.Fatalf("error = %v", err)
	}
	if log, _ := os.ReadFile(logPath); len(log) != 0 {
		t.Fatalf("refusal ran Nix:\n%s", log)
	}
}

// Real Nix: locking the current flake.nix with an override records the old
// URL as original (the 3.8.0 defect, rejected by the candidate check), while
// locking the proposed flake.nix records the new one and passes it.
func TestCandidateLockOriginalWithRealNix(t *testing.T) {
	if _, err := exec.LookPath("nix"); err != nil {
		t.Skip("real Nix integration requires nix in PATH")
	}
	release := func(name string) string {
		directory := filepath.Join(t.TempDir(), name)
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "flake.nix"), []byte(`{ outputs = { self }: { version = "`+name+`"; }; }`), 0600); err != nil {
			t.Fatal(err)
		}
		return "path:" + directory
	}
	oldURL, newURL := release("a"), release("b")
	declaration := func(url string) string {
		return "{\n  inputs.nixorium.url = \"" + url + "\";\n  outputs = { self, nixorium }: { version = nixorium.version; };\n}\n"
	}
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", declaration(oldURL))
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.nix"); err != nil {
		t.Fatal(err)
	}
	if _, err := runBoundedNix(t.Context(), 64*1024, "flake", "lock", "git+file://"+repository); err != nil {
		t.Skipf("real Nix cannot lock a local flake here: %v", err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "add", "flake.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "deployment"); err != nil {
		t.Fatal(err)
	}
	current := readGitReviewFile(t, repository, "flake.lock")
	proposed := []byte(declaration(newURL))
	originalPath := func(lock []byte) string {
		node, _, err := directRootInputNode(lock, "nixorium")
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Locked   struct{ Path string } `json:"locked"`
			Original struct{ Path string } `json:"original"`
		}
		if err := json.Unmarshal(node, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded.Original.Path + " " + decoded.Locked.Path
	}
	check := func(lock []byte) error {
		source, err := candidateFlakeSource(t.Context(), repository, map[string][]byte{"flake.nix": proposed, "flake.lock": lock})
		if err != nil {
			t.Fatal(err)
		}
		_, err = runBoundedNix(t.Context(), 64*1024, "flake", "lock", "--no-update-lock-file", source)
		return err
	}

	// The 3.8.0 procedure.
	legacyPath := filepath.Join(t.TempDir(), "legacy.lock")
	if _, err := runBoundedNix(t.Context(), 64*1024, "flake", "lock", "git+file://"+repository, "--override-input", "nixorium", newURL, "--output-lock-file", legacyPath); err != nil {
		t.Fatal(err)
	}
	legacy, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := originalPath(legacy); got != strings.TrimPrefix(oldURL, "path:")+" "+strings.TrimPrefix(newURL, "path:") {
		t.Fatalf("legacy lock original/locked = %q", got)
	}
	if err := check(legacy); err == nil || !strings.Contains(err.Error(), "requires lock file changes") {
		t.Fatalf("legacy pair passed the candidate check: %v", err)
	}

	// The fixed procedure.
	fixed, err := lockUpdateCandidate(t.Context(), repository, proposed, "nixorium", newURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := originalPath(fixed); got != strings.TrimPrefix(newURL, "path:")+" "+strings.TrimPrefix(newURL, "path:") {
		t.Fatalf("fixed lock original/locked = %q", got)
	}
	if err := check(fixed); err != nil {
		t.Fatalf("fixed pair failed the candidate check: %v", err)
	}
	if readGitReviewFile(t, repository, "flake.lock") != current {
		t.Fatal("candidate locking changed the deployment flake.lock")
	}
}
