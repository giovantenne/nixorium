package main

import (
	"bufio"
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

func parseHomeFilesArguments(arguments []string) (options, error) {
	o := options{command: "home-files"}
	if len(arguments) == 0 {
		return o, errors.New("home-files requires plan, apply or import plan")
	}
	for i := 0; i < len(arguments); i++ {
		arg := arguments[i]
		switch arg {
		case "import":
			if o.subcommand != "" {
				return o, errors.New("import must follow home-files")
			}
			o.subcommand = "import"
		case "plan":
			if o.subcommand == "import" {
				o.subcommand = "import-plan"
			} else if o.subcommand == "" {
				o.subcommand = "plan"
			} else {
				return o, errors.New("only one home-files action may be selected")
			}
		case "apply":
			if o.subcommand != "" {
				return o, errors.New("apply must follow home-files")
			}
			o.subcommand = "apply"
		case "--repo", "--file", "--expect", "--source", "--destination":
			i++
			if i >= len(arguments) || arguments[i] == "" {
				return o, fmt.Errorf("%s requires a value", arg)
			}
			switch arg {
			case "--repo":
				o.repository = arguments[i]
			case "--file":
				o.file = arguments[i]
			case "--expect":
				o.expect = arguments[i]
			case "--source":
				o.homeFilesSource = arguments[i]
			case "--destination":
				o.homeFilesDestination = arguments[i]
			}
		case "--replace":
			o.homeFilesReplace = true
		case "--json":
			o.json = true
		case "--yes":
			o.yes = true
		case "--help", "-h":
			o.help = true
		default:
			return o, fmt.Errorf("unknown home-files argument %q", arg)
		}
	}
	if o.help {
		return o, nil
	}
	if o.subcommand != "plan" && o.subcommand != "apply" && o.subcommand != "import-plan" {
		return o, errors.New("home-files requires plan, apply or import plan")
	}
	if o.subcommand == "import-plan" {
		if o.homeFilesSource == "" || o.homeFilesDestination == "" || o.file != "" || o.yes || o.expect != "" {
			return o, errors.New("home-files import plan requires --source and --destination; it cannot save or use --file")
		}
	} else {
		if o.file == "" {
			return o, errors.New("home-files plan/apply requires --file")
		}
		if o.homeFilesSource != "" || o.homeFilesDestination != "" || o.homeFilesReplace {
			return o, errors.New("--source, --destination and --replace require home-files import plan")
		}
		if o.subcommand == "plan" && (o.expect != "" || o.yes) {
			return o, errors.New("--expect and --yes require home-files apply")
		}
		if o.subcommand == "apply" && o.expect == "" {
			return o, errors.New("home-files apply requires --expect from home-files plan")
		}
	}
	return o, nil
}

type homeFilesCommandManager interface {
	Load(context.Context, string) domain.HomeFilesPlanReport
	Capture(context.Context, string, string, string) domain.HomeFilesCaptureReport
	Plan(context.Context, string, domain.HomeFilesDraft) domain.HomeFilesPlanReport
	Apply(context.Context, string, domain.HomeFilesDraft, string) domain.HomeFilesApplyReport
}

func runHomeFilesCommand(ctx context.Context, repository string, o options, stdout, stderr io.Writer) int {
	manager := app.NewHomeFilesManager(adapters.Local{})
	if o.subcommand == "import-plan" {
		return runHomeFilesImport(ctx, manager, repository, o, stdout, stderr)
	}
	draft, err := adapters.ReadHomeFilesDraft(o.file)
	if err != nil {
		fmt.Fprintln(stderr, "Error: read initial-file candidate:", err)
		return 1
	}
	confirm := func(plan domain.HomeFilesPlanReport) (bool, error) {
		if !presentation.IsInteractive(os.Stdin) {
			return false, errors.New("home-files apply requires an interactive terminal or explicit --yes")
		}
		output := stdout
		if o.json {
			output = stderr
		}
		presentation.HomeFilesPlanText(output, plan)
		fmt.Fprint(output, "Type SAVE to write only the initial-file manifest and captured copies: ")
		value, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && len(value) == 0 {
			return false, err
		}
		return strings.TrimSpace(value) == "SAVE", nil
	}
	return runHomeFilesWithManager(ctx, manager, repository, draft, o, confirm, stdout, stderr)
}

func runHomeFilesImport(ctx context.Context, m homeFilesCommandManager, repository string, o options, stdout, stderr io.Writer) int {
	loaded := m.Load(ctx, repository)
	if loaded.HasErrors() || loaded.Candidate == nil {
		return outputHomeFilesPlan(stdout, stderr, loaded, o.json)
	}
	capture := m.Capture(ctx, repository, o.homeFilesSource, o.homeFilesDestination)
	if capture.HasErrors() {
		report := domain.HomeFilesPlanReport{SchemaVersion: 1, Operation: "home-files-plan", State: capture.State, Repository: repository, Issues: capture.Issues, Message: capture.Message}
		return outputHomeFilesPlan(stdout, stderr, report, o.json)
	}
	profile, err := app.MergeHomeFilesCapture(loaded.Candidate.Profile, capture, o.homeFilesReplace)
	if err != nil {
		report := domain.HomeFilesPlanReport{SchemaVersion: 1, Operation: "home-files-plan", State: "conflict", Repository: repository, Issues: []domain.ValidationIssue{{Field: "destination", Message: err.Error()}}, Message: "A declaration conflicts with this import. Review a new plan using --replace to replace that destination explicitly."}
		return outputHomeFilesPlan(stdout, stderr, report, o.json)
	}
	draft := *loaded.Candidate
	draft.Profile = profile
	draft.CaptureIDs = append(append([]string{}, draft.CaptureIDs...), capture.CaptureID)
	if loaded.Snapshot != nil {
		draft.BaseFingerprint = loaded.Snapshot.BaseFingerprint
	}
	report := m.Plan(ctx, repository, draft)
	if !o.json && !report.HasErrors() {
		fmt.Fprintln(stdout, "Use --json to retain this report's candidate draft and immutable capture IDs for home-files apply --file. The source path is not saved.")
	}
	return outputHomeFilesPlan(stdout, stderr, report, o.json)
}

func runHomeFilesWithManager(ctx context.Context, m homeFilesCommandManager, repository string, draft domain.HomeFilesDraft, o options, confirm func(domain.HomeFilesPlanReport) (bool, error), stdout, stderr io.Writer) int {
	if o.subcommand == "plan" {
		return outputHomeFilesPlan(stdout, stderr, m.Plan(ctx, repository, draft), o.json)
	}
	if !o.yes {
		plan := m.Plan(ctx, repository, draft)
		if plan.HasErrors() || plan.ReviewToken != o.expect {
			result := domain.HomeFilesApplyReport{SchemaVersion: 1, Operation: "home-files-apply", State: plan.State, Repository: repository, Issues: plan.Issues, Message: plan.Message}
			if !plan.HasErrors() {
				result.State = "conflict"
				result.Message = "The initial-file proposal changed after review; create a new plan."
				result.Issues = []domain.ValidationIssue{{Field: "reviewToken", Message: result.Message}}
			}
			return outputHomeFilesApply(stdout, stderr, result, o.json)
		}
		if plan.State != "unchanged" {
			approved, err := confirm(plan)
			if err != nil {
				return commandOutputError(err, stderr)
			}
			if !approved {
				return outputHomeFilesApply(stdout, stderr, domain.HomeFilesApplyReport{SchemaVersion: 1, Operation: "home-files-apply", State: "cancelled", Repository: repository, Message: "Initial-file save cancelled; no configuration changed."}, o.json)
			}
		}
	}
	return outputHomeFilesApply(stdout, stderr, m.Apply(ctx, repository, draft, o.expect), o.json)
}

func outputHomeFilesPlan(stdout, stderr io.Writer, r domain.HomeFilesPlanReport, jsonOutput bool) int {
	if jsonOutput {
		if err := presentation.JSON(stdout, r); err != nil {
			return commandOutputError(err, stderr)
		}
	} else {
		presentation.HomeFilesPlanText(stdout, r)
	}
	if r.HasErrors() {
		return 1
	}
	return 0
}
func outputHomeFilesApply(stdout, stderr io.Writer, r domain.HomeFilesApplyReport, jsonOutput bool) int {
	if jsonOutput {
		if err := presentation.JSON(stdout, r); err != nil {
			return commandOutputError(err, stderr)
		}
	} else {
		presentation.HomeFilesApplyText(stdout, r)
	}
	if r.HasErrors() {
		return 1
	}
	return 0
}
