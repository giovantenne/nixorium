package adapters

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

const settingsFileName = "lab-settings.json"

func (Local) ReadSettings(repository string) ([]byte, error) {
	path := filepath.Join(repository, settingsFileName)
	content, _, err := readRegularFileNoFollowLimit(path, 1024*1024)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", settingsFileName, err)
	}
	return content, nil
}

func (Local) WriteSettings(repository string, settings domain.LabSettingsFile) error {
	return writeSettings(repository, nil, settings)
}

func (Local) WriteSettingsIfUnchanged(repository string, expected []byte, settings domain.LabSettingsFile) error {
	return writeSettings(repository, expected, settings)
}

func writeSettings(repository string, expected []byte, settings domain.LabSettingsFile) error {
	data, err := domain.MarshalLabSettings(settings)
	if err != nil {
		return err
	}
	rootDescriptor, err := syscall.Open(repository, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open deployment root: %w", err)
	}
	root := os.NewFile(uintptr(rootDescriptor), repository)
	defer root.Close()
	if err := syscall.Flock(rootDescriptor, syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock deployment root: %w", err)
	}
	defer syscall.Flock(rootDescriptor, syscall.LOCK_UN)

	target := filepath.Join(repository, settingsFileName)
	if expected != nil {
		current, _, readErr := readRegularFileNoFollowLimit(target, 1024*1024)
		if readErr != nil || !bytes.Equal(current, expected) {
			return domain.ErrSettingsConflict
		}
	}
	if targetInfo, statErr := os.Lstat(target); statErr == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
			return errors.New("lab-settings.json must be a regular file and not a symlink")
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect lab-settings.json: %w", statErr)
	}

	temporary, err := os.CreateTemp(repository, ".lab-settings.json.*")
	if err != nil {
		return fmt.Errorf("create settings draft: %w", err)
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
		return fmt.Errorf("secure settings draft: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write settings draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync settings draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close settings draft: %w", err)
	}
	if expected != nil {
		current, _, readErr := readRegularFileNoFollowLimit(target, 1024*1024)
		if readErr != nil || !bytes.Equal(current, expected) {
			return domain.ErrSettingsConflict
		}
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace lab-settings.json: %w", err)
	}
	keepTemporary = false
	if err := root.Sync(); err != nil {
		return fmt.Errorf("sync deployment root: %w", err)
	}
	return nil
}

func readRegularFileNoFollowLimit(path string, maximumBytes int64) ([]byte, uint32, error) {
	fileDescriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, 0, err
	}
	file := os.NewFile(uintptr(fileDescriptor), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errors.New("not a regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(content)) > maximumBytes {
		return nil, 0, errors.New("file is unexpectedly large")
	}
	return content, uint32(info.Mode().Perm()), nil
}
