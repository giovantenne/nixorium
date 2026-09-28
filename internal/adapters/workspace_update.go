package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const workspaceUpdateProjection = `f: let w = f.nixoriumWorkspace or null; in if w == null then null else assert f ? nixoriumValidateWorkspaceCandidate && f.nixoriumValidateWorkspaceCandidate (builtins.toJSON w.declared) == true; w`

func readUpdateWorkspace(ctx context.Context, flake string, common []string) (*domain.WorkspaceResolution, error) {
	args := []string{"eval", "--json",
		"--no-update-lock-file", "--option", "allow-import-from-derivation", "false", "--option", "accept-flake-config", "false"}
	if len(common) == 0 {
		// CLI overrides apply to flake installables, not builtins.getFlake.
		// Only the current pin uses getFlake to tolerate legacy missing output.
		args = append(args, "--impure", "--no-write-lock-file", "--expr", "("+workspaceUpdateProjection+") (builtins.getFlake "+workspaceUpdateNixString(flake)+")")
	} else {
		args = append(args, flake+"#nixoriumWorkspace")
	}
	output, err := runBoundedNix(ctx, 1024*1024, append(args, common...)...)
	if err != nil {
		// A deployment hook can print secrets. Do not expose arbitrary evaluator
		// output while explaining a workspace capability/prerequisite failure.
		return nil, errors.New("workspace update review failed; check the pinned workspace output, validation hook and prerequisites")
	}
	var resolved *domain.WorkspaceResolution
	if err := json.Unmarshal([]byte(output), &resolved); err != nil {
		return nil, errors.New("invalid workspace update metadata")
	}
	if resolved != nil {
		if err := domain.ValidateWorkspaceResolution(*resolved); err != nil {
			return nil, err
		}
		if len(common) != 0 {
			data, _ := domain.MarshalWorkspaceProfile(resolved.Declared)
			// The strict profile schema contains only fixed names, ASCII IDs,
			// enums, booleans and integers; no arbitrary user Nix expression.
			check := []string{"eval", flake + "#nixoriumValidateWorkspaceCandidate", "--json", "--apply", "validator: validator " + workspaceUpdateNixString(string(data)),
				"--no-update-lock-file", "--option", "allow-import-from-derivation", "false", "--option", "accept-flake-config", "false"}
			validation, err := runBoundedNix(ctx, 1024, append(check, common...)...)
			if err != nil || strings.TrimSpace(validation) != "true" {
				return nil, errors.New("candidate workspace validation hook rejected the update")
			}
		}
	}
	return resolved, nil
}

func workspaceUpdateNixString(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "${", `\${`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(value) + `"`
}

func inspectWorkspaceUpdate(ctx context.Context, flake string, common []string) (*domain.WorkspaceUpdateImpact, error) {
	current, err := readUpdateWorkspace(ctx, flake, nil)
	if err != nil {
		return nil, err
	}
	if current == nil {
		// Legacy deployments have no prepared profile to compare. Keep their
		// existing update workflow; do not invent a workspace or migrate it.
		return nil, nil
	}
	proposed, err := readUpdateWorkspace(ctx, flake, common)
	if err != nil {
		return nil, err
	}
	// An input update is not permission to enable/disable reset or change the
	// student/destinations. Such changes require a separate migration review.
	if proposed == nil || current.RuntimeEnabled != proposed.RuntimeEnabled ||
		current.StudentUser != proposed.StudentUser || !reflect.DeepEqual(current.Targets, proposed.Targets) {
		return nil, errors.New("workspace capability, runtime opt-in or destinations changed; review a separate migration before updating")
	}
	return &domain.WorkspaceUpdateImpact{Current: current, Proposed: proposed}, nil
}
