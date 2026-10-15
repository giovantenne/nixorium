package domain

import "time"

// GitUndoConfirmation confirms undoing the last saved change.
const GitUndoConfirmation = "UNDO"

// GitUndoFile is one file of the saved change that would be undone.
type GitUndoFile struct {
	Path string `json:"path"`
	// Change says what the saved change did to the file: added, modified,
	// deleted or type-changed. The undo does the opposite.
	Change string `json:"change"`
	// Special marks a symbolic link, a submodule or a name with control
	// characters. Nixorium never undoes those.
	Special bool `json:"-"`
}

// GitUndoFacts are the observations about HEAD that decide whether the last
// saved change can be undone. The adapter only reports them; the application
// layer decides.
type GitUndoFacts struct {
	Revision string
	// Parents are the parents of HEAD; a first saved version has none.
	Parents []string
	// Tree and ParentTree identify the saved states after and before HEAD.
	Tree       string
	ParentTree string
	// Branch is empty when HEAD is detached.
	Branch string
	// Subject is the first line of the message, safe to print.
	Subject string
	Date    time.Time
	// Reverts is the commit that HEAD says it reverts, when it does.
	Reverts string
	Files   []GitUndoFile
	// Truncated is true when the change touches more files than are listed.
	Truncated bool
	// InProgress names an unfinished Git operation (merge, rebase,
	// cherry-pick or revert).
	InProgress string
}

// GitUndoProposal is the checked content of an undo: the saved state before
// HEAD, and the redacted patch that moves the files back to it.
type GitUndoProposal struct {
	TreeID string
	Diff   GitDiff
}

// GitUndoCommit describes the saved change that would be undone.
type GitUndoCommit struct {
	Revision string        `json:"revision"`
	Parent   string        `json:"parent,omitempty"`
	Subject  string        `json:"subject"`
	Date     time.Time     `json:"date"`
	Files    []GitUndoFile `json:"files"`
}

// GitUndoPlanReport is the review of undoing the last saved change.
type GitUndoPlanReport struct {
	SchemaVersion int       `json:"schemaVersion"`
	Operation     string    `json:"operation"`
	GeneratedAt   time.Time `json:"generatedAt"`
	State         string    `json:"state"`
	Repository    string    `json:"repository"`
	// Revision is HEAD: the saved change that will be undone.
	Revision string        `json:"revision,omitempty"`
	Undone   GitUndoCommit `json:"undone"`
	// CommitMessage is the full message of the undo commit; its first line
	// is "Undo: " followed by the subject of the undone change.
	CommitMessage string            `json:"commitMessage,omitempty"`
	TreeID        string            `json:"treeId,omitempty"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Confirmation  string            `json:"confirmation,omitempty"`
	Diff          GitDiff           `json:"diff"`
	Impacts       []SettingImpact   `json:"impacts"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r GitUndoPlanReport) HasErrors() bool {
	return r.State != "ready" || len(r.Issues) > 0
}

// Title is the first line of the undo commit message.
func (r GitUndoPlanReport) Title() string {
	return gitMessageTitle(r.CommitMessage)
}

// GitUndoReport is the outcome of undoing the last saved change.
type GitUndoReport struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Operation        string            `json:"operation"`
	State            string            `json:"state"`
	Repository       string            `json:"repository"`
	PreviousRevision string            `json:"previousRevision,omitempty"`
	Revision         string            `json:"revision,omitempty"`
	CommitMessage    string            `json:"commitMessage,omitempty"`
	Committed        bool              `json:"committed"`
	RetrySafe        bool              `json:"retrySafe"`
	Message          string            `json:"message,omitempty"`
	Impacts          []SettingImpact   `json:"impacts"`
	Issues           []ValidationIssue `json:"issues"`
}

func (r GitUndoReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || r.State == "partial" || len(r.Issues) > 0
}

// Title is the first line of the undo commit message.
func (r GitUndoReport) Title() string {
	return gitMessageTitle(r.CommitMessage)
}

func gitMessageTitle(message string) string {
	for index := 0; index < len(message); index++ {
		if message[index] == '\n' {
			return message[:index]
		}
	}
	return message
}

// GitUndoImpacts say what follows saving the undo, in the wording of a
// settings change: the saved configuration is not applied by saving it. A
// controller-only deployment has no computers to update.
func GitUndoImpacts(clients bool) []SettingImpact {
	impacts := []SettingImpact{{Kind: "controller-apply", Detail: "Apply to controller so this controller uses the configuration saved before that change."}}
	if clients {
		impacts = append(impacts, SettingImpact{Kind: "client-update", Detail: "Update computers so the clients use it too; offline computers keep the changed configuration until updated."})
	}
	return impacts
}
