package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// Client updates that wait for their computers: the review queues computers
// that are off; nixorium-deferred-updates.timer runs "deploy queue run",
// which updates those that answer again together, with one ordinary update.

func newDeferredUpdateManager() *app.DeferredUpdateManager {
	local := adapters.Local{}
	deployments := app.NewDeploymentManager(local)
	return app.NewDeferredUpdateManager(adapters.ManagedDeferredUpdates(), local, func(ctx context.Context, repository string, hosts []string, revision string) (domain.DeploymentExecutionReport, bool) {
		return executeDeploymentOperationGated(ctx, deployments, repository, strings.Join(hosts, ","), revision, io.Discard, nil)
	})
}

func runDeploymentQueueCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := newDeferredUpdateManager()
	switch options.subcommand {
	case "queue-cancel":
		hosts := strings.Split(options.on, ",")
		removed, err := manager.Cancel(hosts)
		if err != nil {
			fmt.Fprintln(stderr, "Error: queued updates were not changed:", err)
			return 1
		}
		fmt.Fprintf(stdout, "Removed %s; those computers keep their current system.\n", map[bool]string{true: "1 queued update", false: fmt.Sprintf("%d queued updates", removed)}[removed == 1])
		return 0
	case "queue-run":
		report := manager.Run(ctx, repository)
		if options.json {
			return commandOutputError(presentation.JSON(stdout, report), stderr)
		}
		presentation.DeferredUpdateRunText(stdout, report)
		if report.HasErrors() || report.State == "failed" {
			return 1
		}
		return 0
	}
	status := manager.Status(ctx, repository)
	if options.json {
		if err := presentation.JSON(stdout, status); err != nil {
			return commandOutputError(err, stderr)
		}
	} else {
		presentation.DeferredUpdateStatusText(stdout, status)
	}
	if status.HasErrors() {
		return 1
	}
	return 0
}

// queueUnreachableComputers records the reviewed revision for the plan's
// computers that did not answer and returns the reachable selection still to
// update now ("" when none is reachable).
func queueUnreachableComputers(plan domain.DeploymentPlanReport, stdout io.Writer) (string, error) {
	reachable, unreachable := app.SplitDeploymentByAvailability(plan)
	if len(unreachable) == 0 {
		return plan.Requested, nil
	}
	if err := newDeferredUpdateManager().Queue(plan, unreachable); err != nil {
		return "", err
	}
	fmt.Fprintf(stdout, "Queued for when they are switched on: %s\n", strings.Join(unreachable, ", "))
	return strings.Join(reachable, ","), nil
}

// applyDeploymentQueued queues the reviewed plan's computers that did not
// answer and updates the others now, at the same reviewed revision.
func applyDeploymentQueued(ctx context.Context, manager *app.DeploymentManager, repository string, plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
	reachable, unreachable := app.SplitDeploymentByAvailability(plan)
	if len(unreachable) > 0 {
		if err := newDeferredUpdateManager().Queue(plan, unreachable); err != nil {
			return domain.DeploymentExecutionReport{SchemaVersion: domain.SchemaVersion, Operation: "deploy-apply", State: "blocked", Repository: plan.Repository, Requested: plan.Requested, Revision: plan.Revision, Targets: []domain.DeploymentTarget{}, Phase: domain.DeploymentPhasePreflight, RetrySafe: true,
				Message: "the computers that are off could not be queued; nothing was started: " + err.Error(), Issues: []domain.ValidationIssue{{Field: "queue", Message: err.Error()}}}
		}
	}
	if len(reachable) == 0 {
		return domain.DeploymentExecutionReport{SchemaVersion: domain.SchemaVersion, Operation: "deploy-apply", State: "completed", Repository: plan.Repository, Requested: plan.Requested, Revision: plan.Revision, Targets: []domain.DeploymentTarget{}, Phase: domain.DeploymentPhaseComplete, RetrySafe: true, Queued: unreachable,
			Message: "no selected computer is reachable now; all of them update when switched on", Issues: []domain.ValidationIssue{}}
	}
	report := executeDeploymentOperationWithProgress(ctx, manager, repository, strings.Join(reachable, ","), plan.Revision, io.Discard, observe)
	report.Queued = unreachable
	// Computers that stopped answering after the build follow the same
	// reviewed choice as those that were already off.
	if len(report.Skipped) > 0 {
		if err := newDeferredUpdateManager().Queue(plan, report.Skipped); err != nil {
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "queue", Message: "the computers that stopped answering could not be queued: " + err.Error()})
			return report
		}
		report.Queued = append(report.Queued, report.Skipped...)
		if report.State == "partial" && !report.RecoveryRequired && len(report.Issues) == 0 && report.Verification.Verified == report.Verification.Attempted {
			report.State = "completed"
			report.Message = fmt.Sprintf("%d computer(s) updated and verified; %s stopped answering after the build and update when they answer again", report.Verification.Verified, strings.Join(report.Skipped, ", "))
		}
	}
	return report
}
