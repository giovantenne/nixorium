package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// RecoverySource observes persistent blockers cheaply: file state, locks and
// service state only, never Nix evaluation or the network.
type RecoverySource interface {
	PendingDeployment() (domain.PendingDeployment, bool, error)
	RemoteReservationPresent() bool
	TemplateResetPending(string) bool
	OperationLockHolder() (domain.OperationHolder, bool)
	LabSettingsIssues(string) []domain.ValidationIssue
	ControllerActivationDrift() string
	ServiceState(context.Context, string) domain.ServiceState
}

// recorderSource tells whether the process that wrote the pending record is
// still alive (the same PID and start time).
type recorderSource interface {
	ProcessRunning(pid int, start string) bool
}

// updateRunning reports a pending record whose update is still under way:
// its recorder is alive, has not returned from its apply, and holds the
// operation lock. The Overview then shows the running operation, not an
// interrupted update. A recorder that is alive but no longer holds the lock
// left its record behind, so the record is reported.
func (inspector *RecoveryInspector) updateRunning(pending domain.PendingDeployment, err error, busy bool) bool {
	recorder, ok := inspector.source.(recorderSource)
	return ok && err == nil && busy && !pending.ApplyReturned && pending.ControllerPID > 0 &&
		recorder.ProcessRunning(pending.ControllerPID, pending.ControllerPIDStart)
}

type backupDueSource interface {
	BackupDue(string) (string, bool)
}

// deferredUpdatesSource summarizes queued client updates: those that will
// still be applied, and those that need attention (stale or failing).
type deferredUpdatesSource interface {
	DeferredUpdateSummary(context.Context, string) (waiting, attention int)
}

// networkChangeSource reads a saved change of laboratory network and the
// controller's addresses, both locally.
type networkChangeSource interface {
	SavedNetworkChange(string) (domain.LabSettings, bool)
	InterfaceAddresses(string) ([]string, error)
}

type RecoveryInspector struct {
	source RecoverySource
	now    func() time.Time
}

func NewRecoveryInspector(source RecoverySource) *RecoveryInspector {
	return &RecoveryInspector{source: source, now: time.Now}
}

func (inspector *RecoveryInspector) Observe(ctx context.Context, repository string) domain.RecoveryReport {
	report := domain.RecoveryReport{SchemaVersion: domain.SchemaVersion, Operation: "recovery-status", GeneratedAt: inspector.now().UTC(), State: "clear", Conditions: []domain.BlockingCondition{}}
	// Titles come from the situation catalogue so the Overview, failure
	// notices, the recovery screens and the guide say the same sentence.
	add := func(kind, code, detail, blocks string, since *time.Time, affects []string) {
		step, _ := domain.NextStepByCode(code)
		report.Conditions = append(report.Conditions, domain.BlockingCondition{Kind: kind, Title: step.Title, Detail: detail, Since: since, Blocks: blocks, Affects: affects, Next: step})
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		root = repository
	}

	holder, busy := inspector.source.OperationLockHolder()
	if pending, present, pendingErr := inspector.source.PendingDeployment(); present && !inspector.updateRunning(pending, pendingErr, busy) {
		detail := ""
		var since *time.Time
		var affects []string
		if pendingErr != nil {
			detail = "The record of the interrupted update cannot be read (" + pendingErr.Error() + "). Keep it as it is and collect a support report from Maintenance → Diagnostics."
		} else {
			started := pending.StartedAt
			since = &started
			for _, target := range pending.Targets {
				affects = append(affects, target.Name)
			}
			detail = fmt.Sprintf("Update of %s started %s; how it ended is unknown.", computerList(affects), started.Local().Format("2006-01-02 15:04"))
		}
		add(domain.RecoveryDeploymentPending, "DEPLOY-PENDING", detail, "Every other maintenance operation, and classroom controls on the computers it names. Update Nixorium and Apply to controller stay available.", since, affects)
	}
	if inspector.source.RemoteReservationPresent() {
		add(domain.RecoveryUSBReserved, "USB-RESERVED", "Resume it to verify the installed computer, or close it if it was abandoned.", "Other controller and client operations. Apply to controller waits only while an installation is under way or unresolved.", nil, nil)
	}
	if inspector.source.TemplateResetPending(root) {
		add(domain.RecoveryResetPending, "RESET-PENDING", "Open it to finish or undo the reset.", "Every operation that reads the configuration.", nil, nil)
	}
	listener := inspector.source.ServiceState(ctx, PXEListenerUnit)
	network := inspector.source.ServiceState(ctx, PXENetworkUnit)
	switch mode := ObservePXELifecycle(listener, network, domain.PXEPreparationState{}); mode.Mode {
	case "degraded", "recovery-required":
		add(domain.RecoveryPXE, "PXE-ACTIVE", pxeRecoveryDetail(mode), "Client operations and a new network installation.", nil, nil)
	}
	if busy {
		detail := "Other operations wait until it finishes."
		if holder.Known {
			detail = upperFirst(holder.Description()) + ". " + detail
		}
		add(domain.RecoveryOperationBusy, "OP-BUSY", detail, "Controller and client operations.", nil, nil)
	}
	if issues := inspector.source.LabSettingsIssues(root); len(issues) > 0 {
		detail := issues[0].Field + ": " + issues[0].Message
		if len(issues) > 1 {
			detail += fmt.Sprintf(" (and %d more)", len(issues)-1)
		}
		add(domain.RecoverySettingsInvalid, "SETTINGS-INVALID", detail, "Saving settings, applying to the controller and updating computers.", nil, nil)
	}
	if drift := inspector.source.ControllerActivationDrift(); drift != "" {
		add(domain.RecoveryControllerChanged, "CONTROLLER-NOT-APPLIED", upperFirst(drift)+".", "", nil, nil)
	}
	if backups, ok := inspector.source.(backupDueSource); ok {
		if reason, due := backups.BackupDue(root); due {
			add(domain.RecoveryBackupDue, "BACKUP-DUE", reason, "", nil, nil)
		}
	}
	if queued, ok := inspector.source.(deferredUpdatesSource); ok {
		if waiting, attention := queued.DeferredUpdateSummary(ctx, root); waiting+attention > 0 {
			// One condition: an entry that needs a review must not be
			// presented as "nothing to do", whatever else is still waiting.
			code, detail := "UPDATES-QUEUED", fmt.Sprintf("%s will update when switched on.", countLabel(waiting, "computer"))
			if attention > 0 {
				code = "UPDATES-NEED-REVIEW"
				detail = "Queued updates: " + attentionLabel(attention)
				if waiting > 0 {
					detail += fmt.Sprintf(", %s will update when switched on", countLabel(waiting, "computer"))
				}
				detail += ". Open the queue to review: a configuration changed after queueing, or an update kept failing."
			}
			add(domain.RecoveryDeferredUpdates, code, detail, "", nil, nil)
		}
	}
	if changes, ok := inspector.source.(networkChangeSource); ok {
		if lab, changing := changes.SavedNetworkChange(root); changing {
			add(domain.RecoveryNetworkChanging, "NETWORK-CHANGING", domain.NetworkChangingDetail(lab, controllerOnBothNetworks(changes, lab)), "", nil, nil)
		}
	}
	if len(report.Conditions) > 0 {
		report.State = "attention"
	}
	return report
}

// controllerOnBothNetworks reports whether the controller already carries
// its new and its previous laboratory address, that is, whether it was
// applied since the change was saved.
func controllerOnBothNetworks(source networkChangeSource, lab domain.LabSettings) bool {
	current, err := domain.ControllerStaticAddress(lab)
	if err != nil || lab.NetworkTransition == nil {
		return false
	}
	previousLab := lab
	previousLab.NetworkBase, previousLab.NetworkPrefix = lab.NetworkTransition.PreviousBase, lab.NetworkTransition.PreviousPrefixLength
	previous, err := domain.ControllerStaticAddress(previousLab)
	if err != nil {
		return false
	}
	addresses, err := source.InterfaceAddresses(lab.ControllerInterface())
	return err == nil && containsIP(addresses, current) && containsIP(addresses, previous)
}

// pxeRecoveryDetail says in plain words how an interrupted network
// installation left the controller.
func pxeRecoveryDetail(state domain.PXELifecycleState) string {
	if state.Mode == "recovery-required" {
		return "It was interrupted: the controller still uses its temporary network setup. Open it to recover the controller network."
	}
	return "It was interrupted: one of its services stopped or failed. Open it to recover the controller network."
}

// computerList names a few computers and counts the rest.
func computerList(names []string) string {
	if len(names) <= 4 {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s, … (%d computers)", strings.Join(names[:3], ", "), len(names))
}

func upperFirst(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
