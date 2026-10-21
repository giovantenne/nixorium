package main

import (
	"context"
	"errors"
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

// parseSoftwareFlags reads the software plan/apply flags.
func parseSoftwareFlags(result *options, arguments []string, index int) (int, bool, error) {
	switch arguments[index] {
	case "--package":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--package requires a catalog identifier")
		}
		result.softwarePackage = arguments[index]
	case "--preset":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--preset requires a software profile id")
		}
		result.softwarePreset = arguments[index]
	case "--exclude":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--exclude requires comma-separated package ids")
		}
		result.softwareExclude = arguments[index]
	case "--query":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--query requires a package-name fragment")
		}
		result.softwareQuery = arguments[index]
	case "--scope":
		index++
		if index >= len(arguments) || arguments[index] == "" {
			return index, true, errors.New("--scope requires shared, controller, all-clients, group:NAME, or clients:pcNN,...")
		}
		result.softwareScope = arguments[index]
	case "--remove":
		result.remove = true
	default:
		return index, false, nil
	}
	return index, true, nil
}

// parseSoftwareWord reads the words that only mean something after software.
func parseSoftwareWord(result *options, word string) (bool, error) {
	switch word {
	case "catalog":
		if result.command != "software" || result.subcommand != "" {
			return true, errors.New("catalog must follow software")
		}
		result.subcommand = "catalog"
		return true, nil
	case "presets":
		if result.command != "software" || result.subcommand != "" {
			return true, errors.New("presets must follow software")
		}
		result.subcommand = "presets"
		return true, nil
	case "preset":
		if result.command != "software" || result.subcommand != "" {
			return true, errors.New("preset must follow software")
		}
		result.subcommand = "preset"
		return true, nil
	case "search":
		if result.command != "software" || result.subcommand != "" {
			return true, errors.New("search must follow software")
		}
		result.subcommand = "search"
		return true, nil
	case "plan":
		if result.command == "software" && result.subcommand == "preset" {
			result.subcommand = "preset-plan"
			return true, nil
		}
	case "apply":
		if result.command == "software" && result.subcommand == "preset" {
			result.subcommand = "preset-apply"
			return true, nil
		}
	}
	return false, nil
}

// validateSoftware rejects option combinations that do not belong to the command.
func validateSoftware(result *options) error {
	if result.command == "software" && result.subcommand != "catalog" && result.subcommand != "search" && result.subcommand != "presets" && result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "preset-plan" && result.subcommand != "preset-apply" {
		return errors.New("software requires catalog, search, presets, plan, apply, preset plan, or preset apply")
	}
	if result.command == "software" && result.subcommand == "search" && result.softwareQuery == "" {
		return errors.New("software search requires --query")
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") && (result.softwarePackage == "" || result.softwareScope == "") {
		return fmt.Errorf("software %s requires --package and --scope", result.subcommand)
	}
	if result.command == "software" && (result.subcommand == "plan" || result.subcommand == "apply") {
		if _, err := parseSoftwareScope(result.softwareScope); err != nil {
			return err
		}
	}
	if result.command == "software" && (result.subcommand == "preset-plan" || result.subcommand == "preset-apply") && (result.softwarePreset == "" || result.softwareScope == "") {
		return fmt.Errorf("software %s requires --preset and --scope", strings.ReplaceAll(result.subcommand, "-", " "))
	}
	if result.command == "software" && (result.subcommand == "preset-plan" || result.subcommand == "preset-apply") {
		if _, err := parseSoftwareScope(result.softwareScope); err != nil {
			return err
		}
	}
	if (result.softwarePackage != "" || result.remove) && (result.command != "software" || (result.subcommand != "plan" && result.subcommand != "apply")) {
		return errors.New("--package and --remove are only valid with software plan or apply")
	}
	if result.softwareScope != "" && (result.command != "software" || (result.subcommand != "plan" && result.subcommand != "apply" && result.subcommand != "preset-plan" && result.subcommand != "preset-apply")) {
		return errors.New("--scope is only valid with software plan/apply or software preset plan/apply")
	}
	if (result.softwarePreset != "" || result.softwareExclude != "") && (result.command != "software" || (result.subcommand != "preset-plan" && result.subcommand != "preset-apply")) {
		return errors.New("--preset and --exclude are only valid with software preset plan or apply")
	}
	if result.softwareQuery != "" && (result.command != "software" || result.subcommand != "search") {
		return errors.New("--query is only valid with software search")
	}
	if result.command == "software" && result.subcommand == "apply" && result.expect == "" {
		return errors.New("software apply requires --expect from software plan")
	}
	if result.command == "software" && result.subcommand == "preset-apply" && result.expect == "" {
		return errors.New("software preset apply requires --expect from software preset plan")
	}
	return nil
}
