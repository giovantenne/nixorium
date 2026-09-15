package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
)

const SoftwareSchemaVersion = 1

const (
	SoftwareScopeAllClients = "all-clients"
	SoftwareScopeGroup      = "group"
	SoftwareScopeClients    = "clients"
)

var softwarePackagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+._-]{0,79}$`)
var softwareGroupPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

var ErrSoftwareConflict = errors.New("software declaration changed since review")
var ErrSoftwareDurability = errors.New("software declaration replaced but durable storage could not be confirmed")

type SoftwareScope struct {
	Kind    string   `json:"kind"`
	Group   string   `json:"group,omitempty"`
	Clients []string `json:"clients,omitempty"`
}

type SoftwareDeclaration struct {
	Package string        `json:"package"`
	Scope   SoftwareScope `json:"scope"`
	Origin  string        `json:"origin,omitempty"`
}

type LabSoftwareFile struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Packages      []SoftwareDeclaration `json:"packages"`
}

type SoftwareCatalogItem struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Summary      string `json:"summary"`
	Availability string `json:"availability"`
}

type SoftwareDefinition struct {
	SchemaVersion int                   `json:"schemaVersion"`
	ManagedFile   string                `json:"managedFile"`
	Clients       []string              `json:"clients"`
	Groups        map[string][]string   `json:"groups"`
	Catalog       []SoftwareCatalogItem `json:"catalog"`
	Packages      []SoftwareDeclaration `json:"packages"`
}

type SoftwareCatalogReport struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Operation     string                `json:"operation"`
	State         string                `json:"state"`
	Repository    string                `json:"repository"`
	ManagedFile   string                `json:"managedFile,omitempty"`
	Clients       []string              `json:"clients"`
	Groups        map[string][]string   `json:"groups"`
	Catalog       []SoftwareCatalogItem `json:"catalog"`
	Packages      []SoftwareDeclaration `json:"packages"`
	Message       string                `json:"message,omitempty"`
	Issues        []ValidationIssue     `json:"issues"`
}

func (r SoftwareCatalogReport) HasErrors() bool { return len(r.Issues) > 0 }

type SoftwareChangeRequest struct {
	Package string        `json:"package"`
	Present bool          `json:"present"`
	Scope   SoftwareScope `json:"scope"`
}

type SoftwareChangePlanReport struct {
	SchemaVersion   int                   `json:"schemaVersion"`
	Operation       string                `json:"operation"`
	State           string                `json:"state"`
	Repository      string                `json:"repository"`
	ManagedFile     string                `json:"managedFile"`
	Request         SoftwareChangeRequest `json:"request"`
	Candidate       LabSoftwareFile       `json:"candidate"`
	BaseFingerprint string                `json:"baseFingerprint,omitempty"`
	ReviewToken     string                `json:"reviewToken,omitempty"`
	Confirmation    string                `json:"confirmation,omitempty"`
	AffectedClients []string              `json:"affectedClients"`
	Issues          []ValidationIssue     `json:"issues"`
	Message         string                `json:"message,omitempty"`
}

func (r SoftwareChangePlanReport) HasErrors() bool {
	return len(r.Issues) > 0 || r.State == "invalid" || r.State == "conflict"
}

type SoftwareChangeApplyReport struct {
	SchemaVersion   int                   `json:"schemaVersion"`
	Operation       string                `json:"operation"`
	State           string                `json:"state"`
	Repository      string                `json:"repository"`
	ManagedFile     string                `json:"managedFile"`
	Request         SoftwareChangeRequest `json:"request"`
	AffectedClients []string              `json:"affectedClients"`
	Issues          []ValidationIssue     `json:"issues"`
	Message         string                `json:"message,omitempty"`
}

func (r SoftwareChangeApplyReport) HasErrors() bool {
	return len(r.Issues) > 0 || r.State == "invalid" || r.State == "conflict" || r.State == "failed" || r.State == "partial"
}

func DecodeLabSoftware(data []byte) (LabSoftwareFile, error) {
	var software LabSoftwareFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&software); err != nil {
		return software, fmt.Errorf("decode lab software: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return software, errors.New("decode lab software: trailing JSON data")
	}
	if err := ValidateLabSoftware(software); err != nil {
		return software, err
	}
	return software, nil
}

func MarshalLabSoftware(software LabSoftwareFile) ([]byte, error) {
	software = NormalizeLabSoftware(software)
	if err := ValidateLabSoftware(software); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(software, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode lab software: %w", err)
	}
	return append(data, '\n'), nil
}

func NormalizeLabSoftware(software LabSoftwareFile) LabSoftwareFile {
	result := LabSoftwareFile{SchemaVersion: software.SchemaVersion, Packages: append([]SoftwareDeclaration(nil), software.Packages...)}
	for index := range result.Packages {
		result.Packages[index].Origin = ""
		result.Packages[index].Scope.Clients = append([]string(nil), result.Packages[index].Scope.Clients...)
		sort.Strings(result.Packages[index].Scope.Clients)
	}
	sort.Slice(result.Packages, func(i, j int) bool { return result.Packages[i].Package < result.Packages[j].Package })
	return result
}

func ValidateLabSoftware(software LabSoftwareFile) error {
	if software.SchemaVersion != SoftwareSchemaVersion {
		return fmt.Errorf("unsupported lab software schema %d", software.SchemaVersion)
	}
	seen := map[string]bool{}
	for _, entry := range software.Packages {
		if !softwarePackagePattern.MatchString(entry.Package) || seen[entry.Package] || entry.Origin != "" {
			return errors.New("lab software contains an invalid or duplicate package")
		}
		seen[entry.Package] = true
		if err := ValidateSoftwareScope(entry.Scope); err != nil {
			return fmt.Errorf("package %s: %w", entry.Package, err)
		}
	}
	return nil
}

func ValidateSoftwareScope(scope SoftwareScope) error {
	switch scope.Kind {
	case SoftwareScopeAllClients:
		if scope.Group != "" || len(scope.Clients) != 0 {
			return errors.New("all-clients scope contains unrelated fields")
		}
	case SoftwareScopeGroup:
		if !softwareGroupPattern.MatchString(scope.Group) || len(scope.Clients) != 0 {
			return errors.New("group scope is invalid")
		}
	case SoftwareScopeClients:
		if scope.Group != "" || len(scope.Clients) == 0 {
			return errors.New("client scope is empty or contains unrelated fields")
		}
		seen := map[string]bool{}
		for _, name := range scope.Clients {
			if !clientNamePattern.MatchString(name) || seen[name] {
				return errors.New("client scope contains an invalid or duplicate identity")
			}
			seen[name] = true
		}
	default:
		return fmt.Errorf("unknown software scope %q", scope.Kind)
	}
	return nil
}

func SoftwareFingerprint(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func SoftwareReviewToken(baseFingerprint string, candidate []byte) string {
	digest := sha256.Sum256(append(append([]byte(baseFingerprint), 0), candidate...))
	return "sha256:" + hex.EncodeToString(digest[:])
}
