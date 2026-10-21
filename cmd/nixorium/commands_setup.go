package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func runConfigCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewSettingsManager(adapters.Local{}).WithNetworkObserver(adapters.Local{})
	var err error
	switch options.subcommand {
	case "validate":
		report := manager.Validate(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.ConfigValidationText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "plan", "apply":
		candidate, readErr := readCandidateSettings(options.file)
		if readErr != nil {
			fmt.Fprintln(stderr, "Error: read candidate settings:", readErr)
			return 1
		}
		if options.subcommand == "plan" {
			report := manager.Plan(ctx, repository, candidate)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.ConfigPlanText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
			break
		}
		report := manager.Apply(ctx, repository, candidate, options.expect)
		recordOperationWarning(stderr, report)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.ConfigApplyText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	}
	return commandOutputError(err, stderr)
}

func runSetupCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewSetupManager(adapters.Local{})
	var err error
	switch options.subcommand {
	case "configure":
		if !options.guided {
			return runSetupConfigure(ctx, repository, stdout, stderr, false)
		}
		return runDashboardProgram(ctx, repository, true, stderr)
	case "apply":
		return runSetupApply(ctx, repository, stdout, stderr, options.yes, options.json)
	case "install-secrets":
		report := app.NewSystemActions(adapters.Local{}).InstallSecrets(ctx)
		report.Message = operationRecordMessage(report.Message, report)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.ActionText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "keys":
		var report domain.KeyReconcileReport
		var reconcileErr error
		if options.verifyOnly {
			report = manager.VerifyKeys(ctx, repository)
		} else {
			report, reconcileErr = manager.ReconcileKeys(ctx, repository)
			recordOperationWarning(stderr, report)
		}
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.KeyReconcileText(stdout, report)
		}
		if err == nil && reconcileErr != nil {
			err = reconcileErr
		}
		if report.State != "ready" && err == nil {
			return 1
		}
	default:
		report := manager.Status(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.SetupText(stdout, report)
		}
	}
	return commandOutputError(err, stderr)
}

func runPXECommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	var err error
	switch options.subcommand {
	case "prepare":
		writePXEPreparationActivity(stderr)
		local := adapters.Local{}
		report := runWithManagedProgress(
			func() domain.ActionReport { return app.NewSystemActions(local).PreparePXE(ctx) },
			func() (domain.OperationProgress, error) {
				return app.NewOperationProgressManager(local).Current("pxe-prepare")
			},
			stderr,
		)
		report.Message = operationRecordMessage(report.Message, report)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.ActionText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "start":
		return runPXEStart(ctx, repository, stdout, stderr, options.yes, options.json)
	case "stop", "recover":
		manager := app.NewPXELifecycle(adapters.Local{})
		report := domain.PXELifecycleReport{}
		if options.subcommand == "stop" {
			report = manager.Stop(ctx, repository)
		} else {
			report = manager.Recover(ctx, repository)
		}
		report.Message = operationRecordMessage(report.Message, report)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.PXELifecycleText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	}
	return commandOutputError(err, stderr)
}

// parseSetupFlags reads the setup flags.
func parseSetupFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--verify-only":
		result.verifyOnly = true
	default:
		return index, false, nil
	}
	return index, true, nil
}

// parseSetupWord reads the words that only mean something after setup.
func parseSetupWord(result *options, word string) (bool, error) {
	switch word {
	case "keys":
		if result.command != "setup" || result.subcommand != "" {
			return true, errors.New("keys must follow setup")
		}
		result.subcommand = "keys"
		return true, nil
	case "configure":
		if result.command != "setup" || result.subcommand != "" {
			return true, errors.New("configure must follow setup")
		}
		result.subcommand = "configure"
		return true, nil
	case "install-secrets":
		if result.command != "setup" || result.subcommand != "" {
			return true, errors.New("install-secrets must follow setup")
		}
		result.subcommand = "install-secrets"
		return true, nil
	case "status":
		if result.command == "setup" && result.subcommand == "" {
			result.subcommand = "status"
			return true, nil
		}
	}
	return false, nil
}

// validateVerifyOnly rejects option combinations that do not belong to the command.
func validateVerifyOnly(result *options) error {
	if result.verifyOnly && (result.command != "setup" || result.subcommand != "keys") {
		return errors.New("--verify-only is only valid with setup keys")
	}
	return nil
}

// validateSetupCommand rejects option combinations that do not belong to the command.
func validateSetupCommand(result *options) error {
	if result.command == "setup" && result.subcommand == "" {
		result.subcommand = "configure"
		result.guided = true
	}
	if result.command == "setup" && result.subcommand != "status" && result.subcommand != "keys" && result.subcommand != "configure" && result.subcommand != "install-secrets" && result.subcommand != "apply" {
		return errors.New("setup requires configure, status, keys, install-secrets, or apply")
	}
	return nil
}

// validateSetupInteractive rejects option combinations that do not belong to the command.
func validateSetupInteractive(result *options) error {
	if result.command == "setup" && result.subcommand == "configure" && result.json {
		return errors.New("--json is not valid with interactive setup configure")
	}
	return nil
}
