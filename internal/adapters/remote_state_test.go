package adapters

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestRemoteInstallStateStorePublishesPrivateStrictState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewRemoteInstallStateStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion,
		OperationID:   "0123456789abcdef0123456789abcdef",
		State:         "prepared",
		Events:        []domain.RemoteInstallProgress{},
	}
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(session.OperationID)
	if err != nil || loaded.State != "prepared" {
		t.Fatalf("loaded=%+v error=%v", loaded, err)
	}
	info, err := os.Stat(filepath.Join(directory, session.OperationID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state mode=%v", info.Mode().Perm())
	}
}

func TestRemoteInstallStateStorePersistsTargetIndependentArtifacts(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewRemoteInstallStateStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	artifacts := domain.RemoteInstallArtifacts{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: id, Repository: "/deployment",
		DeploymentRevision: "0123456789abcdef0123456789abcdef01234567",
		BundlePath:         "/nix/store/11111111111111111111111111111111-remote-installer", BundleClosureBytes: 1024,
		SystemPath: "/nix/store/22222222222222222222222222222222-nixos-system-pc01-test", SystemClosureBytes: 2048,
		HostName: "pc01", HostInterface: "enp0s2", HostStaticIP: "10.0.0.1",
		CachePublicKey: "cache.example:YWJjZA==", AdminPublicKey: "ssh-ed25519 YWJjZA== admin@test",
		PreparedAt: time.Unix(1, 0).UTC(), Issues: []domain.ValidationIssue{},
	}
	session := domain.RemoteInstallSession{SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: id, State: "artifacts-ready", Artifacts: &artifacts, Events: []domain.RemoteInstallProgress{}}
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(id)
	if err != nil || loaded.Artifacts == nil || loaded.Artifacts.HostName != "pc01" || loaded.Preparation != nil || loaded.Bootstrap != nil {
		t.Fatalf("loaded target-independent artifacts=%+v error=%v", loaded, err)
	}
}

func TestRemoteInstallStateStoreRejectsSymlinkAndMismatchedState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewRemoteInstallStateStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(directory, id+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(id); err == nil {
		t.Fatal("symlink state was followed")
	}
	if err := store.Save(domain.RemoteInstallSession{SchemaVersion: 1, OperationID: id, State: "prepared", Events: []domain.RemoteInstallProgress{}}); err == nil {
		t.Fatal("unsafe existing state was replaced")
	}
}
