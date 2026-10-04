package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ShareFile is one file or folder sent to students' desktops, by its path
// relative to the sent folder; files carry their SHA-256.
type ShareFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size,omitempty"`
	Dir    bool   `json:"dir,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type ShareTarget struct {
	HostMeta
	Eligible bool   `json:"eligible"`
	Detail   string `json:"detail,omitempty"`
}

// SharePlan reviews sending prepared files to students' desktops. Transfer
// names the prepared copy on the controller; the review token binds the
// files themselves, so the same files prepared again keep the token.
type SharePlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Requested     string            `json:"requested"`
	Transfer      string            `json:"transfer"`
	Files         []ShareFile       `json:"files"`
	Bytes         int64             `json:"bytes"`
	Targets       []ShareTarget     `json:"targets"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Message       string            `json:"message"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p SharePlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }

func ShareReviewToken(p SharePlan) string {
	p.ReviewToken = ""
	p.Message = ""
	p.Transfer = ""
	p.Targets = append([]ShareTarget(nil), p.Targets...)
	for i := range p.Targets {
		p.Targets[i].Detail = ""
	}
	content, _ := json.Marshal(p)
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type ShareOutcome struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type ShareReport struct {
	SchemaVersion int            `json:"schemaVersion"`
	Operation     string         `json:"operation"`
	State         string         `json:"state"`
	Targets       []ShareOutcome `json:"targets"`
	Message       string         `json:"message"`
}

func (r ShareReport) HasErrors() bool { return r.State != "completed" }
