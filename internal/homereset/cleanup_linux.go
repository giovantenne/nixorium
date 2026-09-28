// Package homereset contains internal filesystem primitives, not a public reset
// command. Callers must separately establish the managed account, trusted mount,
// inactive sessions, snapshot policy, and recovery behavior before using them.
package homereset

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const childResolution = unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV

// RemoveEphemeral removes only the selected relative paths below directory.
// The complete list and its existing trees are checked before any unlink.
// Symlinks at a selected leaf (or inside its tree) are unlinked, never followed;
// symlinks in a selected path's parents and all nested mounts are rejected.
//
// The caller must keep the home quiescent. Descriptor-relative operations also
// constrain traversal during races, but this is not a transaction: I/O errors
// during removal can leave a partial result, which must not be retried blindly.
// Unsupported kernel confinement primitives fail closed, without a fallback.
func RemoveEphemeral(directory string, paths []string) error {
	if err := validatePaths(paths); err != nil {
		return err
	}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || directory == "/" || directory == "/home" {
		return errors.New("home cleanup requires an explicit canonical directory")
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, directory, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW,
		Resolve: unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return fmt.Errorf("open home cleanup root: %w", err)
	}
	defer unix.Close(fd)
	root, err := statEntry(fd, "", unix.AT_EMPTY_PATH)
	if err != nil {
		return err
	}
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(fd, &filesystem); err != nil {
		return err
	}
	if filesystem.Type == unix.BTRFS_SUPER_MAGIC && root.Mask&unix.STATX_SUBVOL == 0 {
		return errors.New("kernel did not provide the required Btrfs subvolume identity")
	}
	// The first pass is read-only, including checks for mounts hidden deeper in
	// an earlier selection. A later invalid selection must not delete anything.
	for _, remove := range []bool{false, true} {
		for _, relative := range paths {
			parent, err := unix.Openat2(fd, path.Dir(relative), &unix.OpenHow{
				Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW,
				Resolve: childResolution,
			})
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect cleanup parent %q: %w", relative, err)
			}
			parentInfo, parentErr := statEntry(parent, "", unix.AT_EMPTY_PATH)
			if parentErr == nil {
				parentErr = sameBoundary(root, parentInfo)
			}
			if parentErr == nil {
				parentErr = visit(parent, path.Base(relative), root, 0, remove)
			}
			unix.Close(parent)
			if parentErr != nil {
				return fmt.Errorf("clean ephemeral path %q: %w", relative, parentErr)
			}
		}
	}
	return nil
}

func validatePaths(paths []string) error {
	if len(paths) > 128 {
		return errors.New("too many ephemeral paths")
	}
	for i, name := range paths {
		if name == "." || !fs.ValidPath(name) || len(name) > 4096 || strings.ContainsAny(name, "\x00\r\n\\") {
			return errors.New("ephemeral paths must be canonical relative paths")
		}
		for _, char := range name {
			if char < 32 || char == 127 {
				return errors.New("ephemeral paths must not contain control characters")
			}
		}
		for _, previous := range paths[:i] {
			if name == previous || strings.HasPrefix(name, previous+"/") || strings.HasPrefix(previous, name+"/") {
				return errors.New("ephemeral paths must not overlap")
			}
		}
	}
	return nil
}

func statEntry(fd int, name string, flags int) (unix.Statx_t, error) {
	var stat unix.Statx_t
	const required = unix.STATX_TYPE | unix.STATX_MNT_ID
	err := unix.Statx(fd, name, flags|unix.AT_SYMLINK_NOFOLLOW|unix.AT_NO_AUTOMOUNT, required|unix.STATX_SUBVOL, &stat)
	if err != nil {
		return stat, err
	}
	if stat.Mask&required != required {
		return stat, errors.New("kernel did not provide the required mount identity")
	}
	return stat, nil
}

func sameBoundary(root, entry unix.Statx_t) error {
	if entry.Mnt_id != root.Mnt_id || entry.Dev_major != root.Dev_major || entry.Dev_minor != root.Dev_minor {
		return errors.New("ephemeral tree contains a nested mount or filesystem")
	}
	if root.Mask&unix.STATX_SUBVOL != 0 && (entry.Mask&unix.STATX_SUBVOL == 0 || entry.Subvol != root.Subvol) {
		return errors.New("ephemeral tree contains a nested subvolume")
	}
	return nil
}

func visit(parent int, name string, root unix.Statx_t, depth int, remove bool) error {
	if depth > 128 {
		return errors.New("ephemeral tree exceeds the supported depth")
	}
	stat, err := statEntry(parent, name, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := sameBoundary(root, stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		if remove {
			return unix.Unlinkat(parent, name, 0)
		}
		return nil
	}
	fd, err := unix.Openat2(parent, name, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW,
		Resolve: childResolution,
	})
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(fd), name)
	defer dir.Close()
	opened, err := statEntry(fd, "", unix.AT_EMPTY_PATH)
	if err != nil {
		return err
	}
	if err := sameBoundary(root, opened); err != nil {
		return err
	}
	for {
		entries, err := dir.ReadDir(128)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		for _, entry := range entries {
			if err := visit(fd, entry.Name(), root, depth+1, remove); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if remove {
		return unix.Unlinkat(parent, name, unix.AT_REMOVEDIR)
	}
	return nil
}
