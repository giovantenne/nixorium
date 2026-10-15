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
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// The guided change of laboratory network:
//
//	network migrate plan --to <base/prefix> [--acknowledge-unreachable] [--json]
//	network migrate start --to <base/prefix> --expect <token> [--acknowledge-unreachable] [--yes] [--json]
//	network migrate status [--json]
//	network migrate finish|undo [--expect <token> [--yes]] [--json]
//
// finish and undo without --expect print their review. Each saved step
// changes no computer: Apply to controller and Update computers follow as
// their own reviewed operations.

var networkMigrationActions = map[string]string{
	"plan":   domain.NetworkMigrationStart,
	"start":  domain.NetworkMigrationStart,
	"finish": domain.NetworkMigrationFinish,
	"undo":   domain.NetworkMigrationUndo,
}

func parseNetworkArguments(arguments []string) (options, error) {
	result := options{command: "network"}
	if len(arguments) < 2 || arguments[0] != "migrate" {
		return result, errors.New("use network migrate plan|start|status|finish|undo")
	}
	result.subcommand = arguments[1]
	if _, known := networkMigrationActions[result.subcommand]; !known && result.subcommand != "status" {
		return result, errors.New("use network migrate plan|start|status|finish|undo")
	}
	flags := flag.NewFlagSet("network migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&result.repository, "repo", "", "deployment path")
	flags.StringVar(&result.network, "to", "", "new laboratory network")
	flags.StringVar(&result.expect, "expect", "", "review token")
	flags.BoolVar(&result.json, "json", false, "JSON report")
	flags.BoolVar(&result.yes, "yes", false, "save the reviewed step")
	flags.BoolVar(&result.acknowledgeUnreachable, "acknowledge-unreachable", false, "continue without the computers that are off")
	if err := flags.Parse(arguments[2:]); err != nil {
		return result, err
	}
	if flags.NArg() != 0 {
		return result, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	starting := result.subcommand == "plan" || result.subcommand == "start"
	switch {
	case starting && result.network == "":
		return result, fmt.Errorf("network migrate %s requires --to <network/prefix>, for example --to 10.1.0.0/24", result.subcommand)
	case !starting && result.network != "":
		return result, errors.New("--to is only valid with network migrate plan or start")
	case !starting && result.acknowledgeUnreachable:
		return result, errors.New("--acknowledge-unreachable is only valid with network migrate plan or start")
	case (result.subcommand == "plan" || result.subcommand == "status") && (result.expect != "" || result.yes):
		return result, fmt.Errorf("--expect and --yes are not valid with network migrate %s", result.subcommand)
	case result.subcommand == "start" && result.expect == "":
		return result, errors.New("network migrate start requires --expect from a fresh network migrate plan")
	case result.yes && result.expect == "":
		return result, errors.New("--yes requires --expect from a fresh review")
	}
	if starting {
		if _, _, err := domain.ParseLabNetwork(result.network); err != nil {
			return result, fmt.Errorf("--to: %w", err)
		}
	}
	return result, nil
}

func newNetworkMigrationManager() *app.NetworkMigrationManager {
	local := adapters.Local{}
	settings := app.NewSettingsManager(local)
	save := app.NewSettingsSaveManager(settings, app.NewGitReviewManager(local), app.NewGitCommitManager(local))
	return app.NewNetworkMigrationManager(local, settings, save, app.NewRecoveryInspector(local), newDeferredUpdateManager(), app.NewHostTrustManager(local))
}

func runNetworkCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := newNetworkMigrationManager()
	if options.subcommand == "status" {
		status := manager.Status(ctx, repository)
		if options.json {
			if err := presentation.JSON(stdout, status); err != nil {
				return commandOutputError(err, stderr)
			}
		} else {
			presentation.NetworkMigrationStatusText(stdout, status)
		}
		if status.HasErrors() {
			return 1
		}
		return 0
	}
	request := app.NetworkMigrationRequest{Action: networkMigrationActions[options.subcommand], AcknowledgeOff: options.acknowledgeUnreachable}
	if request.Action == domain.NetworkMigrationStart {
		base, prefix, err := domain.ParseLabNetwork(options.network)
		if err != nil {
			fmt.Fprintln(stderr, "Error:", err)
			return 2
		}
		request.Base, request.Prefix = base, prefix
	}
	plan := manager.Plan(ctx, repository, request)
	if options.subcommand == "plan" || options.expect == "" || plan.HasErrors() {
		if options.json {
			if err := presentation.JSON(stdout, plan); err != nil {
				return commandOutputError(err, stderr)
			}
		} else {
			presentation.NetworkMigrationPlanText(stdout, plan)
			if !plan.HasErrors() {
				fmt.Fprintf(stdout, "To save this step: %s --expect %s\n", networkMigrationApplyCommand(options), plan.ReviewToken)
			}
		}
		if plan.HasErrors() {
			return 1
		}
		return 0
	}
	if plan.ReviewToken != options.expect {
		fmt.Fprintln(stderr, "The change of laboratory network was reviewed in another situation; review it again.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintf(stderr, "Error: network migrate %s requires an interactive terminal or explicit --yes\n", options.subcommand)
			return 2
		}
		confirmationOutput := stdout
		if options.json {
			confirmationOutput = stderr
		}
		approved, err := presentation.ConfirmNetworkMigration(os.Stdin, confirmationOutput, plan)
		if err != nil {
			fmt.Fprintln(stderr, "Error: read confirmation:", err)
			return 1
		}
		if !approved {
			fmt.Fprintln(confirmationOutput, "Nothing was saved.")
			return 0
		}
	}
	result := manager.Apply(ctx, plan, options.expect)
	if options.json {
		if err := presentation.JSON(stdout, result); err != nil {
			return commandOutputError(err, stderr)
		}
	} else {
		presentation.NetworkMigrationResultText(stdout, result)
	}
	if result.HasErrors() {
		return 1
	}
	return 0
}

func networkMigrationApplyCommand(options options) string {
	switch options.subcommand {
	case "plan", "start":
		command := "nixorium network migrate start --to " + options.network
		if options.acknowledgeUnreachable {
			command += " --acknowledge-unreachable"
		}
		return command
	default:
		return "nixorium network migrate " + options.subcommand
	}
}
