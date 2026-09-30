package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Operator terms for what a controller application changes, in display order.
const (
	controllerChangeSettings = "Saved settings"
	controllerChangeSoftware = "Software selection"
	controllerChangeStudent  = "Student preferences"
	controllerChangeNixorium = "Nixorium version"
	controllerChangePackages = "System and packages"
	controllerChangeFlake    = "Deployment definition (flake.nix)"
	controllerChangeOther    = "Local modules, assets and other files"
)

var controllerChangeOrder = []string{
	controllerChangeSettings, controllerChangeSoftware, controllerChangeStudent,
	controllerChangeNixorium, controllerChangePackages, controllerChangeFlake, controllerChangeOther,
}

// recordedControllerRevision reads the revision of the last successful
// controller activation. An absent or invalid record yields "".
func recordedControllerRevision() string {
	data, mode, err := readRegularFileNoFollowLimit(controllerActivationPath, maximumControllerActivationBytes)
	if err != nil || mode != 0o644 {
		return ""
	}
	record, err := domain.DecodeControllerActivation(data)
	if err != nil {
		return ""
	}
	return record.Revision
}

// controllerChanges compares Git objects only: the revision this controller
// last activated successfully and the reviewed revision. known is false when
// either revision is missing or unreadable, never a guess.
func controllerChanges(ctx context.Context, repository, applied, reviewed string) ([]string, bool) {
	if !fullGitObjectIDPattern.MatchString(applied) || !fullGitObjectIDPattern.MatchString(reviewed) {
		return nil, false
	}
	if applied == reviewed {
		return []string{}, true
	}
	environment := workspaceEnvironment()
	names, truncated, err := runBoundedGitWithEnvironment(ctx, repository, environment, 1024*1024,
		"-c", "core.quotePath=false", "diff", "--no-renames", "--name-only", "-z", applied, reviewed, "--")
	if err != nil || truncated {
		return nil, false
	}
	found := map[string]bool{}
	for _, name := range strings.Split(strings.TrimSuffix(names, "\x00"), "\x00") {
		switch {
		case name == "":
		case name == "lab-settings.json" || name == "lab-config.nix":
			found[controllerChangeSettings] = true
		case name == "lab-software.json":
			found[controllerChangeSoftware] = true
		case name == domain.WorkspaceFileName || name == "workspace-catalog.nix":
			found[controllerChangeStudent] = true
		case name == "flake.nix":
			found[controllerChangeFlake] = true
		case name == "flake.lock":
			categories, ok := controllerLockChanges(ctx, repository, applied, reviewed)
			if !ok {
				return nil, false
			}
			for _, category := range categories {
				found[category] = true
			}
		default:
			found[controllerChangeOther] = true
		}
	}
	result := []string{}
	for _, category := range controllerChangeOrder {
		if found[category] {
			result = append(result, category)
		}
	}
	return result, true
}

// controllerLockChanges separates the Nixorium input from every other locked
// input, which together determine the operating system and packages.
func controllerLockChanges(ctx context.Context, repository, applied, reviewed string) ([]string, bool) {
	type lock struct {
		Root  string                     `json:"root"`
		Nodes map[string]json.RawMessage `json:"nodes"`
	}
	read := func(revision string) (lock, bool) {
		var result lock
		data, truncated, err := runBoundedGitWithEnvironment(ctx, repository, workspaceEnvironment(), 4*1024*1024,
			"show", revision+":flake.lock")
		if err != nil || truncated || json.Unmarshal([]byte(data), &result) != nil || result.Nodes == nil {
			return result, false
		}
		return result, true
	}
	before, ok := read(applied)
	if !ok {
		return nil, false
	}
	after, ok := read(reviewed)
	if !ok {
		return nil, false
	}
	nixoriumNode := func(value lock) string {
		var root struct {
			Inputs map[string]any `json:"inputs"`
		}
		if json.Unmarshal(value.Nodes[value.Root], &root) != nil {
			return ""
		}
		name, _ := root.Inputs["nixorium"].(string)
		return name
	}
	beforeName, afterName := nixoriumNode(before), nixoriumNode(after)
	categories := []string{}
	if beforeName == "" || afterName == "" || !bytes.Equal(before.Nodes[beforeName], after.Nodes[afterName]) {
		categories = append(categories, controllerChangeNixorium)
	}
	for name, node := range after.Nodes {
		if name != afterName && name != after.Root && !bytes.Equal(node, before.Nodes[name]) {
			categories = append(categories, controllerChangePackages)
			break
		}
	}
	if len(before.Nodes) != len(after.Nodes) && len(categories) == 0 {
		categories = append(categories, controllerChangePackages)
	}
	return categories, true
}
