package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// Support observation cannot enroll new SSH trust or invoke configured hooks.
// Unknown/changed keys remain unknown until handled in a separate workflow.
func (Local) SupportCurrentSystems(ctx context.Context, hosts []domain.HostMeta, timeout time.Duration) map[string]domain.HostSystemProbe {
	return probeCurrentSystems(ctx, hosts, timeout, func(ctx context.Context, host domain.HostMeta, timeout time.Duration) domain.HostSystemProbe {
		return probeCurrentSystemArguments(ctx, timeout, supportSSHArguments(host, timeout))
	})
}

func supportSSHArguments(host domain.HostMeta, timeout time.Duration) []string {
	arguments := sshCurrentSystemArguments(host, timeout)
	for index, argument := range arguments {
		if argument == "StrictHostKeyChecking=accept-new" {
			arguments[index] = "StrictHostKeyChecking=yes"
		}
	}
	return append([]string{"-F", "/dev/null", "-o", "UpdateHostKeys=no", "-o", "ControlMaster=no", "-o", "ControlPath=none",
		"-o", "PermitLocalCommand=no", "-o", "ProxyCommand=none", "-o", "ProxyJump=none", "-o", "ClearAllForwardings=yes",
		"-o", "IdentityAgent=none", "-o", "ForwardAgent=no"}, arguments...)
}

func (Local) WriteSupport(ctx context.Context, snapshot domain.SupportSnapshot) domain.SupportExportResult {
	root, err := userStateRoot()
	if err != nil {
		return supportExportFailure()
	}
	return writeSupport(ctx, root, snapshot, nil)
}

func supportExportFailure() domain.SupportExportResult {
	return domain.SupportExportResult{State: "failed", Message: "Could not export to the private local state directory. Check ownership, permissions, symlinks, free space and unnamed-file support; no report was published."}
}

func writeSupport(ctx context.Context, stateRoot string, snapshot domain.SupportSnapshot, syncDirectory func(*os.File) error) domain.SupportExportResult {
	if !snapshot.Valid() || ctx.Err() != nil {
		return supportExportFailure()
	}
	directory, err := openSupportDirectory(stateRoot)
	if err != nil {
		return supportExportFailure()
	}
	defer directory.Close()
	// An unnamed inode cannot expose a partial report or be replaced by a
	// competing pathname before publication. Unsupported filesystems fail closed.
	fd, err := unix.Openat(int(directory.Fd()), ".", unix.O_TMPFILE|unix.O_WRONLY|unix.O_CLOEXEC, 0600)
	if err != nil {
		return supportExportFailure()
	}
	file := os.NewFile(uintptr(fd), "support report")
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return supportExportFailure()
	}
	if _, err := file.WriteString(snapshot.JSON()); err != nil {
		return supportExportFailure()
	}
	if err := file.Sync(); err != nil || ctx.Err() != nil {
		return supportExportFailure()
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return supportExportFailure()
	}
	name := "support-" + hex.EncodeToString(random[:]) + ".json"
	if err := unix.Linkat(unix.AT_FDCWD, fmt.Sprintf("/proc/self/fd/%d", fd), int(directory.Fd()), name, unix.AT_SYMLINK_FOLLOW); err != nil {
		return supportExportFailure()
	}
	result := domain.SupportExportResult{State: "saved", Path: filepath.Join(directory.Name(), name), SHA256: snapshot.Digest(), Message: "Saved the exact preview locally. Nothing was uploaded."}
	if syncDirectory == nil {
		syncDirectory = func(file *os.File) error { return file.Sync() }
	}
	if err := syncDirectory(directory); err != nil || !supportDirectoryStillNamed(directory) {
		result.State = "partial"
		result.Message = "The local report was created, but directory identity or durability is unconfirmed. Inspect local state before retrying; nothing was uploaded."
	}
	return result
}

func supportDirectoryStillNamed(directory *os.File) bool {
	fd, err := unix.Openat2(unix.AT_FDCWD, directory.Name(), &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS})
	if err != nil {
		return false
	}
	defer unix.Close(fd)
	var held, named unix.Stat_t
	return unix.Fstat(int(directory.Fd()), &held) == nil && unix.Fstat(fd, &named) == nil && held.Dev == named.Dev && held.Ino == named.Ino
}

// Walk from a held root descriptor, never following any symlink. Newly created
// directories are private; existing ancestors must not permit other users to
// replace entries. The root-owned sticky temporary directory supports tests.
func openSupportDirectory(stateRoot string) (*os.File, error) {
	if !filepath.IsAbs(stateRoot) || filepath.Clean(stateRoot) != stateRoot || stateRoot == "/" || len(stateRoot) > 4096 {
		return nil, errors.New("invalid support state directory")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	current := os.NewFile(uintptr(fd), "/")
	parts := strings.Split(strings.TrimPrefix(filepath.Join(stateRoot, "nixorium", "support"), "/"), "/")
	for index, part := range parts {
		next, openErr := unix.Openat(int(current.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			if err := unix.Mkdirat(int(current.Fd()), part, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
				current.Close()
				return nil, err
			}
			if err := current.Sync(); err != nil {
				current.Close()
				return nil, err
			}
			next, openErr = unix.Openat(int(current.Fd()), part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			current.Close()
			return nil, openErr
		}
		child := os.NewFile(uintptr(next), filepath.Join(current.Name(), part))
		current.Close()
		current = child
		var stat unix.Stat_t
		if err := unix.Fstat(next, &stat); err != nil {
			current.Close()
			return nil, err
		}
		owned := stat.Uid == uint32(os.Geteuid())
		trustedSticky := stat.Uid == 0 && stat.Mode&unix.S_ISVTX != 0
		unsafe := (!owned && stat.Uid != 0) || (stat.Mode&0022 != 0 && !trustedSticky)
		if index >= len(parts)-2 {
			unsafe = !owned || stat.Mode&0777 != 0700
		}
		if unsafe {
			current.Close()
			return nil, errors.New("unsafe support directory ownership or permissions")
		}
	}
	return current, nil
}
