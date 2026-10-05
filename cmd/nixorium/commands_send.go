package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/presentation"
)

// runSendCommand sends a file or folder to the students' desktops through
// the classroom service. Apply prepares the files again and needs the same
// review token, which binds the files' paths, sizes and contents.
func runSendCommand(ctx context.Context, options options, stdout, stderr io.Writer) int {
	plan := planShare(ctx, options.on, options.file)
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
		fmt.Fprintln(stderr, "The files or the computers changed since the review; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Send apply requires an interactive terminal or --yes.")
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
	report := applyShare(ctx, plan)
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
