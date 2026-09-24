package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

type RemoteInstallStateStore struct {
	directory string
}

func NewRemoteInstallStateStore(directory string) (*RemoteInstallStateStore, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("remote installation state directory must be absolute")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, fmt.Errorf("inspect remote installation state directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		return nil, errors.New("remote installation state directory must be a private real directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("remote installation state directory belongs to another user")
	}
	return &RemoteInstallStateStore{directory: directory}, nil
}

func (store *RemoteInstallStateStore) Load(operationID string) (domain.RemoteInstallSession, error) {
	var session domain.RemoteInstallSession
	if !remoteStateID(operationID) {
		return session, errors.New("remote installation state ID is invalid")
	}
	directory, err := openRemoteStateDirectory(store.directory)
	if err != nil {
		return session, err
	}
	defer directory.Close()
	descriptor, err := syscall.Openat(int(directory.Fd()), operationID+".json", syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return session, fmt.Errorf("open remote installation state: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), operationID+".json")
	defer file.Close()
	if err := validatePrivateOwnedFile(file, syscall.S_IFREG, 0600); err != nil {
		return session, fmt.Errorf("inspect remote installation state: %w", err)
	}
	info, err := file.Stat()
	if err != nil || info.Size() < 2 || info.Size() > domain.RemoteInstallFactsMaxBytes {
		return session, errors.New("remote installation state has an invalid size")
	}
	data, err := io.ReadAll(io.LimitReader(file, domain.RemoteInstallFactsMaxBytes+1))
	if err != nil {
		return session, fmt.Errorf("read remote installation state: %w", err)
	}
	session, err = domain.DecodeRemoteInstallSession(data)
	if err != nil {
		return session, err
	}
	if session.OperationID != operationID {
		return session, errors.New("remote installation state filename and identity differ")
	}
	return session, nil
}

func openRemoteStateDirectory(path string) (*os.File, error) {
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open remote installation state directory: %w", err)
	}
	directory := os.NewFile(uintptr(descriptor), path)
	if err := validatePrivateOwnedFile(directory, syscall.S_IFDIR, 0700); err != nil {
		directory.Close()
		return nil, fmt.Errorf("inspect remote installation state directory: %w", err)
	}
	return directory, nil
}

func (store *RemoteInstallStateStore) Save(session domain.RemoteInstallSession) error {
	if !remoteStateID(session.OperationID) {
		return errors.New("remote installation state ID is invalid")
	}
	content, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode remote installation state: %w", err)
	}
	content = append(content, '\n')
	if len(content) > domain.RemoteInstallFactsMaxBytes {
		return errors.New("remote installation state exceeds the size limit")
	}
	if _, err := domain.DecodeRemoteInstallSession(content); err != nil {
		return err
	}
	target := filepath.Join(store.directory, session.OperationID+".json")
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0600 {
			return errors.New("existing remote installation state is unsafe")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.CreateTemp(store.directory, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create remote installation state: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("publish remote installation state: %w", err)
	}
	committed = true
	directory, err := os.Open(store.directory)
	if err != nil {
		return err
	}
	err = directory.Sync()
	_ = directory.Close()
	return err
}

func remoteStateID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
