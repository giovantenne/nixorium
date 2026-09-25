package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

const (
	remoteInstallSocketPath         = "/run/nixorium/remote-install/control.sock"
	remoteInstallDeploymentPathFile = "/etc/nixorium/deployment-path"
	remoteInstallAdminHome          = "/home/admin"
)

func runInstallCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	if err := requireManagedRemoteInstallClient(repository); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if err := ensureRemoteInstallWorker(ctx); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	switch options.subcommand {
	case "usb-prepare":
		response, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{Operation: domain.RemoteInstallPrepareOperation, Host: options.host})
		return renderRemoteInstallResponse(response, err, options.json, stdout, stderr)
	case "usb-start":
		return runUSBInstallStart(ctx, options.host, stdout, stderr)
	case "usb-reboot", "usb-close":
		return runInteractiveRemoteInstallAction(ctx, options.subcommand, options.operationID, stdout, stderr)
	default:
		operation := map[string]domain.RemoteInstallOperation{
			"usb-status": domain.RemoteInstallStatusOperation, "usb-reconcile": domain.RemoteInstallReconcileOperation,
			"usb-verify": domain.RemoteInstallVerifyOperation, "usb-cancel": domain.RemoteInstallCancelOperation,
		}[options.subcommand]
		response, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{Operation: operation, OperationID: options.operationID})
		return renderRemoteInstallResponse(response, err, options.json, stdout, stderr)
	}
}

func runUSBInstallStart(ctx context.Context, host string, stdout, stderr io.Writer) int {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintln(stderr, "Error: install usb start requires a controlling terminal")
		return 2
	}
	defer terminal.Close()
	reader := bufio.NewReaderSize(terminal, 4096)
	fmt.Fprintln(terminal, "Boot the official NixOS Minimal ISO in UEFI mode, enable SSH, and read its Ed25519 fingerprint from the local console.")
	address, err := promptRemoteInstallValue(reader, terminal, "Live IPv4 address")
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	fingerprint, err := promptRemoteInstallValue(reader, terminal, "Console host fingerprint (SHA256:...)")
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	request, err := newRemoteInstallRequest(domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallBootstrapOperation, Host: host, Address: address, Fingerprint: fingerprint,
	})
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	password, err := (presentation.TerminalSecretReader{Input: terminal, Output: terminal}).ReadSecret("Temporary live ISO password: ")
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	secret, err := adapters.NewLivePassword(password)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	bootstrap, err := adapters.RemoteInstallIPCSecretRequest(ctx, remoteInstallSocketPath, request, secret)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	presentation.RemoteInstallResponseText(terminal, bootstrap)
	if remoteInstallResponseFailed(bootstrap) || bootstrap.OperationID == "" {
		return 1
	}
	prepared, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallPrepareOperation, OperationID: bootstrap.OperationID, Host: host,
	})
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	presentation.RemoteInstallResponseText(terminal, prepared)
	if remoteInstallResponseFailed(prepared) || prepared.Session == nil || prepared.Session.Preparation == nil {
		return 1
	}
	disk, err := promptRemoteInstallValue(reader, terminal, "Exact eligible disk path to erase")
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	allowRotation := false
	if prepared.Session.Preparation.KnownHostConflict {
		fmt.Fprintln(terminal, "The static address has a different known host key. The verified new key will replace only that reviewed entry after successful post-boot verification.")
		allowRotation, err = confirmRemoteInstall(reader, terminal, "ROTATE HOST KEY")
		if err != nil || !allowRotation {
			return cancelUSBStart(ctx, bootstrap.OperationID, terminal, stderr)
		}
	}
	plan, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallPlanOperation, OperationID: bootstrap.OperationID, Disk: disk, HostKeyRotation: allowRotation,
	})
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	presentation.RemoteInstallResponseText(terminal, plan)
	if remoteInstallResponseFailed(plan) || plan.Plan == nil {
		return 1
	}
	approved, err := confirmRemoteInstall(reader, terminal, plan.Plan.Confirmation)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if !approved {
		return cancelUSBStart(ctx, bootstrap.OperationID, terminal, stderr)
	}
	applied, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallApplyOperation, OperationID: bootstrap.OperationID,
		ReviewToken: plan.Plan.ReviewToken, Confirmation: plan.Plan.Confirmation,
	})
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	presentation.RemoteInstallResponseText(stdout, applied)
	if !remoteInstallResponseFailed(applied) {
		fmt.Fprintf(stdout, "Track this independent job with: nixorium install usb status --id %s\n", bootstrap.OperationID)
	}
	if remoteInstallResponseFailed(applied) {
		return 1
	}
	return 0
}

func cancelUSBStart(ctx context.Context, operationID string, output, stderr io.Writer) int {
	response, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{Operation: domain.RemoteInstallCancelOperation, OperationID: operationID})
	if err != nil {
		fmt.Fprintln(stderr, "Error: no apply was sent, but pre-apply cleanup could not be confirmed:", err)
		return 1
	}
	presentation.RemoteInstallResponseText(output, response)
	if remoteInstallResponseFailed(response) || response.State != "cancelled" {
		fmt.Fprintln(stderr, "Error: no apply was sent, but pre-apply cleanup requires reconciliation")
		return 1
	}
	fmt.Fprintln(output, "Installation cancelled before apply.")
	return 0
}

func runInteractiveRemoteInstallAction(ctx context.Context, action, operationID string, stdout, stderr io.Writer) int {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(stderr, "Error: install usb %s requires a controlling terminal\n", strings.TrimPrefix(action, "usb-"))
		return 2
	}
	defer terminal.Close()
	status, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{Operation: domain.RemoteInstallStatusOperation, OperationID: operationID})
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	presentation.RemoteInstallResponseText(terminal, status)
	word := "REBOOT"
	operation := domain.RemoteInstallRebootOperation
	if action == "usb-close" {
		word = "CLOSE"
		operation = domain.RemoteInstallCloseOperation
	}
	approved, err := confirmRemoteInstall(bufio.NewReaderSize(terminal, 4096), terminal, word)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if !approved {
		fmt.Fprintln(terminal, "No action was sent.")
		return 0
	}
	response, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{Operation: operation, OperationID: operationID})
	return renderRemoteInstallResponse(response, err, false, stdout, stderr)
}

func promptRemoteInstallValue(reader *bufio.Reader, writer io.Writer, label string) (string, error) {
	if _, err := fmt.Fprintf(writer, "%s: ", label); err != nil {
		return "", err
	}
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 512 || strings.ContainsRune(value, '\x00') {
		return "", errors.New("interactive value is empty or exceeds its safe limit")
	}
	return value, nil
}

func confirmRemoteInstall(reader *bufio.Reader, writer io.Writer, expected string) (bool, error) {
	fmt.Fprintf(writer, "Type %s to continue: ", expected)
	value, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(value) == expected, nil
}

func ensureRemoteInstallWorker(ctx context.Context) error {
	if err := (adapters.Local{}).StartSystemUnit(ctx, "nixorium-remote-install.service"); err != nil {
		return fmt.Errorf("start the fixed remote installation worker: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, err := remoteInstallRequest(ctx, domain.RemoteInstallRequest{Operation: domain.RemoteInstallWorkerProbeOperation})
		if err == nil && response.State == "ready" {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("remote installation worker did not publish its private control socket")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func remoteInstallRequest(ctx context.Context, request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
	validated, err := newRemoteInstallRequest(request)
	if err != nil {
		return domain.RemoteInstallResponse{}, err
	}
	return adapters.RemoteInstallIPCRequest(ctx, remoteInstallSocketPath, validated)
}

func newRemoteInstallRequest(request domain.RemoteInstallRequest) (domain.RemoteInstallRequest, error) {
	requestID, err := domain.NewRemoteOperationID()
	if err != nil {
		return request, err
	}
	request.SchemaVersion = domain.RemoteInstallSchemaVersion
	request.RequestID = requestID
	content, err := json.Marshal(request)
	if err != nil {
		return request, err
	}
	return domain.DecodeRemoteInstallRequest(content)
}

func renderRemoteInstallResponse(response domain.RemoteInstallResponse, err error, jsonOutput bool, stdout, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if jsonOutput {
		if err := presentation.JSON(stdout, response); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.RemoteInstallResponseText(stdout, response)
	}
	if remoteInstallResponseFailed(response) {
		return 1
	}
	return 0
}

func remoteInstallResponseFailed(response domain.RemoteInstallResponse) bool {
	if strings.Contains(response.State, "failed") {
		return true
	}
	switch response.State {
	case "blocked", "failed", "unavailable", "reconciliation-required":
		return true
	default:
		return false
	}
}

func requireManagedRemoteInstallClient(repository string) error {
	descriptor, err := syscall.Open(remoteInstallDeploymentPathFile, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("inspect fixed remote installation deployment: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), remoteInstallDeploymentPathFile)
	defer file.Close()
	var stat syscall.Stat_t
	if err := syscall.Fstat(descriptor, &stat); err != nil || stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Mode&0022 != 0 || stat.Uid != 0 || stat.Size < 2 || stat.Size > 4096 {
		return errors.New("fixed remote installation deployment file is unsafe")
	}
	content, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return err
	}
	fixed := strings.TrimSpace(string(content))
	if strings.ContainsAny(fixed, "\r\n\x00") || !filepath.IsAbs(fixed) || filepath.Clean(fixed) != fixed || fixed != repository {
		return errors.New("--repo must resolve to the deployment fixed for the controller worker")
	}
	homeInfo, err := os.Lstat(remoteInstallAdminHome)
	if err != nil {
		return fmt.Errorf("inspect controller administrator home: %w", err)
	}
	homeStat, ok := homeInfo.Sys().(*syscall.Stat_t)
	if !ok || !homeInfo.IsDir() || homeInfo.Mode()&os.ModeSymlink != 0 || homeStat.Uid != uint32(os.Geteuid()) || os.Geteuid() == 0 {
		return errors.New("USB SSH installation must run as the controller administrator account")
	}
	return nil
}
