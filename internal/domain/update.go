package domain

import "time"

type UpdateChannel string

const (
	UpdateChannelStable     UpdateChannel = "stable"
	UpdateChannelPrerelease UpdateChannel = "prerelease"
	UpdateChannelMoving     UpdateChannel = "moving"
)

type UpdateReleaseRef struct {
	Tag      string `json:"tag"`
	ObjectID string `json:"objectId"`
}

type UpdateRelease struct {
	Tag      string        `json:"tag"`
	ObjectID string        `json:"objectId"`
	Channel  UpdateChannel `json:"channel"`
}

type UpdateCheckReport struct {
	SchemaVersion  int               `json:"schemaVersion"`
	Operation      string            `json:"operation"`
	GeneratedAt    time.Time         `json:"generatedAt"`
	State          string            `json:"state"`
	Repository     string            `json:"repository"`
	Upstream       string            `json:"upstream,omitempty"`
	CurrentRef     string            `json:"currentRef,omitempty"`
	CurrentRev     string            `json:"currentRevision,omitempty"`
	CurrentChannel UpdateChannel     `json:"currentChannel,omitempty"`
	Development    []UpdateRelease   `json:"development"`
	Stable         []UpdateRelease   `json:"stable"`
	Prerelease     []UpdateRelease   `json:"prerelease"`
	Truncated      bool              `json:"truncated"`
	Issues         []ValidationIssue `json:"issues"`
}

func (r UpdateCheckReport) HasErrors() bool {
	return r.State != "available" || len(r.Issues) > 0
}

type UpdateInputSnapshot struct {
	SourceURL    string
	SourcePrefix string
	CurrentRef   string
	CurrentRev   string
	FlakeContent []byte
	LockContent  []byte
	HasLock      bool
	FlakeMode    uint32
	LockMode     uint32
}

type UpdateProposal struct {
	FlakeContent []byte
	LockContent  []byte
	Diff         GitDiff
	Checks       []UpdateCheck
	PackageBase  *PackageBaseChange
}

// PackageBaseChange reports observed pins, not a certification of runtime compatibility.
type PackageBaseChange struct {
	Source          string `json:"source"`
	CurrentChannel  string `json:"currentChannel"`
	CurrentRevision string `json:"currentRevision"`
	TargetChannel   string `json:"targetChannel"`
	TargetRevision  string `json:"targetRevision"`
	Validation      string `json:"validation"`
}

type PackageBaseStatus struct {
	Operation  string            `json:"operation"`
	Repository string            `json:"repository"`
	Source     string            `json:"source"`
	Channel    string            `json:"channel"`
	Revision   string            `json:"revision"`
	Issues     []ValidationIssue `json:"issues"`
}

type UpdateCheck struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Message string `json:"message"`
}

type UpdatePlanPhase string

const (
	UpdatePlanPhaseInspect  UpdatePlanPhase = "inspect"
	UpdatePlanPhaseLock     UpdatePlanPhase = "lock"
	UpdatePlanPhaseEvaluate UpdatePlanPhase = "evaluate"
	UpdatePlanPhaseBuild    UpdatePlanPhase = "build"
	UpdatePlanPhaseReview   UpdatePlanPhase = "review"
	UpdatePlanPhaseVerify   UpdatePlanPhase = "verify"
)

type UpdatePlanProgress struct {
	Phase   UpdatePlanPhase
	Detail  string
	Current int
	Total   int
}

type UpdatePlanReport struct {
	Kind            string              `json:"kind,omitempty"`
	AllowUnverified bool                `json:"allowUnverified,omitempty"`
	PackageBase     *PackageBaseChange  `json:"packageBase,omitempty"`
	SchemaVersion   int                 `json:"schemaVersion"`
	Operation       string              `json:"operation"`
	GeneratedAt     time.Time           `json:"generatedAt"`
	State           string              `json:"state"`
	Repository      string              `json:"repository"`
	Revision        string              `json:"revision,omitempty"`
	CurrentRef      string              `json:"currentRef,omitempty"`
	CurrentRev      string              `json:"currentRevision,omitempty"`
	CurrentChannel  UpdateChannel       `json:"currentChannel,omitempty"`
	Target          string              `json:"target,omitempty"`
	TargetChannel   UpdateChannel       `json:"targetChannel,omitempty"`
	Downgrade       bool                `json:"downgrade"`
	ReviewToken     string              `json:"reviewToken,omitempty"`
	Confirmation    string              `json:"confirmation,omitempty"`
	Diff            GitDiff             `json:"diff"`
	Checks          []UpdateCheck       `json:"checks"`
	Issues          []ValidationIssue   `json:"issues"`
	Snapshot        UpdateInputSnapshot `json:"-"`
	Proposal        UpdateProposal      `json:"-"`
}

func (r UpdatePlanReport) HasErrors() bool {
	return r.State != "ready" || len(r.Issues) > 0
}

type UpdateApplyReport struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Operation        string            `json:"operation"`
	State            string            `json:"state"`
	Repository       string            `json:"repository"`
	Revision         string            `json:"revision,omitempty"`
	Target           string            `json:"target,omitempty"`
	Updated          bool              `json:"updated"`
	RetrySafe        bool              `json:"retrySafe"`
	RecoveryRequired bool              `json:"recoveryRequired"`
	Message          string            `json:"message,omitempty"`
	Issues           []ValidationIssue `json:"issues"`
}

func (r UpdateApplyReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || r.State == "partial" || len(r.Issues) > 0
}
