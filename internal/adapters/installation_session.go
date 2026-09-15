package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

const maximumInstallationSessionBytes = int64(256 * 1024)

func (Local) InstallationSession(repository string) (domain.InstallationSessionRecord, bool, error) {
	stateRoot, err := userStateRoot()
	if err != nil {
		return domain.InstallationSessionRecord{}, false, err
	}
	return readInstallationSession(stateRoot, repository)
}

func (Local) WriteInstallationSession(repository string, record domain.InstallationSessionRecord) error {
	stateRoot, err := userStateRoot()
	if err != nil {
		return err
	}
	return writeInstallationSession(stateRoot, repository, record)
}

func installationSessionLocation(stateRoot, repository string) (string, string, error) {
	if !filepath.IsAbs(stateRoot) {
		return "", "", errors.New("operation state directory must be absolute")
	}
	if !filepath.IsAbs(repository) || filepath.Clean(repository) != repository {
		return "", "", errors.New("deployment repository must be an absolute clean path")
	}
	digest := sha256.Sum256([]byte(repository))
	directory := filepath.Join(stateRoot, "nixorium", "installation-sessions")
	return directory, filepath.Join(directory, hex.EncodeToString(digest[:])+".json"), nil
}

func readInstallationSession(stateRoot, repository string) (domain.InstallationSessionRecord, bool, error) {
	directory, path, err := installationSessionLocation(stateRoot, repository)
	if err != nil {
		return domain.InstallationSessionRecord{}, false, err
	}
	if _, statErr := os.Lstat(directory); statErr == nil {
		if err := checkPrivateDirectory(filepath.Dir(directory)); err != nil {
			return domain.InstallationSessionRecord{}, false, fmt.Errorf("inspect Nixorium state directory: %w", err)
		}
		if err := checkPrivateDirectory(directory); err != nil {
			return domain.InstallationSessionRecord{}, false, fmt.Errorf("inspect installation session directory: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return domain.InstallationSessionRecord{}, false, fmt.Errorf("inspect installation session directory: %w", statErr)
	}
	content, mode, err := readRegularFileNoFollowLimit(path, maximumInstallationSessionBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.InstallationSessionRecord{}, false, nil
		}
		return domain.InstallationSessionRecord{}, false, fmt.Errorf("read installation session: %w", err)
	}
	if mode != 0600 {
		return domain.InstallationSessionRecord{}, false, fmt.Errorf("installation session mode is %04o, want 0600", mode)
	}
	record, err := domain.DecodeInstallationSession(content)
	if err != nil {
		return domain.InstallationSessionRecord{}, false, err
	}
	if record.Repository != repository {
		return domain.InstallationSessionRecord{}, false, errors.New("installation session repository does not match its path")
	}
	return record, true, nil
}

func writeInstallationSession(stateRoot, repository string, record domain.InstallationSessionRecord) error {
	directory, target, err := installationSessionLocation(stateRoot, repository)
	if err != nil {
		return err
	}
	if record.Repository != repository {
		return errors.New("installation session repository does not match write target")
	}
	content, err := domain.MarshalInstallationSession(record)
	if err != nil {
		return err
	}
	productDirectory := filepath.Dir(directory)
	if err := os.MkdirAll(productDirectory, 0700); err != nil {
		return fmt.Errorf("create Nixorium state directory: %w", err)
	}
	if err := requirePrivateDirectory(productDirectory); err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create installation session directory: %w", err)
	}
	if err := requirePrivateDirectory(directory); err != nil {
		return err
	}
	if info, statErr := os.Lstat(target); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("installation session must be a regular file and not a symlink")
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect installation session: %w", statErr)
	}

	temporary, err := os.CreateTemp(directory, ".installation-session.*")
	if err != nil {
		return fmt.Errorf("create installation session draft: %w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure installation session draft: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write installation session draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync installation session draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close installation session draft: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace installation session: %w", err)
	}
	keepTemporary = false
	directoryDescriptor, err := syscall.Open(directory, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open installation session directory: %w", err)
	}
	directoryFile := os.NewFile(uintptr(directoryDescriptor), directory)
	defer directoryFile.Close()
	if err := directoryFile.Sync(); err != nil {
		return fmt.Errorf("sync installation session directory: %w", err)
	}
	return nil
}
