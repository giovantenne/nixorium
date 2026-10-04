package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// LockAction locks or unlocks students' computers: the classroom extension
// covers the screen and takes the keyboard and mouse.
type LockAction string

const (
	LockOn  LockAction = "lock"
	LockOff LockAction = "unlock"
)

func (a LockAction) Valid() bool { return a == LockOn || a == LockOff }

// Locked is the state the action leads to.
func (a LockAction) Locked() bool { return a == LockOn }

type LockTarget struct {
	HostMeta
	// Reachable means the computer answered with a signed-in session.
	Reachable bool   `json:"reachable"`
	Locked    bool   `json:"locked"`
	Eligible  bool   `json:"eligible"`
	Detail    string `json:"detail,omitempty"`
}

type LockPlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Requested     string            `json:"requested"`
	Action        LockAction        `json:"action"`
	Targets       []LockTarget      `json:"targets"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Message       string            `json:"message"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p LockPlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }

// LockReviewToken binds a review to its action, targets and expiry.
func LockReviewToken(p LockPlan) string {
	p.ReviewToken = ""
	p.Message = ""
	p.Targets = append([]LockTarget(nil), p.Targets...)
	for i := range p.Targets {
		p.Targets[i].Detail = ""
	}
	content, _ := json.Marshal(p)
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type LockOutcome struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type LockReport struct {
	SchemaVersion int           `json:"schemaVersion"`
	Operation     string        `json:"operation"`
	State         string        `json:"state"`
	Action        LockAction    `json:"action"`
	Targets       []LockOutcome `json:"targets"`
	Message       string        `json:"message"`
}

func (r LockReport) HasErrors() bool { return r.State != "completed" }
