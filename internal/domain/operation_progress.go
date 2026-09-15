package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
)

const OperationProgressSchemaVersion = 1

var operationProgressPhases = map[string]map[string]bool{
	"pxe-prepare": {
		"starting": true, "validate": true, "network": true, "artifacts": true,
		"clients": true, "publish": true, "complete": true,
	},
	"controller-apply": {
		"starting": true, "validate": true, "build": true, "activate": true,
		"verify": true, "complete": true,
	},
}

// OperationProgress is the bounded, terminal-safe status published by a
// systemd-owned long-running operation. Recent contains at most five activity
// messages; it deliberately is not an arbitrary command log.
type OperationProgress struct {
	SchemaVersion int       `json:"schemaVersion"`
	Operation     string    `json:"operation"`
	State         string    `json:"state"`
	Phase         string    `json:"phase"`
	StartedAt     time.Time `json:"startedAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	Current       int       `json:"current,omitempty"`
	Total         int       `json:"total,omitempty"`
	Recent        []string  `json:"recent"`
}

func DecodeOperationProgress(data []byte) (OperationProgress, error) {
	var progress OperationProgress
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&progress); err != nil {
		return progress, fmt.Errorf("decode operation progress: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return progress, errors.New("decode operation progress: trailing JSON data")
	}
	if progress.SchemaVersion != OperationProgressSchemaVersion {
		return progress, fmt.Errorf("unsupported operation progress schema version %d", progress.SchemaVersion)
	}
	phases, supported := operationProgressPhases[progress.Operation]
	if !supported {
		return progress, fmt.Errorf("unsupported progress operation %q", progress.Operation)
	}
	if progress.State != "running" && progress.State != "completed" && progress.State != "failed" {
		return progress, fmt.Errorf("invalid operation progress state %q", progress.State)
	}
	if !phases[progress.Phase] {
		return progress, fmt.Errorf("invalid operation progress phase %q", progress.Phase)
	}
	if (progress.State == "completed") != (progress.Phase == "complete") {
		return progress, errors.New("operation progress completion state and phase differ")
	}
	if progress.StartedAt.IsZero() || progress.UpdatedAt.IsZero() || progress.UpdatedAt.Before(progress.StartedAt) {
		return progress, errors.New("invalid operation progress timestamps")
	}
	if progress.Current < 0 || progress.Total < 0 || progress.Current > progress.Total || progress.Total > 10000 {
		return progress, errors.New("invalid operation progress counters")
	}
	if progress.Recent == nil || len(progress.Recent) > 5 {
		return progress, errors.New("invalid operation progress activity count")
	}
	for _, activity := range progress.Recent {
		if activity == "" || strings.TrimSpace(activity) != activity || len(activity) > 256 || containsUnsafeTerminalCharacter(activity) {
			return progress, errors.New("invalid operation progress activity")
		}
	}
	return progress, nil
}

func containsUnsafeTerminalCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
			return true
		}
	}
	return false
}
