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
// which updates each of them, one at a time, once it answers again.

func newDeferredUpdateManager() *app.DeferredUpdateManager {
	local := adapters.Local{}
	deployments := app.NewDeploymentManager(local)
	return app.NewDeferredUpdateManager(adapters.ManagedDeferredUpdates(), local, func(ctx context.Context, repository, host, revision string) (domain.DeploymentExecutionReport, bool) {
		return executeDeploymentOperationGated(ctx, deployments, repository, host, revision, io.Discard, nil)
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
