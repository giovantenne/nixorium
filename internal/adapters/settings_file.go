package adapters

import (
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
	data, err := domain.MarshalLabSettings(settings)
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(repository)
	if err != nil {
		return fmt.Errorf("inspect deployment root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("deployment root must be a directory and not a symlink")
	}
	target := filepath.Join(repository, settingsFileName)
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
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace lab-settings.json: %w", err)
	}
	keepTemporary = false
	directory, err := os.Open(repository)
	if err != nil {
		return fmt.Errorf("open deployment root for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
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
