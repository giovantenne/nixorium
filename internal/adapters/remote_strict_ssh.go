package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const strictLiveSSHCommandTimeout = 30 * time.Minute

type StrictLiveSSH struct {
	address        string
	port           int
	privateKeyPath string
	knownHostsPath string
	sshExecutable  string
}

func NewStrictLiveSSH(session VerifiedLiveSession) (*StrictLiveSSH, error) {
	if netAddress := canonicalRemoteIPv4(session.Address); netAddress == "" || session.Port < 1 || session.Port > 65535 {
		return nil, errors.New("strict live SSH endpoint is invalid")
	}
	for _, path := range []string{session.PrivateKeyPath, session.KnownHostsPath} {
		if !filepathIsAbsoluteClean(path) {
			return nil, errors.New("strict live SSH credential path is invalid")
		}
		descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return nil, fmt.Errorf("open strict live SSH credential: %w", err)
		}
		file := os.NewFile(uintptr(descriptor), path)
		err = validatePrivateOwnedFile(file, syscall.S_IFREG, 0600)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("inspect strict live SSH credential: %w", err)
		}
	}
	return &StrictLiveSSH{
		address: session.Address, port: session.Port, privateKeyPath: session.PrivateKeyPath,
		knownHostsPath: session.KnownHostsPath, sshExecutable: "ssh",
	}, nil
}

func (connection *StrictLiveSSH) PullBundle(ctx context.Context, cache domain.RemoteInstallCache, bundlePath string) error {
	if err := domain.ValidateRemoteInstallCache(cache); err != nil {
		return err
	}
	if !domain.ValidStorePath(bundlePath) {
		return errors.New("remote installer bundle path is invalid")
	}
	command := strings.Join([]string{
		"/run/current-system/sw/bin/nix", "--extra-experimental-features", "'nix-command flakes'", "copy",
		"--from", cache.URL, bundlePath,
		"--option", "substituters", cache.URL,
		"--option", "trusted-public-keys", cache.PublicKey,
		"--option", "require-sigs", "true",
		"--option", "fallback", "false",
		"--option", "builders", "''",
	}, " ")
	_, err := connection.run(ctx, command, nil, 64*1024)
	if err != nil {
		return fmt.Errorf("pull signed remote installer bundle: %w", err)
	}
	return nil
}

func (connection *StrictLiveSSH) CheckCacheEndpoint(ctx context.Context, cache domain.RemoteInstallCache) error {
	if err := domain.ValidateRemoteInstallCache(cache); err != nil {
		return err
	}
	command := "/run/current-system/sw/bin/curl --fail --silent --show-error --max-time 5 " + cache.URL + "/nix-cache-info"
	output, err := connection.run(ctx, command, nil, 16*1024)
	if err != nil {
		return err
	}
	if !strings.Contains(string(output), "StoreDir: /nix/store\n") && string(output) != "StoreDir: /nix/store" {
		return errors.New("cache endpoint returned invalid nix-cache-info")
	}
	return nil
}

func (connection *StrictLiveSSH) VerifySignedClosure(ctx context.Context, cache domain.RemoteInstallCache, storePath string) error {
	if err := domain.ValidateRemoteInstallCache(cache); err != nil {
		return err
	}
	if !domain.ValidStorePath(storePath) {
		return errors.New("closure store path is invalid")
	}
	command := strings.Join([]string{
		"/run/current-system/sw/bin/nix", "--extra-experimental-features", "'nix-command flakes'", "path-info",
		"--store", cache.URL, "--recursive", storePath,
		"--option", "trusted-public-keys", cache.PublicKey,
		"--option", "require-sigs", "true", "--option", "fallback", "false", "--option", "builders", "''",
	}, " ")
	if _, err := connection.run(ctx, command, nil, domain.RemoteInstallFactsMaxBytes); err != nil {
		return fmt.Errorf("verify signed closure metadata: %w", err)
	}
	return nil
}

func (connection *StrictLiveSSH) VerifyWiredInterface(ctx context.Context, interfaceName string) error {
	if !domain.ValidRemoteInterfaceName(interfaceName) {
		return errors.New("remote live interface name is invalid")
	}
	command := "/run/current-system/sw/bin/test ! -d /sys/class/net/" + interfaceName + "/wireless"
	if _, err := connection.run(ctx, command, nil, 1024); err != nil {
		return fmt.Errorf("remote installation requires the declared wired interface: %w", err)
	}
	return nil
}

func (connection *StrictLiveSSH) Probe(ctx context.Context, bundlePath string) (domain.RemoteMachineFacts, error) {
	if !domain.ValidStorePath(bundlePath) {
		return domain.RemoteMachineFacts{}, errors.New("remote installer bundle path is invalid")
	}
	command := bundlePath + "/bin/nixorium-remote-client-installer probe"
	output, err := connection.run(ctx, command, nil, domain.RemoteInstallFactsMaxBytes)
	if err != nil {
		return domain.RemoteMachineFacts{}, err
	}
	return domain.DecodeRemoteMachineFacts(output)
}

func (connection *StrictLiveSSH) ValidatePlan(ctx context.Context, bundlePath string, plan []byte) error {
	if !domain.ValidStorePath(bundlePath) || len(plan) == 0 || len(plan) > domain.RemoteInstallPlanMaxBytes {
		return errors.New("remote installation plan transfer is invalid")
	}
	if _, err := domain.DecodeRemoteInstallPlan(plan); err != nil {
		return err
	}
	command := bundlePath + "/bin/nixorium-remote-client-installer validate-plan"
	output, err := connection.run(ctx, command, plan, 1024)
	if err != nil {
		return err
	}
	if string(output) != "valid\n" {
		return errors.New("remote installer returned an invalid plan acknowledgement")
	}
	return nil
}

func (connection *StrictLiveSSH) Status(ctx context.Context, bundlePath, operationID string) (domain.RemoteInstallReceipt, error) {
	if !domain.ValidStorePath(bundlePath) || !remoteStateID(operationID) {
		return domain.RemoteInstallReceipt{}, errors.New("remote installation status selector is invalid")
	}
	command := bundlePath + "/bin/nixorium-remote-client-installer status " + operationID
	output, err := connection.run(ctx, command, nil, domain.RemoteInstallPlanMaxBytes)
	if err != nil {
		return domain.RemoteInstallReceipt{}, err
	}
	return domain.DecodeRemoteInstallReceipt(output)
}

func (connection *StrictLiveSSH) run(ctx context.Context, remoteCommand string, input []byte, maximum int) ([]byte, error) {
	commandContext, cancel := context.WithTimeout(ctx, strictLiveSSHCommandTimeout)
	defer cancel()
	arguments := connection.arguments(remoteCommand)
	command := exec.CommandContext(commandContext, connection.sshExecutable, arguments...)
	command.Env = environmentWithout(os.Environ(), "SSH_AUTH_SOCK", "SSH_ASKPASS", "DISPLAY")
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	output := &boundedCommandBuffer{limit: maximum}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if output.truncated {
		return nil, errors.New("strict SSH output exceeded its limit")
	}
	if err != nil {
		return nil, fmt.Errorf("strict SSH command failed: %w: %s", err, sanitizeOperationLog(output.buffer.Bytes()))
	}
	return append([]byte(nil), output.buffer.Bytes()...), nil
}

func (connection *StrictLiveSSH) arguments(remoteCommand string) []string {
	return []string{
		"-F", "/dev/null", "-i", connection.privateKeyPath, "-p", strconv.Itoa(connection.port),
		"-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none",
		"-o", "UserKnownHostsFile=" + connection.knownHostsPath,
		"-o", "GlobalKnownHostsFile=/dev/null", "-o", "StrictHostKeyChecking=yes",
		"-o", "HostKeyAlgorithms=ssh-ed25519", "-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no",
		"-o", "PubkeyAuthentication=yes", "-o", "PreferredAuthentications=publickey",
		"-o", "ClearAllForwardings=yes", "-o", "ForwardAgent=no", "-o", "ForwardX11=no",
		"-o", "PermitLocalCommand=no", "-o", "ProxyCommand=none", "-o", "ProxyJump=none",
		"-o", "UpdateHostKeys=no", "-o", "ControlMaster=no", "-o", "ControlPath=none",
		"-o", "CanonicalizeHostname=no", "-o", "RequestTTY=no", "-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=10", "-o", "ConnectionAttempts=1",
		"-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3",
		"root@" + connection.address, remoteCommand,
	}
}

func environmentWithout(environment []string, names ...string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		keep := true
		for _, name := range names {
			if strings.HasPrefix(entry, name+"=") {
				keep = false
				break
			}
		}
		if keep {
			result = append(result, entry)
		}
	}
	return result
}

func canonicalRemoteIPv4(value string) string {
	parsed := net.ParseIP(value)
	if parsed == nil || parsed.To4() == nil || parsed.String() != value {
		return ""
	}
	return value
}

func filepathIsAbsoluteClean(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value
}
