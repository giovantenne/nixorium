package homereset

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAdditionalHomeFileIntegrityAndModes(t *testing.T) {
	home := t.TempDir()
	content := []byte("#!/bin/sh\nexit 0\n")
	name := filepath.Join(home, "lesson")
	if err := os.WriteFile(name, content, 0600); err != nil {
		t.Fatal(err)
	}
	entries := []domain.HomeFilesSeedEntry{{Destination: "lesson", SHA256: fmt.Sprintf("%x", sha256.Sum256(content)), Size: int64(len(content)), Executable: true}}
	if err := validateHomeFilesManifest(home, entries, true); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(name)
	if info.Mode().Perm() != 0700 {
		t.Fatal(info.Mode())
	}
	entries[0].Executable = false
	if err := validateHomeFilesManifest(home, entries, true); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(name)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	entries[0].SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("changed")))
	if err := validateHomeFilesManifest(home, entries, true); err == nil {
		t.Fatal("mismatch accepted")
	}
}
func TestAdditionalHomeFileRejectsLinksAndProtectedPaths(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "a"), []byte("a"), 0600)
	os.Symlink(outside, filepath.Join(home, "linked"))
	entry := domain.HomeFilesSeedEntry{Destination: "linked/a", SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("a"))), Size: 1}
	if err := validateHomeFilesManifest(home, []domain.HomeFilesSeedEntry{entry}, true); err == nil {
		t.Fatal("ancestor link accepted")
	}
	entry.Destination = ".bashrc"
	if err := validateHomeFilesManifest(home, []domain.HomeFilesSeedEntry{entry}, true); err == nil {
		t.Fatal("protected destination accepted")
	}
}
