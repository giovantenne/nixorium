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

type backupDueSource interface {
	BackupDue(string) (string, bool)
}

// deferredUpdatesSource summarizes queued client updates: those that will
// still be applied, and those that need attention (stale or failing).
type deferredUpdatesSource interface {
	DeferredUpdateSummary(context.Context, string) (waiting, attention int)
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

	if pending, present, pendingErr := inspector.source.PendingDeployment(); present {
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
			detail = fmt.Sprintf("Update of %s to %s; how it ended is unknown.", started.Local().Format("2006-01-02 15:04"), computerList(affects))
		}
		add(domain.RecoveryDeploymentPending, "DEPLOY-PENDING", detail, "Every controller and client operation, including classroom controls.", since, affects)
	}
	if inspector.source.RemoteReservationPresent() {
		add(domain.RecoveryUSBReserved, "USB-RESERVED", "Resume it to verify the installed computer, or close it if it was abandoned.", "Other controller and client operations.", nil, nil)
	}
	if inspector.source.TemplateResetPending(root) {
		add(domain.RecoveryResetPending, "RESET-PENDING", "Open it to finish or undo the reset.", "Every operation that reads the configuration.", nil, nil)
	}
	listener := inspector.source.ServiceState(ctx, PXEListenerUnit)
	network := inspector.source.ServiceState(ctx, PXENetworkUnit)
	switch mode := ObservePXELifecycle(listener, network, domain.PXEPreparationState{}); mode.Mode {
	case "degraded", "recovery-required":
		add(domain.RecoveryPXE, "PXE-ACTIVE", mode.Detail, "Client operations and a new network installation.", nil, nil)
	}
	if holder, busy := inspector.source.OperationLockHolder(); busy {
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
			detail := fmt.Sprintf("%s will update when switched on.", countLabel(waiting, "computer"))
			if attention > 0 {
				detail = fmt.Sprintf("Queued updates: %s. Their configuration changed after queueing, or the last attempt failed; open the queue to review them.", attentionLabel(attention))
			}
			add(domain.RecoveryDeferredUpdates, "UPDATES-QUEUED", detail, "", nil, nil)
		}
	}
	if len(report.Conditions) > 0 {
		report.State = "attention"
	}
	return report
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
