package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

const WorkspaceFileName = "workspace-profile.json"

var ErrWorkspaceConflict = errors.New("workspace source changed since review")
var ErrWorkspaceDurability = errors.New("workspace profile replaced but durable storage could not be confirmed")

// WorkspaceSnapshot identifies the evaluated Git source and the managed file
// separately: a first, untracked profile is not part of the Git fetcher source.
// Fingerprints must cover actual content, not timestamps or Git status labels.
type WorkspaceSnapshot struct {
	Revision          string `json:"revision"`
	SourceFingerprint string `json:"sourceFingerprint"`
	PinFingerprint    string `json:"pinFingerprint"`
	BaseFingerprint   string `json:"baseFingerprint"`
	BaseExists        bool   `json:"baseExists"`
}

type WorkspaceApplication struct {
	ID      string `json:"id"`
	Package string `json:"package"`
	Browser bool   `json:"browser"`
}

type WorkspaceExtension struct {
	ID                 string   `json:"id"`
	Package            string   `json:"package"`
	RequiredPackages   []string `json:"requiredPackages"`
	RequiredExtensions []string `json:"requiredExtensions"`
	// Writable extensions are copied into the home instead of linked.
	Writable bool   `json:"writable"`
	Version  string `json:"version,omitempty"`
}

type WorkspaceCatalog struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Baseline      WorkspaceProfile       `json:"baseline"`
	Applications  []WorkspaceApplication `json:"applications"`
	Extensions    []WorkspaceExtension   `json:"extensions"`
}

type WorkspaceTarget struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type WorkspacePackage struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

// WorkspaceResolution mirrors the preparation output, not running host state.
// Both deployment candidate hooks must succeed before this is returned.
type WorkspaceResolution struct {
	SchemaVersion    int                  `json:"schemaVersion"`
	State            string               `json:"state"`
	ManagedFile      string               `json:"managedFile"`
	StudentUser      string               `json:"studentUser"`
	Seed             string               `json:"seed"`
	Declared         WorkspaceProfile     `json:"declared"`
	Effective        WorkspaceProfile     `json:"effective"`
	Catalog          WorkspaceCatalog     `json:"catalog"`
	RequiredPackages []string             `json:"requiredPackages"`
	Packages         []WorkspacePackage   `json:"packages"`
	Extensions       []WorkspaceExtension `json:"extensions"`
	Targets          []WorkspaceTarget    `json:"targets"`
}

type WorkspaceInspection struct {
	Snapshot   WorkspaceSnapshot   `json:"snapshot"`
	Base       *WorkspaceProfile   `json:"base"`
	Resolution WorkspaceResolution `json:"resolution"`
}

type WorkspacePlanReport struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Operation     string               `json:"operation"`
	State         string               `json:"state"`
	Repository    string               `json:"repository"`
	ManagedFile   string               `json:"managedFile"`
	Candidate     *WorkspaceProfile    `json:"candidate,omitempty"`
	Inspection    *WorkspaceInspection `json:"inspection,omitempty"`
	ReviewToken   string               `json:"reviewToken,omitempty"`
	Confirmation  string               `json:"confirmation,omitempty"`
	Issues        []ValidationIssue    `json:"issues"`
	Message       string               `json:"message"`
}

func (r WorkspacePlanReport) HasErrors() bool {
	return len(r.Issues) != 0 || (r.State != "ready" && r.State != "unchanged")
}

type WorkspaceApplyReport struct {
	Recorded         bool              `json:"recorded,omitempty"`
	Revision         string            `json:"revision,omitempty"`
	SchemaVersion    int               `json:"schemaVersion"`
	Operation        string            `json:"operation"`
	State            string            `json:"state"`
	Repository       string            `json:"repository"`
	ManagedFile      string            `json:"managedFile"`
	StudentUser      string            `json:"studentUser,omitempty"`
	Targets          []WorkspaceTarget `json:"targets"`
	RecoveryRequired bool              `json:"recoveryRequired,omitempty"`
	Issues           []ValidationIssue `json:"issues"`
	Message          string            `json:"message"`
}

func (r WorkspaceApplyReport) HasErrors() bool {
	return len(r.Issues) != 0 || (r.State != "saved" && r.State != "unchanged" && r.State != "cancelled")
}

var workspaceFingerprintPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var workspaceRevisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var workspaceHostPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ValidateWorkspaceInspection checks the adapter contract, not Nix package
// availability or baseline composition. Those remain deployment-hook decisions.
func ValidateWorkspaceInspection(inspection WorkspaceInspection, candidate WorkspaceProfile) error {
	snapshot, resolved := inspection.Snapshot, inspection.Resolution
	if !workspaceRevisionPattern.MatchString(snapshot.Revision) ||
		!workspaceFingerprintPattern.MatchString(snapshot.SourceFingerprint) ||
		!workspaceFingerprintPattern.MatchString(snapshot.PinFingerprint) ||
		!workspaceFingerprintPattern.MatchString(snapshot.BaseFingerprint) ||
		snapshot.BaseExists != (inspection.Base != nil) {
		return errors.New("incomplete workspace source identity")
	}
	if err := ValidateWorkspaceResolution(resolved); err != nil {
		return err
	}
	if inspection.Base != nil {
		if _, err := MarshalWorkspaceProfile(*inspection.Base); err != nil {
			return errors.New("invalid base profile in workspace inspection")
		}
	}
	want, err := MarshalWorkspaceProfile(candidate)
	if err != nil {
		return errors.New("invalid candidate in workspace inspection")
	}
	got, _ := MarshalWorkspaceProfile(resolved.Declared)
	if !bytes.Equal(want, got) {
		return errors.New("resolved workspace does not match the requested profile")
	}
	return nil
}

// ValidateWorkspaceResolution checks versioned metadata independently of the
// save snapshot so update reviews can also compare current and candidate pins.
func ValidateWorkspaceResolution(resolved WorkspaceResolution) error {
	if resolved.SchemaVersion != WorkspaceSchemaVersion || resolved.State != "prepared" ||
		resolved.ManagedFile != WorkspaceFileName || !userNamePattern.MatchString(resolved.StudentUser) ||
		resolved.Catalog.SchemaVersion != WorkspaceSchemaVersion {
		return errors.New("unsupported workspace preparation metadata")
	}
	if !ValidStorePath(resolved.Seed) {
		return errors.New("invalid workspace seed in preparation metadata")
	}
	profiles := []WorkspaceProfile{resolved.Declared, resolved.Effective, resolved.Catalog.Baseline}
	for _, profile := range profiles {
		if _, err := MarshalWorkspaceProfile(profile); err != nil {
			return errors.New("invalid profile in workspace inspection")
		}
	}
	if len(resolved.Targets) == 0 || resolved.Targets[0].Role != "controller" {
		return errors.New("workspace destinations must include the controller student")
	}
	names := make(map[string]bool)
	for i, target := range resolved.Targets {
		if !workspaceHostPattern.MatchString(target.Name) || names[target.Name] ||
			(i > 0 && target.Role != "client") {
			return errors.New("invalid workspace destinations")
		}
		names[target.Name] = true
	}
	return nil
}

// WorkspaceReviewToken binds all resolved metadata, including baseline,
// catalog, versions, dependencies, boot behavior and controller destinations.
func WorkspaceReviewToken(repository string, candidate WorkspaceProfile, inspection WorkspaceInspection) (string, error) {
	if err := ValidateWorkspaceInspection(inspection, candidate); err != nil {
		return "", err
	}
	normalized, err := MarshalWorkspaceProfile(candidate)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(struct {
		Protocol   string              `json:"protocol"`
		Repository string              `json:"repository"`
		Candidate  json.RawMessage     `json:"candidate"`
		Inspection WorkspaceInspection `json:"inspection"`
	}{"workspace-review-v1", repository, normalized, inspection})
	if err != nil {
		return "", fmt.Errorf("encode workspace review: %w", err)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
