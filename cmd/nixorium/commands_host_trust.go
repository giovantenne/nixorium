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

func parseHostTrustArguments(arguments []string) (options, error) {
	result := options{command: "host-key"}
	if len(arguments) == 0 || (arguments[0] != "plan" && arguments[0] != "apply") {
		return result, errors.New("host-key requires plan or apply")
	}
	result.subcommand = arguments[0]
	flags := flag.NewFlagSet("host-key", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&result.repository, "repo", "", "deployment path")
	flags.StringVar(&result.host, "host", "", "one inventory client")
	flags.StringVar(&result.expect, "expect", "", "review token")
	flags.BoolVar(&result.json, "json", false, "JSON report")
	flags.BoolVar(&result.yes, "yes", false, "authorize reviewed rotation")
	if err := flags.Parse(arguments[1:]); err != nil {
		return result, err
	}
	if flags.NArg() != 0 || !domain.ValidRemoteHostName(result.host) {
		return result, errors.New("host-key requires exactly one --host pcNN")
	}
	if result.subcommand == "plan" && (result.expect != "" || result.yes) {
		return result, errors.New("--expect and --yes are only valid with host-key apply")
	}
	if result.subcommand == "apply" && result.expect == "" {
		return result, errors.New("host-key apply requires --expect from a fresh plan")
	}
	return result, nil
}

func runHostTrustCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewHostTrustManager(adapters.Local{})
	plan := manager.Plan(ctx, repository, options.host)
	if options.subcommand == "plan" || plan.HasErrors() {
		if options.json {
			_ = presentation.JSON(stdout, plan)
		} else {
			presentation.HostTrustPlanText(stdout, plan)
		}
		if plan.HasErrors() {
			return 1
		}
		return 0
	}
	if plan.ReviewToken != options.expect {
		fmt.Fprintln(stderr, "Host-key review changed; create a fresh plan.")
		return 1
	}
	if !options.yes {
		if !presentation.IsInteractive(os.Stdin) {
			fmt.Fprintln(stderr, "Host-key apply requires interactive confirmation or --yes.")
			return 2
		}
		approved, err := presentation.ConfirmHostTrust(os.Stdin, stderr, plan)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !approved {
			return 0
		}
	}
	report := manager.Apply(ctx, plan, options.expect)
	report.Message = operationRecordMessage(report.Message, report)
	if options.json {
		_ = presentation.JSON(stdout, report)
	} else {
		fmt.Fprintln(stdout, report.Message)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
