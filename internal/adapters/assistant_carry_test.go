package adapters

import (
	"errors"
	"os"
	"testing"
)

func TestAssistantCarryOverProposal(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		repo := workspaceRepository(t)
		writeGitReviewFile(t, repo, "flake.nix", "{ outputs = _: {}; }\n")
		writeGitReviewFile(t, repo, "modules/shared.nix", "{ }\n")
		writeGitReviewFile(t, repo, "modules/old.nix", "{ }\n")
		workspaceTestGit(t, repo, "add", "--all")
		workspaceTestGit(t, repo, "commit", "-qm", "synthetic base")
		state := t.TempDir()
		if err := os.Chmod(state, 0700); err != nil {
			t.Fatal(err)
		}
		store := AssistantWorkspaceStore{Directory: state}
		old, err := store.CreateConversation(repo)
		if err != nil {
			t.Fatal(err)
		}
		workspace, err := store.Open(old.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = workspace.Write("modules/lazyvim.nix", "{ programs.neovim.enable = true; }\n", false); err != nil {
			t.Fatal(err)
		}
		if err = workspace.Write("modules/shared.nix", "{ imports = [ ./lazyvim.nix ]; }\n", false); err != nil {
			t.Fatal(err)
		}
		if err = workspace.Delete("modules/old.nix"); err != nil {
			t.Fatal(err)
		}
		workspace.Close()
		// The deployment moves on: an unrelated file, or a proposal file.
		changed := "flake.lock"
		if conflict {
			changed = "modules/shared.nix"
		}
		writeGitReviewFile(t, repo, changed, "{ updated = true; }\n")
		workspaceTestGit(t, repo, "add", "--all")
		workspaceTestGit(t, repo, "commit", "-qm", "synthetic update")
		next, err := store.CreateConversation(repo)
		if err != nil {
			t.Fatal(err)
		}
		carried, err := store.CarryOverProposal(old.ID, next.ID)
		if conflict {
			if !errors.Is(err, ErrAssistantProposalConflict) || carried != 0 {
				t.Fatal("conflicting proposal carried over", carried, err)
			}
			if count, _ := store.ChangedFiles(next.ID); count != 0 {
				t.Fatal("conflict left files in the new chat", count)
			}
			continue
		}
		if err != nil || carried != 3 {
			t.Fatal("proposal not carried over", carried, err)
		}
		target, err := store.Open(next.ID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := target.Read("modules/shared.nix")
		if err != nil || string(data) != "{ imports = [ ./lazyvim.nix ]; }\n" {
			t.Fatal("carried file differs", string(data), err)
		}
		if _, err = target.Read("modules/old.nix"); err == nil {
			t.Fatal("carried deletion missing")
		}
		target.Close()
	}
}
