package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"
)

type InternetAction string

const (
	InternetBlock   InternetAction = "block"
	InternetUnblock InternetAction = "unblock"
)

func (a InternetAction) Valid() bool { return a == InternetBlock || a == InternetUnblock }
func (a InternetAction) DesiredState() string {
	if a == InternetBlock {
		return "blocked"
	}
	return "enabled"
}

var internetBootID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type InternetObservation struct {
	SchemaVersion int    `json:"schemaVersion"`
	BootID        string `json:"bootId"`
	State         string `json:"state"`
	Detail        string `json:"detail,omitempty"`
}

func (o InternetObservation) Valid() bool {
	return o.SchemaVersion == SchemaVersion && internetBootID.MatchString(o.BootID) && (o.State == "enabled" || o.State == "blocked" || o.State == "unknown")
}

type InternetTarget struct {
	HostMeta
	Observed InternetObservation `json:"observed"`
	Eligible bool                `json:"eligible"`
}
type InternetPlan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	Requested     string            `json:"requested"`
	Action        InternetAction    `json:"action"`
	Targets       []InternetTarget  `json:"targets"`
	ExpiresAt     time.Time         `json:"expiresAt"`
	ReviewToken   string            `json:"reviewToken,omitempty"`
	Message       string            `json:"message"`
	Issues        []ValidationIssue `json:"issues"`
}

func (p InternetPlan) HasErrors() bool { return p.State != "ready" || len(p.Issues) > 0 }
func InternetReviewToken(p InternetPlan) string {
	p.ReviewToken = ""
	p.Message = ""
	p.Targets = append([]InternetTarget(nil), p.Targets...)
	for i := range p.Targets {
		p.Targets[i].Observed.Detail = ""
	}
	content, _ := json.Marshal(p)
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type InternetOutcome struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}
type InternetReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Action        InternetAction    `json:"action"`
	Targets       []InternetOutcome `json:"targets"`
	Message       string            `json:"message"`
}

func (r InternetReport) HasErrors() bool { return r.State != "completed" }
