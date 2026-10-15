package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDetectInterfaceReload(t *testing.T) {
	root := t.TempDir()
	store := "/nix/store/0123456789abcdfghijklmnpqrsvwxyz-nixorium-1.2.3"
	oldPackage := filepath.Join(root, "old", "bin", "nixorium")
	newPackage := filepath.Join(root, "new", "bin", "nixorium")
	writeExecutable(t, oldPackage)
	writeExecutable(t, newPackage)
	writeExecutable(t, filepath.Join(root, "old", "bin", ".nixorium-wrapped"))
	link := filepath.Join(root, "system", "nixorium")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(newPackage, link); err != nil {
		t.Fatal(err)
	}
	running := func(path string) func() (string, error) {
		return func() (string, error) { return path, nil }
	}

	if _, reload := detectInterfaceReload(running(newPackage), link); reload {
		t.Fatal("the same binary through a symlink must not reload")
	}
	wrapped := filepath.Join(root, "new", "bin", ".nixorium-wrapped")
	writeExecutable(t, wrapped)
	if _, reload := detectInterfaceReload(running(wrapped), link); reload {
		t.Fatal("the wrapped program of the installed package must not reload")
	}
	if _, reload := detectInterfaceReload(running(filepath.Join(root, "old", "bin", ".nixorium-wrapped")), link); !reload {
		t.Fatal("a different package must reload")
	}
	if _, reload := detectInterfaceReload(running(oldPackage), link); !reload {
		t.Fatal("a different binary must reload")
	}
	if _, reload := detectInterfaceReload(running(oldPackage), filepath.Join(root, "missing")); reload {
		t.Fatal("a missing installed command must not reload")
	}
	if _, reload := detectInterfaceReload(running(oldPackage), filepath.Join(root, "old")); reload {
		t.Fatal("a directory is not an installed command")
	}
	if _, reload := detectInterfaceReload(func() (string, error) { return "", errors.New("unknown") }, link); reload {
		t.Fatal("an unknown running binary must not reload")
	}
	reload, ok := detectInterfaceReload(running(oldPackage), link)
	if !ok || reload.Version != "" {
		t.Fatalf("a path outside the store names no version: %+v", reload)
	}
	if got := installedStorePackage.FindStringSubmatch(store + "/bin/nixorium"); len(got) != 2 || got[1] != "1.2.3" {
		t.Fatalf("store path version not parsed: %v", got)
	}
}

func TestReopenInterfaceExecsInstalledBinary(t *testing.T) {
	original := execInterface
	defer func() { execInterface = original }()
	var gotPath string
	var gotArgv, gotEnv []string
	execInterface = func(path string, argv, environment []string) error {
		gotPath, gotArgv, gotEnv = path, argv, environment
		return nil
	}
	var stderr bytes.Buffer
	code := reopenInterface("/run/current-system/sw/bin/nixorium", []string{"--repo", "/lab"}, []string{"A=1"}, &stderr)
	if code != 0 || stderr.Len() != 0 || gotPath != "/run/current-system/sw/bin/nixorium" ||
		!reflect.DeepEqual(gotArgv, []string{"/run/current-system/sw/bin/nixorium", "--repo", "/lab"}) || !reflect.DeepEqual(gotEnv, []string{"A=1"}) {
		t.Fatalf("unexpected exec: path=%q argv=%v env=%v code=%d stderr=%q", gotPath, gotArgv, gotEnv, code, stderr.String())
	}
}

func TestReopenInterfaceFailureStaysCalm(t *testing.T) {
	original := execInterface
	defer func() { execInterface = original }()
	execInterface = func(string, []string, []string) error { return errors.New("exec format error") }
	var stderr bytes.Buffer
	code := reopenInterface("/run/current-system/sw/bin/nixorium", nil, nil, &stderr)
	if code != 0 || stderr.String() != "Reopen Nixorium to use the updated interface.\n" {
		t.Fatalf("exec failure handling: code=%d stderr=%q", code, stderr.String())
	}
}
