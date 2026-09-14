package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const ControllerActivationSchemaVersion = 1

type ControllerActivationRecord struct {
	SchemaVersion int       `json:"schemaVersion"`
	Revision      string    `json:"revision"`
	SystemPath    string    `json:"systemPath"`
	ActivatedAt   time.Time `json:"activatedAt"`
}

func DecodeControllerActivation(data []byte) (ControllerActivationRecord, error) {
	var record ControllerActivationRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return record, fmt.Errorf("decode controller activation: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return record, errors.New("decode controller activation: trailing JSON data")
	}
	if record.SchemaVersion != ControllerActivationSchemaVersion {
		return record, fmt.Errorf("unsupported controller activation schema version %d", record.SchemaVersion)
	}
	if !gitRevisionPattern.MatchString(record.Revision) {
		return record, errors.New("controller activation revision is not a full Git object ID")
	}
	if !validStorePath(record.SystemPath) {
		return record, errors.New("controller activation system path is not a Nix store path")
	}
	if record.ActivatedAt.IsZero() {
		return record, errors.New("controller activation timestamp is missing")
	}
	return record, nil
}

type ControllerRebuildPhase string

const (
	ControllerRebuildPhasePreflight ControllerRebuildPhase = "preflight"
	ControllerRebuildPhaseApply     ControllerRebuildPhase = "apply"
	ControllerRebuildPhaseVerify    ControllerRebuildPhase = "verify"
	ControllerRebuildPhaseComplete  ControllerRebuildPhase = "complete"
)

type ControllerRebuildPlanReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Controller    string            `json:"controller,omitempty"`
	Revision      string            `json:"revision,omitempty"`
	Current       bool              `json:"current"`
	CurrentDetail string            `json:"currentDetail,omitempty"`
	Confirmation  string            `json:"confirmation,omitempty"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r ControllerRebuildPlanReport) HasErrors() bool {
	return len(r.Issues) > 0
}

type ControllerRebuildExecutionReport struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Operation     string                 `json:"operation"`
	State         string                 `json:"state"`
	Repository    string                 `json:"repository"`
	Controller    string                 `json:"controller,omitempty"`
	Revision      string                 `json:"revision,omitempty"`
	Phase         ControllerRebuildPhase `json:"phase"`
	Unit          string                 `json:"unit,omitempty"`
	Applied       bool                   `json:"applied"`
	Verified      bool                   `json:"verified"`
	RetrySafe     bool                   `json:"retrySafe"`
	Message       string                 `json:"message,omitempty"`
	Issues        []ValidationIssue      `json:"issues"`
}

func (r ControllerRebuildExecutionReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || len(r.Issues) > 0
}
