package adapters

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestVerifyInstalledRemotePinsPreservedHostKeyAndExactIdentity(t *testing.T) {
	runtimeRoot := t.TempDir()
	operationID := "0123456789abcdef0123456789abcdef"
	operationDirectory := filepath.Join(runtimeRoot, operationID)
	if err := os.Mkdir(operationDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	privateKey := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(privateKey, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	_, hostPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostPublic, err := ssh.NewPublicKey(hostPrivate.Public())
	if err != nil {
		t.Fatal(err)
	}
	preparation := validInstalledVerificationPreparation(operationID)
	preparation.HostKeyPublic = string(ssh.MarshalAuthorizedKey(hostPublic))[:len(ssh.MarshalAuthorizedKey(hostPublic))-1]
	preparation.HostFingerprint = ssh.FingerprintSHA256(hostPublic)
	executable := filepath.Join(t.TempDir(), "ssh")
	script := "#!/bin/sh\nprintf '%s\\n' pc01 '" + preparation.SystemPath + "' '" + preparation.DeploymentRevision + "'\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	state, err := verifyInstalledRemoteWithExecutable(context.Background(), runtimeRoot, privateKey, preparation, executable)
	if err != nil || state.Hostname != "pc01" || state.SystemPath != preparation.SystemPath {
		t.Fatalf("state=%+v error=%v", state, err)
	}
	knownHosts, err := os.ReadFile(filepath.Join(operationDirectory, "installed_known_hosts"))
	if err != nil || len(knownHosts) == 0 {
		t.Fatalf("known-hosts=%q error=%v", knownHosts, err)
	}
}

func TestVerifyInstalledRemoteRejectsIdentityMismatch(t *testing.T) {
	runtimeRoot := t.TempDir()
	operationID := "0123456789abcdef0123456789abcdef"
	if err := os.Mkdir(filepath.Join(runtimeRoot, operationID), 0700); err != nil {
		t.Fatal(err)
	}
	privateKey := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(privateKey, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	_, hostPrivate, _ := ed25519.GenerateKey(rand.Reader)
	hostPublic, _ := ssh.NewPublicKey(hostPrivate.Public())
	preparation := validInstalledVerificationPreparation(operationID)
	preparation.HostKeyPublic = string(ssh.MarshalAuthorizedKey(hostPublic))[:len(ssh.MarshalAuthorizedKey(hostPublic))-1]
	preparation.HostFingerprint = ssh.FingerprintSHA256(hostPublic)
	executable := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' pc02 /nix/store/wrong 0000000000000000000000000000000000000000\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyInstalledRemoteWithExecutable(context.Background(), runtimeRoot, privateKey, preparation, executable); err == nil {
		t.Fatal("mismatched post-boot identity accepted")
	}
}

func validInstalledVerificationPreparation(operationID string) domain.RemoteInstallPreparation {
	return domain.RemoteInstallPreparation{
		OperationID: operationID, Repository: "/deployment", DeploymentRevision: "0123456789abcdef0123456789abcdef01234567",
		BundlePath: "/nix/store/11111111111111111111111111111111-remote-installer", BundleClosureBytes: 1024,
		SystemPath: "/nix/store/00000000000000000000000000000000-nixos-system-pc01-test", SystemClosureBytes: 2048,
		Host:           domain.RemoteInstallHost{Name: "pc01", Interface: "enp0s2", LiveIP: "192.0.2.20", StaticIP: "10.0.0.1"},
		Cache:          domain.RemoteInstallCache{URL: "http://192.0.2.10:5000", PublicKey: "cache.example:YWJjZA=="},
		AdminPublicKey: "ssh-ed25519 YWJjZA== admin@test",
		Facts: domain.RemoteMachineFacts{
			SchemaVersion: 1, VariantID: "installer", VersionID: "26.05", Architecture: "x86_64", UEFI: true, SudoReady: true,
			BootID: "01234567-89ab-cdef-0123-456789abcdef", Interfaces: []domain.RemoteNetworkInterface{}, Disks: []domain.RemoteDisk{},
		},
		PreparedAt: time.Unix(1, 0).UTC(), Issues: []domain.ValidationIssue{},
	}
}
