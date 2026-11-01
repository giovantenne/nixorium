package domain

import (
	"time"
)

// BroadcastPlan reviews showing the teacher's screen on students' computers.
type BroadcastPlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Requested     string            `json:"requested"`
	Targets       []LockTarget      `json:"targets"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Message       string            `json:"message"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p BroadcastPlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }

func BroadcastReviewToken(p BroadcastPlan) string {
	p.ReviewToken = ""
	p.Message = ""
	p.Targets = append([]LockTarget(nil), p.Targets...)
	for i := range p.Targets {
		p.Targets[i].Detail = ""
	}
	return reviewTokenDigest(p)
}
