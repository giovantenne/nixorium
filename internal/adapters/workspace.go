package adapters

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// Do not name the JSON field outPath: Nix coerces such attribute sets to a
// path string during JSON serialization, discarding the other identity fields.
const workspaceSourceExpression = `
let
  deployment = builtins.getFlake (builtins.getEnv "NIXORIUM_DEPLOYMENT_FLAKE");
in { source = {
  path = deployment.sourceInfo.outPath;
  inherit (deployment.sourceInfo) narHash;
}; }
`

const workspaceCandidateExpression = `
let
  deployment = builtins.getFlake (builtins.getEnv "NIXORIUM_DEPLOYMENT_FLAKE");
  candidate = builtins.getEnv "NIXORIUM_WORKSPACE_CANDIDATE";
in
assert deployment.nixoriumValidateWorkspaceCandidate candidate == true;
{
  source = {
    path = deployment.sourceInfo.outPath;
    inherit (deployment.sourceInfo) narHash;
  };
  resolution = deployment.nixoriumResolveWorkspaceCandidate candidate;
}
`

type workspaceNixSource struct {
	OutPath string `json:"path"`
	NarHash string `json:"narHash"`
}

type workspaceEvaluation struct {
	Source     workspaceNixSource          `json:"source"`
	Resolution *domain.WorkspaceResolution `json:"resolution,omitempty"`
}

type workspaceLocalState struct {
	Revision    string
	Index       string
	Pin         string
	Base        string
	BaseExists  bool
	RootDevice  uint64
	RootInode   uint64
	Profile     *domain.WorkspaceProfile `json:"-"`
	LockContent []byte                   `json:"-"`
}

// InspectWorkspace evaluates through the Git fetcher, never a path fetch of
// the working directory (which could copy ignored credentials into the store).
func (Local) InspectWorkspace(ctx context.Context, repository string, candidate domain.WorkspaceProfile) (domain.WorkspaceInspection, error) {
	data, err := domain.MarshalWorkspaceProfile(candidate)
	if err != nil {
		return domain.WorkspaceInspection{}, err
	}
	root, err := openWorkspaceRoot(repository, unix.LOCK_SH)
	if err != nil {
		return domain.WorkspaceInspection{}, err
	}
	defer root.Close()
	snapshot, base, resolved, err := inspectWorkspaceSource(ctx, root, data)
	if err != nil {
		return domain.WorkspaceInspection{}, err
	}
	// Re-export after evaluation: tracked worktree contents may have changed
	// without changing HEAD or the index while the deployment was evaluated.
	after, _, _, err := inspectWorkspaceSource(ctx, root, nil)
	if err != nil {
		return domain.WorkspaceInspection{}, err
	}
	if snapshot != after {
		return domain.WorkspaceInspection{}, domain.ErrWorkspaceConflict
	}
	if resolved == nil {
		return domain.WorkspaceInspection{}, errors.New("deployment did not return workspace candidate metadata")
	}
	inspection := domain.WorkspaceInspection{Snapshot: snapshot, Base: base, Resolution: *resolved}
	if err := domain.ValidateWorkspaceInspection(inspection, candidate); err != nil {
		return domain.WorkspaceInspection{}, err
	}
	return inspection, nil
}

func (Local) WriteWorkspaceIfUnchanged(ctx context.Context, repository string, expected domain.WorkspaceSnapshot, candidate domain.WorkspaceProfile) error {
	data, err := domain.MarshalWorkspaceProfile(candidate)
	if err != nil {
		return err
	}
	root, err := openWorkspaceRoot(repository, unix.LOCK_EX)
	if err != nil {
		return err
	}
	defer root.Close()
	inspect := func() (domain.WorkspaceSnapshot, error) {
		snapshot, _, _, err := inspectWorkspaceSource(ctx, root, nil)
		return snapshot, err
	}
	return writeWorkspaceFile(ctx, root, expected, data, inspect, root.Sync)
}

func openWorkspaceRoot(repository string, lock int) (*os.File, error) {
	if !filepath.IsAbs(repository) || filepath.Clean(repository) != repository || repository == "/" {
		return nil, errors.New("workspace requires an absolute deployment directory")
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, repository, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, fmt.Errorf("open workspace deployment without symlinks: %w", err)
	}
	root := os.NewFile(uintptr(fd), repository)
	if err := unix.Flock(fd, lock|unix.LOCK_NB); err != nil {
		root.Close()
		return nil, fmt.Errorf("lock workspace deployment: %w", err)
	}
	return root, nil
}

// All file reads and writes stay anchored to this descriptor. Git/Nix still
// use the repository pathname, so reject replacement of that directory too.
func workspaceRootIdentity(root *os.File) (unix.Stat_t, error) {
	var held, current unix.Stat_t
	if err := unix.Fstat(int(root.Fd()), &held); err != nil {
		return held, err
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, root.Name(), &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return held, domain.ErrWorkspaceConflict
	}
	defer unix.Close(fd)
	if err := unix.Fstat(fd, &current); err != nil {
		return held, err
	}
	if held.Dev != current.Dev || held.Ino != current.Ino {
		return held, domain.ErrWorkspaceConflict
	}
	return held, nil
}

func readWorkspaceFile(root *os.File, name string, limit int64) ([]byte, unix.Stat_t, error) {
	var info unix.Stat_t
	fd, err := unix.Openat2(int(root.Fd()), name, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return nil, info, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	if err := unix.Fstat(fd, &info); err != nil {
		return nil, info, err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG || info.Size > limit {
		return nil, info, errors.New("workspace input must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, info, err
	}
	if int64(len(data)) > limit {
		return nil, info, errors.New("workspace input exceeds its size limit")
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return nil, info, err
	}
	// Reading may update atime; it is not content or write authorization.
	after.Atim = info.Atim
	if after != info {
		return nil, info, domain.ErrWorkspaceConflict
	}
	return data, info, nil
}

func workspaceDigest(value any) string {
	// All callers pass concrete structs, strings or byte slices.
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func workspaceEnvironment() []string {
	result := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if !strings.HasPrefix(name, "GIT_") && name != "NIXORIUM_DEPLOYMENT_FLAKE" && name != "NIXORIUM_WORKSPACE_CANDIDATE" {
			result = append(result, item)
		}
	}
	return append(result, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}

func workspaceGit(ctx context.Context, repository string, args ...string) (string, error) {
	result, truncated, err := runBoundedGitWithEnvironment(ctx, repository, workspaceEnvironment(), 1024*1024, args...)
	if err != nil {
		return "", err
	}
	if truncated {
		return "", errors.New("workspace Git source exceeds the 1 MiB manifest limit")
	}
	return result, nil
}

func readWorkspaceLocalState(ctx context.Context, root *os.File) (workspaceLocalState, error) {
	var state workspaceLocalState
	info, err := workspaceRootIdentity(root)
	if err != nil {
		return state, err
	}
	state.RootDevice, state.RootInode = uint64(info.Dev), info.Ino
	top, err := workspaceGit(ctx, root.Name(), "rev-parse", "--show-toplevel")
	if err != nil {
		return state, err
	}
	if strings.TrimSpace(top) != root.Name() {
		return state, errors.New("workspace path must be the Git deployment root")
	}
	revision, err := workspaceGit(ctx, root.Name(), "rev-parse", "--verify", "HEAD")
	if err != nil {
		return state, err
	}
	state.Revision = strings.TrimSpace(revision)
	if !fullGitObjectIDPattern.MatchString(state.Revision) {
		return state, errors.New("workspace requires a committed Git revision")
	}
	manifest, err := workspaceGit(ctx, root.Name(), "ls-files", "--stage", "-z")
	if err != nil {
		return state, err
	}
	tracked := make(map[string]bool)
	for _, entry := range strings.Split(strings.TrimSuffix(manifest, "\x00"), "\x00") {
		header, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || fields[2] != "0" || (fields[0] != "100644" && fields[0] != "100755" && fields[0] != "120000") {
			return state, errors.New("workspace requires a regular Git source without unresolved entries or submodules")
		}
		tracked[path] = true
	}
	for _, path := range privateDeploymentPaths {
		if tracked[path] {
			return state, errors.New("refusing workspace evaluation because a private key is tracked")
		}
	}
	if !tracked["flake.nix"] || !tracked["flake.lock"] {
		return state, errors.New("workspace requires tracked flake.nix and flake.lock")
	}
	state.Index = workspaceDigest(manifest)
	// Refuse special files before invoking Nix, including a FIFO lock or flake.
	if _, _, err := readWorkspaceFile(root, "flake.nix", 1024*1024); err != nil {
		return state, fmt.Errorf("read flake.nix: %w", err)
	}
	lock, _, err := readWorkspaceFile(root, "flake.lock", 4*1024*1024)
	if err != nil {
		return state, fmt.Errorf("read flake.lock: %w", err)
	}
	state.LockContent, state.Pin = lock, workspaceDigest(lock)
	data, fileInfo, err := readWorkspaceFile(root, domain.WorkspaceFileName, domain.WorkspaceMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		state.Base = workspaceDigest("absent-workspace-profile")
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read workspace profile: %w", err)
	}
	if fileInfo.Nlink != 1 {
		return state, errors.New("workspace profile must not have hard links")
	}
	profile, issues := domain.DecodeWorkspaceProfile(data)
	if len(issues) != 0 {
		return state, errors.New("existing workspace profile is invalid; inspect it before planning changes")
	}
	state.BaseExists, state.Profile = true, &profile
	state.Base = workspaceDigest(struct {
		Data           []byte
		Mode, UID, GID uint32
		Device, Inode  uint64
	}{data, fileInfo.Mode, fileInfo.Uid, fileInfo.Gid, uint64(fileInfo.Dev), fileInfo.Ino})
	return state, nil
}

func inspectWorkspaceSource(ctx context.Context, root *os.File, candidate []byte) (domain.WorkspaceSnapshot, *domain.WorkspaceProfile, *domain.WorkspaceResolution, error) {
	var snapshot domain.WorkspaceSnapshot
	before, err := readWorkspaceLocalState(ctx, root)
	if err != nil {
		return snapshot, nil, nil, err
	}
	flake, err := deploymentFlakeReference(root.Name())
	if err != nil {
		return snapshot, nil, nil, err
	}
	expression := workspaceSourceExpression
	if candidate != nil {
		expression = workspaceCandidateExpression
	}
	command := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval",
		"--impure", "--json", "--no-write-lock-file", "--no-update-lock-file",
		"--option", "allow-import-from-derivation", "false", "--option", "accept-flake-config", "false", "--expr", expression)
	configureCommandCancellation(command)
	command.Env = append(workspaceEnvironment(), "NIXORIUM_DEPLOYMENT_FLAKE="+flake, "NIXORIUM_WORKSPACE_CANDIDATE="+string(candidate))
	output := &boundedCommandBuffer{limit: 1024 * 1024}
	diagnostics := &boundedCommandBuffer{limit: 64 * 1024}
	command.Stdout, command.Stderr = output, diagnostics
	if err := command.Run(); err != nil {
		// Deployment evaluation can print private values in traces. Never copy
		// its arbitrary diagnostics into the structured application report.
		return snapshot, nil, nil, fmt.Errorf("workspace evaluation failed; check pinned inputs and both deployment candidate hooks: %w", err)
	}
	if output.truncated {
		return snapshot, nil, nil, errors.New("workspace metadata exceeds the 1 MiB limit")
	}
	var evaluated workspaceEvaluation
	decoder := json.NewDecoder(bytes.NewReader(output.buffer.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evaluated); err != nil {
		return snapshot, nil, nil, errors.New("invalid workspace evaluation metadata")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return snapshot, nil, nil, errors.New("trailing workspace evaluation metadata")
	}
	if !domain.ValidStorePath(evaluated.Source.OutPath) || !strings.HasPrefix(evaluated.Source.NarHash, "sha256-") {
		return snapshot, nil, nil, errors.New("workspace evaluation omitted its immutable source identity")
	}
	after, err := readWorkspaceLocalState(ctx, root)
	if err != nil {
		return snapshot, nil, nil, err
	}
	if workspaceDigest(before) != workspaceDigest(after) {
		return snapshot, nil, nil, domain.ErrWorkspaceConflict
	}
	// Verify that evaluation really consumed the reviewed lock, including on
	// first save when the profile itself may still be untracked.
	storeFD, err := unix.Openat2(unix.AT_FDCWD, evaluated.Source.OutPath, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return snapshot, nil, nil, err
	}
	storeRoot := os.NewFile(uintptr(storeFD), evaluated.Source.OutPath)
	defer storeRoot.Close()
	lock, _, err := readWorkspaceFile(storeRoot, "flake.lock", 4*1024*1024)
	if err != nil {
		return snapshot, nil, nil, fmt.Errorf("read evaluated workspace lock: %w", err)
	}
	if !bytes.Equal(lock, after.LockContent) {
		return snapshot, nil, nil, domain.ErrWorkspaceConflict
	}
	snapshot = domain.WorkspaceSnapshot{
		Revision: after.Revision, PinFingerprint: after.Pin, BaseFingerprint: after.Base, BaseExists: after.BaseExists,
		SourceFingerprint: workspaceDigest(struct {
			Local  workspaceLocalState
			Source workspaceNixSource
		}{after, evaluated.Source}),
	}
	return snapshot, after.Profile, evaluated.Resolution, nil
}

func writeWorkspaceFile(ctx context.Context, root *os.File, expected domain.WorkspaceSnapshot, data []byte,
	inspect func() (domain.WorkspaceSnapshot, error), syncDirectory func() error) error {
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := inspect()
		if err != nil {
			return err
		}
		if current != expected {
			return domain.ErrWorkspaceConflict
		}
		return ctx.Err()
	}
	if err := check(); err != nil {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	name := ".workspace-profile." + hex.EncodeToString(random[:]) + ".tmp"
	fd, err := unix.Openat(int(root.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return fmt.Errorf("create workspace draft: %w", err)
	}
	defer unix.Unlinkat(int(root.Fd()), name, 0)
	temporary := os.NewFile(uintptr(fd), name)
	defer temporary.Close()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write workspace draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync workspace draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close workspace draft: %w", err)
	}
	if err := check(); err != nil {
		return err
	}
	if _, err := workspaceRootIdentity(root); err != nil {
		return err
	}
	// Re-read the destination after the potentially slow source export too.
	local, err := readWorkspaceLocalState(ctx, root)
	if err != nil {
		return err
	}
	if local.Base != expected.BaseFingerprint || local.BaseExists != expected.BaseExists || local.Revision != expected.Revision || local.Pin != expected.PinFingerprint {
		return domain.ErrWorkspaceConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	flags := uint(0)
	if !expected.BaseExists {
		flags = unix.RENAME_NOREPLACE
	}
	if err := unix.Renameat2(int(root.Fd()), name, int(root.Fd()), domain.WorkspaceFileName, flags); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return domain.ErrWorkspaceConflict
		}
		return fmt.Errorf("replace workspace profile: %w", err)
	}
	if err := syncDirectory(); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrWorkspaceDurability, err)
	}
	return nil
}
