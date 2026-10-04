package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/presentation"
)

// runDesktopCommand sends the caller's desktop through the classroom
// service. Apply prepares the files again and needs the same review token,
// which binds the files' paths, sizes and contents.
func runDesktopCommand(ctx context.Context, options options, stdout, stderr io.Writer) int {
	plan := planDesktopShare(ctx, options.on)
	if options.subcommand == "plan" || plan.HasErrors() {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.SharePlanText(stdout, plan)
		}
		if plan.HasErrors() {
			return 1
		}
		return 0
	}
	if options.expect != plan.ReviewToken {
		fmt.Fprintln(stderr, "Your desktop or the computers changed since the review; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Desktop apply requires an interactive terminal or --yes.")
			return 2
		}
		approved, err := presentation.ConfirmShare(os.Stdin, stderr, plan)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !approved {
			return 0
		}
	}
	report := applyDesktopShare(ctx, plan)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		presentation.ShareReportText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
