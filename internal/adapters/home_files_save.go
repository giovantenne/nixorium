package adapters

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func writeHomeStageManifest(root *os.File, data []byte) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	name := ".home-stage." + hex.EncodeToString(nonce[:])
	fd, err := unix.Openat(int(root.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(int(root.Fd()), name, 0)
	file := os.NewFile(uintptr(fd), name)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err = errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return unix.Renameat(int(root.Fd()), name, int(root.Fd()), domain.HomeFilesFileName)
}

func homeFilesDirectory(parent *os.File, name string) (*os.File, error) {
	if err := unix.Mkdirat(int(parent.Fd()), name, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
		return nil, err
	}
	fd, err := unix.Openat2(int(parent.Fd()), name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	if err != nil {
		return nil, err
	}
	child := os.NewFile(uintptr(fd), name)
	var info unix.Stat_t
	if err = unix.Fstat(fd, &info); err != nil || info.Uid != uint32(os.Geteuid()) || info.Mode&0022 != 0 {
		child.Close()
		return nil, errors.New("initial file storage must be privately owned and not writable by other accounts")
	}
	if err = parent.Sync(); err != nil {
		child.Close()
		return nil, err
	}
	return child, nil
}

func homeFilesDirectoryIdentity(parent, child *os.File, name string) error {
	var held, current unix.Stat_t
	if err := unix.Fstat(int(child.Fd()), &held); err != nil {
		return err
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if held.Dev != current.Dev || held.Ino != current.Ino || current.Mode&unix.S_IFMT != unix.S_IFDIR {
		return domain.ErrHomeFilesConflict
	}
	return nil
}

func (local Local) WriteHomeFilesIfUnchanged(ctx context.Context, repository string, expected domain.HomeFilesSnapshot, draft domain.HomeFilesDraft) (err error) {
	root, err := openWorkspaceRoot(repository, unix.LOCK_EX)
	if err != nil {
		return err
	}
	defer root.Close()
	fresh, err := local.inspectHomeFilesLocked(ctx, root, draft)
	if err != nil {
		return err
	}
	if fresh.Snapshot != expected {
		return domain.ErrHomeFilesConflict
	}
	payloads, _, err := homeFilesPayloads(ctx, root, draft)
	if err != nil {
		return err
	}
	data, err := domain.MarshalHomeFilesProfile(draft.Profile)
	if err != nil {
		return err
	}
	storage, err := homeFilesDirectory(root, domain.HomeFilesDirectoryName)
	if err != nil {
		return err
	}
	defer storage.Close()
	imports, err := homeFilesDirectory(storage, "imports")
	if err != nil {
		return err
	}
	defer imports.Close()
	created := map[string]unix.Stat_t{}
	published := false
	defer func() {
		if !published {
			failed := false
			for name, expectedInfo := range created {
				current, info, readErr := readWorkspaceFile(imports, name, domain.HomeFilesMaxFileBytes)
				if readErr != nil || info.Ino != expectedInfo.Ino || info.Dev != expectedInfo.Dev || info.Nlink != 1 || info.Uid != uint32(os.Geteuid()) || fmt.Sprintf("%x", sha256.Sum256(current)) != name {
					failed = true
					continue
				}
				if unlinkErr := unix.Unlinkat(int(imports.Fd()), name, 0); unlinkErr != nil {
					failed = true
				}
			}
			if syncErr := imports.Sync(); syncErr != nil {
				failed = true
			}
			if failed {
				err = errors.Join(err, domain.ErrHomeFilesDurability)
			}
		}
	}()
	for hash, content := range payloads {
		if err = ctx.Err(); err != nil {
			return err
		}
		old, _, readErr := readWorkspaceFile(imports, hash, domain.HomeFilesMaxFileBytes)
		if readErr == nil {
			if !bytes.Equal(old, content) {
				return domain.ErrHomeFilesConflict
			}
			continue
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		fd, openErr := unix.Openat(int(imports.Fd()), hash, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if openErr != nil {
			return openErr
		}
		file := os.NewFile(uintptr(fd), hash)
		var info unix.Stat_t
		if statErr := unix.Fstat(fd, &info); statErr != nil {
			file.Close()
			return statErr
		}
		created[hash] = info
		_, writeErr := file.Write(content)
		syncErr := file.Sync()
		closeErr := file.Close()
		if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
			return err
		}
	}
	if err = imports.Sync(); err != nil {
		return err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	name := ".home-files." + hex.EncodeToString(nonce[:]) + ".tmp"
	fd, err := unix.Openat(int(root.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(int(root.Fd()), name, 0)
	file := os.NewFile(uintptr(fd), name)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	// Recheck the complete tracked source after publication of only untracked,
	// immutable captured copies. The manifest remains the publication point.
	check, err := local.inspectHomeFilesLocked(ctx, root, draft)
	if err != nil {
		return err
	}
	if check.Snapshot != expected {
		return domain.ErrHomeFilesConflict
	}
	state, _, err := readHomeFilesState(ctx, root)
	if err != nil {
		return err
	}
	if state.Base != expected.BaseFingerprint || state.BaseExists != expected.BaseExists || state.Revision != expected.Revision || state.Pin != expected.PinFingerprint {
		return domain.ErrHomeFilesConflict
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = workspaceRootIdentity(root); err != nil {
		return err
	}
	if err = homeFilesDirectoryIdentity(root, storage, domain.HomeFilesDirectoryName); err != nil {
		return err
	}
	if err = homeFilesDirectoryIdentity(storage, imports, "imports"); err != nil {
		return err
	}
	flags := uint(0)
	if !expected.BaseExists {
		flags = unix.RENAME_NOREPLACE
	}
	if err = unix.Renameat2(int(root.Fd()), name, int(root.Fd()), domain.HomeFilesFileName, flags); err != nil {
		return err
	}
	published = true
	if err = root.Sync(); err != nil {
		return fmt.Errorf("%w: manifest replaced", domain.ErrHomeFilesDurability)
	}
	return nil
}

func (local Local) RecordHomeFilesCandidate(ctx context.Context, repository, expectedRevision string, draft domain.HomeFilesDraft) (string, error) {
	root, err := openWorkspaceRoot(repository, unix.LOCK_EX)
	if err != nil {
		return "", err
	}
	defer root.Close()
	wanted, err := domain.MarshalHomeFilesProfile(draft.Profile)
	if err != nil {
		return "", err
	}
	actual, info, err := readWorkspaceFile(root, domain.HomeFilesFileName, domain.HomeFilesMaxBytes)
	if err != nil || !bytes.Equal(actual, wanted) || info.Nlink != 1 || info.Mode&0777 != 0600 {
		return "", domain.ErrHomeFilesConflict
	}
	revision, err := local.GitRevision(ctx, repository)
	if err != nil || revision != expectedRevision {
		return "", domain.ErrHomeFilesConflict
	}
	paths := []string{domain.HomeFilesFileName}
	for _, f := range draft.Profile.Files {
		if f.Source != nil {
			paths = append(paths, domain.HomeFilesDirectoryName+"/"+*f.Source)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	arguments := append([]string{"diff", "--cached", "--name-only", "-z", "--no-ext-diff", "--no-textconv", "HEAD", "--"}, paths...)
	staged, truncated, err := runBoundedGit(ctx, repository, 1024*1024, arguments...)
	if err != nil || truncated || staged != "" {
		return "", domain.ErrHomeFilesConflict
	}
	proposal, err := local.GitCommitProposal(ctx, repository, paths)
	if err != nil {
		return "", err
	}
	blob, truncated, err := runBoundedGit(ctx, repository, domain.HomeFilesMaxBytes, "cat-file", "blob", proposal.TreeID+":"+domain.HomeFilesFileName)
	if err != nil || truncated || !bytes.Equal([]byte(blob), wanted) {
		return "", domain.ErrHomeFilesConflict
	}
	for _, path := range paths {
		if path == domain.HomeFilesFileName {
			continue
		}
		hash := strings.TrimPrefix(path, domain.HomeFilesDirectoryName+"/imports/")
		content, truncated, err := runBoundedGit(ctx, repository, domain.HomeFilesMaxFileBytes, "cat-file", "blob", proposal.TreeID+":"+path)
		if err != nil || truncated || fmt.Sprintf("%x", sha256.Sum256([]byte(content))) != hash {
			return "", domain.ErrHomeFilesConflict
		}
	}
	if _, err = workspaceRootIdentity(root); err != nil {
		return "", err
	}
	commit, err := local.CommitGitPaths(ctx, repository, paths, "chore: update initial student files", expectedRevision, proposal.TreeID)
	if err == nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		for _, id := range draft.CaptureIDs {
			// The commit is made; a leftover capture expires on its own.
			_ = local.DiscardHomeFilesCapture(cleanup, repository, id)
		}
	}
	return commit, err
}
