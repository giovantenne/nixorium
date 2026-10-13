package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func TestHomeFilesCaptureAndCleanup(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repository := t.TempDir()
	source := t.TempDir()
	os.Mkdir(filepath.Join(source, "empty"), 0700)
	content := []byte("#!/bin/sh\nexit 0\n")
	name := filepath.Join(source, "lesson")
	os.WriteFile(name, content, 0755)
	if err := unix.Setxattr(name, "user.capture-fixture", []byte("not-copied"), 0); err != nil && err != unix.ENOTSUP {
		t.Fatal(err)
	}
	r, err := (Local{}).CaptureHomeFiles(t.Context(), repository, source, "Documents/Exercises")
	if err != nil || r.HasErrors() || len(r.Files) != 1 || len(r.Directories) != 2 || r.Files[0].Executable {
		t.Fatal(r, err)
	}
	_, stage, err := readHomeCapture(repository, r.CaptureID)
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimPrefix(*r.Files[0].Source, "imports/")
	data, info, err := readWorkspaceFile(stage, hash, domain.HomeFilesMaxFileBytes)
	if err != nil || string(data) != string(content) || info.Mode&0777 != 0600 {
		t.Fatal(info, err)
	}
	if _, err := unix.Getxattr(fmt.Sprintf("/proc/self/fd/%d/%s", stage.Fd(), hash), "user.capture-fixture", nil); err == nil {
		t.Fatal("source attributes copied")
	}
	stage.Close()
	os.WriteFile(name, []byte("source changed later"), 0755)
	_, stage, err = readHomeCapture(repository, r.CaptureID)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err = readWorkspaceFile(stage, hash, domain.HomeFilesMaxFileBytes)
	stage.Close()
	if err != nil || string(data) != string(content) {
		t.Fatal("capture reread live source", err)
	}
	if err := (Local{}).DiscardHomeFilesCapture(t.Context(), repository, r.CaptureID); err != nil {
		t.Fatal(err)
	}
	if _, stage, err = readHomeCapture(repository, r.CaptureID); err == nil {
		stage.Close()
		t.Fatal("discarded capture retained")
	}
}

func TestHomeFilesCaptureRejectsUnsafeSources(t *testing.T) {
	for _, kind := range []string{"symlink", "ancestor-symlink", "hardlink", "fifo", "credential", "protected", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			repository := t.TempDir()
			directory := t.TempDir()
			source := filepath.Join(directory, "plain")
			os.WriteFile(source, []byte("non-secret"), 0600)
			destination := "Documents/file"
			switch kind {
			case "symlink":
				os.Rename(source, source+"-target")
				os.Symlink(source+"-target", source)
			case "ancestor-symlink":
				os.Symlink(directory, filepath.Join(repository, "link"))
				source = filepath.Join(repository, "link", "plain")
			case "hardlink":
				os.Link(source, source+"-linked")
			case "fifo":
				os.Remove(source)
				unix.Mkfifo(source, 0600)
			case "credential":
				os.WriteFile(source, []byte("-----BEGIN OPENSSH PRIVATE KEY-----"), 0600)
			case "protected":
				destination = ".config/autostart/a.desktop"
			}
			ctx := t.Context()
			if kind == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if _, err := (Local{}).CaptureHomeFiles(ctx, repository, source, destination); err == nil {
				t.Fatal("unsafe source accepted")
			}
			root, err := openHomeCaptures()
			if err != nil {
				t.Fatal(err)
			}
			entries, _ := root.ReadDir(-1)
			root.Close()
			if len(entries) != 0 {
				t.Fatal("failed capture left private data", entries)
			}
		})
	}
}

func TestHomeFilesDraftReaders(t *testing.T) {
	text := "lesson"
	draft := domain.HomeFilesDraft{Profile: domain.HomeFilesProfile{SchemaVersion: 1, Files: []domain.HomeFile{{Destination: "a", Text: &text}}}, CaptureIDs: []string{strings.Repeat("a", 32)}}
	for _, value := range []any{draft.Profile, draft, domain.HomeFilesPlanReport{SchemaVersion: 1, Candidate: &draft, Issues: []domain.ValidationIssue{}, Entries: []domain.HomeFilesEntry{}, Changes: []domain.HomeFilesChange{}, Targets: []domain.WorkspaceTarget{}}} {
		data, _ := json.Marshal(value)
		name := filepath.Join(t.TempDir(), "draft.json")
		os.WriteFile(name, data, 0600)
		got, err := ReadHomeFilesDraft(name)
		if err != nil || len(got.Profile.Files) != 1 {
			t.Fatal(err, got)
		}
	}
	for _, input := range []string{`{"profile":{"schemaVersion":1,"files":[{"destination":"a","text":"x","text":"y"}]}}`, `{"profile":{"schemaVersion":1},"CaptureIds":[]}`, `{"profile":{"schemaVersion":1},"captureIds":null}`} {
		name := filepath.Join(t.TempDir(), "bad.json")
		os.WriteFile(name, []byte(input), 0600)
		if _, err := ReadHomeFilesDraft(name); err == nil {
			t.Fatal("malformed wrapper accepted", input)
		}
	}
}

func TestHomeFilesOpaqueGitProposal(t *testing.T) {
	repository := workspaceRepository(t)
	content := []byte(strings.Repeat("large non-secret class material\n", 20000))
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	path := domain.HomeFilesDirectoryName + "/imports/" + hash
	os.MkdirAll(filepath.Dir(filepath.Join(repository, path)), 0700)
	os.WriteFile(filepath.Join(repository, path), content, 0600)
	proposal, err := (Local{}).GitCommitProposal(t.Context(), repository, []string{path})
	if err != nil || strings.Contains(proposal.Diff.Content, "large non-secret") || !strings.Contains(proposal.Diff.Content, "000000 -> 100644") {
		t.Fatal(proposal, err)
	}
	workspaceTestGit(t, repository, "add", path)
	workspaceTestGit(t, repository, "commit", "-qm", "opaque fixture")
	if _, err := (Local{}).GitCommitProposal(t.Context(), repository, []string{path}); err == nil {
		t.Fatal("unchanged opaque blob accepted")
	}
	os.Chmod(filepath.Join(repository, path), 0700)
	proposal, err = (Local{}).GitCommitProposal(t.Context(), repository, []string{path})
	if err != nil || !strings.Contains(proposal.Diff.Content, "100644 -> 100755") {
		t.Fatal(proposal, err)
	}
	os.Remove(filepath.Join(repository, path))
	proposal, err = (Local{}).GitCommitProposal(t.Context(), repository, []string{path})
	if err != nil || !strings.Contains(proposal.Diff.Content, "100644 -> 000000") {
		t.Fatal(proposal, err)
	}
}

func TestHomeFilesRealNixReviewedSave(t *testing.T) {
	if os.Getenv("NIXORIUM_TEST_WORKSPACE_NIX") != "1" {
		t.Skip("requires explicit real-Nix integration")
	}
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repository := workspaceRepository(t)
	writeGitReviewFile(t, repository, "flake.nix", homeFilesIntegrationFlake)
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = true; }`)
	workspaceTestGit(t, repository, "add", "flake.nix", "policy.nix")
	workspaceTestGit(t, repository, "commit", "-qm", "home file fixture")
	writeGitReviewFile(t, repository, "secret-key", "ignored-private-sentinel")
	source := filepath.Join(t.TempDir(), "lesson.txt")
	os.WriteFile(source, []byte("captured course material\n"), 0755)
	local := Local{}
	manager := app.NewHomeFilesManager(local)
	capture := manager.Capture(t.Context(), repository, source, "Desktop/lesson.txt")
	if capture.HasErrors() {
		t.Fatal(capture)
	}
	profile, err := app.MergeHomeFilesCapture(domain.HomeFilesProfile{SchemaVersion: 1}, capture, false)
	if err != nil {
		t.Fatal(err)
	}
	text := "theme = dark\n"
	profile.Files = append(profile.Files, domain.HomeFile{Destination: ".config/ghostty/config", Text: &text})
	draft := domain.HomeFilesDraft{Profile: profile, CaptureIDs: []string{capture.CaptureID}}
	plan := manager.Plan(t.Context(), repository, draft)
	if plan.HasErrors() || len(plan.Changes) != 2 {
		ref, _ := local.EvaluationSource(t.Context(), repository, "")
		out, evalErr := runBoundedNix(t.Context(), 4096, "flake", "archive", ref, "--json", "--no-write-lock-file", "--no-update-lock-file")
		t.Log("synthetic archive diagnostic", out, evalErr)
		t.Fatalf("plan: %+v", plan)
	}
	os.WriteFile(source, []byte("changed original after capture"), 0600)
	writeGitReviewFile(t, repository, "module.nix", "# unrelated staged edit\n")
	workspaceTestGit(t, repository, "add", "module.nix")
	// The review must reject tracked source drift; make a fresh plan with the
	// exact same captured snapshot, without reading the changed original.
	if r := manager.Apply(t.Context(), repository, draft, plan.ReviewToken); r.State != "conflict" {
		t.Fatal(r)
	}
	plan = manager.Plan(t.Context(), repository, draft)
	if plan.HasErrors() {
		t.Fatal(plan)
	}
	before := workspaceTestGit(t, repository, "diff", "--cached", "--", "module.nix")
	save := app.NewHomeFilesSaveManager(local, app.NewGitReviewManager(local))
	result := save.Save(t.Context(), repository, draft, plan.ReviewToken)
	if result.HasErrors() || !result.Recorded {
		t.Fatalf("save: %+v", result)
	}
	if after := workspaceTestGit(t, repository, "diff", "--cached", "--", "module.nix"); before != after {
		t.Fatal("unrelated index changed")
	}
	profileData := []byte(readGitReviewFile(t, repository, domain.HomeFilesFileName))
	stored, issues := domain.DecodeHomeFilesProfile(profileData)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, file := range stored.Files {
		if file.Source != nil && readGitReviewFile(t, repository, domain.HomeFilesDirectoryName+"/"+*file.Source) != "captured course material\n" {
			t.Fatal("original changed captured bytes")
		}
	}
	if result := manager.Apply(t.Context(), repository, draft, plan.ReviewToken); result.State != "conflict" {
		t.Fatal("old review replayed", result)
	}
	writeGitReviewFile(t, repository, "policy.nix", `{ allow = false; }`)
	draft.CaptureIDs = nil
	if p := manager.Plan(t.Context(), repository, draft); !p.HasErrors() {
		t.Fatal("downstream validator ignored")
	}
}

const homeFilesIntegrationFlake = `{
 outputs = { self }:
 let
  policy = import ./policy.nix;
  resolve = raw: { schemaVersion = 1; state = "prepared"; managedFile = "home-files.json";
    studentUser = "learner"; seed = "/nix/store/00000000000000000000000000000000-home";
    declared = builtins.fromJSON raw; targets = [{name="controller";role="controller";}]; };
 in assert !(builtins.pathExists (./. + "/secret-key")); {
  nixoriumValidateHomeFilesCandidate = raw: policy.allow;
  nixoriumResolveHomeFilesCandidate = resolve;
  nixoriumHomeFiles = resolve (if builtins.pathExists (./. + "/home-files.json") then builtins.readFile ./home-files.json else ''{"schemaVersion":1}'');
 };
}`
