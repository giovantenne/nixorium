package domain

import (
	"errors"
	"fmt"
	"strings"
)

// NextStep is one entry of the situation catalogue: a blocking or uncertain
// state, why Nixorium stops there, and the way forward. Codes are stable and
// explained in the troubleshooting guide; every screen, the CLI and the
// guide use the same Title so the operator meets one sentence everywhere.
type NextStep struct {
	Code string `json:"code"`
	// Title says what is wrong in one plain sentence, without
	// implementation words.
	Title string `json:"title,omitempty"`
	// Why says what Nixorium protects by stopping here.
	Why string `json:"why,omitempty"`
	// Action is the verb phrase used in key hints and "Next:" lines.
	Action  string `json:"action"`
	TUI     string `json:"tui,omitempty"`
	Command string `json:"command,omitempty"`
	// Screen names the dedicated TUI flow (a BlockingCondition kind) that
	// resolves this situation; empty when there is none.
	Screen string `json:"screen,omitempty"`
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
	"DEPLOY-PENDING": {
		Code:   "DEPLOY-PENDING",
		Title:  "A previous update of the student computers was interrupted",
		Why:    "Nixorium does not know how it ended on every computer, so the student computers wait until you review it.",
		Action: "Review the interrupted update",
		TUI:    "Overview", Command: "nixorium deploy recover plan", Screen: RecoveryDeploymentPending,
	},
	"OP-BUSY": {
		Code:   "OP-BUSY",
		Title:  "Another Nixorium operation is still running",
		Why:    "Two operations cannot change the laboratory at the same time.",
		Action: "Wait for it to finish, or open its progress from the Overview",
		TUI:    "Overview", Command: "nixorium status",
	},
	"USB-RESERVED": {
		Code:   "USB-RESERVED",
		Title:  "A USB installation was not finished",
		Why:    "The computer may be half installed; Nixorium keeps it reserved until you resume or close that installation.",
		Action: "Resume or close the USB installation",
		TUI:    "Installation → USB over SSH", Command: "nixorium install usb status", Screen: RecoveryUSBReserved,
	},
	"RESET-PENDING": {
		Code:   "RESET-PENDING",
		Title:  "A reset of the configuration template was interrupted",
		Why:    "The configuration may be half replaced. The previous version is kept as a backup.",
		Action: "Finish or undo the reset",
		TUI:    "Overview", Command: "nixorium template-reset recover plan", Screen: RecoveryResetPending,
	},
	"GIT-DIRTY": {
		Code:   "GIT-DIRTY",
		Title:  "The configuration has unsaved changes",
		Why:    "Operations use the last saved version; unsaved changes would be left out or lost.",
		Action: "Review the changes and save or discard them",
		TUI:    "Maintenance → Review Git changes", Command: "nixorium git review",
	},
	"REVIEW-EXPIRED": {
		Code:   "REVIEW-EXPIRED",
		Title:  "The review is too old",
		Why:    "Nixorium only applies what it checked recently.",
		Action: "Create a new review and confirm again",
		TUI:    "n New review",
	},
	"REVIEW-CHANGED": {
		Code:   "REVIEW-CHANGED",
		Title:  "Something changed after the review",
		Why:    "Nixorium only applies exactly what you reviewed.",
		Action: "Create a new review and confirm again",
		TUI:    "n New review",
	},
	"PXE-ACTIVE": {
		Code:   "PXE-ACTIVE",
		Title:  "Network installation is still active",
		Why:    "While the controller serves installations it uses a temporary network setup.",
		Action: "Finish the network installation, or recover the controller network",
		TUI:    "Installation → Network boot (PXE)", Command: "nixorium pxe stop", Screen: RecoveryPXE,
	},
	"SETTINGS-INVALID": {
		Code:   "SETTINGS-INVALID",
		Title:  "The laboratory settings are not valid",
		Why:    "Nixorium refuses to apply settings it cannot check.",
		Action: "Repair the settings",
		TUI:    "Maintenance → Change settings", Command: "nixorium config validate", Screen: RecoverySettingsInvalid,
	},
	"CLIENT-UNCONFIRMED": {
		Code:   "CLIENT-UNCONFIRMED",
		Title:  "A computer did not confirm the request",
		Why:    "It may have done it anyway; repeating blindly could do it twice.",
		Action: "Check the computer in Computer inventory before trying again",
		TUI:    "Computers → Computer inventory", Command: "nixorium hosts",
	},
	"DISK-LOW": {
		Code:   "DISK-LOW",
		Title:  "The controller is low on disk space",
		Why:    "A build needs room in the Nix store.",
		Action: "Free disk space",
		TUI:    "Maintenance → Free disk space", Command: "nixorium cleanup plan --on controller",
	},
	"CONTROLLER-NOT-APPLIED": {
		Code:   "CONTROLLER-NOT-APPLIED",
		Title:  "The controller is not running its saved configuration",
		Why:    "This happens after an interrupted update or after starting an older version from the boot menu.",
		Action: "Apply the saved configuration to the controller",
		TUI:    "Maintenance → Apply to controller", Command: "nixorium controller plan", Screen: RecoveryControllerChanged,
	},
	"UPDATES-QUEUED": {
		Code:   "UPDATES-QUEUED",
		Title:  "Some computers will update when they are switched on",
		Why:    "The controller updates each of them as soon as it answers.",
		Action: "Nothing to do",
		TUI:    "Computers → Queued updates", Command: "nixorium deploy queue status", Screen: RecoveryDeferredUpdates,
	},
	"BACKUP-DUE": {
		Code:   "BACKUP-DUE",
		Title:  "A backup of this laboratory is due",
		Why:    "Keys and settings changed since the last one.",
		Action: "Back up the lab",
		TUI:    "Maintenance → Back up lab", Command: "nixorium backup plan --remote git@github.com:OWNER/REPOSITORY.git --branch main", Screen: RecoveryBackupDue,
	},
	"NETWORK-LOCKED": {
		Code:   "NETWORK-LOCKED",
		Title:  "The laboratory network cannot be changed while computers are configured",
		Why:    "The computers would keep their old addresses and lose contact with the controller.",
		Action: "Keep the saved network values here and move the computers with the guided change of the laboratory network",
		TUI:    "Maintenance → Change network addressing",
	},
	"NETWORK-CHANGING": {
		Code:   "NETWORK-CHANGING",
		Title:  "The laboratory network is being changed",
		Why:    "Until the change is finished the controller answers on both networks, so computers still on the previous one stay reachable.",
		Action: "Continue the change of laboratory network: update the computers, then finish or undo it",
		TUI:    "Maintenance → Change network addressing", Command: "nixorium network migrate status", Screen: RecoveryNetworkChanging,
	},
	"EVAL-FAILED": {
		Code:   "EVAL-FAILED",
		Title:  "The configuration has an error",
		Why:    "Nothing can be built until it is fixed.",
		Action: "Open Diagnostics and fix the first reported error",
		TUI:    "Maintenance → Diagnostics", Command: "nixorium doctor", Stop: true,
	},
	"PIN-OUTDATED": {
		Code:   "PIN-OUTDATED",
		Title:  "The saved Nixorium version is recorded inconsistently",
		Why:    "The student computers are built from a fixed copy of the saved configuration, which cannot correct this record by itself.",
		Action: "Repeat the update for the configured Nixorium version",
		TUI:    "Maintenance → Update Nixorium", Command: "nixorium update check",
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

// BlockedError is a refusal caused by a catalogued situation. Its text stays
// the technical message (logs, JSON output and support reports keep it);
// screens use Code to show the catalogue entry instead.
type BlockedError struct {
	Code string
	Err  error
}

// Blocked wraps err as a refusal for the catalogued situation code.
func Blocked(code string, err error) error {
	return &BlockedError{Code: code, Err: err}
}

func (err *BlockedError) Error() string {
	if err.Err == nil {
		return err.Code
	}
	return err.Err.Error()
}

func (err *BlockedError) Unwrap() error {
	return err.Err
}

// BlockedCode lets errors with their own type (such as a held operation
// lock) name their catalogued situation.
func (err *BlockedError) BlockedCode() string {
	return err.Code
}

type blockedCoder interface {
	BlockedCode() string
}

// BlockedCodeOf returns the catalogued situation of a typed refusal.
func BlockedCodeOf(err error) (string, bool) {
	var coder blockedCoder
	if err != nil && errors.As(err, &coder) {
		if _, found := nextSteps[coder.BlockedCode()]; found {
			return coder.BlockedCode(), true
		}
	}
	return "", false
}

// EvaluationError reports a failed Nix evaluation as EVAL-FAILED unless the
// cause already names a more specific situation.
func EvaluationError(what string, err error) error {
	wrapped := fmt.Errorf("evaluate %s: %w", what, err)
	if _, typed := BlockedCodeOf(err); typed {
		return wrapped
	}
	return Blocked("EVAL-FAILED", wrapped)
}

// EvaluationIssue is the validation issue form of EvaluationError.
func EvaluationIssue(field, what string, err error) ValidationIssue {
	code, typed := BlockedCodeOf(err)
	if !typed {
		code = "EVAL-FAILED"
	}
	return BlockedIssue(field, code, fmt.Sprintf("evaluate %s: %v", what, err))
}

// NextStepForError recognizes a catalogued situation in an error: a typed
// refusal first, then the text of the error.
func NextStepForError(err error) (NextStep, bool) {
	if err == nil {
		return NextStep{}, false
	}
	var coder blockedCoder
	if errors.As(err, &coder) {
		if step, found := nextSteps[coder.BlockedCode()]; found {
			return step, true
		}
	}
	return NextStepFor(err.Error())
}

// NextStepFor recognizes the shared blocking situations in an operator
// message. Messages that already carry a code (teacher text) are left alone.
// Typed refusals are recognized by NextStepForError before their text is
// matched here.
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
	case has("managed settings are invalid", "deployment settings are invalid"):
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
	case has("flake.lock does not match flake.nix", "requires lock file changes", `flake.lock": read-only file system`):
		code = "PIN-OUTDATED"
	case has("nix evaluation rejected", "evaluate labmeta"):
		code = "EVAL-FAILED"
	default:
		return NextStep{}, false
	}
	return nextSteps[code], true
}

// Headline states the situation after what the caller tried, for example
// "The controller was not updated: a previous update ... was interrupted."
// Without a prefix it is the Title as a sentence.
func (step NextStep) Headline(prefix string) string {
	if prefix == "" {
		return sentence(step.Title)
	}
	title := step.Title
	if title != "" {
		title = strings.ToLower(title[:1]) + title[1:]
	}
	return strings.TrimRight(prefix, ".: ") + ": " + sentence(title)
}

func sentence(text string) string {
	if text == "" || strings.HasSuffix(text, ".") {
		return text
	}
	return text + "."
}

// Line renders the step for terminals.
func (step NextStep) Line() string {
	line := "Next (" + step.Code + "): " + sentence(step.Action)
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

// BlockedIssue is a validation issue for a catalogued situation; it carries
// its next step even when the message does not name the situation.
func BlockedIssue(field, code, message string) ValidationIssue {
	issue := ValidationIssue{Field: field, Message: message}
	if step, found := nextSteps[code]; found {
		issue.Next = &step
	}
	return issue
}
