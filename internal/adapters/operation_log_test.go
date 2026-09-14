package adapters

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeploymentOperationCreatesPrivateDurableLogAndSerializesRuns(t *testing.T) {
	stateRoot := t.TempDir()
	now := time.Date(2026, 9, 13, 12, 30, 0, 123, time.UTC)
	operation, err := openDeploymentOperation(stateRoot, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := operation.Writer().Write([]byte("deployment output\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeploymentOperation(stateRoot, now.Add(time.Second)); err == nil {
		t.Fatal("concurrent deployment operation was accepted")
	}
	path := operation.Path
	if err := operation.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("log mode = %o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "deployment output\n" {
		t.Fatalf("log = %q, error = %v", data, err)
	}
	second, err := openDeploymentOperation(stateRoot, now.Add(time.Second))
	if err != nil {
		t.Fatalf("retry after close: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentOperationRejectsSymlinkDirectory(t *testing.T) {
	stateRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(stateRoot, "nixorium"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(stateRoot, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(stateRoot, "nixorium", "operations")); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeploymentOperation(stateRoot, time.Now()); err == nil {
		t.Fatal("symlink operation directory was accepted")
	}
}

func TestDeploymentOperationRejectsSymlinkLock(t *testing.T) {
	stateRoot := t.TempDir()
	directory := filepath.Join(stateRoot, "nixorium", "operations")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(stateRoot, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, "deploy.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeploymentOperation(stateRoot, time.Now()); err == nil {
		t.Fatal("symlink deployment lock was accepted")
	}
}
