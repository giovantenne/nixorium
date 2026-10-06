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
	add := func(kind, code, title, detail, blocks string, since *time.Time, affects []string) {
		step, _ := domain.NextStepByCode(code)
		report.Conditions = append(report.Conditions, domain.BlockingCondition{Kind: kind, Title: title, Detail: detail, Since: since, Blocks: blocks, Affects: affects, Next: step})
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
			detail = "The record cannot be read safely: " + pendingErr.Error() + ". Keep it and collect a support report."
		} else {
			started := pending.StartedAt
			since = &started
			for _, target := range pending.Targets {
				affects = append(affects, target.Name)
			}
			detail = fmt.Sprintf("Revision %s to %s, started %s. Its result was not observed.", shortRevision(pending.Revision), strings.Join(affects, ", "), started.Local().Format("2006-01-02 15:04"))
		}
		add(domain.RecoveryDeploymentPending, "DEPLOY-PENDING", "An interrupted client update blocks other operations", detail, "Every controller and client operation, including classroom controls.", since, affects)
	}
	if inspector.source.RemoteReservationPresent() {
		add(domain.RecoveryUSBReserved, "USB-RESERVED", "A USB installation is unfinished", "Resume it to verify the installed computer, or close it if it was abandoned.", "Other controller and client operations.", nil, nil)
	}
	if inspector.source.TemplateResetPending(root) {
		add(domain.RecoveryResetPending, "RESET-PENDING", "A deployment template reset was interrupted", "The configuration may be half replaced; the backup of the previous configuration is kept.", "Every operation that reads the configuration.", nil, nil)
	}
	listener := inspector.source.ServiceState(ctx, PXEListenerUnit)
	network := inspector.source.ServiceState(ctx, PXENetworkUnit)
	switch mode := ObservePXELifecycle(listener, network, domain.PXEPreparationState{}); mode.Mode {
	case "degraded", "recovery-required":
		add(domain.RecoveryPXE, "PXE-ACTIVE", "Controller network recovery required", mode.Detail, "Client operations and a new network installation.", nil, nil)
	}
	if holder, busy := inspector.source.OperationLockHolder(); busy {
		title := "Another operation is running"
		if holder.Known {
			title = holder.Description()
		}
		add(domain.RecoveryOperationBusy, "OP-BUSY", title, "Other operations wait until it finishes.", "Controller and client operations.", nil, nil)
	}
	if issues := inspector.source.LabSettingsIssues(root); len(issues) > 0 {
		detail := issues[0].Field + ": " + issues[0].Message
		if len(issues) > 1 {
			detail += fmt.Sprintf(" (and %d more)", len(issues)-1)
		}
		add(domain.RecoverySettingsInvalid, "SETTINGS-INVALID", "The laboratory settings are not valid", detail, "Saving settings, applying to the controller and updating computers.", nil, nil)
	}
	if drift := inspector.source.ControllerActivationDrift(); drift != "" {
		add(domain.RecoveryControllerChanged, "CONTROLLER-NOT-APPLIED", "This controller is not running its last applied configuration", "This happens after starting an older version from the boot menu or after an interrupted activation; "+drift+".", "", nil, nil)
	}
	if backups, ok := inspector.source.(backupDueSource); ok {
		if reason, due := backups.BackupDue(root); due {
			add(domain.RecoveryBackupDue, "BACKUP-DUE", "A backup of this controller is due", reason, "", nil, nil)
		}
	}
	if queued, ok := inspector.source.(deferredUpdatesSource); ok {
		if waiting, attention := queued.DeferredUpdateSummary(ctx, root); waiting+attention > 0 {
			title := fmt.Sprintf("%s will update when switched on", countLabel(waiting, "computer"))
			detail := "The controller updates each of them as soon as it answers."
			if attention > 0 {
				title = fmt.Sprintf("Queued updates: %s need attention", countLabel(attention, "computer"))
				detail = "Their configuration changed after queueing, or the last attempt failed; open the queue to review them."
			}
			add(domain.RecoveryDeferredUpdates, "UPDATES-QUEUED", title, detail, "", nil, nil)
		}
	}
	if len(report.Conditions) > 0 {
		report.State = "attention"
	}
	return report
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
