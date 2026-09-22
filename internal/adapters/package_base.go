package adapters

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

// PackageBase uses the same locked writer as framework updates, but owns only
// nixpkgs. It never refreshes the framework or an unrelated private input.
type PackageBase struct{ Local }

var managedPackageBaseInput = regexp.MustCompile(`(?m)^((?:[ \t]*inputs\.nixpkgs| {4}nixpkgs)\.url[ \t]*=[ \t]*")([^"\r\n]+)("[ \t]*;[ \t]*)$`)
var stablePackageBaseURL = regexp.MustCompile(`^github:NixOS/nixpkgs/(nixos-[0-9]{2}\.(?:05|11))$`)
var packageBaseFollows = regexp.MustCompile(`(?m)^[ \t]*(?:inputs\.)?nixorium\.inputs\.nixpkgs\.follows[ \t]*=[ \t]*"nixpkgs"[ \t]*;[ \t]*$`)

type packageBaseNode struct {
	Locked   struct{ Type, Owner, Repo, Rev, NarHash string } `json:"locked"`
	Original struct{ Type, Owner, Repo, Ref string }          `json:"original"`
}

func (PackageBase) InspectUpdateInput(repository string) (domain.UpdateInputSnapshot, error) {
	s, err := (Local{}).InspectUpdateInput(repository)
	if err != nil {
		return s, err
	}
	return inspectPackageBase(s)
}

func inspectPackageBase(s domain.UpdateInputSnapshot) (domain.UpdateInputSnapshot, error) {
	matches := managedPackageBaseInput.FindAllSubmatch(s.FlakeContent, -1)
	if len(matches) != 1 || !packageBaseFollows.Match(s.FlakeContent) {
		return s, errors.New("a direct nixpkgs URL and nixorium.inputs.nixpkgs.follows = \"nixpkgs\" are required; legacy/custom deployments need an explicit reviewed migration")
	}
	url := string(matches[0][2])
	match := stablePackageBaseURL.FindStringSubmatch(url)
	if match == nil {
		return s, errors.New("managed system updates require github:NixOS/nixpkgs/nixos-YY.MM (05 or 11); custom sources remain a manual operation")
	}
	nodeBytes, present, err := directRootInputNode(s.LockContent, "nixpkgs")
	if err != nil || !present {
		return s, errors.New("a direct locked root nixpkgs input is required; migrate legacy deployments at their exact current revision first")
	}
	var node packageBaseNode
	if err := json.Unmarshal(nodeBytes, &node); err != nil {
		return s, err
	}
	hash, hashErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(node.Locked.NarHash, "sha256-"))
	if node.Locked.Type != "github" || node.Locked.Owner != "NixOS" || node.Locked.Repo != "nixpkgs" ||
		node.Original.Type != "github" || node.Original.Owner != "NixOS" || node.Original.Repo != "nixpkgs" || node.Original.Ref != match[1] ||
		!remoteGitObjectID.MatchString(node.Locked.Rev) || !strings.HasPrefix(node.Locked.NarHash, "sha256-") || hashErr != nil || len(hash) != 32 {
		return s, errors.New("nixpkgs declaration and actual locked source/ref/revision/hash do not agree")
	}
	var doc struct {
		Root  string
		Nodes map[string]struct{ Inputs map[string]json.RawMessage }
	}
	if err := json.Unmarshal(s.LockContent, &doc); err != nil {
		return s, err
	}
	var core string
	if err := json.Unmarshal(doc.Nodes[doc.Root].Inputs["nixorium"], &core); err != nil {
		return s, errors.New("framework must be a direct input")
	}
	var follows []string
	if err := json.Unmarshal(doc.Nodes[core].Inputs["nixpkgs"], &follows); err != nil || len(follows) != 1 || follows[0] != "nixpkgs" {
		return s, errors.New("locked Nixorium must follow the deployment root nixpkgs input")
	}
	s.SourceURL, s.SourcePrefix, s.CurrentRef, s.CurrentRev = url, "NixOS/nixpkgs", match[1], node.Locked.Rev
	return s, nil
}

func proposedPackageBaseFlake(s domain.UpdateInputSnapshot, target string) ([]byte, error) {
	url := "github:NixOS/nixpkgs/" + target
	if !stablePackageBaseURL.MatchString(url) {
		return nil, errors.New("invalid NixOS channel")
	}
	indices := managedPackageBaseInput.FindAllSubmatchIndex(s.FlakeContent, -1)
	if len(indices) != 1 {
		return nil, errors.New("ambiguous nixpkgs declaration")
	}
	i := indices[0]
	out := append([]byte{}, s.FlakeContent[:i[4]]...)
	out = append(out, url...)
	return append(out, s.FlakeContent[i[5]:]...), nil
}

// Compare the entire graph, not only direct root nodes: transitive private
// dependencies and follows edges must also remain unchanged.
func preserveOtherPackageBaseNodes(before, after []byte) error {
	var old, next rawFlakeLockDocument
	if err := json.Unmarshal(before, &old); err != nil {
		return err
	}
	if err := json.Unmarshal(after, &next); err != nil {
		return err
	}
	var root struct{ Inputs map[string]json.RawMessage }
	if err := json.Unmarshal(old.Nodes[old.Root], &root); err != nil {
		return err
	}
	var base string
	if err := json.Unmarshal(root.Inputs["nixpkgs"], &base); err != nil || base == "" || base == old.Root {
		return errors.New("invalid root nixpkgs node")
	}
	if old.Root != next.Root || len(old.Nodes) != len(next.Nodes) {
		return errors.New("system update changed the input graph")
	}
	for name, content := range old.Nodes {
		if name == base {
			var a, b map[string]json.RawMessage
			if json.Unmarshal(content, &a) != nil || json.Unmarshal(next.Nodes[name], &b) != nil {
				return errors.New("invalid updated nixpkgs node")
			}
			delete(a, "locked")
			delete(a, "original")
			delete(b, "locked")
			delete(b, "original")
			ac, _ := json.Marshal(a)
			bc, _ := json.Marshal(b)
			if !bytes.Equal(ac, bc) {
				return errors.New("system update changed nixpkgs input edges or metadata")
			}
			continue
		}
		var a, b any
		if err := json.Unmarshal(content, &a); err != nil {
			return err
		}
		if err := json.Unmarshal(next.Nodes[name], &b); err != nil {
			return fmt.Errorf("system update removed input %s", name)
		}
		ac, _ := json.Marshal(a)
		bc, _ := json.Marshal(b)
		if !bytes.Equal(ac, bc) {
			return fmt.Errorf("system update changed unrelated input %s", name)
		}
	}
	return nil
}

func (p PackageBase) PrepareUpdate(ctx context.Context, repo, target string) (domain.UpdateProposal, error) {
	return p.PrepareUpdateWithProgress(ctx, repo, target, nil)
}

func (p PackageBase) PrepareUpdateWithProgress(ctx context.Context, repo, target string, progress func(domain.UpdatePlanProgress)) (domain.UpdateProposal, error) {
	if err := ensurePrivateFilesUntracked(ctx, repo); err != nil {
		return domain.UpdateProposal{}, err
	}
	s, err := p.InspectUpdateInput(repo)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	proposed, err := proposedPackageBaseFlake(s, target)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	flake, err := deploymentFlakeReference(repo)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	file, err := os.CreateTemp("", "nixorium-base-lock-*.json")
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Close(); err != nil {
		return domain.UpdateProposal{}, err
	}
	if err := os.Remove(path); err != nil {
		return domain.UpdateProposal{}, err
	}
	url := "github:NixOS/nixpkgs/" + target
	emitUpdateProgress(progress, domain.UpdatePlanPhaseLock, "Resolving the system/package base candidate", 0, 0)
	if _, err := runBoundedNix(ctx, 256*1024, "flake", "lock", flake, "--override-input", "nixpkgs", url, "--update-input", "nixpkgs", "--output-lock-file", path); err != nil {
		return domain.UpdateProposal{}, err
	}
	lock, _, err := readRegularFileNoFollowLimit(path, 4*1024*1024)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	if err := preserveOtherPackageBaseNodes(s.LockContent, lock); err != nil {
		return domain.UpdateProposal{}, err
	}
	candidate := s
	candidate.FlakeContent, candidate.LockContent = proposed, lock
	candidate, err = inspectPackageBase(candidate)
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("candidate pin: %w", err)
	}
	if target == s.CurrentRef && s.CurrentRev == candidate.CurrentRev {
		return domain.UpdateProposal{}, errors.New("the selected system/package base is already at this revision; no update is available")
	}
	common := []string{"--override-input", "nixpkgs", url, "--reference-lock-file", path, "--no-write-lock-file"}
	proposal, err := validateUpdateCandidate(ctx, flake, common, s, proposed, lock, progress, true)
	if err != nil {
		return proposal, err
	}
	proposal.Diff.Scope = "package-base"
	proposal.PackageBase = &domain.PackageBaseChange{Source: "github:NixOS/nixpkgs", CurrentChannel: s.CurrentRef, CurrentRevision: s.CurrentRev, TargetChannel: target, TargetRevision: candidate.CurrentRev, Validation: "build-verified; runtime and hardware unverified"}
	return proposal, nil
}
