package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentFlakeReferenceUsesAbsoluteEscapedGitURL(t *testing.T) {
	reference, err := deploymentFlakeReference("/tmp/lab deployment")
	if err != nil {
		t.Fatal(err)
	}
	if want := "git+file:///tmp/lab%20deployment"; reference != want {
		t.Fatalf("deploymentFlakeReference() = %q; want %q", reference, want)
	}
}

func TestEnsurePrivateFilesUntrackedRejectsStagedSecret(t *testing.T) {
	repository := t.TempDir()
	if _, err := run(context.Background(), "git", "init", "-q", repository); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(repository, "secret-key")
	if err := os.WriteFile(secret, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateFilesUntracked(context.Background(), repository); err != nil {
		t.Fatalf("ignored/untracked private file was rejected: %v", err)
	}
	if _, err := run(context.Background(), "git", "-C", repository, "add", "-f", "secret-key"); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateFilesUntracked(context.Background(), repository); err == nil || !strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("tracked private file error = %v", err)
	}
}
