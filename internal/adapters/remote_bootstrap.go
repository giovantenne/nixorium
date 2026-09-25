package adapters

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	remoteBootstrapMaximumOutput = 1024 * 1024
	remoteBootstrapTimeout       = 10 * time.Second
	remoteProbeTimeout           = 30 * time.Second
	remoteRevokeCommand          = "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/sh -c 'set -eu; target=$(/run/current-system/sw/bin/cat); file=/root/.ssh/authorized_keys; temporary=$(/run/current-system/sw/bin/mktemp /root/.ssh/.authorized_keys.XXXXXX); /run/current-system/sw/bin/grep -Fvx -- \"$target\" \"$file\" >\"$temporary\" || test $? = 1; /run/current-system/sw/bin/chown root:root \"$temporary\"; /run/current-system/sw/bin/chmod 0600 \"$temporary\"; /run/current-system/sw/bin/mv -fT \"$temporary\" \"$file\"'"
)

type LivePassword struct {
	mutex sync.Mutex
	value []byte
}

func (bootstrap LiveBootstrap) CloseSession(ctx context.Context, session VerifiedLiveSession) error {
	if !remoteStateID(session.OperationID) || !strings.HasPrefix(session.PublicKeyLine, "restrict ") {
		return errors.New("live session cleanup identity is invalid")
	}
	directory := filepath.Join(bootstrap.runtimeRoot, session.OperationID)
	if filepath.Dir(session.PrivateKeyPath) != directory || filepath.Dir(session.KnownHostsPath) != directory {
		return errors.New("live session cleanup paths differ from the managed runtime")
	}
	connection, err := NewStrictLiveSSH(session)
	if err != nil {
		return err
	}
	if _, err := connection.run(ctx, remoteRevokeCommand, []byte(session.PublicKeyLine+"\n"), 8*1024); err != nil {
		return fmt.Errorf("revoke ephemeral live root key: %w", err)
	}
	if err := os.Remove(session.PrivateKeyPath); err != nil {
		return fmt.Errorf("remove ephemeral live root key: %w", err)
	}
	if err := os.Remove(session.KnownHostsPath); err != nil {
		return fmt.Errorf("remove live known-hosts: %w", err)
	}
	if err := os.Remove(directory); err != nil {
		return fmt.Errorf("remove live session runtime: %w", err)
	}
	root, err := os.Open(bootstrap.runtimeRoot)
	if err == nil {
		err = root.Sync()
		_ = root.Close()
	}
	return err
}

func (bootstrap LiveBootstrap) DiscardLocalSession(session VerifiedLiveSession) error {
	if !remoteStateID(session.OperationID) {
		return errors.New("live session cleanup identity is invalid")
	}
	directory := filepath.Join(bootstrap.runtimeRoot, session.OperationID)
	for _, path := range []string{session.PrivateKeyPath, session.KnownHostsPath, filepath.Join(directory, "installed_known_hosts")} {
		if path == "" || filepath.Dir(path) != directory {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(directory); err != nil {
		return fmt.Errorf("remove resolved live session runtime: %w", err)
	}
	return nil
}

func NewLivePassword(value []byte) (*LivePassword, error) {
	if len(value) == 0 || len(value) > 1024 || bytes.IndexByte(value, 0) >= 0 {
		zeroBytes(value)
		return nil, errors.New("live password must contain between 1 and 1024 non-NUL bytes")
	}
	secret := &LivePassword{value: append([]byte(nil), value...)}
	for index := range value {
		value[index] = 0
	}
	return secret, nil
}

func (secret *LivePassword) snapshot() ([]byte, error) {
	secret.mutex.Lock()
	defer secret.mutex.Unlock()
	if len(secret.value) == 0 {
		return nil, errors.New("live password has already been consumed")
	}
	return append([]byte(nil), secret.value...), nil
}

func (secret *LivePassword) Destroy() {
	if secret == nil {
		return
	}
	secret.mutex.Lock()
	defer secret.mutex.Unlock()
	for index := range secret.value {
		secret.value[index] = 0
	}
	secret.value = nil
}

type VerifiedLiveSession struct {
	OperationID     string
	Address         string
	Port            int
	HostPublicKey   string
	HostFingerprint string
	PrivateKeyPath  string
	KnownHostsPath  string
	PublicKeyLine   string
	Facts           domain.RemoteMachineFacts
}

type LiveBootstrap struct {
	runtimeRoot string
	port        int
	timeout     time.Duration
}

type liveBootstrapError struct {
	err                error
	cleanupUnconfirmed bool
}

func (failure *liveBootstrapError) Error() string { return failure.err.Error() }
func (failure *liveBootstrapError) Unwrap() error { return failure.err }

func BootstrapCleanupUnconfirmed(err error) bool {
	var failure *liveBootstrapError
	return errors.As(err, &failure) && failure.cleanupUnconfirmed
}

func NewLiveBootstrap() LiveBootstrap {
	return LiveBootstrap{runtimeRoot: "/run/nixorium/remote-install", port: 22, timeout: remoteBootstrapTimeout}
}

func (bootstrap LiveBootstrap) Establish(ctx context.Context, operationID, address, expectedFingerprint string, password *LivePassword) (result VerifiedLiveSession, resultErr error) {
	if !remoteStateID(operationID) {
		password.Destroy()
		return result, errors.New("remote installation operation ID is invalid")
	}
	parsedAddress := net.ParseIP(address)
	if parsedAddress == nil || parsedAddress.To4() == nil || parsedAddress.String() != address || parsedAddress.IsLinkLocalUnicast() || parsedAddress.IsUnspecified() || parsedAddress.IsMulticast() {
		password.Destroy()
		return result, errors.New("live installer address is not a canonical usable IPv4 address")
	}
	if !validSSHFingerprint(expectedFingerprint) {
		password.Destroy()
		return result, errors.New("live installer fingerprint is not canonical SHA256")
	}
	secret, err := password.snapshot()
	if err != nil {
		password.Destroy()
		return result, err
	}
	defer func() {
		for index := range secret {
			secret[index] = 0
		}
		password.Destroy()
	}()

	var verifiedHostKey ssh.PublicKey
	hostKeyCallback := func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if key.Type() != ssh.KeyAlgoED25519 {
			return fmt.Errorf("live installer offered unsupported host key algorithm %q", key.Type())
		}
		fingerprint := ssh.FingerprintSHA256(key)
		if fingerprint != expectedFingerprint {
			return fmt.Errorf("live installer host key fingerprint differs: got %s", fingerprint)
		}
		verifiedHostKey = key
		return nil
	}
	passwordText := string(secret) // unavoidable copy required by x/crypto/ssh; lifetime is bounded to this call.
	client, err := bootstrap.dial(ctx, address, &ssh.ClientConfig{
		User:              "nixos",
		Auth:              []ssh.AuthMethod{ssh.Password(passwordText)},
		HostKeyCallback:   hostKeyCallback,
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		Timeout:           bootstrap.timeout,
	})
	if err != nil {
		return result, fmt.Errorf("authenticate supported live installer: %w", err)
	}
	defer client.Close()

	facts, err := probeSupportedLiveInstaller(ctx, client)
	if err != nil {
		return result, err
	}
	if verifiedHostKey == nil {
		return result, errors.New("live installer host key was not verified")
	}
	privateKey, publicLine, err := generateEphemeralSSHKey(operationID)
	if err != nil {
		return result, err
	}
	defer zeroBytes(privateKey)
	sessionDirectory, privatePath, knownHostsPath, err := bootstrap.writeSessionFiles(operationID, address, verifiedHostKey, privateKey)
	if err != nil {
		return result, err
	}
	cleanupFiles := true
	defer func() {
		if cleanupFiles {
			_ = os.RemoveAll(sessionDirectory)
		}
	}()

	restrictedLine := "restrict " + publicLine
	if err := runSSHCommandContext(ctx, client, "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/install -d -m 0700 -o root -g root /root/.ssh", nil, 8*1024); err != nil {
		return result, fmt.Errorf("prepare live root authorization: %w", err)
	}
	if err := runSSHCommandContext(ctx, client, "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/tee -a /root/.ssh/authorized_keys", []byte(restrictedLine+"\n"), 8*1024); err != nil {
		return result, fmt.Errorf("authorize ephemeral live root key: %w", err)
	}
	keyAdded := true
	defer func() {
		if keyAdded {
			cleanupContext, cancel := context.WithTimeout(context.Background(), remoteProbeTimeout)
			defer cancel()
			if cleanupErr := revokeEphemeralKey(cleanupContext, client, restrictedLine); cleanupErr != nil {
				if resultErr == nil {
					resultErr = cleanupErr
				}
				resultErr = &liveBootstrapError{err: fmt.Errorf("%w; ephemeral live key cleanup was not confirmed: %v", resultErr, cleanupErr), cleanupUnconfirmed: true}
			}
		}
	}()
	if err := runSSHCommandContext(ctx, client, "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/chown root:root /root/.ssh/authorized_keys", nil, 8*1024); err != nil {
		return result, fmt.Errorf("secure live root authorization owner: %w", err)
	}
	if err := runSSHCommandContext(ctx, client, "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/chmod 0600 /root/.ssh/authorized_keys", nil, 8*1024); err != nil {
		return result, fmt.Errorf("secure live root authorization mode: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return result, fmt.Errorf("parse ephemeral live root key: %w", err)
	}
	rootClient, err := bootstrap.dial(ctx, address, &ssh.ClientConfig{
		User:              "root",
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback:   hostKeyCallback,
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		Timeout:           bootstrap.timeout,
	})
	if err != nil {
		return result, fmt.Errorf("verify ephemeral live root session: %w", err)
	}
	if err := runSSHCommandContext(ctx, rootClient, "/run/current-system/sw/bin/true", nil, 8*1024); err != nil {
		rootClient.Close()
		return result, fmt.Errorf("verify ephemeral live root command: %w", err)
	}
	rootClient.Close()

	if err := runSSHCommandContext(ctx, client, "/run/wrappers/bin/sudo -n /run/current-system/sw/bin/passwd -l nixos", nil, 8*1024); err != nil {
		return result, fmt.Errorf("lock temporary live password: %w", err)
	}
	deniedClient, denialErr := bootstrap.dial(ctx, address, &ssh.ClientConfig{
		User:              "nixos",
		Auth:              []ssh.AuthMethod{ssh.Password(passwordText)},
		HostKeyCallback:   hostKeyCallback,
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		Timeout:           bootstrap.timeout,
	})
	if denialErr == nil {
		deniedClient.Close()
		return result, errors.New("live password login still succeeds after credential transition")
	}
	if !strings.Contains(denialErr.Error(), "unable to authenticate") && !strings.Contains(denialErr.Error(), "no supported methods remain") {
		return result, fmt.Errorf("could not prove that live password authentication is disabled: %w", denialErr)
	}

	keyAdded = false
	cleanupFiles = false
	result = VerifiedLiveSession{
		OperationID: operationID, Address: address, Port: bootstrap.port,
		HostPublicKey:   strings.TrimSpace(string(ssh.MarshalAuthorizedKey(verifiedHostKey))),
		HostFingerprint: expectedFingerprint, PrivateKeyPath: privatePath,
		KnownHostsPath: knownHostsPath, PublicKeyLine: restrictedLine, Facts: facts,
	}
	return result, nil
}

func (bootstrap LiveBootstrap) dial(ctx context.Context, address string, configuration *ssh.ClientConfig) (*ssh.Client, error) {
	endpoint := net.JoinHostPort(address, strconv.Itoa(bootstrap.port))
	dialer := net.Dialer{Timeout: bootstrap.timeout}
	connection, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return nil, err
	}
	_ = connection.SetDeadline(time.Now().Add(bootstrap.timeout))
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, endpoint, configuration)
	if err != nil {
		connection.Close()
		return nil, err
	}
	_ = connection.SetDeadline(time.Time{})
	return ssh.NewClient(clientConnection, channels, requests), nil
}

func probeSupportedLiveInstaller(ctx context.Context, client *ssh.Client) (domain.RemoteMachineFacts, error) {
	commands := []struct {
		command string
		limit   int
	}{
		{"/run/current-system/sw/bin/cat /etc/os-release", 64 * 1024},
		{"/run/current-system/sw/bin/uname -m", 1024},
		{"/run/current-system/sw/bin/test -d /sys/firmware/efi", 1024},
		{"/run/current-system/sw/bin/cat /proc/sys/kernel/random/boot_id", 1024},
		{"/run/wrappers/bin/sudo -n /run/current-system/sw/bin/true", 1024},
		{"/run/current-system/sw/bin/ip -j -4 address show scope global", remoteBootstrapMaximumOutput},
	}
	outputs := make([][]byte, len(commands))
	for index, command := range commands {
		output, err := runSSHCommandOutputContext(ctx, client, command.command, nil, command.limit)
		if err != nil {
			return domain.RemoteMachineFacts{}, fmt.Errorf("live installer probe step %d failed: %w", index+1, err)
		}
		outputs[index] = output
	}
	release, err := parseOSRelease(outputs[0])
	if err != nil || release["VARIANT_ID"] != "installer" || !strings.HasPrefix(release["VERSION_ID"], "26.05") || strings.TrimSpace(string(outputs[1])) != "x86_64" {
		return domain.RemoteMachineFacts{}, errors.New("target is not the supported NixOS 26.05 x86_64 installer")
	}
	bootID := strings.TrimSpace(string(outputs[3]))
	interfaces, err := parseLiveInterfaces(outputs[5])
	if err != nil {
		return domain.RemoteMachineFacts{}, err
	}
	facts := domain.RemoteMachineFacts{
		SchemaVersion: domain.RemoteInstallSchemaVersion, VariantID: release["VARIANT_ID"],
		VersionID: release["VERSION_ID"], BuildID: release["BUILD_ID"], Architecture: "x86_64",
		UEFI: true, SudoReady: true, BootID: bootID, Interfaces: interfaces, Disks: []domain.RemoteDisk{},
	}
	if _, err := domain.DecodeRemoteMachineFacts(mustJSON(facts)); err != nil {
		return domain.RemoteMachineFacts{}, err
	}
	return facts, nil
}

func runSSHCommandContext(ctx context.Context, client *ssh.Client, command string, input []byte, maximum int) error {
	_, err := runSSHCommandOutputContext(ctx, client, command, input, maximum)
	return err
}

func runSSHCommandOutputContext(ctx context.Context, client *ssh.Client, command string, input []byte, maximum int) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	if input != nil {
		session.Stdin = bytes.NewReader(input)
	}
	output := &boundedCommandBuffer{limit: maximum}
	session.Stdout = output
	session.Stderr = output
	if err := session.Start(command); err != nil {
		return nil, err
	}
	result := make(chan error, 1)
	go func() { result <- session.Wait() }()
	timer := time.NewTimer(remoteProbeTimeout)
	defer timer.Stop()
	select {
	case err = <-result:
	case <-ctx.Done():
		_ = client.Close()
		<-result
		return nil, ctx.Err()
	case <-timer.C:
		_ = client.Close()
		<-result
		return nil, errors.New("remote command timed out")
	}
	if output.truncated {
		return nil, errors.New("remote command output exceeded its limit")
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, sanitizeOperationLog(output.buffer.Bytes()))
	}
	return append([]byte(nil), output.buffer.Bytes()...), nil
}

func generateEphemeralSSHKey(operationID string) ([]byte, string, error) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	public, err := ssh.NewPublicKey(private.Public())
	if err != nil {
		return nil, "", err
	}
	block, err := ssh.MarshalPrivateKey(private, "nixorium-remote-install:"+operationID)
	if err != nil {
		return nil, "", err
	}
	return pem.EncodeToMemory(block), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public))) + " nixorium-session:" + operationID, nil
}

func (bootstrap LiveBootstrap) writeSessionFiles(operationID, address string, hostKey ssh.PublicKey, privateKey []byte) (string, string, string, error) {
	if bootstrap.port < 1 || bootstrap.port > 65535 {
		return "", "", "", errors.New("remote SSH port is invalid")
	}
	rootInfo, err := os.Lstat(bootstrap.runtimeRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm() != 0700 {
		return "", "", "", errors.New("remote installation runtime root is unsafe")
	}
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok || rootStat.Uid != uint32(os.Geteuid()) {
		return "", "", "", errors.New("remote installation runtime root belongs to another user")
	}
	directory := filepath.Join(bootstrap.runtimeRoot, operationID)
	if err := os.Mkdir(directory, 0700); err != nil {
		return "", "", "", fmt.Errorf("create remote installation runtime: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(directory)
		}
	}()
	privatePath := filepath.Join(directory, "id_ed25519")
	knownHostsPath := filepath.Join(directory, "known_hosts")
	if err := writeExclusivePrivateFile(privatePath, privateKey); err != nil {
		return directory, "", "", err
	}
	hostPattern := address
	if bootstrap.port != 22 {
		hostPattern = net.JoinHostPort(address, strconv.Itoa(bootstrap.port))
	}
	if err := writeExclusivePrivateFile(knownHostsPath, []byte(knownhosts.Line([]string{hostPattern}, hostKey)+"\n")); err != nil {
		return directory, "", "", err
	}
	success = true
	return directory, privatePath, knownHostsPath, nil
}

func writeExclusivePrivateFile(path string, content []byte) error {
	descriptor, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(descriptor), path)
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func revokeEphemeralKey(ctx context.Context, client *ssh.Client, line string) error {
	return runSSHCommandContext(ctx, client, remoteRevokeCommand, []byte(line+"\n"), 8*1024)
}

func parseOSRelease(data []byte) (map[string]string, error) {
	if len(data) == 0 || len(data) > 64*1024 {
		return nil, errors.New("live installer release record has an invalid size")
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		_, duplicate := values[key]
		if !found || key == "" || strings.ContainsAny(key, " \t") || duplicate {
			return nil, errors.New("live installer release record is invalid")
		}
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return nil, errors.New("live installer release record contains invalid quoting")
			}
			value = unquoted
		} else if strings.ContainsAny(value, " \t\"'") {
			return nil, errors.New("live installer release record contains unsupported syntax")
		}
		values[key] = value
	}
	return values, nil
}

func parseLiveInterfaces(data []byte) ([]domain.RemoteNetworkInterface, error) {
	if err := domain.ValidateStrictRemoteJSON(data, remoteBootstrapMaximumOutput); err != nil {
		return nil, errors.New("live installer network inventory is invalid")
	}
	var raw []struct {
		Name     string `json:"ifname"`
		InfoKind string `json:"link_type"`
		Address  []struct {
			Family string `json:"family"`
			Local  string `json:"local"`
		} `json:"addr_info"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil || len(raw) > domain.RemoteInstallMaximumNICs {
		return nil, errors.New("live installer network inventory is invalid")
	}
	result := make([]domain.RemoteNetworkInterface, 0, len(raw))
	for _, networkInterface := range raw {
		entry := domain.RemoteNetworkInterface{Name: networkInterface.Name, Addresses: []string{}}
		for _, address := range networkInterface.Address {
			if address.Family == "inet" {
				entry.Addresses = append(entry.Addresses, address.Local)
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

func validSSHFingerprint(value string) bool {
	if !strings.HasPrefix(value, "SHA256:") || len(value) != len("SHA256:")+43 {
		return false
	}
	for _, character := range strings.TrimPrefix(value, "SHA256:") {
		if !(character >= 'A' && character <= 'Z') && !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '+' && character != '/' {
			return false
		}
	}
	return true
}

func mustJSON(value any) []byte {
	content, _ := json.Marshal(value)
	return content
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
