package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireOverviewActions wires startup, inspection, hosts, services, logs and support callbacks of the dashboard.
func wireOverviewActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	inspector := deps.inspector
	setupManager := deps.setupManager
	configurationStateManager := deps.configurationStateManager
	serviceManager := deps.serviceManager
	operationLogManager := deps.operationLogManager
	supportManager := deps.supportManager
	updateNotifications := deps.updateNotifications
	settingsManager := deps.settingsManager
	ctx := deps.ctx
	repository := deps.repository
	setupMode := deps.setupMode
	actions.OpenURL = desktopOpener()
	actions.LoadClientSetup = func(context.Context) (bool, error) {
		settings, _, err := settingsManager.CurrentForEditing(repository)
		return settings.InstallationSettingsComplete(), err
	}
	actions.LoadDisclaimer = func(context.Context) (bool, error) { return local.DisclaimerAccepted(repository) }
	actions.AcceptDisclaimer = func() error { return local.AcceptDisclaimer(repository) }
	actions.CheckUpdateNotification = func(ctx context.Context) (domain.UpdateNotification, error) {
		return updateNotifications.Check(ctx, repository)
	}
	actions.DismissUpdateNotification = func(notice domain.UpdateNotification) error { return updateNotifications.Dismiss(repository, notice) }
	actions.RunningVersion = nixoriumVersion
	actions.LoadInitial = func(ctx context.Context) (domain.StatusReport, domain.SetupReport, error) {
		if setupMode {
			return domain.StatusReport{}, setupManager.Status(ctx, repository), nil
		}
		setup := setupManager.InitialStatus(ctx, repository)
		if setupStartsBeforeDashboardInspection(setup) {
			return domain.StatusReport{}, setup, nil
		}
		report, err := inspector.Startup(ctx, repository)
		if err != nil {
			return domain.StatusReport{}, domain.SetupReport{}, err
		}
		return report, setup, nil
	}
	actions.LoadInventory = func(requestContext context.Context) (domain.StatusReport, error) {
		return inspector.Inventory(requestContext, repository)
	}
	actions.LoadDoctor = func(ctx context.Context) (domain.DoctorReport, error) {
		return inspector.Doctor(ctx, repository, app.DoctorOptions{})
	}
	actions.Telemetry = app.NewTelemetryManager(adapters.DefaultTelemetry()).Run
	actions.PreviewSupport = func(requestContext context.Context) (domain.SupportSnapshot, error) {
		return supportManager.Preview(requestContext, repository, nixoriumVersion)
	}
	actions.ExportSupport = func(snapshot domain.SupportSnapshot) domain.SupportExportResult {
		return supportManager.Export(ctx, snapshot)
	}
	actions.Refresh = func(ctx context.Context) (domain.StatusReport, error) {
		return inspector.Status(ctx, repository)
	}
	actions.LoadHosts = func(ctx context.Context) (domain.HostsReport, error) {
		return inspector.Hosts(ctx, repository)
	}
	actions.PlanHostTrust = func(readContext context.Context, name string) domain.HostTrustPlan {
		return app.NewHostTrustManager(local).Plan(readContext, repository, name)
	}
	actions.ApplyHostTrust = func(plan domain.HostTrustPlan) domain.HostTrustResult {
		report := app.NewHostTrustManager(local).Apply(ctx, plan, plan.ReviewToken)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.LoadConfigurationState = func(ctx context.Context) domain.ConfigurationStateReport {
		return configurationStateManager.Load(ctx, repository)
	}
	actions.LoadManagedJobs = local.ObserveManagedJobs
	actions.LoadServices = func(ctx context.Context) domain.ServicesReport {
		return serviceManager.Status(ctx, repository)
	}
	actions.RestartService = func(service string) domain.ServiceActionReport {
		report := serviceManager.Restart(ctx, repository, service)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.LoadLogs = func(ctx context.Context) domain.OperationLogsReport {
		return operationLogManager.List()
	}
	actions.LoadLog = func(ctx context.Context, id string) domain.OperationLogReport {
		return operationLogManager.Show(id)
	}
	actions.LoadRecovery = func(ctx context.Context) domain.RecoveryReport {
		return app.NewRecoveryInspector(local).Observe(ctx, repository)
	}
	actions.LabComputerCount = func() (int, bool) {
		raw, err := local.ReadSettings(repository)
		if err != nil {
			return 0, false
		}
		settings, issues := domain.DecodeLabSettings(raw)
		if len(issues) > 0 {
			return 0, false
		}
		return settings.Lab.PCCount, true
	}
	actions.OpenClassroomView = classroomViewAction()
}
