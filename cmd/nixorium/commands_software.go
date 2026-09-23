package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func runSoftwareCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	manager := app.NewSoftwareManager(adapters.Local{})
	var err error
	switch options.subcommand {
	case "catalog":
		report := manager.Catalog(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.SoftwareCatalogText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "search":
		report := manager.Search(ctx, repository, options.softwareQuery)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.SoftwareSearchText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "presets":
		report := manager.Presets(ctx, repository)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.SoftwarePresetCatalogText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	case "preset-plan", "preset-apply":
		scope, scopeErr := parseSoftwareScope(options.softwareScope)
		if scopeErr != nil {
			fmt.Fprintln(stderr, "Error:", scopeErr)
			return 2
		}
		exclude := []string{}
		if options.softwareExclude != "" {
			exclude = strings.Split(options.softwareExclude, ",")
		}
		request := domain.SoftwarePresetRequest{Preset: options.softwarePreset, Scope: scope, Exclude: exclude}
		if options.subcommand == "preset-plan" {
			report := manager.PlanPreset(ctx, repository, request)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.SoftwarePresetPlanText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
			break
		}
		plan := manager.PlanPreset(ctx, repository, request)
		if plan.HasErrors() {
			if options.json {
				_ = presentation.JSON(stdout, plan)
			} else {
				presentation.SoftwarePresetPlanText(stderr, plan)
			}
			return 1
		}
		if !options.yes {
			if !presentation.IsInteractive(os.Stdin) {
				fmt.Fprintln(stderr, "Error: software preset apply requires an interactive terminal or explicit --yes")
				return 2
			}
			confirmationOutput := stdout
			if options.json {
				confirmationOutput = stderr
			}
			approved, confirmErr := presentation.ConfirmSoftwarePreset(os.Stdin, confirmationOutput, plan)
			if confirmErr != nil {
				fmt.Fprintln(stderr, "Error: read confirmation:", confirmErr)
				return 1
			}
			if !approved {
				fmt.Fprintln(confirmationOutput, "Software profile cancelled; lab-software.json was not changed.")
				return 0
			}
		}
		report := manager.ApplyPresetPlan(ctx, plan, options.expect)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.SoftwarePresetApplyText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	default:
		scope, scopeErr := parseSoftwareScope(options.softwareScope)
		if scopeErr != nil {
			fmt.Fprintln(stderr, "Error:", scopeErr)
			return 2
		}
		request := domain.SoftwareChangeRequest{Package: options.softwarePackage, Present: !options.remove, Scope: scope}
		if options.subcommand == "plan" {
			report := manager.Plan(ctx, repository, request)
			if options.json {
				err = presentation.JSON(stdout, report)
			} else {
				presentation.SoftwareChangePlanText(stdout, report)
			}
			if report.HasErrors() {
				return 1
			}
			break
		}
		plan := manager.Plan(ctx, repository, request)
		if plan.HasErrors() {
			if options.json {
				_ = presentation.JSON(stdout, plan)
			} else {
				presentation.SoftwareChangePlanText(stderr, plan)
			}
			return 1
		}
		if !options.yes {
			if !presentation.IsInteractive(os.Stdin) {
				fmt.Fprintln(stderr, "Error: software apply requires an interactive terminal or explicit --yes")
				return 2
			}
			confirmationOutput := stdout
			if options.json {
				confirmationOutput = stderr
			}
			approved, confirmErr := presentation.ConfirmSoftwareChange(os.Stdin, confirmationOutput, plan)
			if confirmErr != nil {
				fmt.Fprintln(stderr, "Error: read confirmation:", confirmErr)
				return 1
			}
			if !approved {
				fmt.Fprintln(confirmationOutput, "Software change cancelled; lab-software.json was not changed.")
				return 0
			}
		}
		report := manager.ApplyPlan(ctx, plan, options.expect)
		if options.json {
			err = presentation.JSON(stdout, report)
		} else {
			presentation.SoftwareChangeApplyText(stdout, report)
		}
		if report.HasErrors() {
			return 1
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}
