package adapters

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

const maximumDeploymentHistoryBytes = int64(1024 * 1024)

func (Local) DeploymentHistory(repository string) (domain.DeploymentHistory, error) {
	stateRoot, err := userStateRoot()
	if err != nil {
		return domain.DeploymentHistory{}, err
	}
	return readDeploymentHistory(stateRoot, repository)
}

func (Local) RecordSuccessfulDeployments(repository string, updates map[string]domain.LastSuccessfulDeployment) error {
	stateRoot, err := userStateRoot()
	if err != nil {
		return err
	}
	return writeDeploymentHistory(stateRoot, repository, updates)
}

func userStateRoot() (string, error) {
	stateRoot := os.Getenv("XDG_STATE_HOME")
	if stateRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home for Nixorium state: %w", err)
		}
		stateRoot = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(stateRoot) {
		return "", errors.New("Nixorium state directory must be absolute")
	}
	return stateRoot, nil
}

func deploymentHistoryLocation(stateRoot, repository string) (string, string, error) {
	if !filepath.IsAbs(stateRoot) {
		return "", "", errors.New("operation state directory must be absolute")
	}
	if !filepath.IsAbs(repository) || filepath.Clean(repository) != repository {
		return "", "", errors.New("deployment repository must be an absolute clean path")
	}
	digest := sha256.Sum256([]byte(repository))
	directory := filepath.Join(stateRoot, "nixorium", "deployments")
	return directory, filepath.Join(directory, hex.EncodeToString(digest[:])+".json"), nil
}

func readDeploymentHistory(stateRoot, repository string) (domain.DeploymentHistory, error) {
	history := domain.DeploymentHistory{
		SchemaVersion: domain.SchemaVersion,
		Repository:    repository,
		Hosts:         map[string]domain.LastSuccessfulDeployment{},
	}
	directory, path, err := deploymentHistoryLocation(stateRoot, repository)
	if err != nil {
		return domain.DeploymentHistory{}, err
	}
	if _, statErr := os.Lstat(directory); statErr == nil {
		if err := checkPrivateDirectory(filepath.Dir(directory)); err != nil {
			return domain.DeploymentHistory{}, fmt.Errorf("inspect Nixorium state directory: %w", err)
		}
		if err := checkPrivateDirectory(directory); err != nil {
			return domain.DeploymentHistory{}, fmt.Errorf("inspect deployment history directory: %w", err)
		}
	} else if !os.IsNotExist(statErr) {
		return domain.DeploymentHistory{}, fmt.Errorf("inspect deployment history directory: %w", statErr)
	}
	content, mode, err := readRegularFileNoFollowLimit(path, maximumDeploymentHistoryBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return history, nil
		}
		return domain.DeploymentHistory{}, fmt.Errorf("read deployment history: %w", err)
	}
	if mode != 0600 {
		return domain.DeploymentHistory{}, fmt.Errorf("deployment history mode is %04o, want 0600", mode)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil {
		return domain.DeploymentHistory{}, fmt.Errorf("decode deployment history: %w", err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return domain.DeploymentHistory{}, fmt.Errorf("decode deployment history: %w", err)
	}
	if history.SchemaVersion != domain.SchemaVersion {
		return domain.DeploymentHistory{}, fmt.Errorf("unsupported deployment history schema %d", history.SchemaVersion)
	}
	if history.Repository != repository {
		return domain.DeploymentHistory{}, errors.New("deployment history repository does not match its path")
	}
	if history.Hosts == nil {
		return domain.DeploymentHistory{}, errors.New("deployment history hosts must be an object")
	}
	for name, record := range history.Hosts {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\\r\n\t") {
			return domain.DeploymentHistory{}, fmt.Errorf("deployment history contains invalid host %q", name)
		}
		if !validGitRevision(record.Revision) || !validSystemPath(record.SystemPath) || record.VerifiedAt.IsZero() {
			return domain.DeploymentHistory{}, fmt.Errorf("deployment history contains invalid state for %s", name)
		}
		record.VerifiedAt = record.VerifiedAt.UTC()
		history.Hosts[name] = record
	}
	return history, nil
}

func checkPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("must be a directory and not a symlink")
	}
	if info.Mode().Perm() != 0700 {
		return fmt.Errorf("mode is %04o, want 0700", info.Mode().Perm())
	}
	return nil
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func writeDeploymentHistory(stateRoot, repository string, updates map[string]domain.LastSuccessfulDeployment) error {
	if len(updates) == 0 {
		return nil
	}
	history, err := readDeploymentHistory(stateRoot, repository)
	if err != nil {
		return err
	}
	for name, record := range updates {
		if strings.TrimSpace(name) == "" || !validGitRevision(record.Revision) || !validSystemPath(record.SystemPath) || record.VerifiedAt.IsZero() {
			return fmt.Errorf("refuse invalid successful deployment record for %q", name)
		}
		record.VerifiedAt = record.VerifiedAt.UTC()
		history.Hosts[name] = record
	}
	content, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("encode deployment history: %w", err)
	}
	content = append(content, '\n')

	directory, target, err := deploymentHistoryLocation(stateRoot, repository)
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
		return fmt.Errorf("create deployment history directory: %w", err)
	}
	if err := requirePrivateDirectory(directory); err != nil {
		return err
	}
	if info, statErr := os.Lstat(target); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("deployment history must be a regular file and not a symlink")
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect deployment history: %w", statErr)
	}

	temporary, err := os.CreateTemp(directory, ".deployment-history.*")
	if err != nil {
		return fmt.Errorf("create deployment history draft: %w", err)
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
		return fmt.Errorf("secure deployment history draft: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write deployment history draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync deployment history draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close deployment history draft: %w", err)
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace deployment history: %w", err)
	}
	keepTemporary = false
	directoryDescriptor, err := syscall.Open(directory, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open deployment history directory: %w", err)
	}
	directoryFile := os.NewFile(uintptr(directoryDescriptor), directory)
	defer directoryFile.Close()
	if err := directoryFile.Sync(); err != nil {
		return fmt.Errorf("sync deployment history directory: %w", err)
	}
	return nil
}
