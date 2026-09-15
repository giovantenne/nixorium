package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/giovantenne/nixorium/internal/domain"
)

const softwareFileName = "lab-software.json"

const softwareCandidateValidationExpression = `
let
  deployment = builtins.getFlake (builtins.getEnv "NIXORIUM_DEPLOYMENT_FLAKE");
  candidate = builtins.fromJSON (builtins.readFile (builtins.getEnv "NIXORIUM_SOFTWARE_CANDIDATE_FILE"));
in deployment.nixoriumValidateSoftwareCandidate candidate
`

func (Local) SoftwareDefinition(ctx context.Context, repository string) (domain.SoftwareDefinition, error) {
	var definition domain.SoftwareDefinition
	err := nixJSON(ctx, repository, "nixoriumSoftware", &definition)
	return definition, err
}

func (Local) ReadSoftware(repository string) ([]byte, error) {
	content, _, err := readRegularFileNoFollowLimit(filepath.Join(repository, softwareFileName), 1024*1024)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", softwareFileName, err)
	}
	return content, nil
}

func (Local) ValidateSoftwareCandidate(ctx context.Context, repository string, software domain.LabSoftwareFile) error {
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return err
	}
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return err
	}
	data, err := domain.MarshalLabSoftware(software)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp("", "nixorium-software-candidate-*.json")
	if err != nil {
		return fmt.Errorf("create software candidate: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure software candidate: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write software candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync software candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close software candidate: %w", err)
	}
	command := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval", "--impure", "--json", "--expr", softwareCandidateValidationExpression)
	command.Env = append(os.Environ(), "NIXORIUM_DEPLOYMENT_FLAKE="+flake, "NIXORIUM_SOFTWARE_CANDIDATE_FILE="+path)
	output := &boundedCommandBuffer{limit: 64 * 1024}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		detail := sanitizeOperationLog(output.buffer.Bytes())
		if output.truncated {
			detail += "\n(output truncated)"
		}
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("software candidate evaluation failed: %s", detail)
	}
	return nil
}

func (Local) WriteSoftwareIfUnchanged(repository string, expected []byte, software domain.LabSoftwareFile) error {
	data, err := domain.MarshalLabSoftware(software)
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
	target := filepath.Join(repository, softwareFileName)
	current, _, readErr := readRegularFileNoFollowLimit(target, 1024*1024)
	if readErr != nil || !bytes.Equal(current, expected) {
		return domain.ErrSoftwareConflict
	}
	if info, statErr := os.Lstat(target); statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		if statErr != nil {
			return fmt.Errorf("inspect %s: %w", softwareFileName, statErr)
		}
		return errors.New("lab-software.json must be a regular file and not a symlink")
	}
	temporary, err := os.CreateTemp(repository, ".lab-software.json.*")
	if err != nil {
		return fmt.Errorf("create software draft: %w", err)
	}
	temporaryPath := temporary.Name()
	keep := true
	defer func() {
		if keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure software draft: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write software draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync software draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close software draft: %w", err)
	}
	current, _, readErr = readRegularFileNoFollowLimit(target, 1024*1024)
	if readErr != nil || !bytes.Equal(current, expected) {
		return domain.ErrSoftwareConflict
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace lab-software.json: %w", err)
	}
	keep = false
	if err := root.Sync(); err != nil {
		return fmt.Errorf("%w: sync deployment root: %v", domain.ErrSoftwareDurability, err)
	}
	return nil
}
