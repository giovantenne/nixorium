package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		return true, "active system matches the reviewed controller configuration", nil
	}
	return false, "reviewed controller configuration is not the active system generation", nil
}
