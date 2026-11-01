package main

import (
	"context"
	"strings"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireUpdatesActions wires Nixorium and package base updates and the Git review/save callbacks of the dashboard.
func wireUpdatesActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	gitReviewManager := deps.gitReviewManager
	gitCommitManager := deps.gitCommitManager
	updateManager := deps.updateManager
	baseManager := deps.baseManager
	updateSaveManager := deps.updateSaveManager
	baseSaveManager := deps.baseSaveManager
	ctx := deps.ctx
	repository := deps.repository
	actions.LoadPackageBase = func(ctx context.Context) domain.PackageBaseStatus { return baseManager.PackageBaseStatus(repository) }
	actions.PlanPackageBase = func(ctx context.Context, target string, allowUnverified bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
		return baseManager.PlanWithProgress(ctx, repository, target, allowUnverified, false, progress)
	}
	actions.SavePackageBase = func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
		report := baseSaveManager.Save(ctx, plan)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.LoadGitReview = func(ctx context.Context) domain.GitReviewReport {
		return gitReviewManager.Review(ctx, repository)
	}
	actions.PlanGitDiscard = func(ctx context.Context, paths string) domain.GitCommitPlanReport {
		return app.NewGitDiscardManager(local).Plan(ctx, repository, paths)
	}
	actions.ApplyGitDiscard = func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
		report := app.NewGitDiscardManager(local).Apply(ctx, repository, strings.Join(plan.Paths, ","), plan.ReviewToken)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.PlanGitUndo = func(ctx context.Context) domain.GitUndoPlanReport {
		return app.NewGitUndoManager(local).Plan(ctx, repository)
	}
	actions.ApplyGitUndo = func(plan domain.GitUndoPlanReport) domain.GitUndoReport {
		report := app.NewGitUndoManager(local).Apply(ctx, repository, plan.ReviewToken)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.PlanGitCommit = func(ctx context.Context, paths string) domain.GitCommitPlanReport {
		return gitCommitManager.Plan(ctx, repository, paths)
	}
	actions.ApplyGitCommit = func(plan domain.GitCommitPlanReport) domain.GitCommitReport {
		report := gitCommitManager.Apply(ctx, repository, strings.Join(plan.Paths, ","), plan.ReviewToken)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.CheckUpdate = func(ctx context.Context) domain.UpdateCheckReport {
		return updateManager.Check(ctx, repository)
	}
	actions.PlanUpdate = func(ctx context.Context, target string, allowPrerelease, allowDowngrade bool) domain.UpdatePlanReport {
		return updateManager.Plan(ctx, repository, target, allowPrerelease, allowDowngrade)
	}
	actions.PlanUpdateWithProgress = func(ctx context.Context, target string, allowPrerelease, allowDowngrade bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
		return updateManager.PlanWithProgress(ctx, repository, target, allowPrerelease, allowDowngrade, progress)
	}
	actions.SaveUpdate = func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
		report := updateSaveManager.Save(ctx, plan)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
}
