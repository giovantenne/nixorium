package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// Undo the last saved change of the deployment:
//
//	git undo plan [--json]
//	git undo apply --expect <review-token> [--yes] [--json]
//
// plan reviews the saved change that would be undone and prints a token.
// apply saves the undo as a new local change. It changes no computer and
// pushes nothing: Apply to controller and Update computers follow as their
// own reviewed operations.

const gitUndoUsage = "use git undo plan, or git undo apply --expect <review-token>"

func parseGitUndoArguments(arguments []string) (options, error) {
	result := options{command: "git"}
	if len(arguments) == 0 {
		return result, errors.New(gitUndoUsage)
	}
	switch arguments[0] {
	case "plan", "apply":
		result.subcommand = "undo-" + arguments[0]
	case "-h", "--help", "help":
		result.help = true
		return result, nil
	default:
		return result, errors.New(gitUndoUsage)
	}
	flags := flag.NewFlagSet("git undo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&result.repository, "repo", "", "deployment path")
	flags.StringVar(&result.expect, "expect", "", "review token")
	flags.BoolVar(&result.json, "json", false, "JSON report")
	flags.BoolVar(&result.yes, "yes", false, "undo without the typed confirmation")
	flags.BoolVar(&result.help, "help", false, "usage")
	flags.BoolVar(&result.help, "h", false, "usage")
	if err := flags.Parse(arguments[1:]); err != nil {
		return result, err
	}
	if flags.NArg() != 0 {
		return result, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if result.help {
		return result, nil
	}
	switch {
	case result.subcommand == "undo-plan" && (result.expect != "" || result.yes):
		return result, errors.New("--expect and --yes are not valid with git undo plan")
	case result.subcommand == "undo-apply" && result.expect == "":
		return result, errors.New("git undo apply requires --expect from git undo plan")
	}
	return result, nil
}

func runGitUndoCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewGitUndoManager(adapters.Local{})
	plan := manager.Plan(ctx, repository)
	if options.subcommand == "undo-plan" {
		var err error
		if options.json {
			err = presentation.JSON(stdout, plan)
		} else {
			presentation.GitUndoPlanText(stdout, plan)
			if !plan.HasErrors() {
				fmt.Fprintf(stdout, "To undo it: nixorium git undo apply --expect %s\n", plan.ReviewToken)
			}
		}
		if plan.HasErrors() {
			return 1
		}
		return commandOutputError(err, stderr)
	}
	if plan.HasErrors() {
		if options.json {
			if err := presentation.JSON(stdout, plan); err != nil {
				fmt.Fprintln(stderr, "Error:", err)
			}
		} else {
			presentation.GitUndoPlanText(stderr, plan)
		}
		return 1
	}
	if plan.ReviewToken != options.expect {
		report := manager.Apply(ctx, repository, options.expect)
		if options.json {
			_ = presentation.JSON(stdout, report)
		} else {
			presentation.GitUndoResultText(stderr, report)
		}
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Error: git undo apply requires an interactive terminal or explicit --yes")
			return 2
		}
		confirmationOutput := stdout
		if options.json {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmGitUndo(os.Stdin, confirmationOutput, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Nothing was undone; HEAD, the index and the files were not changed.")
			return 0
		}
	}
	report := manager.Apply(ctx, repository, options.expect)
	report.Message = operationRecordMessage(report.Message, report)
	if options.json {
		if err := presentation.JSON(stdout, report); err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 1
		}
	} else {
		presentation.GitUndoResultText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
