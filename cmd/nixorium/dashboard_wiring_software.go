package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireSoftwareActions wires the software catalog and profile callbacks of the dashboard.
func wireSoftwareActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	softwareManager := deps.softwareManager
	softwareSaveManager := deps.softwareSaveManager
	softwarePresetSaveManager := deps.softwarePresetSaveManager
	ctx := deps.ctx
	repository := deps.repository
	actions.LoadSoftware = func(ctx context.Context) domain.SoftwareCatalogReport {
		return softwareManager.Catalog(ctx, repository)
	}
	actions.SearchSoftware = func(searchContext context.Context, query string) domain.SoftwareSearchReport {
		return softwareManager.Search(searchContext, repository, query)
	}
	actions.PlanSoftware = func(ctx context.Context, request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
		return softwareManager.Plan(ctx, repository, request)
	}
	actions.SaveSoftware = func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
		return softwareSaveManager.Save(ctx, plan)
	}
	actions.LoadSoftwarePresets = func(ctx context.Context) domain.SoftwarePresetCatalogReport {
		return softwareManager.Presets(ctx, repository)
	}
	actions.PlanSoftwarePreset = func(ctx context.Context, request domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport {
		return softwareManager.PlanPreset(ctx, repository, request)
	}
	actions.SaveSoftwarePreset = func(plan domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport {
		return softwarePresetSaveManager.Save(ctx, plan)
	}
}
