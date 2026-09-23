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
	"strings"
)

const SoftwarePresetSchemaVersion = 1

var softwarePresetIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

type SoftwarePreset struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Packages    []string `json:"packages"`
}

type SoftwarePresetCatalog struct {
	SchemaVersion int              `json:"schemaVersion"`
	DefaultPreset string           `json:"defaultPreset"`
	Presets       []SoftwarePreset `json:"presets"`
}

type SoftwarePresetCatalogReport struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Operation     string                 `json:"operation"`
	State         string                 `json:"state"`
	Repository    string                 `json:"repository"`
	Catalog       *SoftwarePresetCatalog `json:"catalog,omitempty"`
	Fingerprint   string                 `json:"fingerprint,omitempty"`
	Issues        []ValidationIssue      `json:"issues"`
	Message       string                 `json:"message,omitempty"`
}

func (r SoftwarePresetCatalogReport) HasErrors() bool {
	return r.State == "failed" || len(r.Issues) > 0
}

type SoftwarePresetRequest struct {
	Preset  string        `json:"preset"`
	Scope   SoftwareScope `json:"scope"`
	Exclude []string      `json:"exclude"`
}

type SoftwarePresetPlanReport struct {
	SchemaVersion      int                   `json:"schemaVersion"`
	Operation          string                `json:"operation"`
	State              string                `json:"state"`
	Repository         string                `json:"repository"`
	ManagedFile        string                `json:"managedFile"`
	Request            SoftwarePresetRequest `json:"request"`
	Preset             SoftwarePreset        `json:"preset"`
	SelectedPackages   []SoftwareCatalogItem `json:"selectedPackages"`
	Existing           []SoftwareDeclaration `json:"existing"`
	Additions          []SoftwareDeclaration `json:"additions"`
	Candidate          LabSoftwareFile       `json:"candidate"`
	CatalogFingerprint string                `json:"catalogFingerprint,omitempty"`
	BaseFingerprint    string                `json:"baseFingerprint,omitempty"`
	ReviewToken        string                `json:"reviewToken,omitempty"`
	Confirmation       string                `json:"confirmation,omitempty"`
	AffectedController string                `json:"affectedController,omitempty"`
	AffectedClients    []string              `json:"affectedClients"`
	Issues             []ValidationIssue     `json:"issues"`
	Message            string                `json:"message,omitempty"`
}

func (r SoftwarePresetPlanReport) HasErrors() bool {
	return r.State == "invalid" || r.State == "failed" || r.State == "conflict" || len(r.Issues) > 0
}

type SoftwarePresetApplyReport struct {
	SchemaVersion      int                   `json:"schemaVersion"`
	Operation          string                `json:"operation"`
	State              string                `json:"state"`
	Repository         string                `json:"repository"`
	ManagedFile        string                `json:"managedFile"`
	Request            SoftwarePresetRequest `json:"request"`
	Preset             SoftwarePreset        `json:"preset"`
	Existing           []SoftwareDeclaration `json:"existing"`
	Additions          []SoftwareDeclaration `json:"additions"`
	AffectedController string                `json:"affectedController,omitempty"`
	AffectedClients    []string              `json:"affectedClients"`
	Revision           string                `json:"revision,omitempty"`
	RecoveryRequired   bool                  `json:"recoveryRequired,omitempty"`
	Issues             []ValidationIssue     `json:"issues"`
	Message            string                `json:"message,omitempty"`
}

func (r SoftwarePresetApplyReport) HasErrors() bool {
	return r.State == "invalid" || r.State == "failed" || r.State == "conflict" || r.State == "partial" || len(r.Issues) > 0
}

func DecodeSoftwarePresetCatalog(data []byte) (SoftwarePresetCatalog, error) {
	var catalog SoftwarePresetCatalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return catalog, fmt.Errorf("decode software presets: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return catalog, errors.New("decode software presets: trailing JSON data")
	}
	catalog = NormalizeSoftwarePresetCatalog(catalog)
	if err := ValidateSoftwarePresetCatalog(catalog); err != nil {
		return catalog, err
	}
	return catalog, nil
}

func MarshalSoftwarePresetCatalog(catalog SoftwarePresetCatalog) ([]byte, error) {
	catalog = NormalizeSoftwarePresetCatalog(catalog)
	if err := ValidateSoftwarePresetCatalog(catalog); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode software presets: %w", err)
	}
	return append(data, '\n'), nil
}

func NormalizeSoftwarePresetCatalog(catalog SoftwarePresetCatalog) SoftwarePresetCatalog {
	result := SoftwarePresetCatalog{
		SchemaVersion: catalog.SchemaVersion,
		DefaultPreset: strings.TrimSpace(catalog.DefaultPreset),
		Presets:       append([]SoftwarePreset(nil), catalog.Presets...),
	}
	for index := range result.Presets {
		result.Presets[index].ID = strings.TrimSpace(result.Presets[index].ID)
		result.Presets[index].Label = strings.TrimSpace(result.Presets[index].Label)
		result.Presets[index].Description = strings.TrimSpace(result.Presets[index].Description)
		result.Presets[index].Packages = append([]string(nil), result.Presets[index].Packages...)
		sort.Strings(result.Presets[index].Packages)
	}
	return result
}

func ValidateSoftwarePresetCatalog(catalog SoftwarePresetCatalog) error {
	if catalog.SchemaVersion != SoftwarePresetSchemaVersion {
		return fmt.Errorf("unsupported software preset schema %d", catalog.SchemaVersion)
	}
	if len(catalog.Presets) == 0 {
		return errors.New("software preset catalog must contain at least one preset")
	}
	seenPresets := map[string]bool{}
	defaultFound := false
	for index, preset := range catalog.Presets {
		if !softwarePresetIDPattern.MatchString(preset.ID) || seenPresets[preset.ID] {
			return fmt.Errorf("presets[%d].id is invalid or duplicated", index)
		}
		seenPresets[preset.ID] = true
		if preset.ID == catalog.DefaultPreset {
			defaultFound = true
		}
		if preset.Label == "" || len(preset.Label) > 80 {
			return fmt.Errorf("presets[%d].label must be 1 to 80 characters", index)
		}
		if preset.Description == "" || len(preset.Description) > 240 {
			return fmt.Errorf("presets[%d].description must be 1 to 240 characters", index)
		}
		if len(preset.Packages) == 0 {
			return fmt.Errorf("presets[%d].packages must not be empty", index)
		}
		seenPackages := map[string]bool{}
		for _, packageID := range preset.Packages {
			if len(packageID) > 80 || !softwarePackagePattern.MatchString(packageID) || seenPackages[packageID] {
				return fmt.Errorf("preset %s contains an invalid or duplicate package", preset.ID)
			}
			seenPackages[packageID] = true
		}
	}
	if !softwarePresetIDPattern.MatchString(catalog.DefaultPreset) || !defaultFound {
		return errors.New("defaultPreset must name a catalog preset")
	}
	return nil
}

func SoftwarePresetCatalogFingerprint(catalog SoftwarePresetCatalog) (string, error) {
	data, err := MarshalSoftwarePresetCatalog(catalog)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
