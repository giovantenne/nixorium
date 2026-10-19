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
	// ControllerPIDStart is the start time of that process (field 22 of
	// /proc/PID/stat). With the PID it tells the recorder from an unrelated
	// process that reused the number after a reboot. Older records lack it.
	ControllerPIDStart string `json:"controllerPidStart,omitempty"`
	// Activations, written by the deployment engine, follow each computer
	// from planned (nothing done on it yet) to started (its system profile
	// may have changed) to finished (its activation result is known).
	// Records of older versions have none: every target counts as started.
	Activations []PendingActivation `json:"activations,omitempty"`
	// ApplyReturned: the engine finished its apply and left the record only
	// for computers whose result is unknown; the recorder process no longer
	// acts on any computer, even if it is still open.
	ApplyReturned bool `json:"applyReturned,omitempty"`
}

// Activation states of a computer in a pending client update.
const (
	ActivationPlanned  = "planned"
	ActivationStarted  = "started"
	ActivationFinished = "finished"
)

// Results of a finished activation (switch-to-configuration exit codes).
const (
	ActivationActivated      = "activated"
	ActivationFailedUnits    = "activated-with-failed-units"
	ActivationRebootRequired = "activated-reboot-required"
	ActivationFailed         = "failed"
	ActivationUnknown        = "unknown"
)

// PendingActivation is one computer's progress in a pending client update.
type PendingActivation struct {
	Name         string `json:"name"`
	State        string `json:"state"`
	ActivationID string `json:"activationId,omitempty"`
	Result       string `json:"result,omitempty"`
	ExitCode     *int   `json:"exitCode,omitempty"`
}

// ActivationOf returns the engine's progress for a computer. Records without
// activations (older versions) report every target as started.
func (record PendingDeployment) ActivationOf(name string) PendingActivation {
	for _, activation := range record.Activations {
		if activation.Name == name {
			return activation
		}
	}
	return PendingActivation{Name: name, State: ActivationStarted}
}

// ActivationResult maps the exit code of switch-to-configuration (or 200 when
// the system profile could not be set) to a result.
func ActivationResult(exitCode int) string {
	switch exitCode {
	case 0:
		return ActivationActivated
	case 4:
		return ActivationFailedUnits
	case 100:
		return ActivationRebootRequired
	default:
		return ActivationFailed
	}
}

// ActivationSwitched reports whether a result means the computer now runs the
// new system.
func ActivationSwitched(result string) bool {
	return result == ActivationActivated || result == ActivationFailedUnits || result == ActivationRebootRequired
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
	// RecoveryDeferredUpdates: computers wait for a queued update; it blocks
	// nothing.
	RecoveryDeferredUpdates = "deferred-updates"
	// RecoveryNetworkChanging: a change of laboratory network is saved and
	// not finished; it blocks nothing.
	RecoveryNetworkChanging = "network-changing"
)
