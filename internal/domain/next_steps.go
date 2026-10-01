package domain

import "strings"

// NextStep tells the operator what to do after a refusal or an uncertain
// result. Codes are stable and explained in the troubleshooting guide.
type NextStep struct {
	Code    string `json:"code"`
	Action  string `json:"action"`
	TUI     string `json:"tui,omitempty"`
	Command string `json:"command,omitempty"`
	// Stop means: do not retry; inspect and collect a support report.
	Stop bool `json:"stop,omitempty"`
}

// Messages shared by adapters and the classifier below.
const (
	MessageDeploymentPending = "an unfinished client deployment blocks new operations"
	MessageUSBReserved       = "a USB installation remains reserved"
	MessageResetPending      = "unfinished deployment template reset"
)

var nextSteps = map[string]NextStep{
	"OP-BUSY": {
		Code: "OP-BUSY", Action: "Wait for the running operation to finish, or open its progress from the Overview.",
		TUI: "Overview", Command: "nixorium status",
	},
	"DEPLOY-PENDING": {
		Code: "DEPLOY-PENDING", Action: "Review and finish the interrupted client update before anything else.",
		TUI: "Overview → Interrupted client update", Command: "nixorium deploy recover plan",
	},
	"USB-RESERVED": {
		Code: "USB-RESERVED", Action: "Resume, verify or close the unfinished USB installation.",
		TUI: "Installation → USB over SSH", Command: "nixorium install usb status",
	},
	"RESET-PENDING": {
		Code: "RESET-PENDING", Action: "Finish or undo the interrupted template reset.",
		TUI: "Overview → Interrupted template reset", Command: "nixorium template-reset recover plan",
	},
	"GIT-DIRTY": {
		Code: "GIT-DIRTY", Action: "Save or inspect the uncommitted configuration changes.",
		TUI: "Maintenance → Review Git changes", Command: "nixorium git review",
	},
	"REVIEW-EXPIRED": {
		Code: "REVIEW-EXPIRED", Action: "The review is too old; create a new review and confirm again.",
		TUI: "n New review",
	},
	"REVIEW-CHANGED": {
		Code: "REVIEW-CHANGED", Action: "Something changed after the review; create a new review and confirm again.",
		TUI: "n New review",
	},
	"PXE-ACTIVE": {
		Code: "PXE-ACTIVE", Action: "Finish network installation, or recover controller networking first.",
		TUI: "Installation → Network boot (PXE)", Command: "nixorium pxe stop",
	},
	"SETTINGS-INVALID": {
		Code: "SETTINGS-INVALID", Action: "Repair the laboratory settings before applying anything.",
		TUI: "Maintenance → Change settings", Command: "nixorium config validate",
	},
	"CLIENT-UNCONFIRMED": {
		Code: "CLIENT-UNCONFIRMED", Action: "Check the computer before trying again; do not repeat the request blindly.",
		TUI: "Computers → Computer inventory", Command: "nixorium hosts",
	},
	"DISK-LOW": {
		Code: "DISK-LOW", Action: "Remove old system versions after review.",
		TUI: "Maintenance → Free disk space", Command: "nixorium cleanup plan --on controller",
	},
	"CONTROLLER-NOT-APPLIED": {
		Code: "CONTROLLER-NOT-APPLIED", Action: "Apply the saved configuration to this controller.",
		TUI: "Maintenance → Apply to controller", Command: "nixorium controller plan",
	},
	"BACKUP-DUE": {
		Code: "BACKUP-DUE", Action: "Create an encrypted backup of this controller and keep it, and its passphrase, away from it.",
		TUI: "Maintenance → Back up the controller", Command: "nixorium backup create --to DIRECTORY",
	},
	"EVAL-FAILED": {
		Code: "EVAL-FAILED", Action: "The configuration does not evaluate; run diagnostics and fix the first reported error. Do not retry other operations.",
		TUI: "Maintenance → Diagnostics", Command: "nixorium doctor", Stop: true,
	},
}

// NextStepByCode returns a catalog entry.
func NextStepByCode(code string) (NextStep, bool) {
	step, found := nextSteps[code]
	return step, found
}

// NextStepCodes lists every catalog code.
func NextStepCodes() []string {
	codes := make([]string, 0, len(nextSteps))
	for code := range nextSteps {
		codes = append(codes, code)
	}
	return codes
}

// NextStepFor recognizes the shared blocking situations in an operator
// message. Messages that already carry a code (teacher text) are left alone.
func NextStepFor(message string) (NextStep, bool) {
	text := strings.ToLower(message)
	if strings.Contains(text, "code: ") {
		return NextStep{}, false
	}
	has := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(text, part) {
				return true
			}
		}
		return false
	}
	code := ""
	switch {
	case has(strings.ToLower(MessageDeploymentPending), "pending deployment"):
		code = "DEPLOY-PENDING"
	case has(strings.ToLower(MessageUSBReserved), "usb installation reservation"):
		code = "USB-RESERVED"
	case has(MessageResetPending):
		code = "RESET-PENDING"
	case has("already running", "another nixorium operation is running", "another client operation", "another controller or client operation"):
		code = "OP-BUSY"
	case has("network installation is active", "installation mode is active", "controller network or installation state requires recovery"):
		code = "PXE-ACTIVE"
	case has("managed settings are invalid"):
		code = "SETTINGS-INVALID"
	case has("changed path(s)", "uncommitted changes"):
		code = "GIT-DIRTY"
	case has("expired", "observations are stale", "review is too old"):
		code = "REVIEW-EXPIRED"
	case has("token does not match", "changed after review", "fresh plan", "fresh review", "no longer matches the reviewed"):
		code = "REVIEW-CHANGED"
	case has("could not be confirmed", "not confirmed", "unconfirmed"):
		code = "CLIENT-UNCONFIRMED"
	case has("disk space may be too low", "low nix store disk space"):
		code = "DISK-LOW"
	case has("nix evaluation rejected", "evaluate labmeta"):
		code = "EVAL-FAILED"
	default:
		return NextStep{}, false
	}
	return nextSteps[code], true
}

// Line renders the step for terminals.
func (step NextStep) Line() string {
	line := "Next (" + step.Code + "): " + step.Action
	switch {
	case step.TUI != "" && step.Command != "":
		line += " In Nixorium: " + step.TUI + ". Command: " + step.Command + "."
	case step.TUI != "":
		line += " In Nixorium: " + step.TUI + "."
	case step.Command != "":
		line += " Command: " + step.Command + "."
	}
	return line
}

// WithNextSteps attaches a next step to each recognized issue.
func WithNextSteps(issues []ValidationIssue) []ValidationIssue {
	for index := range issues {
		if issues[index].Next != nil {
			continue
		}
		if step, found := NextStepFor(issues[index].Message); found {
			issues[index].Next = &step
		}
	}
	return issues
}
