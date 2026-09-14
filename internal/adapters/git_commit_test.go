package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitGitPathsCommitsOnlyReviewedPaths(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "module.nix", "{ ... }: { services.openssh.enable = false; }\n")
	writeGitReviewFile(t, repository, "lab-settings.json", strings.Replace(readGitReviewFile(t, repository, "lab-settings.json"), "$6$old$admin", "$6$new$admin", 1))
	if _, err := run(context.Background(), "git", "-C", repository, "add", "lab-settings.json"); err != nil {
		t.Fatal(err)
	}
	revision, err := (Local{}).GitRevision(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"module.nix"})
	if err != nil {
		t.Fatal(err)
	}
	current, err := (Local{}).CommitGitPaths(context.Background(), repository, []string{"module.nix"}, "chore: update laboratory deployment", revision, proposal.TreeID)
	if err != nil || current == revision {
		t.Fatalf("commit revision = %q, error = %v", current, err)
	}
	headModule, err := run(context.Background(), "git", "-C", repository, "show", "HEAD:module.nix")
	if err != nil || !strings.Contains(headModule, "false") {
		t.Fatalf("selected path was not committed: %q, %v", headModule, err)
	}
	headSettings, err := run(context.Background(), "git", "-C", repository, "show", "HEAD:lab-settings.json")
	if err != nil || strings.Contains(headSettings, "$6$new$admin") {
		t.Fatalf("unrelated staged path entered commit: %q, %v", headSettings, err)
	}
	status, err := run(context.Background(), "git", "-C", repository, "status", "--porcelain=v1")
	if err != nil || !strings.Contains(status, "M  lab-settings.json") || strings.Contains(status, "module.nix") {
		t.Fatalf("unrelated index/worktree state was not preserved: %q, %v", status, err)
	}
}

func TestGitCommitProposalRejectsSecretsTransformsAndStaleContent(t *testing.T) {
	repository := newGitReviewRepository(t)
	writeGitReviewFile(t, repository, "unsafe.nix", `{ ... }: { password = "plaintext"; }`+"\n")
	if _, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"unsafe.nix"}); err == nil || !strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("plaintext secret was accepted: %v", err)
	}
	writeGitReviewFile(t, repository, ".gitattributes", "filtered filter=danger\n")
	if _, err := run(context.Background(), "git", "-C", repository, "add", ".gitattributes"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "commit", "-qm", "attributes"); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "filtered", "content\n")
	if _, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"filtered"}); err == nil || !strings.Contains(err.Error(), "content-transforming attribute") {
		t.Fatalf("Git filter attribute was accepted: %v", err)
	}
	writeGitReviewFile(t, repository, "safe", "one\n")
	revision, _ := (Local{}).GitRevision(context.Background(), repository)
	proposal, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"safe"})
	if err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "safe", "two\n")
	if _, err := (Local{}).CommitGitPaths(context.Background(), repository, []string{"safe"}, "chore: update laboratory deployment", revision, proposal.TreeID); err == nil || !strings.Contains(err.Error(), "content changed") {
		t.Fatalf("stale proposal was committed: %v", err)
	}
}

func TestCommitGitPathsDisablesRepositoryHooks(t *testing.T) {
	repository := newGitReviewRepository(t)
	hooks := filepath.Join(repository, ".hooks")
	if err := os.Mkdir(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(repository, "hook-ran")
	hook := filepath.Join(hooks, "post-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "config", "core.hooksPath", hooks); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "safe", "content\n")
	revision, _ := (Local{}).GitRevision(context.Background(), repository)
	proposal, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"safe"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).CommitGitPaths(context.Background(), repository, []string{"safe"}, "chore: update laboratory deployment", revision, proposal.TreeID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("repository hook ran or marker inspection failed: %v", err)
	}
}

func TestGitCommitProposalRejectsDirectoriesAndSymbolicLinks(t *testing.T) {
	repository := newGitReviewRepository(t)
	directory := filepath.Join(repository, "modules")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	writeGitReviewFile(t, repository, "modules/site.nix", "{ ... }: { }\n")
	if _, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"modules"}); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("directory selection was accepted: %v", err)
	}
	if err := os.Symlink("modules/site.nix", filepath.Join(repository, "linked-module.nix")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"linked-module.nix"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symbolic link selection was accepted: %v", err)
	}
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "outside.nix"), []byte("private\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(repository, "linked-directory")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{}).GitCommitProposal(context.Background(), repository, []string{"linked-directory/outside.nix"}); err == nil || !strings.Contains(err.Error(), "outside the repository") {
		t.Fatalf("path through an external symbolic-link directory was accepted: %v", err)
	}
}

func readGitReviewFile(t *testing.T, repository, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repository, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
