package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	controllerActivationPath         = "/var/lib/nixorium/controller/applied.json"
	maximumControllerActivationBytes = 16 * 1024
)

func (local Local) ControllerApplied(ctx context.Context, repository string) (bool, string) {
	applied, detail, err := local.ControllerState(ctx, repository)
	if err != nil {
		return false, err.Error()
	}
	return applied, detail
}

func (local Local) ControllerState(ctx context.Context, repository string) (bool, string, error) {
	meta, err := local.LabMeta(ctx, repository)
	if err != nil {
		return false, "", fmt.Errorf("cannot evaluate controller identity: %w", err)
	}
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return false, "", fmt.Errorf("cannot resolve deployment flake: %w", err)
	}
	reference := fmt.Sprintf("%s#nixosConfigurations.%s.config.system.build.toplevel", flake, meta.Controller.Name)
	desired, err := runOutput(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval", reference, "--raw", "--no-write-lock-file")
	if err != nil {
		return false, "", fmt.Errorf("cannot evaluate desired controller generation: %w", err)
	}
	desired = strings.TrimSpace(desired)
	active, err := filepath.EvalSymlinks("/run/current-system")
	if err != nil {
		if os.IsNotExist(err) {
			return false, "no active NixOS system generation was found", nil
		}
		return false, "", fmt.Errorf("cannot inspect active system generation: %w", err)
	}
	if active == desired {
		revision, revisionErr := local.GitRevision(ctx, repository)
		if revisionErr != nil {
			return false, "", fmt.Errorf("cannot resolve reviewed deployment revision: %w", revisionErr)
		}
		data, mode, recordErr := readRegularFileNoFollowLimit(controllerActivationPath, maximumControllerActivationBytes)
		if recordErr != nil {
			if os.IsNotExist(recordErr) {
				return false, "active system matches, but no successful controller activation is recorded", nil
			}
			return false, fmt.Sprintf("active system matches, but the controller activation record is unreadable: %v", recordErr), nil
		}
		current, detail := controllerActivationMatches(data, mode, revision, desired)
		return current, detail, nil
	}
	return false, "reviewed controller configuration is not the active system generation", nil
}

func controllerActivationMatches(data []byte, mode uint32, revision, desired string) (bool, string) {
	if mode != 0o644 {
		return false, fmt.Sprintf("active system matches, but the controller activation record has unsafe mode %04o", mode)
	}
	record, err := domain.DecodeControllerActivation(data)
	if err != nil {
		return false, fmt.Sprintf("active system matches, but the controller activation record is invalid: %v", err)
	}
	if record.Revision != revision {
		return false, "active system matches, but its successful activation record belongs to another deployment revision"
	}
	if record.SystemPath != desired {
		return false, "active system matches, but its successful activation record names another system generation"
	}
	return true, "active system and successful activation record match the reviewed controller configuration"
}
