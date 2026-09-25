package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/giovantenne/nixorium/internal/domain"
)

type InstalledRemoteState struct {
	Hostname   string
	SystemPath string
	Revision   string
}

func VerifyInstalledRemote(ctx context.Context, runtimeRoot, installedPrivateKey string, preparation domain.RemoteInstallPreparation) (InstalledRemoteState, error) {
	return verifyInstalledRemoteWithExecutable(ctx, runtimeRoot, installedPrivateKey, preparation, "ssh")
}

func verifyInstalledRemoteWithExecutable(ctx context.Context, runtimeRoot, installedPrivateKey string, preparation domain.RemoteInstallPreparation, sshExecutable string) (InstalledRemoteState, error) {
	var result InstalledRemoteState
	if err := domain.ValidateRemoteInstallPreparation(preparation); err != nil {
		return result, err
	}
	if !filepath.IsAbs(runtimeRoot) || filepath.Clean(runtimeRoot) != runtimeRoot {
		return result, errors.New("remote verification runtime is invalid")
	}
	keyInfo, err := os.Lstat(installedPrivateKey)
	if err != nil {
		return result, fmt.Errorf("inspect installed-system admin key: %w", err)
	}
	keyStat, ok := keyInfo.Sys().(*syscall.Stat_t)
	if !ok || !keyInfo.Mode().IsRegular() || keyInfo.Mode()&os.ModeSymlink != 0 || keyInfo.Mode().Perm() != 0600 || keyStat.Uid != uint32(os.Geteuid()) {
		return result, errors.New("installed-system admin key is unsafe")
	}
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(preparation.HostKeyPublic + "\n"))
	if err != nil || publicKey.Type() != ssh.KeyAlgoED25519 || ssh.FingerprintSHA256(publicKey) != preparation.HostFingerprint {
		return result, errors.New("preserved installed-system host key is invalid")
	}
	operationDirectory := filepath.Join(runtimeRoot, preparation.OperationID)
	// Only the administrator key and persisted host pin are needed after boot.
	// Recreate this public pin directory when a controller reboot cleared /run.
	if err := ensurePrivateOwnedDirectory(runtimeRoot); err != nil {
		return result, fmt.Errorf("inspect remote verification root: %w", err)
	}
	if err := os.Mkdir(operationDirectory, 0700); err != nil && !os.IsExist(err) {
		return result, fmt.Errorf("create remote verification runtime: %w", err)
	}
	if err := ensurePrivateOwnedDirectory(operationDirectory); err != nil {
		return result, fmt.Errorf("inspect remote verification runtime: %w", err)
	}
	knownHostsPath := filepath.Join(operationDirectory, "installed_known_hosts")
	expected := []byte(knownhosts.Line([]string{preparation.Host.StaticIP}, publicKey) + "\n")
	if err := ensureExactPrivateFile(knownHostsPath, expected); err != nil {
		return result, err
	}
	connection, err := NewStrictLiveSSH(VerifiedLiveSession{
		Address: preparation.Host.StaticIP, Port: 22, PrivateKeyPath: installedPrivateKey, KnownHostsPath: knownHostsPath,
	})
	if err != nil {
		return result, err
	}
	connection.sshExecutable = sshExecutable
	command := "/run/current-system/sw/bin/hostname; /run/current-system/sw/bin/readlink -f /run/current-system; /run/current-system/sw/bin/nixos-version --configuration-revision"
	output, err := connection.run(ctx, command, nil, 16*1024)
	if err != nil {
		return result, fmt.Errorf("verify installed system over pinned SSH: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 3 {
		return result, errors.New("installed system returned an invalid identity record")
	}
	result = InstalledRemoteState{Hostname: lines[0], SystemPath: lines[1], Revision: lines[2]}
	if result.Hostname != preparation.Host.Name || result.SystemPath != preparation.SystemPath || result.Revision != preparation.DeploymentRevision {
		return result, errors.New("installed hostname, system closure, or deployment revision differs from the reviewed preparation")
	}
	return result, nil
}

func ensureExactPrivateFile(path string, expected []byte) error {
	if err := writeExclusivePrivateFile(path, expected); err == nil {
		return nil
	} else if !os.IsExist(err) {
		return fmt.Errorf("create installed-system known-hosts: %w", err)
	}
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	if err := validatePrivateOwnedFile(file, syscall.S_IFREG, 0600); err != nil {
		return err
	}
	content, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || string(content) != string(expected) {
		return errors.New("existing installed-system known-hosts differs from the reviewed host key")
	}
	return nil
}
