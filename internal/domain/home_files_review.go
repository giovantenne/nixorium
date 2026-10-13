package domain

import "errors"

var ErrHomeFilesConflict = errors.New("initial home files changed since review")
var ErrHomeFilesDurability = errors.New("initial home file storage durability is unconfirmed")

type HomeFilesDraft struct {
	Profile    HomeFilesProfile `json:"profile"`
	CaptureIDs []string         `json:"captureIds,omitempty"`
	// A draft opened by the TUI retains its original manifest identity.
	BaseFingerprint string `json:"baseFingerprint,omitempty"`
}

type HomeFilesSnapshot struct {
	Revision           string `json:"revision"`
	SourceFingerprint  string `json:"sourceFingerprint"`
	PinFingerprint     string `json:"pinFingerprint"`
	BaseFingerprint    string `json:"baseFingerprint"`
	BaseExists         bool   `json:"baseExists"`
	PayloadFingerprint string `json:"payloadFingerprint"`
}

type HomeFilesEntry struct {
	Destination string `json:"destination"`
	SHA256      string `json:"sha256,omitempty"`
	Size        int64  `json:"size"`
	Executable  bool   `json:"executable"`
	Directory   bool   `json:"directory,omitempty"`
}

type HomeFilesCaptureReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	State         string            `json:"state"`
	CaptureID     string            `json:"captureId,omitempty"`
	Destination   string            `json:"destination"`
	Files         []HomeFile        `json:"files"`
	Directories   []string          `json:"directories"`
	Entries       []HomeFilesEntry  `json:"entries"`
	Issues        []ValidationIssue `json:"issues"`
	Message       string            `json:"message"`
}

func (r HomeFilesCaptureReport) HasErrors() bool { return r.State != "captured" || len(r.Issues) != 0 }

type HomeFilesPlanReport struct {
	SchemaVersion int                `json:"schemaVersion"`
	Operation     string             `json:"operation"`
	State         string             `json:"state"`
	Repository    string             `json:"repository"`
	Candidate     *HomeFilesDraft    `json:"candidate,omitempty"`
	Snapshot      *HomeFilesSnapshot `json:"snapshot,omitempty"`
	Entries       []HomeFilesEntry   `json:"entries"`
	StudentUser   string             `json:"studentUser"`
	Targets       []WorkspaceTarget  `json:"targets"`
	ReviewToken   string             `json:"reviewToken,omitempty"`
	Confirmation  string             `json:"confirmation,omitempty"`
	Issues        []ValidationIssue  `json:"issues"`
	Message       string             `json:"message"`
}

func (r HomeFilesPlanReport) HasErrors() bool {
	return (r.State != "ready" && r.State != "unchanged") || len(r.Issues) != 0
}

type HomeFilesApplyReport struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Operation        string            `json:"operation"`
	State            string            `json:"state"`
	Repository       string            `json:"repository"`
	Recorded         bool              `json:"recorded,omitempty"`
	Revision         string            `json:"revision,omitempty"`
	RecoveryRequired bool              `json:"recoveryRequired,omitempty"`
	StudentUser      string            `json:"studentUser,omitempty"`
	Targets          []WorkspaceTarget `json:"targets"`
	Issues           []ValidationIssue `json:"issues"`
	Message          string            `json:"message"`
}

func (r HomeFilesApplyReport) HasErrors() bool {
	return (r.State != "saved" && r.State != "unchanged" && r.State != "cancelled") || len(r.Issues) != 0
}
