package main

import (
	"context"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireDeploymentActions wires client deployment, controller rebuild, queued updates, network change and recovery callbacks of the dashboard.
func wireDeploymentActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	deploymentManager := deps.deploymentManager
	controllerManager := deps.controllerManager
	progressManager := deps.progressManager
	ctx := deps.ctx
	repository := deps.repository
	actions.PlanDeployment = func(ctx context.Context, requested string) domain.DeploymentPlanReport {
		return deploymentManager.Plan(ctx, repository, requested)
	}
	actions.PlanReachableDeployment = func(ctx context.Context, reviewed domain.DeploymentPlanReport) domain.DeploymentPlanReport {
		return deploymentManager.PlanReachable(ctx, repository, reviewed)
	}
	actions.ApplyDeployment = func(requestContext context.Context, plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
		return executeDeploymentOperationWithProgress(requestContext, deploymentManager, repository, plan.Requested, plan.Revision, io.Discard, observe)
	}
	actions.ApplyDeploymentQueued = func(requestContext context.Context, plan domain.DeploymentPlanReport, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
		return applyDeploymentQueued(requestContext, deploymentManager, repository, plan, observe)
	}
	actions.PlanController = func(ctx context.Context) domain.ControllerRebuildPlanReport {
		return controllerManager.Plan(ctx, repository)
	}
	actions.ApplyController = func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
		report := controllerManager.ApplyReviewed(ctx, repository, plan)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.LoadControllerProgress = func() (domain.OperationProgress, error) {
		return progressManager.Current("controller-apply")
	}
	actions.LoadDeferredUpdates = func(ctx context.Context) domain.DeferredUpdateStatus {
		return newDeferredUpdateManager().Status(ctx, repository)
	}
	actions.LoadNetworkMigration = func(ctx context.Context) domain.NetworkMigrationStatus {
		return newNetworkMigrationManager().Status(ctx, repository)
	}
	actions.PlanNetworkMigration = func(ctx context.Context, action, network string, acknowledgeOff bool) domain.NetworkMigrationPlan {
		request := app.NetworkMigrationRequest{Action: action, AcknowledgeOff: acknowledgeOff}
		if action == domain.NetworkMigrationStart {
			request.Base, request.Prefix, _ = domain.ParseLabNetwork(network)
		}
		return newNetworkMigrationManager().Plan(ctx, repository, request)
	}
	actions.ApplyNetworkMigration = func(plan domain.NetworkMigrationPlan) domain.NetworkMigrationResult {
		return newNetworkMigrationManager().Apply(ctx, plan, plan.ReviewToken)
	}
	actions.CancelDeferredUpdates = func(hosts []string) error {
		_, err := newDeferredUpdateManager().Cancel(hosts)
		return err
	}
	actions.PlanDeploymentRecovery = func(ctx context.Context, acknowledge bool) domain.DeploymentRecoveryPlan {
		return app.NewDeploymentRecoveryManager(local).ForProcess(os.Getpid()).Plan(ctx, repository, acknowledge)
	}
	actions.ApplyDeploymentRecovery = func(plan domain.DeploymentRecoveryPlan) domain.DeploymentRecoveryResult {
		result := app.NewDeploymentRecoveryManager(local).ForProcess(os.Getpid()).Apply(ctx, plan, plan.ReviewToken)
		result.Message = operationRecordMessage(result.Message, result)
		return result
	}
	actions.PlanResetRecovery = func(ctx context.Context) domain.TemplateResetRecoveryPlan {
		return adapters.TemplateReset{}.PlanTemplateResetRecovery(ctx, repository)
	}
	actions.ApplyResetRecovery = func(plan domain.TemplateResetRecoveryPlan) domain.TemplateResetRecoveryResult {
		result := adapters.TemplateReset{}.ApplyTemplateResetRecovery(ctx, plan)
		result.Message = operationRecordMessage(result.Message, result)
		return result
	}
}
