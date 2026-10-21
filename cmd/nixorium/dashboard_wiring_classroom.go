package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireClassroomActions wires Internet, lock, share, cleanup, power and wake callbacks of the dashboard.
func wireClassroomActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	shutdownManager := deps.shutdownManager
	wakeManager := deps.wakeManager
	cleanupManager := deps.cleanupManager
	internetManager := deps.internetManager
	lockManager := deps.lockManager
	ctx := deps.ctx
	repository := deps.repository
	actions.PlanInternet = func(ctx context.Context, requested string, action domain.InternetAction) domain.InternetPlan {
		return internetManager.Plan(ctx, repository, requested, action)
	}
	actions.ApplyInternet = func(plan domain.InternetPlan) domain.InternetReport {
		report := internetManager.Apply(ctx, plan, plan.ReviewToken)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.PlanCleanup = func(ctx context.Context, requested string) domain.CleanupPlanReport {
		return cleanupManager.Plan(ctx, repository, requested)
	}
	actions.PlanLock = func(ctx context.Context, requested string, action domain.LockAction) domain.LockPlan {
		return lockManager.Plan(ctx, repository, requested, action)
	}
	actions.ApplyLock = func(plan domain.LockPlan) domain.LockReport {
		return lockManager.Apply(ctx, plan, plan.ReviewToken)
	}
	// The administrator's files go through the classroom service too.
	actions.PlanShare = planShare
	actions.ApplyShare = func(plan domain.SharePlan) domain.ShareReport {
		return applyShare(ctx, plan)
	}
	actions.ApplyCleanup = func(plan domain.CleanupPlanReport) domain.CleanupApplyReport {
		report := cleanupManager.Apply(ctx, plan, plan.ReviewToken)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.PlanShutdown = func(ctx context.Context, requested string, policy domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
		return shutdownManager.Plan(ctx, repository, requested, policy)
	}
	actions.PlanPower = func(ctx context.Context, requested string, policy domain.ShutdownSessionPolicy, action domain.ClientPowerAction) domain.ShutdownPlanReport {
		return shutdownManager.PlanAction(ctx, repository, requested, policy, action)
	}
	actions.ApplyShutdown = func(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
		report := shutdownManager.ApplyPlan(ctx, plan, plan.ReviewToken)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.PlanWake = func(ctx context.Context, requested string) domain.WakePlan {
		return wakeManager.Plan(ctx, repository, requested)
	}
	actions.ApplyWake = func(plan domain.WakePlan) domain.WakeReport {
		return wakeManager.Apply(ctx, plan)
	}
}
