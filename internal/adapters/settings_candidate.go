package adapters

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/giovantenne/nixorium/internal/domain"
)

const candidateValidationExpression = `
let
  deployment = builtins.getFlake (builtins.getEnv "NIXORIUM_DEPLOYMENT_FLAKE");
  candidate = builtins.fromJSON (builtins.readFile (builtins.getEnv "NIXORIUM_CANDIDATE_FILE"));
in deployment.nixoriumValidateCandidate candidate
`

func (Local) ValidateCandidate(ctx context.Context, repository string, settings domain.LabSettingsFile) error {
	data, err := domain.MarshalLabSettings(settings)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp("", "nixorium-settings-candidate-*.json")
	if err != nil {
		return fmt.Errorf("create candidate file: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure candidate file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write candidate file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync candidate file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close candidate file: %w", err)
	}

	command := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval", "--impure", "--json", "--expr", candidateValidationExpression)
	command.Env = append(os.Environ(),
		"NIXORIUM_DEPLOYMENT_FLAKE=path:"+repository,
		"NIXORIUM_CANDIDATE_FILE="+path,
	)
	if _, err := command.Output(); err != nil {
		return fmt.Errorf("candidate evaluation failed: %w", err)
	}
	return nil
}
