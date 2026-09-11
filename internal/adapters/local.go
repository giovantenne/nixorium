package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type Local struct{}

func (Local) LabMeta(ctx context.Context, repository string) (domain.LabMeta, error) {
	var meta domain.LabMeta
	err := nixJSON(ctx, repository, "labMeta", &meta)
	return meta, err
}

func (Local) DeploymentStatus(ctx context.Context, repository string) (domain.DeploymentStatus, error) {
	var status domain.DeploymentStatus
	err := nixJSON(ctx, repository, "deploymentStatus", &status)
	return status, err
}

func (Local) GitState(ctx context.Context, repository string) (domain.GitState, error) {
	output, err := run(ctx, "git", "-C", repository, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return domain.GitState{}, err
	}
	trimmed := strings.TrimSpace(output)
	changes := 0
	paths := []string{}
	if trimmed != "" {
		lines := strings.Split(trimmed, "\n")
		changes = len(lines)
		for _, line := range lines {
			if len(line) >= 4 {
				paths = append(paths, line[3:])
			}
		}
	}
	return domain.GitState{Available: true, Dirty: changes > 0, Changes: changes, Paths: paths}, nil
}

func (Local) ServiceState(ctx context.Context, name string) domain.ServiceState {
	state := domain.ServiceState{Name: name, State: "not-found"}
	output, err := run(ctx, "systemctl", "show", name, "--property=LoadState", "--property=ActiveState", "--no-pager")
	if err != nil && strings.TrimSpace(output) == "" {
		return state
	}
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(line, "=")
		if found {
			values[key] = value
		}
	}
	state.Loaded = values["LoadState"] == "loaded"
	state.Active = values["ActiveState"] == "active"
	if active := values["ActiveState"]; active != "" {
		state.State = active
	}
	return state
}

func (Local) ArtifactState(repository, name, relativePath string) domain.ArtifactState {
	path := filepath.Join(repository, relativePath)
	_, err := os.Stat(path)
	return domain.ArtifactState{Name: name, Path: relativePath, Present: err == nil}
}

func (Local) InterfaceAddresses(name string) ([]string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		value := address.String()
		if ip, _, parseErr := net.ParseCIDR(value); parseErr == nil {
			value = ip.String()
		}
		result = append(result, value)
	}
	return result, nil
}

func (Local) FreeBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func (Local) CommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (Local) CacheKeyState(ctx context.Context, repository string) (domain.CacheKeyState, error) {
	state := domain.CacheKeyState{}
	privatePath := filepath.Join(repository, "secret-key")
	privateKey, mode, err := readRegularFileNoFollow(privatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("read Harmonia signing key: %w", err)
	}
	state.PrivatePresent = true
	state.PrivateMode = mode

	publicPath := filepath.Join(repository, "keys", "cache-public-key")
	if _, statErr := os.Lstat(publicPath); os.IsNotExist(statErr) {
		publicPath = filepath.Join(repository, "public-key")
	}
	publicKey, _, err := readRegularFileNoFollow(publicPath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("read Harmonia public key: %w", err)
	}
	state.PublicPresent = true

	converted, err := runWithInput(ctx, privateKey, "nix", "--extra-experimental-features", "nix-command flakes", "key", "convert-secret-to-public")
	if err != nil {
		return state, fmt.Errorf("derive Harmonia public key: %w", err)
	}
	state.Matches = strings.TrimSpace(converted) == strings.TrimSpace(string(publicKey))
	return state, nil
}

func (Local) ListeningPorts() ([]domain.PortUse, error) {
	files := []struct {
		path     string
		protocol string
		state    string
	}{
		{"/proc/net/tcp", "tcp", "0A"},
		{"/proc/net/tcp6", "tcp", "0A"},
		{"/proc/net/udp", "udp", "07"},
		{"/proc/net/udp6", "udp", "07"},
	}
	uses := []domain.PortUse{}
	readFiles := 0
	for _, file := range files {
		parsed, err := parseProcNet(file.path, file.protocol, file.state)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		readFiles++
		uses = append(uses, parsed...)
	}
	if readFiles == 0 {
		return nil, errors.New("kernel socket tables are unavailable")
	}
	return uses, nil
}

func (Local) AddressOwners(address string) ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	owners := []string{}
	for _, iface := range interfaces {
		addresses, addressErr := iface.Addrs()
		if addressErr != nil {
			return nil, addressErr
		}
		for _, candidate := range addresses {
			value := candidate.String()
			if ip, _, parseErr := net.ParseCIDR(value); parseErr == nil {
				value = ip.String()
			}
			if value == address {
				owners = append(owners, iface.Name)
				break
			}
		}
	}
	return owners, nil
}

func (Local) CacheHealth(ctx context.Context, address string, port int) error {
	requestContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, fmt.Sprintf("http://%s:%d/nix-cache-info", address, port), nil)
	if err != nil {
		return err
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return err
	}
	if !bytes.Contains(body, []byte("StoreDir:")) {
		return errors.New("response is not a Nix binary cache")
	}
	return nil
}

func (Local) SSHReachability(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]bool {
	type result struct {
		name      string
		reachable bool
	}
	results := make(chan result, len(hosts))
	for _, host := range hosts {
		host := host
		go func() {
			dialer := net.Dialer{Timeout: timeout}
			connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host.IP, "22"))
			if err == nil {
				connection.Close()
			}
			results <- result{name: host.Name, reachable: err == nil}
		}()
	}
	reachable := make(map[string]bool, len(hosts))
	for range hosts {
		result := <-results
		reachable[result.name] = result.reachable
	}
	return reachable
}

func (Local) ControllerBuild(ctx context.Context, repository, controllerName string) error {
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return err
	}
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return err
	}
	reference := flake + "#nixosConfigurations." + controllerName + ".config.system.build.toplevel"
	_, err = run(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "build", reference, "--no-write-lock-file", "--no-link")
	return err
}

func nixJSON(ctx context.Context, repository, attribute string, destination any) error {
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return err
	}
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return err
	}
	reference := flake + "#" + attribute
	output, err := runOutput(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval", reference, "--json", "--no-write-lock-file")
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(output), destination); err != nil {
		return fmt.Errorf("decode %s JSON: %w", attribute, err)
	}
	return nil
}

func run(ctx context.Context, name string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return string(output), fmt.Errorf("%s: %s", name, message)
	}
	return string(output), nil
}

func runOutput(ctx context.Context, name string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s: %s", name, message)
	}
	return stdout.String(), nil
}

func runWithInput(ctx context.Context, input []byte, name string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return string(output), fmt.Errorf("%s: %s", name, message)
	}
	return string(output), nil
}

func readRegularFileNoFollow(path string) ([]byte, uint32, error) {
	const maximumKeyBytes = 64 * 1024
	return readRegularFileNoFollowLimit(path, maximumKeyBytes)
}

func parseProcNet(path, protocol, expectedState string) ([]domain.PortUse, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	uses := []domain.PortUse{}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[3] != expectedState {
			continue
		}
		_, portHex, found := strings.Cut(fields[1], ":")
		if !found {
			continue
		}
		port, parseErr := strconv.ParseInt(portHex, 16, 32)
		if parseErr != nil {
			continue
		}
		uses = append(uses, domain.PortUse{Protocol: protocol, Port: int(port)})
	}
	return uses, nil
}
