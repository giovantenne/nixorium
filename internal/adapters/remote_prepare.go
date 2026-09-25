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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const remotePreparationOutputLimit = 4 * 1024 * 1024
const remotePreparationCommandTimeout = 4 * time.Hour

type remotePreparationSSH interface {
	CheckCacheEndpoint(context.Context, domain.RemoteInstallCache) error
	VerifySignedClosure(context.Context, domain.RemoteInstallCache, string) error
	PullBundle(context.Context, domain.RemoteInstallCache, string) error
	Probe(context.Context, string) (domain.RemoteMachineFacts, error)
	VerifyWiredInterface(context.Context, string) error
}

type RemoteInstallPreparer struct {
	repository          string
	stateRoot           string
	installedPrivateKey string
	installedPublicKey  string
	knownHostsPath      string
	now                 func() time.Time
	interfaceAddresses  func(string) ([]string, error)
	httpClient          *http.Client
	run                 func(context.Context, string, ...string) ([]byte, error)
}

func NewRemoteInstallPreparer(repository, stateRoot string) (*RemoteInstallPreparer, error) {
	for _, path := range []string{repository, stateRoot} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, errors.New("remote preparation paths must be absolute and canonical")
		}
	}
	return &RemoteInstallPreparer{
		repository: repository, stateRoot: stateRoot,
		installedPrivateKey: "/home/admin/.ssh/id_ed25519",
		installedPublicKey:  "/home/admin/.ssh/id_ed25519.pub",
		knownHostsPath:      "/home/admin/.ssh/known_hosts",
		now:                 time.Now,
		interfaceAddresses:  (Local{}).InterfaceAddresses,
		httpClient: &http.Client{
			Timeout: 6 * time.Second,
			Transport: &http.Transport{
				Proxy:               nil,
				DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				DisableKeepAlives:   true,
				ForceAttemptHTTP2:   false,
				MaxIdleConnsPerHost: 1,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("cache health endpoint redirects are forbidden")
			},
		},
		run: runRemotePreparationCommand,
	}, nil
}

func (preparer *RemoteInstallPreparer) Prepare(ctx context.Context, operationID, hostName string, session VerifiedLiveSession) (domain.RemoteInstallPreparation, error) {
	connection, err := NewStrictLiveSSH(session)
	if err != nil {
		return domain.RemoteInstallPreparation{}, err
	}
	return preparer.prepare(ctx, operationID, hostName, session, connection)
}

func (preparer *RemoteInstallPreparer) Discard(operationID string) error {
	if !remoteStateID(operationID) {
		return errors.New("remote preparation operation ID is invalid")
	}
	base := filepath.Join(preparer.stateRoot, "roots")
	if err := ensurePrivateOwnedDirectory(base); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	target := filepath.Join(base, operationID)
	if err := ensurePrivateOwnedDirectory(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove remote preparation GC roots: %w", err)
	}
	directory, err := os.Open(base)
	if err == nil {
		err = directory.Sync()
		_ = directory.Close()
	}
	return err
}

func (preparer *RemoteInstallPreparer) prepare(ctx context.Context, operationID, hostName string, session VerifiedLiveSession, connection remotePreparationSSH) (result domain.RemoteInstallPreparation, resultErr error) {
	if !remoteStateID(operationID) || session.OperationID != operationID || session.Address == "" || session.Facts.BootID == "" {
		return result, errors.New("remote preparation does not match the verified live session")
	}
	if !domain.ValidRemoteHostName(hostName) {
		return result, errors.New("remote preparation host is invalid")
	}
	if err := preparer.validateRepository(ctx); err != nil {
		return result, err
	}
	revisionOutput, err := preparer.run(ctx, "git", "-c", "safe.directory="+preparer.repository, "-C", preparer.repository, "rev-parse", "HEAD")
	if err != nil {
		return result, fmt.Errorf("resolve remote preparation revision: %w", err)
	}
	revision := strings.TrimSpace(string(revisionOutput))
	if len(revision) != 40 && len(revision) != 64 {
		return result, errors.New("remote preparation revision is not a full Git object ID")
	}
	for _, character := range revision {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return result, errors.New("remote preparation revision is not canonical hexadecimal")
		}
	}
	reference, err := deploymentFlakeReference(preparer.repository)
	if err != nil {
		return result, err
	}
	reference += "?rev=" + revision

	meta, err := preparer.evalLabMeta(ctx, reference)
	if err != nil {
		return result, err
	}
	host, found := remotePreparationHost(meta, hostName)
	if !found || host.Interface == "" {
		return result, errors.New("selected client is absent or has no declared interface in the pinned inventory")
	}
	adminPublicKey, err := preparer.verifiedAdminPublicKey(ctx)
	if err != nil {
		return result, err
	}
	cachePublicKey, err := preparer.readPublicKey(filepath.Join(preparer.repository, "keys", "cache-public-key"))
	if err != nil {
		return result, fmt.Errorf("read deployment cache public key: %w", err)
	}
	knownHostConflict, err := KnownHostConflict(preparer.knownHostsPath, host.IP, session.HostPublicKey)
	if err != nil {
		return result, fmt.Errorf("inspect existing static known-host entry: %w", err)
	}

	rootsDirectory, err := preparer.createRootsDirectory(operationID)
	if err != nil {
		return result, fmt.Errorf("create remote preparation GC roots: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(rootsDirectory)
		}
	}()
	systemPath, err := preparer.buildRoot(ctx, reference+"#nixosConfigurations."+hostName+".config.system.build.toplevel", filepath.Join(rootsDirectory, "system"))
	if err != nil {
		return result, err
	}
	bundlePath, err := preparer.buildRoot(ctx, reference+"#packages.x86_64-linux.remoteInstallerBundle", filepath.Join(rootsDirectory, "bundle"))
	if err != nil {
		return result, err
	}
	if !strings.Contains(systemPath, "-nixos-system-"+hostName+"-") {
		return result, errors.New("built system path does not match selected client")
	}
	systemBytes, err := preparer.closureSize(ctx, systemPath)
	if err != nil {
		return result, err
	}
	bundleBytes, err := preparer.closureSize(ctx, bundlePath)
	if err != nil {
		return result, err
	}

	controllerInterface := meta.Controller.Interface
	if controllerInterface == "" {
		controllerInterface = meta.Network.Interface
	}
	addresses, err := preparer.interfaceAddresses(controllerInterface)
	if err != nil {
		return result, fmt.Errorf("observe controller interface addresses: %w", err)
	}
	cache, err := preparer.selectCache(ctx, meta, addresses, cachePublicKey, connection)
	if err != nil {
		return result, err
	}
	if err := connection.VerifySignedClosure(ctx, cache, bundlePath); err != nil {
		return result, err
	}
	if err := connection.VerifySignedClosure(ctx, cache, systemPath); err != nil {
		return result, err
	}
	if err := connection.PullBundle(ctx, cache, bundlePath); err != nil {
		return result, err
	}
	if err := connection.VerifyWiredInterface(ctx, host.Interface); err != nil {
		return result, err
	}
	facts, err := connection.Probe(ctx, bundlePath)
	if err != nil {
		return result, fmt.Errorf("run final remote inventory probe: %w", err)
	}
	if facts.BootID != session.Facts.BootID || !remoteFactsHaveAddress(facts, host.Interface, session.Address) {
		return result, errors.New("live boot identity, declared NIC, or DHCP address changed during preparation")
	}
	if bundleBytes > ^uint64(0)/2 {
		return result, errors.New("installer bundle resource requirement overflows")
	}
	requiredLiveBytes := bundleBytes * 2
	if facts.MemoryAvailableBytes < requiredLiveBytes || facts.StoreAvailableBytes < requiredLiveBytes {
		return result, fmt.Errorf("live installer needs at least %d bytes free in memory and store for the installer bundle", requiredLiveBytes)
	}

	result = domain.RemoteInstallPreparation{
		OperationID: operationID, Repository: preparer.repository, DeploymentRevision: revision,
		BundlePath: bundlePath, BundleClosureBytes: bundleBytes, SystemPath: systemPath, SystemClosureBytes: systemBytes,
		Host:  domain.RemoteInstallHost{Name: host.Name, Interface: host.Interface, LiveIP: session.Address, StaticIP: host.IP},
		Cache: cache, AdminPublicKey: adminPublicKey, HostKeyPublic: session.HostPublicKey,
		HostFingerprint: session.HostFingerprint, KnownHostConflict: knownHostConflict,
		Facts: facts, PreparedAt: preparer.now().UTC(), Issues: []domain.ValidationIssue{},
		Endpoints: []domain.RemoteInstallerEndpoint{{Address: session.Address, Port: session.Port, Fingerprint: session.HostFingerprint}},
	}
	if err := domain.ValidateRemoteInstallPreparation(result); err != nil {
		return domain.RemoteInstallPreparation{}, err
	}
	complete = true
	return result, nil
}

func (preparer *RemoteInstallPreparer) createRootsDirectory(operationID string) (string, error) {
	base := filepath.Join(preparer.stateRoot, "roots")
	if err := ensurePrivateOwnedDirectory(base); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Mkdir(base, 0700); err != nil {
			return "", err
		}
		if err := ensurePrivateOwnedDirectory(base); err != nil {
			return "", err
		}
	}
	target := filepath.Join(base, operationID)
	if err := os.Mkdir(target, 0700); err != nil {
		return "", err
	}
	if err := ensurePrivateOwnedDirectory(target); err != nil {
		_ = os.Remove(target)
		return "", err
	}
	return target, nil
}

func ensurePrivateOwnedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 || !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("remote preparation directory is unsafe")
	}
	return nil
}

func (preparer *RemoteInstallPreparer) validateRepository(ctx context.Context) error {
	info, err := os.Lstat(preparer.repository)
	if err != nil {
		return fmt.Errorf("inspect fixed deployment repository: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("fixed deployment repository must be a real directory owned by the worker user")
	}
	flakeInfo, err := os.Lstat(filepath.Join(preparer.repository, "flake.nix"))
	if err != nil || !flakeInfo.Mode().IsRegular() || flakeInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("fixed deployment repository is not a flake")
	}
	status, err := preparer.run(ctx, "git", "-c", "safe.directory="+preparer.repository, "-C", preparer.repository, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil || len(bytes.TrimSpace(status)) != 0 {
		return errors.New("fixed deployment repository must have a clean worktree")
	}
	tracked, err := preparer.run(ctx, "git", "-c", "safe.directory="+preparer.repository, "-C", preparer.repository, "ls-files", "--", "secret-key", "admin-ssh", "veyon-private-key.pem")
	if err != nil || len(bytes.TrimSpace(tracked)) != 0 {
		return errors.New("private deployment key files must not be tracked")
	}
	return nil
}

func (preparer *RemoteInstallPreparer) evalLabMeta(ctx context.Context, reference string) (domain.LabMeta, error) {
	output, err := preparer.run(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval", reference+"#labMeta", "--json", "--no-write-lock-file")
	if err != nil {
		return domain.LabMeta{}, fmt.Errorf("evaluate pinned client inventory: %w", err)
	}
	var meta domain.LabMeta
	decoder := json.NewDecoder(bytes.NewReader(output))
	if err := decoder.Decode(&meta); err != nil {
		return meta, fmt.Errorf("decode pinned client inventory: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return meta, errors.New("pinned client inventory has trailing JSON")
	}
	return meta, nil
}

func (preparer *RemoteInstallPreparer) buildRoot(ctx context.Context, reference, outLink string) (string, error) {
	output, err := preparer.run(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "build", reference,
		"--no-write-lock-file", "--out-link", outLink, "--print-out-paths")
	if err != nil {
		return "", fmt.Errorf("build remote installation artifact: %w", err)
	}
	path := strings.TrimSpace(string(output))
	if strings.ContainsAny(path, "\r\n\t ") || !domain.ValidStorePath(path) {
		return "", errors.New("remote installation build returned an invalid store path")
	}
	target, err := filepath.EvalSymlinks(outLink)
	if err != nil || target != path {
		return "", errors.New("remote installation GC root does not resolve to the built path")
	}
	return path, nil
}

func (preparer *RemoteInstallPreparer) closureSize(ctx context.Context, storePath string) (uint64, error) {
	output, err := preparer.run(ctx, "nix", "--extra-experimental-features", "nix-command", "path-info", "--json", "--closure-size", storePath)
	if err != nil {
		return 0, fmt.Errorf("measure remote installation closure: %w", err)
	}
	var values map[string]struct {
		ClosureSize json.Number `json:"closureSize"`
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	if err := decoder.Decode(&values); err != nil || len(values) != 1 {
		return 0, errors.New("closure measurement output is invalid")
	}
	entry, ok := values[storePath]
	if !ok {
		return 0, errors.New("closure measurement does not match the requested store path")
	}
	size, err := strconv.ParseUint(entry.ClosureSize.String(), 10, 64)
	if err != nil || size == 0 {
		return 0, errors.New("closure measurement is invalid")
	}
	return size, nil
}

func (preparer *RemoteInstallPreparer) verifiedAdminPublicKey(ctx context.Context) (string, error) {
	repositoryKey, err := preparer.readPublicKey(filepath.Join(preparer.repository, "keys", "admin-ssh.pub"))
	if err != nil {
		return "", fmt.Errorf("read deployment admin public key: %w", err)
	}
	installedKey, err := preparer.readPublicKey(preparer.installedPublicKey)
	if err != nil || normalizedSSHKey(installedKey) != normalizedSSHKey(repositoryKey) {
		return "", errors.New("installed admin public key is absent or differs from the deployment")
	}
	for path, expectedMode := range map[string]os.FileMode{preparer.installedPrivateKey: 0600, preparer.installedPublicKey: 0644} {
		info, inspectErr := os.Lstat(path)
		stat, ok := infoSysStat(info)
		if inspectErr != nil || !ok || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != expectedMode || stat.Uid != uint32(os.Geteuid()) {
			return "", errors.New("installed admin SSH key files are unsafe")
		}
	}
	derived, err := preparer.run(ctx, "ssh-keygen", "-y", "-f", preparer.installedPrivateKey)
	if err != nil || normalizedSSHKey(strings.TrimSpace(string(derived))) != normalizedSSHKey(repositoryKey) {
		return "", errors.New("installed admin private key does not correspond to the deployment public key")
	}
	return repositoryKey, nil
}

func infoSysStat(info os.FileInfo) (*syscall.Stat_t, bool) {
	if info == nil {
		return nil, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return stat, ok
}

func (preparer *RemoteInstallPreparer) readPublicKey(path string) (string, error) {
	content, _, err := readRegularFileNoFollow(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(content))
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("public key file contains multiple lines")
	}
	return value, nil
}

func (preparer *RemoteInstallPreparer) selectCache(ctx context.Context, meta domain.LabMeta, observed []string, publicKey string, connection remotePreparationSSH) (domain.RemoteInstallCache, error) {
	addresses := canonicalObservedIPv4(observed)
	reachable := make([]domain.RemoteInstallCache, 0, len(addresses))
	for _, address := range addresses {
		cache := domain.RemoteInstallCache{URL: "http://" + net.JoinHostPort(address, strconv.Itoa(meta.Network.CachePort)), PublicKey: publicKey}
		if err := domain.ValidateRemoteInstallCache(cache); err != nil || preparer.checkLocalCache(ctx, cache.URL) != nil {
			continue
		}
		if err := connection.CheckCacheEndpoint(ctx, cache); err == nil {
			reachable = append(reachable, cache)
		}
	}
	for _, cache := range reachable {
		parsed, _ := url.Parse(cache.URL)
		host, _, _ := net.SplitHostPort(parsed.Host)
		if host == meta.Controller.DHCPIP {
			return cache, nil
		}
	}
	if len(reachable) == 0 {
		return domain.RemoteInstallCache{}, errors.New("no configured controller cache endpoint is reachable from the live client")
	}
	if len(reachable) != 1 {
		return domain.RemoteInstallCache{}, errors.New("multiple controller cache endpoints are reachable and no configured DHCP endpoint resolves the ambiguity")
	}
	return reachable[0], nil
}

func (preparer *RemoteInstallPreparer) checkLocalCache(ctx context.Context, baseURL string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/nix-cache-info", nil)
	if err != nil {
		return err
	}
	response, err := preparer.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("cache health endpoint returned a non-success status")
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 16*1024+1))
	if err != nil || len(content) > 16*1024 || !bytes.Contains(content, []byte("StoreDir: /nix/store")) {
		return errors.New("cache health endpoint returned invalid content")
	}
	return nil
}

func runRemotePreparationCommand(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	commandContext, cancel := context.WithTimeout(ctx, remotePreparationCommandTimeout)
	defer cancel()
	command := exec.CommandContext(commandContext, name, arguments...)
	stdout := &boundedCommandBuffer{limit: remotePreparationOutputLimit}
	stderr := &boundedCommandBuffer{limit: remotePreparationOutputLimit}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("%s failed: %w: %s", name, err, sanitizeOperationLog(stderr.buffer.Bytes()))
	}
	if stdout.truncated || stderr.truncated {
		return nil, errors.New("remote preparation command output exceeded its limit")
	}
	return append([]byte(nil), stdout.buffer.Bytes()...), nil
}

func remotePreparationHost(meta domain.LabMeta, name string) (domain.HostMeta, bool) {
	for _, host := range meta.Clients.Hosts {
		if host.Name == name {
			return host, true
		}
	}
	return domain.HostMeta{}, false
}

func remoteFactsHaveAddress(facts domain.RemoteMachineFacts, interfaceName, address string) bool {
	for _, networkInterface := range facts.Interfaces {
		if networkInterface.Name != interfaceName {
			continue
		}
		for _, candidate := range networkInterface.Addresses {
			if candidate == address {
				return true
			}
		}
	}
	return false
}

func canonicalObservedIPv4(values []string) []string {
	unique := map[string]bool{}
	for _, value := range values {
		if ip, _, err := net.ParseCIDR(value); err == nil {
			value = ip.String()
		}
		parsed := net.ParseIP(value)
		if parsed != nil && parsed.To4() != nil && parsed.String() == value && !parsed.IsLinkLocalUnicast() && !parsed.IsLoopback() && !parsed.IsMulticast() && !parsed.IsUnspecified() {
			unique[value] = true
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizedSSHKey(value string) string {
	fields := strings.Fields(value)
	if len(fields) < 2 || fields[0] != "ssh-ed25519" {
		return ""
	}
	return fields[0] + " " + fields[1]
}
