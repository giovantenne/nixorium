package domain

import "time"

type GitChange struct {
	Path         string `json:"path"`
	OriginalPath string `json:"originalPath,omitempty"`
	Staged       string `json:"staged,omitempty"`
	Unstaged     string `json:"unstaged,omitempty"`
	Untracked    bool   `json:"untracked"`
	Managed      bool   `json:"managed"`
	Private      bool   `json:"private"`
}

type GitDiff struct {
	Scope     string `json:"scope"`
	Content   string `json:"content,omitempty"`
	Truncated bool   `json:"truncated"`
}

type GitChangeSummary struct {
	Staged     int `json:"staged"`
	Unstaged   int `json:"unstaged"`
	Untracked  int `json:"untracked"`
	Managed    int `json:"managed"`
	Unexpected int `json:"unexpected"`
	Private    int `json:"private"`
}

type GitReviewSnapshot struct {
	Changes []GitChange
	Diffs   []GitDiff
}

type GitReviewReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	GeneratedAt   time.Time         `json:"generatedAt"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Revision      string            `json:"revision,omitempty"`
	Summary       GitChangeSummary  `json:"summary"`
	Changes       []GitChange       `json:"changes"`
	Diffs         []GitDiff         `json:"diffs"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r GitReviewReport) HasErrors() bool {
	return r.State == "blocked" || r.State == "failed" || len(r.Issues) > 0
}
