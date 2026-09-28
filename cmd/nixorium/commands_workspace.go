package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

type workspaceCommandManager interface {
	Plan(context.Context, string, []byte) domain.WorkspacePlanReport
	Apply(context.Context, string, []byte, string) domain.WorkspaceApplyReport
}

func runWorkspaceCommand(ctx context.Context, repository string, options options, stdout, stderr io.Writer) int {
	candidate, err := adapters.ReadWorkspaceCandidate(options.file)
	if err != nil {
		fmt.Fprintln(stderr, "Error: read workspace candidate:", err)
		return 1
	}
	confirm := func(plan domain.WorkspacePlanReport) (bool, error) {
		if !presentation.IsInteractive(os.Stdin) {
			return false, errors.New("workspace apply requires an interactive terminal or explicit --yes")
		}
		output := stdout
		if options.json {
			output = stderr
		}
		return presentation.ConfirmWorkspace(os.Stdin, output, plan)
	}
	return runWorkspaceWithManager(ctx, app.NewWorkspaceManager(adapters.Local{}), repository, candidate, options, confirm, stdout, stderr)
}

func runWorkspaceWithManager(ctx context.Context, manager workspaceCommandManager, repository string, candidate []byte,
	options options, confirm func(domain.WorkspacePlanReport) (bool, error), stdout, stderr io.Writer) int {
	if options.subcommand == "plan" {
		plan := manager.Plan(ctx, repository, candidate)
		if options.json {
			if err := presentation.JSON(stdout, plan); err != nil {
				return commandOutputError(err, stderr)
			}
		} else {
			presentation.WorkspacePlanText(stdout, plan)
		}
		if plan.HasErrors() {
			return 1
		}
		return 0
	}
	if !options.yes {
		plan := manager.Plan(ctx, repository, candidate)
		if plan.HasErrors() || plan.ReviewToken != options.expect {
			report := domain.WorkspaceApplyReport{
				SchemaVersion: domain.WorkspaceSchemaVersion, Operation: "workspace-apply", State: plan.State,
				Repository: repository, ManagedFile: domain.WorkspaceFileName,
				Targets: []domain.WorkspaceTarget{}, Issues: plan.Issues, Message: plan.Message,
			}
			if !plan.HasErrors() {
				report.State = "conflict"
				report.Message = "The workspace proposal changed after review; create a new plan."
				report.Issues = []domain.ValidationIssue{{Field: "reviewToken", Message: report.Message}}
			}
			return outputWorkspaceApply(stdout, stderr, report, options.json)
		}
		if plan.State != "unchanged" {
			approved, err := confirm(plan)
			if err != nil {
				return commandOutputError(err, stderr)
			}
			if !approved {
				report := domain.WorkspaceApplyReport{
					SchemaVersion: domain.WorkspaceSchemaVersion, Operation: "workspace-apply", State: "cancelled",
					Repository: repository, ManagedFile: domain.WorkspaceFileName,
					Targets: []domain.WorkspaceTarget{}, Issues: []domain.ValidationIssue{},
					Message: "Workspace save cancelled; no file was changed.",
				}
				if options.json {
					return commandOutputError(presentation.JSON(stdout, report), stderr)
				}
				presentation.WorkspaceApplyText(stdout, report)
				return 0
			}
		}
	}
	return outputWorkspaceApply(stdout, stderr, manager.Apply(ctx, repository, candidate, options.expect), options.json)
}

func outputWorkspaceApply(stdout, stderr io.Writer, report domain.WorkspaceApplyReport, jsonOutput bool) int {
	if jsonOutput {
		if err := presentation.JSON(stdout, report); err != nil {
			return commandOutputError(err, stderr)
		}
	} else {
		presentation.WorkspaceApplyText(stdout, report)
	}
	if report.HasErrors() {
		return 1
	}
	return 0
}
