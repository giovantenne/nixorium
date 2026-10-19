package domain

import (
	"strings"
	"time"
)

type DeploymentPhase string

const (
	DeploymentPhasePreflight DeploymentPhase = "preflight"
	DeploymentPhaseBuild     DeploymentPhase = "build"
	DeploymentPhaseApply     DeploymentPhase = "apply"
	DeploymentPhaseVerify    DeploymentPhase = "verify"
	DeploymentPhaseComplete  DeploymentPhase = "complete"
)

type DeploymentTarget struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	// PreviousIP is set during a change of laboratory network: a computer
	// that answers only there is updated over it and checked on both.
	PreviousIP string `json:"previousIp,omitempty"`
	// Class is the computer's class (HostMeta.Class).
	Class string `json:"class,omitempty"`
}

// SharedSystem reports a computer whose system is shared by its class
// (ADR 0033): it takes name and address from its identity record, which the
// deployment writes before switching it.
func (target DeploymentTarget) SharedSystem() bool {
	return target.Class != "" && target.Class != target.Name
}

// DeploymentConfigurations counts the systems an update builds: one per
// shared class and one per computer with its own system (ADR 0033).
func DeploymentConfigurations(targets []DeploymentTarget) int {
	seen := map[string]bool{}
	for _, target := range targets {
		seen[ClientSystemName(target.Name, target.Class)] = true
	}
	return len(seen)
}

// ClientSystemName is the name inside a client's system store path: its own
// name, or `nixorium-client-<class>` when it shares its class's system
// (ADR 0033). An empty class is a per-host system.
func ClientSystemName(host, class string) string {
	if class == "" || class == host {
		return host
	}
	return "nixorium-client-" + class
}

// ValidClientClass accepts the class of a client: none (metadata before
// classes), its own name, or a shared class.
func ValidClientClass(host, class string) bool {
	if class == "" || class == host {
		return true
	}
	digits, found := strings.CutPrefix(class, "shared-")
	if !found || digits == "" || len(digits) > 4 {
		return false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// ClientNumber is the number in a client name such as pc07.
func ClientNumber(name string) (int, bool) {
	digits, found := strings.CutPrefix(name, "pc")
	if !found || digits == "" || len(digits) > 4 {
		return 0, false
	}
	number := 0
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return 0, false
		}
		number = number*10 + int(digit-'0')
	}
	return number, number > 0
}

// DeploymentProgress is an in-process, bounded update authored by the
// application while a foreground deployment is running. Output is a bounded,
// terminal-safe tail for the administrator; full output stays in the private log.
type DeploymentProgress struct {
	Phase         DeploymentPhase
	Completed     int
	Total         int
	TargetCurrent int
	TargetTotal   int
	Activity      string
	Output        []string
	LastOutputAt  time.Time
}

// DeploymentUnconfirmedError means a durable pending marker must be recovered
// before another operation. A disconnected command is not remote cancellation.
type DeploymentUnconfirmedError struct{ Err error }

func (e *DeploymentUnconfirmedError) Error() string { return e.Err.Error() }
func (e *DeploymentUnconfirmedError) Unwrap() error { return e.Err }

type DeploymentPlanReport struct {
	Availability       []DeploymentTargetAvailability `json:"availability,omitempty"`
	ReachableRequested string                         `json:"reachableRequested,omitempty"`
	SchemaVersion      int                            `json:"schemaVersion"`
	Operation          string                         `json:"operation"`
	State              string                         `json:"state"`
	Repository         string                         `json:"repository"`
	Requested          string                         `json:"requested"`
	Revision           string                         `json:"revision,omitempty"`
	ColmenaSelector    string                         `json:"colmenaSelector,omitempty"`
	Targets            []DeploymentTarget             `json:"targets"`
	BuildFirst         bool                           `json:"buildFirst"`
	Issues             []ValidationIssue              `json:"issues"`
}

// Availability is a best-effort TCP/SSH-port observation, not authentication,
// power-state evidence or permission to change the reviewed target set.
type DeploymentTargetAvailability struct {
	Name         string          `json:"name"`
	IP           string          `json:"ip"`
	Reachability Reachability    `json:"reachability"`
	SSH          SSHAvailability `json:"ssh"`
	Network      HostNetwork     `json:"network,omitempty"`
}

func (r DeploymentPlanReport) HasErrors() bool {
	return len(r.Issues) > 0
}

type DeploymentExecutionReport struct {
	SchemaVersion    int                           `json:"schemaVersion"`
	Operation        string                        `json:"operation"`
	State            string                        `json:"state"`
	Repository       string                        `json:"repository"`
	Requested        string                        `json:"requested"`
	Revision         string                        `json:"revision,omitempty"`
	ColmenaSelector  string                        `json:"colmenaSelector,omitempty"`
	Targets          []DeploymentTarget            `json:"targets"`
	Phase            DeploymentPhase               `json:"phase"`
	BuildCompleted   bool                          `json:"buildCompleted"`
	ApplyCompleted   bool                          `json:"applyCompleted"`
	Verification     DeploymentVerificationSummary `json:"verification"`
	RetrySafe        bool                          `json:"retrySafe"`
	RecoveryRequired bool                          `json:"recoveryRequired,omitempty"`
	LogPath          string                        `json:"logPath,omitempty"`
	Message          string                        `json:"message,omitempty"`
	Issues           []ValidationIssue             `json:"issues"`
	// Queued names reviewed computers that were off: they get this revision
	// when they answer again (deferred updates).
	Queued []string `json:"queued,omitempty"`
	// Skipped names reviewed computers that stopped answering after the
	// build. Nothing was started on them, so they are never pending.
	Skipped []string `json:"skipped,omitempty"`
}

type LastSuccessfulDeployment struct {
	Revision   string    `json:"revision"`
	SystemPath string    `json:"systemPath"`
	VerifiedAt time.Time `json:"verifiedAt"`
}

type DeploymentTargetVerification struct {
	Reachability     Reachability     `json:"reachability,omitempty"`
	SSH              SSHAvailability  `json:"ssh,omitempty"`
	HostKeyCondition HostKeyCondition `json:"hostKeyCondition,omitempty"`
	Name             string           `json:"name"`
	State            string           `json:"state"`
	Revision         string           `json:"revision,omitempty"`
	SystemPath       string           `json:"systemPath,omitempty"`
	Detail           string           `json:"detail,omitempty"`
	Network          HostNetwork      `json:"network,omitempty"`
}

type DeploymentVerificationSummary struct {
	Attempted int                            `json:"attempted"`
	Verified  int                            `json:"verified"`
	Recorded  int                            `json:"recorded"`
	Targets   []DeploymentTargetVerification `json:"targets"`
	Detail    string                         `json:"detail,omitempty"`
}

type DeploymentHistory struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	Repository    string                              `json:"repository"`
	Hosts         map[string]LastSuccessfulDeployment `json:"hosts"`
}

func (r DeploymentExecutionReport) HasErrors() bool {
	return r.RecoveryRequired || r.State == "blocked" || r.State == "failed" || r.State == "partial" || len(r.Issues) > 0
}
