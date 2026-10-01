package domain

import "time"

// OperationHolder describes the operation holding the shared lock, when it
// could be confirmed against a live process.
type OperationHolder struct {
	Operation string
	User      string
	StartedAt time.Time
	Known     bool
}

// Description says what is running in a few words.
func (h OperationHolder) Description() string {
	if !h.Known {
		return "an operation whose owner is not recorded"
	}
	description := h.Operation
	if h.User != "" && h.User != "root" {
		description += ", started by " + h.User
	} else {
		description += ", started"
	}
	return description + " at " + h.StartedAt.Local().Format("15:04")
}

// BlockingCondition is persistent state that blocks or endangers operations
// until it is resolved. Each condition names its next step.
type BlockingCondition struct {
	Kind    string     `json:"kind"`
	Title   string     `json:"title"`
	Detail  string     `json:"detail,omitempty"`
	Since   *time.Time `json:"since,omitempty"`
	Blocks  string     `json:"blocks,omitempty"`
	Affects []string   `json:"affects,omitempty"`
	Next    NextStep   `json:"next"`
}

type RecoveryReport struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Operation     string              `json:"operation"`
	GeneratedAt   time.Time           `json:"generatedAt"`
	State         string              `json:"state"`
	Conditions    []BlockingCondition `json:"conditions"`
}

// Condition reports whether a condition of the given kind is present.
func (r RecoveryReport) Condition(kind string) (BlockingCondition, bool) {
	for _, condition := range r.Conditions {
		if condition.Kind == kind {
			return condition, true
		}
	}
	return BlockingCondition{}, false
}

// PendingDeployment is the durable record of a client update whose result
// was not observed.
type PendingDeployment struct {
	SchemaVersion int                `json:"schemaVersion"`
	Repository    string             `json:"repository"`
	Revision      string             `json:"revision"`
	Targets       []DeploymentTarget `json:"targets"`
	StartedAt     time.Time          `json:"startedAt"`
	ControllerPID int                `json:"controllerPid"`
}

// Recovery condition kinds.
const (
	RecoveryDeploymentPending = "deployment-pending"
	RecoveryUSBReserved       = "usb-reserved"
	RecoveryResetPending      = "template-reset-pending"
	RecoveryPXE               = "pxe-recovery"
	RecoveryOperationBusy     = "operation-busy"
	RecoverySettingsInvalid   = "settings-invalid"
	RecoveryControllerChanged = "controller-not-applied"
)
