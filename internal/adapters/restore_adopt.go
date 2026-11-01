package adapters

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// RestoreFolder proposes a new folder for Restore lab beside the deployment,
// so that using the restored lab later is a rename on the same disk.
func RestoreFolder(repository string) string {
	if !filepath.IsAbs(repository) {
		return ""
	}
	base := filepath.Join(filepath.Dir(filepath.Clean(repository)), "nixorium-restored")
	for index := 1; index < 100; index++ {
		candidate := base
		if index > 1 {
			candidate = fmt.Sprintf("%s-%d", base, index)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return ""
}

// MountedDrives lists the folders of removable drives mounted for the user
// (GNOME mounts them under /run/media/USER).
func MountedDrives(user string) []string {
	if user == "" {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join("/run/media", user, "*"))
	drives := []string{}
	for _, match := range matches {
		if info, err := os.Stat(match); err == nil && info.IsDir() && unix.Access(match, unix.W_OK) == nil {
			drives = append(drives, match)
		}
	}
	return drives
}

// FindBackupFiles lists encrypted backup files on mounted drives and in the
// home folder, newest first. Only names are read; nothing is decrypted.
func FindBackupFiles(home, user string) []string {
	var patterns []string
	if user != "" {
		patterns = append(patterns, filepath.Join("/run/media", user, "*", "nixorium-backup-*.age"), filepath.Join("/run/media", user, "*", "*", "nixorium-backup-*.age"))
	}
	patterns = append(patterns, filepath.Join("/media", "*", "nixorium-backup-*.age"), filepath.Join("/mnt", "*", "nixorium-backup-*.age"))
	if home != "" {
		patterns = append(patterns, filepath.Join(home, "nixorium-backup-*.age"))
	}
	type found struct {
		path    string
		modTime time.Time
	}
	var files []found
	seen := map[string]bool{}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.Mode().IsRegular() || seen[match] {
				continue
			}
			seen[match] = true
			files = append(files, found{match, info.ModTime()})
		}
	}
	// Names carry the creation time, so the newest backup sorts last by name;
	// the modification time breaks ties between copies on different drives.
	for i := 1; i < len(files); i++ {
		for j := i; j > 0; j-- {
			a, b := files[j-1], files[j]
			if filepath.Base(a.path) > filepath.Base(b.path) || (filepath.Base(a.path) == filepath.Base(b.path) && !a.modTime.Before(b.modTime)) {
				break
			}
			files[j-1], files[j] = b, a
		}
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.path)
	}
	return paths
}

// AdoptRestoredLab makes a restored lab the deployment this controller
// manages. The current deployment folder is kept beside it under a new name
// and the restored folder takes its place; nothing is deleted. It refuses
// when this controller already uses different keys, which a new controller
// installed for the replacement never does.
func (Local) AdoptRestoredLab(repository, restored, adminKey string, now time.Time) (string, error) {
	for _, path := range []string{repository, restored} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
			return "", errors.New("the lab folders must be absolute paths")
		}
	}
	if repository == restored {
		return "", errors.New("the restored lab is already the lab this controller manages")
	}
	if info, err := os.Lstat(restored); err != nil || !info.IsDir() {
		return "", errors.New("the restored lab folder " + restored + " is missing; restore the backup again")
	}
	for _, name := range []string{"flake.nix", "lab-settings.json", ".git"} {
		if _, err := os.Lstat(filepath.Join(restored, name)); err != nil {
			return "", errors.New("the folder " + restored + " does not contain a restored lab")
		}
	}
	if adminKey != "" {
		installed, err := os.ReadFile(adminKey)
		if err == nil {
			restoredKey, _, readErr := readRecoveryFile(filepath.Join(restored, "admin-ssh"), maximumKeyMaterialBytes)
			if readErr != nil || !bytes.Equal(bytes.TrimSpace(installed), bytes.TrimSpace(restoredKey)) {
				return "", errors.New("this controller already has the SSH key of another lab (" + adminKey + "), created when its own lab was prepared, so the restored lab cannot take over automatically; follow \"Replacing the controller\" in the troubleshooting guide")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("read the controller's SSH key: %w", err)
		}
	}
	// Like every change to the deployment, it waits for no other operation
	// and no held computers; the lock is released before the keys are
	// installed, which takes it again.
	gate, err := adoptOperationGate()
	if err != nil {
		return "", err
	}
	defer gate.Close()
	parent, err := openWorkspaceRoot(filepath.Dir(repository), unix.LOCK_EX)
	if err != nil {
		return "", err
	}
	defer parent.Close()
	previous := ""
	if _, err := os.Lstat(repository); err == nil {
		previous = repository + "-before-restore-" + now.Local().Format("20060102-150405")
		if _, err := os.Lstat(previous); !errors.Is(err, os.ErrNotExist) {
			return "", errors.New("a folder named " + previous + " already exists; try again in a minute")
		}
		if err := renameNew(repository, previous); err != nil {
			return "", fmt.Errorf("keep the current lab folder aside: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := renameNew(restored, repository); err != nil {
		if previous != "" {
			if undo := renameNew(previous, repository); undo != nil {
				return previous, fmt.Errorf("move the restored lab into place: %w; the current lab is kept at %s", err, previous)
			}
		}
		if errors.Is(err, unix.EXDEV) {
			return "", errors.New("the restored lab is on another disk; restore it again into a folder next to " + repository)
		}
		return "", fmt.Errorf("move the restored lab into place: %w", err)
	}
	return previous, nil
}

// RecordRestoredBackup notes the restored backup file as this lab's last
// backup: it holds exactly the lab now in use, so a reminder to back up is
// not due until something changes.
func (Local) RecordRestoredBackup(repository, source string, from domain.BackupReport) {
	if from.CreatedAt.IsZero() || source == "" {
		return
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return
	}
	recordBackup(domain.BackupRecord{Repository: root, CreatedAt: from.CreatedAt, Path: source, Revision: from.Revision, KeysDigest: backupKeysDigest(root), SettingsHash: backupSettingsHash(root)})
}

// adoptOperationGate takes the managed operation lock; tests replace it.
var adoptOperationGate = func() (io.Closer, error) {
	gate, err := acquireManagedOperationGate()
	if err != nil {
		return nil, err
	}
	gate.describe("Use restored lab")
	return gate, nil
}

// renameNew moves a folder without ever replacing an existing one.
func renameNew(from, to string) error {
	return unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
}
