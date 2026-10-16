package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

var githubFlakeURL = regexp.MustCompile(`^github:([^/?#]+)/([^/?#]+)(?:/([^?#]+))?$`)

// flakeLockOriginalMismatches compares the declared URL of each managed input
// in flake.nix with the "original" reference recorded for it in flake.lock.
// When they differ, Nix treats the lock as outdated: an immutable store copy
// of the deployment then cannot be evaluated, which is what Colmena uses.
// Only the github form that managed updates write is compared; other forms
// are left to Nix.
func flakeLockOriginalMismatches(flake, lock []byte) ([]string, error) {
	if len(lock) == 0 {
		return nil, nil
	}
	mismatches := []string{}
	for _, input := range []struct {
		name    string
		pattern *regexp.Regexp
	}{{"nixorium", managedNixoriumInput}, {"nixpkgs", managedPackageBaseInput}} {
		matches := input.pattern.FindAllSubmatch(flake, -1)
		if len(matches) != 1 {
			continue
		}
		declared := string(matches[0][2])
		parts := githubFlakeURL.FindStringSubmatch(declared)
		if parts == nil {
			continue
		}
		node, present, err := directRootInputNode(lock, input.name)
		if err != nil || !present {
			// A follows edge or an unusual graph is left to Nix itself.
			continue
		}
		var recorded struct {
			Original struct {
				Type  string `json:"type"`
				Owner string `json:"owner"`
				Repo  string `json:"repo"`
				Ref   string `json:"ref"`
				Rev   string `json:"rev"`
			} `json:"original"`
		}
		if err := json.Unmarshal(node, &recorded); err != nil {
			return nil, fmt.Errorf("decode %s lock node: %w", input.name, err)
		}
		original := recorded.Original
		if original.Type == "" {
			// No recorded original: nothing to compare; Nix decides.
			continue
		}
		if original.Type == "github" && original.Owner == parts[1] && original.Repo == parts[2] && original.Rev == "" && original.Ref == parts[3] {
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("%s: flake.nix declares %s but flake.lock records %s", input.name, declared, describeLockOriginal(original.Type, original.Owner, original.Repo, original.Ref, original.Rev)))
	}
	return mismatches, nil
}

func describeLockOriginal(kind, owner, repo, ref, rev string) string {
	if kind != "github" {
		return "a " + kind + " source"
	}
	url := "github:" + owner + "/" + repo
	if ref != "" {
		url += "/" + ref
	}
	if rev != "" {
		url += "/" + rev
	}
	return url
}

// FlakeLockConsistency reports a deployment whose flake.lock no longer
// matches flake.nix for the managed nixorium or nixpkgs input. It reads only
// the two files and needs no network.
func (Local) FlakeLockConsistency(repository string) error {
	flake, _, err := readRegularFileNoFollowLimit(filepath.Join(repository, "flake.nix"), 1024*1024)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read flake.nix: %w", err)
	}
	lock, _, err := readRegularFileNoFollowLimit(filepath.Join(repository, "flake.lock"), 4*1024*1024)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read flake.lock: %w", err)
	}
	mismatches, err := flakeLockOriginalMismatches(flake, lock)
	if err != nil {
		return err
	}
	if len(mismatches) == 0 {
		return nil
	}
	return domain.Blocked("PIN-OUTDATED", fmt.Errorf("flake.lock does not match flake.nix (%s); %s", strings.Join(mismatches, "; "), flakeLockRepairAdvice(mismatches)))
}

func flakeLockRepairAdvice(mismatches []string) string {
	for _, mismatch := range mismatches {
		if strings.HasPrefix(mismatch, "nixpkgs:") {
			return "in the deployment folder run `nix flake lock`, then save flake.lock in Maintenance → Review Git changes (`nixorium git review`) and apply the controller"
		}
	}
	return "repair it in Maintenance → Update Nixorium by selecting the configured version again (`nixorium update plan --target <configured version>`), or run `nix flake lock` in the deployment folder and save flake.lock in Maintenance → Review Git changes"
}
