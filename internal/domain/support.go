package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"time"
)

const (
	SupportSchemaVersion = 1
	SupportMaxBytes      = 32 * 1024
	SupportMaxFindings   = 128
	SupportMaxHosts      = 1024
	SupportMaxOperations = 50
)

// SupportInput contains private observations, never a directly serializable
// support payload. Nil reports mean that collection was unavailable.
type SupportInput struct {
	Version    string
	Revision   string
	Collected  time.Time
	Status     *StatusReport
	Doctor     *DoctorReport
	Hosts      *HostsReport
	Operations *[]OperationRecord
}

// SupportSnapshot is immutable and can only be constructed by the allowlist
// below. Saving this value exports the exact bytes shown in the preview.
type SupportSnapshot struct{ content string }

// SupportExportResult is local feedback, not part of the shareable payload.
type SupportExportResult struct {
	State   string `json:"state"`
	Path    string `json:"path,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Message string `json:"message"`
}

func (s SupportSnapshot) JSON() string { return s.content }
func (s SupportSnapshot) Valid() bool  { return s.content != "" && len(s.content) <= SupportMaxBytes }
func (s SupportSnapshot) Digest() string {
	sum := sha256.Sum256([]byte(s.content))
	return hex.EncodeToString(sum[:])
}

type supportReport struct {
	SchemaVersion   int                `json:"schemaVersion"`
	GuidanceVersion int                `json:"guidanceVersion"`
	Operation       string             `json:"operation"`
	CollectedAt     time.Time          `json:"collectedAt"`
	Version         string             `json:"commandVersion"`
	Revision        string             `json:"deploymentRevision,omitempty"`
	Status          *supportStatus     `json:"status"`
	Doctor          *supportDoctor     `json:"doctor"`
	Hosts           *supportHosts      `json:"hosts"`
	Operations      *supportOperations `json:"operations"`
	Excluded        []string           `json:"excluded"`
}

type supportStatus struct {
	Version         string `json:"deploymentVersion"`
	Mode            string `json:"deploymentMode"`
	Ready           bool   `json:"deploymentReady"`
	GitAvailable    bool   `json:"gitAvailable"`
	GitDirty        bool   `json:"gitDirty"`
	PXEMode         string `json:"pxeMode"`
	PreparedPresent bool   `json:"preparedPresent"`
	PreparedReady   bool   `json:"preparedReady"`
}

type supportFinding struct {
	ID    string `json:"id"`
	Level Level  `json:"level"`
	Guide string `json:"guide"`
}

type supportDoctor struct {
	Findings []supportFinding `json:"findings"`
	Omitted  int              `json:"omitted"`
}

type supportHosts struct {
	Observed int `json:"observed"`
	SSH      struct {
		Available   int `json:"available"`
		Unavailable int `json:"unavailable"`
		Unknown     int `json:"unknown"`
	} `json:"ssh"`
	Deployment HostDeploymentSummary `json:"deployment"`
	Omitted    int                   `json:"omitted"`
}

type supportOperation struct {
	Operation string `json:"operation"`
	State     string `json:"state"`
	Count     int    `json:"count"`
}

type supportOperations struct {
	Outcomes []supportOperation `json:"outcomes"`
	Omitted  int                `json:"omitted"`
}

var supportVersion = regexp.MustCompile(`^(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})(-(alpha|beta|rc)\.(0|[1-9][0-9]{0,3}))?$`)

func NewSupportSnapshot(input SupportInput) (SupportSnapshot, error) {
	if input.Collected.IsZero() || input.Collected.Year() < 1970 || input.Collected.Year() > 9999 {
		return SupportSnapshot{}, errors.New("support collection time is invalid")
	}
	report := supportReport{
		SchemaVersion:   SupportSchemaVersion,
		GuidanceVersion: SupportGuidanceVersion,
		Operation:       "support-report", CollectedAt: input.Collected.UTC().Truncate(time.Second),
		Version:  safeSupportVersion(input.Version),
		Excluded: []string{"identities", "addresses", "paths", "credentials", "free-text", "configuration", "homes", "raw-logs", "operation-identifiers-and-times"},
	}
	if gitRevisionPattern.MatchString(input.Revision) {
		report.Revision = input.Revision
	}
	if source := input.Status; source != nil && source.SchemaVersion == SchemaVersion {
		report.Status = &supportStatus{
			Version: safeSupportVersion(source.Meta.Version),
			Mode:    supportEnum(source.Meta.DeploymentMode, "laboratory", "controller"),
			Ready:   source.Deployment.Ready, GitAvailable: source.Git.Available, GitDirty: source.Git.Dirty,
			PXEMode:         supportEnum(source.PXE.Mode, "active", "ready", "stopped", "degraded", "recovery-required", "unavailable"),
			PreparedPresent: source.PXEPreparation.Present, PreparedReady: source.PXEPreparation.Ready,
		}
	}
	if source := input.Doctor; source != nil && source.SchemaVersion == SchemaVersion {
		result := &supportDoctor{Findings: []supportFinding{}}
		seen := map[string]bool{}
		for _, finding := range source.Findings[:min(len(source.Findings), SupportMaxFindings)] {
			guide := SupportFindingGuide(finding.ID)
			if guide == "" ||
				(finding.Level != LevelOK && finding.Level != LevelWarning && finding.Level != LevelError) || seen[finding.ID] {
				result.Omitted++
				continue
			}
			seen[finding.ID] = true
			result.Findings = append(result.Findings, supportFinding{ID: finding.ID, Level: finding.Level, Guide: guide})
		}
		result.Omitted += max(0, len(source.Findings)-SupportMaxFindings)
		sort.Slice(result.Findings, func(i, j int) bool { return result.Findings[i].ID < result.Findings[j].ID })
		report.Doctor = result
	}
	if source := input.Hosts; source != nil && source.SchemaVersion == SchemaVersion {
		result := &supportHosts{Observed: min(len(source.Hosts), SupportMaxHosts), Omitted: max(0, len(source.Hosts)-SupportMaxHosts)}
		for _, host := range source.Hosts[:result.Observed] {
			switch host.SSH {
			case SSHAvailable:
				result.SSH.Available++
			case SSHUnavailable:
				result.SSH.Unavailable++
			default:
				result.SSH.Unknown++
			}
			switch host.Deployment {
			case DeploymentCurrent:
				result.Deployment.Current++
			case DeploymentOutdated:
				result.Deployment.Outdated++
			default:
				result.Deployment.Unknown++
			}
		}
		report.Hosts = result
	}
	if input.Operations != nil {
		source := *input.Operations
		result := &supportOperations{Outcomes: []supportOperation{}, Omitted: max(0, len(source)-SupportMaxOperations)}
		counts := map[supportOperation]int{}
		for _, record := range source[:min(len(source), SupportMaxOperations)] {
			operation := supportEnum(record.Operation, "config-apply", "setup-keys", "setup-install-secrets", "setup-apply",
				"pxe-prepare", "pxe-start", "pxe-stop", "pxe-recover", "deploy-apply", "controller-apply",
				"service-restart", "git-commit", "update-apply", "package-base-apply", "shutdown-apply", "restart-apply", "internet-apply", "cleanup-apply", "deploy-recover", "template-reset-recover")
			state := supportEnum(record.State, "completed", "failed", "partial", "blocked", "unchanged", "saved", "ready", "running", "reconciliation-required",
				"applied", "invalid", "conflict", "action-required", "unchecked", "cancelled")
			if operation == "unknown" || state == "unknown" {
				result.Omitted++
				continue
			}
			counts[supportOperation{Operation: operation, State: state}]++
		}
		for outcome, count := range counts {
			outcome.Count = count
			result.Outcomes = append(result.Outcomes, outcome)
		}
		sort.Slice(result.Outcomes, func(i, j int) bool {
			left, right := result.Outcomes[i], result.Outcomes[j]
			if left.Operation != right.Operation {
				return left.Operation < right.Operation
			}
			return left.State < right.State
		})
		report.Operations = result
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil || len(data)+1 > SupportMaxBytes {
		return SupportSnapshot{}, errors.New("support report exceeds the serialization limit")
	}
	return SupportSnapshot{content: string(data) + "\n"}, nil
}

func safeSupportVersion(version string) string {
	if supportVersion.MatchString(version) {
		return version
	}
	return "unknown"
}

func supportEnum(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return candidate
		}
	}
	return "unknown"
}
