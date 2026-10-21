package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireWorkspaceActions wires the student workspace, home files and template reset callbacks of the dashboard.
func wireWorkspaceActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	gitReviewManager := deps.gitReviewManager
	workspaceManager := deps.workspaceManager
	homeFilesManager := deps.homeFilesManager
	workspaceMarketplace := deps.workspaceMarketplace
	templateResetManager := deps.templateResetManager
	ctx := deps.ctx
	repository := deps.repository
	actions.LoadTemplateReset = func(requestContext context.Context) domain.TemplateResetCatalog {
		return templateResetManager.Catalog(requestContext, repository)
	}
	actions.PlanTemplateReset = func(requestContext context.Context, preset string, progress func(string)) domain.TemplateResetPlan {
		return templateResetManager.Plan(requestContext, repository, preset, progress)
	}
	actions.ApplyTemplateReset = func(plan domain.TemplateResetPlan) domain.TemplateResetResult {
		return templateResetManager.Apply(ctx, plan)
	}
	actions.LoadWorkspace = func(requestContext context.Context) domain.WorkspacePlanReport {
		return workspaceManager.Load(requestContext, repository)
	}
	actions.PlanWorkspace = func(requestContext context.Context, candidate domain.WorkspaceProfile) domain.WorkspacePlanReport {
		data, _ := domain.MarshalWorkspaceProfile(candidate)
		return workspaceManager.Plan(requestContext, repository, data)
	}
	actions.ResolveMarketplace = func(requestContext context.Context, id string) domain.WorkspaceMarketplaceReport {
		return workspaceMarketplace.Resolve(requestContext, repository, id)
	}
	actions.SaveWorkspace = func(plan domain.WorkspacePlanReport) domain.WorkspaceApplyReport {
		return app.NewWorkspaceSaveManager(local, gitReviewManager).Save(ctx, plan)
	}
	actions.LoadHomeFiles = func(requestContext context.Context) domain.HomeFilesPlanReport {
		return homeFilesManager.Load(requestContext, repository)
	}
	actions.CaptureHomeFiles = func(requestContext context.Context, source, destination string) domain.HomeFilesCaptureReport {
		return homeFilesManager.Capture(requestContext, repository, source, destination)
	}
	actions.DiscardHomeFilesCapture = func(requestContext context.Context, id string) error {
		return homeFilesManager.DiscardCapture(requestContext, repository, id)
	}
	actions.CompleteHomeFilesPath = local.CompleteHomeFilesPath
	actions.PlanHomeFiles = func(requestContext context.Context, draft domain.HomeFilesDraft) domain.HomeFilesPlanReport {
		return homeFilesManager.Plan(requestContext, repository, draft)
	}
	actions.SaveHomeFiles = func(plan domain.HomeFilesPlanReport) domain.HomeFilesApplyReport {
		return app.NewHomeFilesSaveManager(local, gitReviewManager).Save(ctx, repository, *plan.Candidate, plan.ReviewToken)
	}
}
