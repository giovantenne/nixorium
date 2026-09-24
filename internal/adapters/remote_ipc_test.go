package adapters

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestRemoteInstallIPCAuthenticatesPeerAndBindsResponse(t *testing.T) {
	temporary, err := os.MkdirTemp("/tmp", "nixorium-ipc-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(temporary) })
	runtimeDirectory := filepath.Join(temporary, "runtime")
	if err := os.Mkdir(runtimeDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(runtimeDirectory, "control.sock")
	server := NewRemoteInstallIPCServer(socketPath, func(_ context.Context, request domain.RemoteInstallRequest, _ *LivePassword) domain.RemoteInstallResponse {
		return domain.RemoteInstallResponse{State: "ready", OperationID: request.OperationID}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	for range 100 {
		if info, err := os.Stat(socketPath); err == nil && info.Mode().Perm() == 0600 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	request := domain.RemoteInstallRequest{
		SchemaVersion: domain.RemoteInstallSchemaVersion,
		RequestID:     "0123456789abcdef0123456789abcdef",
		Operation:     domain.RemoteInstallStatusOperation,
		OperationID:   "fedcba9876543210fedcba9876543210",
	}
	response, err := RemoteInstallIPCRequest(context.Background(), socketPath, request)
	if err != nil || response.State != "ready" || response.OperationID != request.OperationID {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRemoteInstallIPCRejectsOversizedFrame(t *testing.T) {
	if _, err := readRemoteIPCFrame(strings.NewReader(strings.Repeat("x", domain.RemoteInstallPlanMaxBytes+1)+"\n"), domain.RemoteInstallPlanMaxBytes); err == nil {
		t.Fatal("oversized IPC frame was accepted")
	}
}

func TestRemoteInstallIPCTransfersBootstrapSecretInDistinctBoundedFrame(t *testing.T) {
	temporary, err := os.MkdirTemp("/tmp", "nixorium-ipc-secret-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(temporary) })
	runtimeDirectory := filepath.Join(temporary, "runtime")
	if err := os.Mkdir(runtimeDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(runtimeDirectory, "control.sock")
	received := make(chan string, 1)
	server := NewRemoteInstallIPCServer(socketPath, func(_ context.Context, _ domain.RemoteInstallRequest, secret *LivePassword) domain.RemoteInstallResponse {
		value, _ := secret.snapshot()
		received <- string(value)
		zeroBytes(value)
		return domain.RemoteInstallResponse{State: "bootstrapped", OperationID: "fedcba9876543210fedcba9876543210"}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	for range 100 {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	input := []byte("temporary-secret")
	secret, err := NewLivePassword(input)
	if err != nil {
		t.Fatal(err)
	}
	request := domain.RemoteInstallRequest{
		SchemaVersion: domain.RemoteInstallSchemaVersion, RequestID: "0123456789abcdef0123456789abcdef",
		Operation: domain.RemoteInstallBootstrapOperation, Host: "pc01", Address: "192.0.2.20",
		Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	response, err := RemoteInstallIPCSecretRequest(context.Background(), socketPath, request, secret)
	if err != nil || response.State != "bootstrapped" || <-received != "temporary-secret" {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	if len(secret.value) != 0 || !bytes.Equal(input, make([]byte, len(input))) {
		t.Fatal("secret ownership was not cleared after IPC transfer")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRemoteInstallIPCRejectsUnsafeSocketDirectory(t *testing.T) {
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	server := NewRemoteInstallIPCServer(filepath.Join(runtimeDirectory, "control.sock"), func(context.Context, domain.RemoteInstallRequest, *LivePassword) domain.RemoteInstallResponse {
		return domain.RemoteInstallResponse{}
	})
	if err := server.Serve(context.Background()); err == nil {
		t.Fatal("unsafe runtime directory was accepted")
	}
}
