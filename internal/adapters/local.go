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
	"sync"
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

// LabMetaAtRevision evaluates the identities of one committed revision, so
// uncommitted edits in the worktree cannot change or break the result.
func (Local) LabMetaAtRevision(ctx context.Context, repository, revision string) (domain.LabMeta, error) {
	var meta domain.LabMeta
	if !validGitRevision(revision) {
		return meta, errors.New("deployment revision is not a full Git object ID")
	}
	err := nixJSONAt(ctx, repository, revision, "labMeta", &meta)
	return meta, err
}

func (Local) DeploymentStatus(ctx context.Context, repository string) (domain.DeploymentStatus, error) {
	var status domain.DeploymentStatus
	err := nixJSON(ctx, repository, "deploymentStatus", &status)
	return status, err
}

func (Local) GitState(ctx context.Context, repository string) (domain.GitState, error) {
	if err := checkTemplateResetPending(repository); err != nil {
		return domain.GitState{}, err
	}
	output, truncated, err := runBoundedGit(ctx, repository, maximumGitStatusBytes, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return domain.GitState{}, err
	}
	if truncated {
		return domain.GitState{}, errors.New("Git worktree status exceeds the 1 MiB safety limit")
	}
	changes, err := parseRawGitPorcelain(output)
	if err != nil {
		return domain.GitState{}, err
	}
	paths := []string{}
	for _, change := range changes {
		paths = append(paths, change.Path)
		if change.OriginalPath != "" {
			paths = append(paths, change.OriginalPath)
		}
	}
	return domain.GitState{Available: true, Dirty: len(changes) > 0, Changes: len(changes), Paths: paths}, nil
}

func (Local) GitRevision(ctx context.Context, repository string) (string, error) {
	output, err := run(ctx, "git", "-C", repository, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	revision := strings.TrimSpace(output)
	if revision == "" {
		return "", errors.New("Git revision is empty")
	}
	return revision, nil
}

func (Local) RunDeploymentPhase(ctx context.Context, plan domain.DeploymentPlanReport, phase domain.DeploymentPhase, output io.Writer) error {
	return runDeploymentPhase(ctx, plan, phase, output, managedCoordinationDirectory, true, deploymentPhaseTimeout(phase))
}

func deploymentCommand(phase domain.DeploymentPhase, selector string) ([]string, error) {
	if selector == "" {
		return nil, errors.New("Colmena selector is empty")
	}
	switch phase {
	case domain.DeploymentPhaseBuild:
		return []string{"build", "--on", selector, "--verbose", "--color", "never"}, nil
	case domain.DeploymentPhaseApply:
		return []string{"apply", "switch", "--on", selector, "--verbose", "--color", "never"}, nil
	default:
		return nil, fmt.Errorf("unsupported deployment phase %q", phase)
	}
}

func configureColmenaSSH(command *exec.Cmd) (func(), error) {
	config, err := os.CreateTemp("", "nixorium-colmena-ssh-*")
	if err != nil {
		return nil, err
	}
	path := config.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}
	content := "Host *\n  BatchMode yes\n  PasswordAuthentication no\n  KbdInteractiveAuthentication no\n  StrictHostKeyChecking accept-new\n  ConnectTimeout 10\n  ConnectionAttempts 1\n  ServerAliveInterval 10\n  ServerAliveCountMax 3\n"
	if _, err := io.WriteString(config, content); err != nil {
		_ = config.Close()
		cleanup()
		return nil, err
	}
	if err := config.Close(); err != nil {
		cleanup()
		return nil, err
	}
	environment := command.Env
	if environment == nil {
		environment = os.Environ()
	}
	command.Env = environmentWithValue(environment, "SSH_CONFIG_FILE", path)
	return cleanup, nil
}

func environmentWithValue(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
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

const maximumConcurrentSSHProbes = 8

func (Local) SSHStatus(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.SSHProbe {
	return probeSSHStatuses(ctx, hosts, timeout, probeHostSSH)
}

func (Local) CurrentSystems(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.HostSystemProbe {
	return probeCurrentSystems(ctx, hosts, timeout, probeCurrentSystem)
}

func probeCurrentSystems(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration, probe func(context.Context, domain.HostMeta, time.Duration) domain.HostSystemProbe) map[string]domain.HostSystemProbe {
	type result struct {
		name  string
		probe domain.HostSystemProbe
	}
	if len(hosts) == 0 {
		return map[string]domain.HostSystemProbe{}
	}
	workerCount := min(len(hosts), maximumConcurrentSSHProbes)
	jobs := make(chan domain.HostMeta)
	results := make(chan result, len(hosts))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for host := range jobs {
				results <- result{name: host.Name, probe: probe(ctx, host, timeout)}
			}
		}()
	}
	go func() {
		for _, host := range hosts {
			jobs <- host
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()
	probes := make(map[string]domain.HostSystemProbe, len(hosts))
	for range hosts {
		result := <-results
		probes[result.name] = result.probe
	}
	return probes
}

func probeCurrentSystem(ctx context.Context, host domain.HostMeta, timeout time.Duration) domain.HostSystemProbe {
	return probeCurrentSystemArguments(ctx, timeout, sshCurrentSystemArguments(host, timeout))
}

func probeCurrentSystemArguments(ctx context.Context, timeout time.Duration, arguments []string) domain.HostSystemProbe {
	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(probeContext, "ssh", arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		if len(detail) > 512 {
			detail = detail[:512]
		}
		return domain.HostSystemProbe{HostKeyCondition: sshHostKeyCondition(string(output)), Detail: "authenticated system observation failed: " + detail}
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 || !validSystemPath(lines[0]) || !validGitRevision(lines[1]) {
		return domain.HostSystemProbe{Detail: "authenticated system observation returned invalid state"}
	}
	return domain.HostSystemProbe{SystemPath: lines[0], Revision: lines[1]}
}

func sshHostKeyCondition(output string) domain.HostKeyCondition {
	if strings.Contains(output, "REMOTE HOST IDENTIFICATION HAS CHANGED") ||
		strings.Contains(output, "has changed and you have requested strict checking") {
		return domain.HostKeyChanged
	}
	return ""
}

func sshCurrentSystemArguments(host domain.HostMeta, timeout time.Duration) []string {
	seconds := max(1, int(timeout.Round(time.Second)/time.Second))
	return []string{
		"-T",
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", seconds),
		"-o", "ConnectionAttempts=1",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "LogLevel=ERROR",
		"root@" + host.IP,
		"nixorium-host-state",
	}
}

func validSystemPath(path string) bool {
	return strings.HasPrefix(path, "/nix/store/") && filepath.Clean(path) == path && !strings.ContainsAny(path, " \t\r\n")
}

func validGitRevision(revision string) bool {
	if len(revision) < 40 || len(revision) > 64 {
		return false
	}
	for _, character := range revision {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func probeSSHStatuses(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration, probe func(context.Context, domain.HostMeta, time.Duration) domain.SSHProbe) map[string]domain.SSHProbe {
	type result struct {
		name  string
		probe domain.SSHProbe
	}
	if len(hosts) == 0 {
		return map[string]domain.SSHProbe{}
	}

	workerCount := min(len(hosts), maximumConcurrentSSHProbes)
	jobs := make(chan domain.HostMeta)
	results := make(chan result, len(hosts))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for host := range jobs {
				results <- result{name: host.Name, probe: probe(ctx, host, timeout)}
			}
		}()
	}
	go func() {
		for _, host := range hosts {
			jobs <- host
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	statuses := make(map[string]domain.SSHProbe, len(hosts))
	for range hosts {
		result := <-results
		statuses[result.name] = result.probe
	}
	return statuses
}

func probeHostSSH(ctx context.Context, host domain.HostMeta, timeout time.Duration) domain.SSHProbe {
	dialer := net.Dialer{Timeout: timeout}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host.IP, "22"))
	if err == nil {
		connection.Close()
		return domain.SSHProbe{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
	}
	return classifySSHError(err)
}

func classifySSHError(err error) domain.SSHProbe {
	probe := domain.SSHProbe{Reachability: domain.ReachabilityUnknown, SSH: domain.SSHUnknown, Detail: err.Error()}
	if errors.Is(err, syscall.ECONNREFUSED) {
		probe.Reachability = domain.ReachabilityReachable
		probe.SSH = domain.SSHUnavailable
		return probe
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) ||
		(errors.As(err, &networkError) && networkError.Timeout()) {
		probe.Reachability = domain.ReachabilityUnreachable
	}
	return probe
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
	return nixJSONAt(ctx, repository, "", attribute, destination)
}

// nixJSONAt evaluates the worktree, or the given committed revision.
func nixJSONAt(ctx context.Context, repository, revision, attribute string, destination any) error {
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return err
	}
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return err
	}
	if revision != "" {
		flake += "?rev=" + revision
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
	configureCommandCancellation(command)
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
	configureCommandCancellation(command)
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
	configureCommandCancellation(command)
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
