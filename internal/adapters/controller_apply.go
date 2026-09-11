package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (local Local) ControllerApplied(ctx context.Context, repository string) (bool, string) {
	meta, err := local.LabMeta(ctx, repository)
	if err != nil {
		return false, fmt.Sprintf("cannot evaluate controller identity: %v", err)
	}
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return false, fmt.Sprintf("cannot resolve deployment flake: %v", err)
	}
	reference := fmt.Sprintf("%s#nixosConfigurations.%s.config.system.build.toplevel", flake, meta.Controller.Name)
	desired, err := runOutput(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval", reference, "--raw", "--no-write-lock-file")
	if err != nil {
		return false, fmt.Sprintf("cannot evaluate desired controller generation: %v", err)
	}
	desired = strings.TrimSpace(desired)
	active, err := filepath.EvalSymlinks("/run/current-system")
	if err != nil {
		if os.IsNotExist(err) {
			return false, "no active NixOS system generation was found"
		}
		return false, fmt.Sprintf("cannot inspect active system generation: %v", err)
	}
	if active == desired {
		return true, "active system matches the reviewed controller configuration"
	}
	return false, "reviewed controller configuration is not the active system generation"
}
