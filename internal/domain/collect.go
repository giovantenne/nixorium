package domain

import (
	"time"
)

// Collecting the files of students' desktops into a folder of whoever uses
// the controller: the reverse of sending files.

// Classroom operations for collecting. The service collects into its
// private temporary folder; whoever asked then saves the result into a
// folder of their own (save.go). Requests carry CollectFolder, the folder
// to review, or the reviewed CollectPlan; answers carry CollectPlan or
// CollectReport.
const (
	ClassroomCollectPlanOperation  ClassroomOperation = "collect-plan"
	ClassroomCollectApplyOperation ClassroomOperation = "collect-apply"
)

// collectOperations are the classroom operations of this file.
var collectOperations = map[ClassroomOperation]bool{
	ClassroomCollectPlanOperation:  true,
	ClassroomCollectApplyOperation: true,
}

type CollectTarget struct {
	HostMeta
	Eligible bool   `json:"eligible"`
	Detail   string `json:"detail,omitempty"`
}

// CollectPlan reviews collecting the desktops of the selected computers.
// Destination is the folder the result goes to, as the requester names it;
// the review token binds it with the computers. What is on the desktops is
// read again when the review is applied.
type CollectPlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Requested     string            `json:"requested"`
	Destination   string            `json:"destination"`
	Targets       []CollectTarget   `json:"targets"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Message       string            `json:"message"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p CollectPlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }

// CollectReviewToken binds a review to its destination, targets and expiry.
func CollectReviewToken(p CollectPlan) string {
	p.ReviewToken = ""
	p.Message = ""
	p.Targets = append([]CollectTarget(nil), p.Targets...)
	for i := range p.Targets {
		p.Targets[i].Detail = ""
	}
	return reviewTokenDigest(p)
}

// Outcomes of one computer in a collection.
const (
	CollectCollected = "collected"
	CollectEmpty     = "empty"
	CollectTooMuch   = "too-much"
	CollectNotSent   = "not-sent"
	CollectFailed    = "failed"
)

type CollectOutcome struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
	Items  int    `json:"items,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// CollectReport is the outcome of a collection. Save names the collected
// files kept for the requester, who copies them with save-items and
// save-read; it is empty when nothing was collected.
type CollectReport struct {
	SchemaVersion int              `json:"schemaVersion"`
	Operation     string           `json:"operation"`
	State         string           `json:"state"`
	Targets       []CollectOutcome `json:"targets"`
	Folder        string           `json:"folder,omitempty"`
	Save          string           `json:"save,omitempty"`
	Message       string           `json:"message"`
}

func (r CollectReport) HasErrors() bool { return r.State != "completed" }
