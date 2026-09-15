package adapters

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOperationProgressAcceptsOnlyBoundedPrivateRegularFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "progress.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readOperationProgressFile(path); err != nil || len(data) == 0 {
		t.Fatalf("data = %q, error = %v", data, err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperationProgressFile(path); err == nil {
		t.Fatal("public progress file was accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maximumOperationProgressBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperationProgressFile(path); err == nil {
		t.Fatal("oversized progress file was accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, "target"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := readOperationProgressFile(path); err == nil {
		t.Fatal("symlinked progress file was accepted")
	}
}

func TestReadOperationProgressRejectsUnknownOperation(t *testing.T) {
	if _, err := (Local{}).ReadOperationProgress("../../secret"); err == nil {
		t.Fatal("unknown operation progress path was accepted")
	}
}
