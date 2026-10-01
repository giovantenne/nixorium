package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

type resetJournal struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Branch            string `json:"branch"`
	OriginalRevision  string `json:"originalRevision"`
	CandidateRevision string `json:"candidateRevision"`
	BackupRef         string `json:"backupRef"`
	ReviewToken       string `json:"reviewToken"`
}

func (TemplateReset) ApplyTemplateReset(ctx context.Context, plan domain.TemplateResetPlan) domain.TemplateResetResult {
	fail := func(err error) domain.TemplateResetResult {
		return domain.TemplateResetResult{State: "blocked", Message: err.Error()}
	}
	gate, err := acquireManagedOperationGate()
	if err != nil {
		return fail(err)
	}
	defer gate.Close()
	gate.describe("Reset deployment template")
	// These units can continue serving an installation after its initiating TUI
	// has closed. A template reset must not invalidate that preparation in flight.
	for _, unit := range []string{"nixorium-pxe.service", "nixorium-pxe-network.service"} {
		command := exec.CommandContext(ctx, "systemctl", "show", unit, "--property=LoadState", "--property=ActiveState", "--no-pager")
		out := &boundedCommandBuffer{limit: 4096}
		command.Stdout, command.Stderr = out, &boundedCommandBuffer{limit: 4096}
		if err := command.Run(); err != nil || out.truncated {
			return fail(errors.New("could not verify PXE service state; no reset was attempted"))
		}
		properties := map[string]string{}
		for _, line := range strings.Split(out.buffer.String(), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				properties[key] = value
			}
		}
		if (properties["LoadState"] != "loaded" && properties["LoadState"] != "not-found") || properties["ActiveState"] != "inactive" {
			return fail(errors.New("finish or recover the active PXE operation before resetting the deployment"))
		}
	}
	root, err := openWorkspaceRoot(plan.Repository, unix.LOCK_EX)
	if err != nil {
		return fail(err)
	}
	defer root.Close()
	return applyResetTransaction(ctx, root, plan, nil)
}

// A multi-file checkout is not atomic. The durable backup and intent survive
// every subsequent failure; general operation preflight rejects that marker.
// afterCheckout is only a fault-injection seam for real-Git transaction tests.
func applyResetTransaction(ctx context.Context, root *os.File, plan domain.TemplateResetPlan, afterCheckout func() error) domain.TemplateResetResult {
	result := domain.TemplateResetResult{State: "blocked"}
	fail := func(err error) domain.TemplateResetResult {
		result.Message = err.Error()
		if result.RecoveryRequired {
			result.State = "recovery-required"
			result.Message = "Reset interrupted. Keep .git/" + resetPendingName + " and the backup ref. Do not apply or deploy this worktree. See docs/deployment-template-reset.md for recovery."
		}
		return result
	}
	token, err := domain.TemplateResetToken(plan)
	if err != nil || plan.HasErrors() || plan.ReviewToken != token || plan.Confirmation != "RESET DEPLOYMENT" || root.Name() != plan.Repository {
		return fail(errors.New("a complete unchanged reset review is required"))
	}
	source, err := inspectResetRepository(ctx, root)
	if err != nil {
		return fail(err)
	}
	expected := plan.Proposal
	expected.Candidate, expected.UpstreamRevision = nil, ""
	if resetFileDigest(source) != resetFileDigest(expected) {
		return fail(errors.New("deployment changed after review; no tracked file was replaced"))
	}
	if err := resetCheckCandidateDestinations(root, plan.Proposal.Candidate); err != nil {
		return fail(err)
	}
	tree, err := resetWriteTree(ctx, root.Name(), plan.Proposal.Candidate)
	if err != nil {
		return fail(err)
	}
	commit, err := resetGit(ctx, root.Name(), nil, 256, "commit-tree", tree, "-p", plan.Revision, "-m", "Reset deployment to pinned template ("+plan.Preset.ID+")")
	if err != nil {
		return fail(err)
	}
	commit = strings.TrimSpace(commit)
	if !fullGitObjectIDPattern.MatchString(commit) {
		return fail(errors.New("invalid candidate commit"))
	}
	if _, err := resetGit(ctx, root.Name(), nil, 4096, "read-tree", "--dry-run", "-m", "-u", plan.Revision, commit); err != nil {
		return fail(err)
	}
	// Recheck immediately before creating the recovery intent. No source file is
	// staged, stashed or reset in order to make this preflight pass.
	current, err := inspectResetRepository(ctx, root)
	if err != nil {
		return fail(err)
	}
	if resetFileDigest(source) != resetFileDigest(current) {
		return fail(errors.New("deployment changed while preparing its backup"))
	}
	backup := "refs/nixorium/template-backups/" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + plan.Revision[:12]
	if _, err := resetGit(ctx, root.Name(), nil, 4096, "update-ref", backup, plan.Revision, strings.Repeat("0", 40)); err != nil {
		return fail(err)
	}
	result.BackupRef = backup
	journal := resetJournal{1, source.Branch, plan.Revision, commit, backup, token}
	result.RecoveryRequired = true
	if err := writeResetJournal(root, journal); err != nil {
		return fail(err)
	}
	// Unlike --reset -u, the two-tree merge refuses tracked local modifications.
	// All ignored/untracked collisions were independently rejected above.
	if _, err := resetGit(ctx, root.Name(), nil, 4096, "read-tree", "-m", "-u", plan.Revision, commit); err != nil {
		return fail(err)
	}
	if afterCheckout != nil {
		if err := afterCheckout(); err != nil {
			return fail(err)
		}
	}
	if err := resetSyncCandidate(root, plan.Proposal.Candidate); err != nil {
		return fail(err)
	}
	if err := resetSyncRemovedParents(root, plan.Proposal.Original); err != nil {
		return fail(err)
	}
	if _, err := resetGit(ctx, root.Name(), nil, 4096, "diff", "--quiet", "--no-ext-diff", "--no-textconv", commit, "--"); err != nil {
		return fail(err)
	}
	if _, err := resetGit(ctx, root.Name(), nil, 4096, "diff", "--cached", "--quiet", "--no-ext-diff", "--no-textconv", commit, "--"); err != nil {
		return fail(err)
	}
	branch, err := resetGit(ctx, root.Name(), nil, 4096, "symbolic-ref", "HEAD")
	if err != nil || strings.TrimSpace(branch) != source.Branch {
		return fail(errors.New("active branch changed during reset"))
	}
	if _, err := resetGit(ctx, root.Name(), nil, 4096, "update-ref", "-m", "Nixorium template reset", source.Branch, commit, plan.Revision); err != nil {
		return fail(err)
	}
	if err := clearResetJournal(root); err != nil {
		return fail(err)
	}
	result.State, result.Revision, result.RecoveryRequired = "saved", commit, false
	result.Message = "Deployment template reset and committed locally. Settings, keys, ignored files and input pins were preserved. Guided home is configured, not applied: review controller/client application and reboot separately. Nothing was pushed."
	return result
}

func resetCheckCandidateDestinations(root *os.File, files map[string]domain.TemplateFile) error {
	for name := range files {
		// Existing ancestors must be real directories on this filesystem. Missing
		// suffixes are created by Git; no untracked directory tree is removed.
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			fd, err := unix.Openat2(int(root.Fd()), parent, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			if err != nil {
				return errors.New("candidate path traverses an unsafe existing directory")
			}
			var info unix.Stat_t
			err = unix.Fstat(fd, &info)
			unix.Close(fd)
			if err != nil || info.Uid != uint32(os.Geteuid()) || info.Mode&0022 != 0 {
				return errors.New("candidate parent has unsafe ownership or permissions")
			}
		}
	}
	return nil
}

func resetWriteTree(ctx context.Context, repository string, files map[string]domain.TemplateFile) (string, error) {
	type entry struct{ mode, hash string }
	blobs := map[string]entry{}
	for name, file := range files {
		output, err := resetGit(ctx, repository, file.Data, 256, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		hash := strings.TrimSpace(output)
		if !fullGitObjectIDPattern.MatchString(hash) {
			return "", errors.New("invalid template blob")
		}
		blobs[name] = entry{file.Mode, hash}
	}
	var tree func(string) (string, error)
	tree = func(prefix string) (string, error) {
		children := map[string]entry{}
		for name, blob := range blobs {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			rest := strings.TrimPrefix(name, prefix)
			first, _, nested := strings.Cut(rest, "/")
			if nested {
				children[first] = entry{mode: "040000"}
			} else {
				children[first] = blob
			}
		}
		names := make([]string, 0, len(children))
		for name := range children {
			names = append(names, name)
		}
		sort.Strings(names)
		var input strings.Builder
		for _, name := range names {
			child := children[name]
			kind := "blob"
			if child.mode == "040000" {
				var err error
				child.hash, err = tree(prefix + name + "/")
				if err != nil {
					return "", err
				}
				kind = "tree"
			}
			fmt.Fprintf(&input, "%s %s %s\t%s\x00", child.mode, kind, child.hash, name)
		}
		output, err := resetGit(ctx, repository, []byte(input.String()), 256, "mktree", "-z")
		if err != nil {
			return "", err
		}
		hash := strings.TrimSpace(output)
		if !fullGitObjectIDPattern.MatchString(hash) {
			return "", errors.New("invalid template tree")
		}
		return hash, nil
	}
	return tree("")
}

func resetGitDirectory(root *os.File) (*os.File, error) {
	fd, err := unix.Openat2(int(root.Fd()), ".git", &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(root.Name(), ".git")), nil
}

func writeResetJournal(root *os.File, journal resetJournal) error {
	directory, err := resetGitDirectory(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	fd, err := unix.Openat(int(directory.Fd()), resetPendingName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), resetPendingName)
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return directory.Sync()
}

func clearResetJournal(root *os.File) error {
	directory, err := resetGitDirectory(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := unix.Unlinkat(int(directory.Fd()), resetPendingName, 0); err != nil {
		return err
	}
	return directory.Sync()
}

func resetSyncCandidate(root *os.File, files map[string]domain.TemplateFile) error {
	for name, file := range files {
		if file.Mode == "120000" {
			parent, err := resetOpenParent(root, name)
			if err != nil {
				return err
			}
			buffer := make([]byte, 4096)
			n, readErr := unix.Readlinkat(int(parent.Fd()), filepath.Base(name), buffer)
			parent.Close()
			if readErr != nil || n >= len(buffer) || string(buffer[:n]) != string(file.Data) {
				return errors.New("candidate symlink changed during checkout")
			}
		} else {
			data, _, err := readWorkspaceFile(root, name, 16*1024*1024)
			if err != nil || resetFileDigest(data) != resetFileDigest(file.Data) {
				return errors.New("candidate bytes changed during checkout")
			}
			fd, err := unix.Openat2(int(root.Fd()), name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
			if err != nil {
				return err
			}
			err = unix.Fsync(fd)
			unix.Close(fd)
			if err != nil {
				return err
			}
		}
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			dir, err := resetOpenParent(root, filepath.Join(parent, "entry"))
			if err != nil {
				return err
			}
			err = dir.Sync()
			dir.Close()
			if err != nil {
				return err
			}
		}
	}
	return root.Sync()
}

func resetSyncRemovedParents(root *os.File, original map[string]domain.TemplateFile) error {
	for name := range original {
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			fd, err := unix.Openat2(int(root.Fd()), parent, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
			if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENOTDIR) {
				continue
			}
			if err != nil {
				return err
			}
			err = unix.Fsync(fd)
			unix.Close(fd)
			if err != nil {
				return err
			}
		}
	}
	return root.Sync()
}

func checkTemplateResetPending(repository string) error {
	_, err := os.Lstat(filepath.Join(repository, ".git", resetPendingName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return errors.New("unfinished deployment template reset: preserve .git/" + resetPendingName + " and recover before other operations")
}
