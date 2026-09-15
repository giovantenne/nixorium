package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

const InstallationSessionSchemaVersion = 1

const InstallationSessionActive = "active"

// InstallationEvidence records authenticated technical evidence and a later,
// explicit operator check. Both remain bound to one identity and Git revision.
type InstallationEvidence struct {
	Name                 string     `json:"name"`
	Revision             string     `json:"revision"`
	SystemPath           string     `json:"systemPath"`
	TechnicalVerifiedAt  time.Time  `json:"technicalVerifiedAt"`
	PracticalConfirmedAt *time.Time `json:"practicalConfirmedAt,omitempty"`
}

// InstallationSessionRecord is private operator state, not desired
// configuration and not proof of current reachability.
type InstallationSessionRecord struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Repository    string                 `json:"repository"`
	State         string                 `json:"state"`
	Revision      string                 `json:"revision"`
	Selected      string                 `json:"selected"`
	StartedAt     time.Time              `json:"startedAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	Evidence      []InstallationEvidence `json:"evidence"`
}

type InstallationSessionReport struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Operation     string                 `json:"operation"`
	State         string                 `json:"state"`
	Repository    string                 `json:"repository"`
	Revision      string                 `json:"revision,omitempty"`
	Selected      string                 `json:"selected,omitempty"`
	StartedAt     time.Time              `json:"startedAt,omitempty"`
	UpdatedAt     time.Time              `json:"updatedAt,omitempty"`
	Evidence      []InstallationEvidence `json:"evidence"`
	Observation   *HostStatus            `json:"observation,omitempty"`
	Message       string                 `json:"message,omitempty"`
	Issues        []ValidationIssue      `json:"issues"`
}

func (r InstallationSessionReport) HasErrors() bool {
	return r.State == "failed" || r.State == "blocked" || len(r.Issues) > 0
}

func DecodeInstallationSession(data []byte) (InstallationSessionRecord, error) {
	var record InstallationSessionRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, fmt.Errorf("decode installation session: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return record, errors.New("decode installation session: trailing JSON data")
	}
	if err := ValidateInstallationSession(record); err != nil {
		return record, err
	}
	return record, nil
}

func MarshalInstallationSession(record InstallationSessionRecord) ([]byte, error) {
	if err := ValidateInstallationSession(record); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode installation session: %w", err)
	}
	return append(data, '\n'), nil
}

func ValidateInstallationSession(record InstallationSessionRecord) error {
	if record.SchemaVersion != InstallationSessionSchemaVersion {
		return fmt.Errorf("unsupported installation session schema %d", record.SchemaVersion)
	}
	if !filepath.IsAbs(record.Repository) || filepath.Clean(record.Repository) != record.Repository || strings.ContainsAny(record.Repository, "\r\n\x00") {
		return errors.New("installation session repository must be an absolute clean path")
	}
	if record.State != InstallationSessionActive {
		return fmt.Errorf("invalid installation session state %q", record.State)
	}
	if !gitRevisionPattern.MatchString(record.Revision) {
		return errors.New("installation session revision is not a full Git object ID")
	}
	if !clientNamePattern.MatchString(record.Selected) {
		return errors.New("installation session selected identity is invalid")
	}
	if record.StartedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.StartedAt) {
		return errors.New("installation session timestamps are invalid")
	}
	seen := map[string]bool{}
	for _, evidence := range record.Evidence {
		if !clientNamePattern.MatchString(evidence.Name) || seen[evidence.Name] {
			return errors.New("installation session contains an invalid or duplicate identity")
		}
		seen[evidence.Name] = true
		if evidence.Revision != record.Revision || !validStorePath(evidence.SystemPath) || evidence.TechnicalVerifiedAt.IsZero() || evidence.TechnicalVerifiedAt.Before(record.StartedAt) || evidence.TechnicalVerifiedAt.After(record.UpdatedAt) {
			return fmt.Errorf("installation evidence for %s is invalid", evidence.Name)
		}
		if evidence.PracticalConfirmedAt != nil && (evidence.PracticalConfirmedAt.Before(evidence.TechnicalVerifiedAt) || evidence.PracticalConfirmedAt.After(record.UpdatedAt)) {
			return fmt.Errorf("practical evidence for %s is invalid", evidence.Name)
		}
	}
	return nil
}
