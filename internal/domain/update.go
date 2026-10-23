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
	Tag       string        `json:"tag"`
	ObjectID  string        `json:"objectId"`
	Channel   UpdateChannel `json:"channel"`
	Downgrade bool          `json:"downgrade,omitempty"`
}

type UpdateCheckReport struct {
	SchemaVersion  int           `json:"schemaVersion"`
	Operation      string        `json:"operation"`
	GeneratedAt    time.Time     `json:"generatedAt"`
	State          string        `json:"state"`
	Repository     string        `json:"repository"`
	Upstream       string        `json:"upstream,omitempty"`
	CurrentRef     string        `json:"currentRef,omitempty"`
	CurrentRev     string        `json:"currentRevision,omitempty"`
	CurrentChannel UpdateChannel `json:"currentChannel,omitempty"`
	VersionFloor   string        `json:"versionFloor,omitempty"`
	// PinOutdated describes a flake.lock whose recorded nixorium reference no
	// longer matches flake.nix. Planning the configured target again repairs it.
	PinOutdated string            `json:"pinOutdated,omitempty"`
	Development []UpdateRelease   `json:"development"`
	Stable      []UpdateRelease   `json:"stable"`
	Prerelease  []UpdateRelease   `json:"prerelease"`
	Truncated   bool              `json:"truncated"`
	Issues      []ValidationIssue `json:"issues"`
}

func (r UpdateCheckReport) HasErrors() bool {
	return r.State != "available" || len(r.Issues) > 0
}

type UpdateInputSnapshot struct {
	SourceURL    string
	SourcePrefix string
	CurrentRef   string
	CurrentRev   string
	// PinOutdated is set when flake.lock records another nixorium reference
	// than flake.nix declares (an update saved by Nixorium 3.8.0 or older).
	PinOutdated  string
	FlakeContent []byte
	LockContent  []byte
	HasLock      bool
	FlakeMode    uint32
	LockMode     uint32
}

type UpdateProposal struct {
	FlakeContent []byte
	LockContent  []byte
	// NixoriumRevision is the Nixorium revision LockContent pins.
	NixoriumRevision string
	Diff             GitDiff
	Checks           []UpdateCheck
	PackageBase      *PackageBaseChange
	Workspace        *WorkspaceUpdateImpact
	// Migrations are the target release's changes to the deployment's own
	// files (ADR 0034), with their outcome on this deployment; MigratedFiles
	// are the files the applied ones change, written when the update is saved.
	Migrations    []DeploymentMigrationResult
	MigratedFiles []MigratedFile
	// MigrationReport and Migrator are the raw report and the store path of
	// the migrator that produced them; the review token binds both.
	MigrationReport []byte
	Migrator        string
}

// MigratedFile is a deployment file before and after the migrations: Create
// marks a new file, Delete a removed one.
type MigratedFile struct {
	Path   string
	Before []byte
	After  []byte
	Mode   uint32
	Create bool
	Delete bool
}

// DeploymentMigrationsFile records, in the deployment, the migrations already
// applied or found done, so each runs once (like Omarchy's migration markers,
// but saved and reviewed with the configuration).
const DeploymentMigrationsFile = "nixorium-migrations.json"

// Outcomes of a deployment migration, as the migrator of the release being
// installed reports them (ADR 0034).
const (
	// MigrationApplied: the migration changed the files it lists; saving
	// the update writes them.
	MigrationApplied = "applied"
	// MigrationDone: the deployment already has the change.
	MigrationDone = "done"
	// MigrationManual: the files were changed by hand; nothing is touched
	// and Detail says what to change.
	MigrationManual = "manual"
)

// DeploymentMigrationResult is one migration of the release being installed.
type DeploymentMigrationResult struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	Detail      string   `json:"detail,omitempty"`
	Files       []string `json:"files,omitempty"`
}

// WorkspaceUpdateImpact compares pinned declarations, not live home state,
// vendor-latest releases, or proof that an extension loads in the editor.
type WorkspaceUpdateImpact struct {
	Current  *WorkspaceResolution `json:"current"`
	Proposed *WorkspaceResolution `json:"proposed"`
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
	Kind            string                 `json:"kind,omitempty"`
	AllowUnverified bool                   `json:"allowUnverified,omitempty"`
	PackageBase     *PackageBaseChange     `json:"packageBase,omitempty"`
	Workspace       *WorkspaceUpdateImpact `json:"workspace,omitempty"`
	// Migrations are the target release's changes to deployment files
	// (ADR 0034), shown in the review and saved with the update.
	Migrations     []DeploymentMigrationResult `json:"migrations,omitempty"`
	SchemaVersion  int                         `json:"schemaVersion"`
	Operation      string                      `json:"operation"`
	GeneratedAt    time.Time                   `json:"generatedAt"`
	State          string                      `json:"state"`
	Repository     string                      `json:"repository"`
	Revision       string                      `json:"revision,omitempty"`
	CurrentRef     string                      `json:"currentRef,omitempty"`
	CurrentRev     string                      `json:"currentRevision,omitempty"`
	CurrentChannel UpdateChannel               `json:"currentChannel,omitempty"`
	Target         string                      `json:"target,omitempty"`
	// TargetRev is the Nixorium revision the candidate locks; on a moving
	// target such as master it tells two revisions of the same name apart.
	TargetRev     string              `json:"targetRevision,omitempty"`
	TargetChannel UpdateChannel       `json:"targetChannel,omitempty"`
	Downgrade     bool                `json:"downgrade"`
	ReviewToken   string              `json:"reviewToken,omitempty"`
	Confirmation  string              `json:"confirmation,omitempty"`
	Diff          GitDiff             `json:"diff"`
	Checks        []UpdateCheck       `json:"checks"`
	Issues        []ValidationIssue   `json:"issues"`
	Snapshot      UpdateInputSnapshot `json:"-"`
	Proposal      UpdateProposal      `json:"-"`
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
