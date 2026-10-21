package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireInstallationActions wires PXE and USB/SSH client installation callbacks of the dashboard.
func wireInstallationActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	lifecycle := deps.lifecycle
	progressManager := deps.progressManager
	ctx := deps.ctx
	repository := deps.repository
	actions.PreparePXE = func() domain.ActionReport {
		report := app.NewSystemActions(local).PreparePXE(ctx)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.LoadPXEProgress = func() (domain.OperationProgress, error) {
		return progressManager.Current("pxe-prepare")
	}
	actions.PlanPXEStart = func(ctx context.Context) domain.PXELifecycleReport {
		return lifecycle.PlanStart(ctx, repository)
	}
	actions.StartPXE = func() domain.PXELifecycleReport {
		report := lifecycle.Start(ctx, repository)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.StopPXE = func() domain.PXELifecycleReport {
		report := lifecycle.Stop(ctx, repository)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.RecoverPXE = func() domain.PXELifecycleReport {
		report := lifecycle.Recover(ctx, repository)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.PrepareRemoteInstall = func(host string) (domain.RemoteInstallResponse, error) {
		return dashboardRemoteInstallRequest(ctx, repository, domain.RemoteInstallRequest{
			Operation: domain.RemoteInstallPrepareOperation, Host: host,
		}, nil)
	}
	actions.ObserveRemoteInstall = func(ctx context.Context, address string) (string, error) {
		return adapters.NewLiveBootstrap().ObserveHostFingerprint(ctx, address)
	}
	actions.BootstrapRemoteInstall = func(host, address, fingerprint string, password []byte) (domain.RemoteInstallResponse, error) {
		return dashboardRemoteInstallRequest(ctx, repository, domain.RemoteInstallRequest{
			Operation: domain.RemoteInstallBootstrapOperation, Host: host, Address: address, Fingerprint: fingerprint,
		}, password)
	}
	actions.RemoteInstallRequest = func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
		return dashboardRemoteInstallRequest(ctx, repository, request, nil)
	}
	actions.RemoteReservationPresent = local.RemoteReservationPresent
	actions.LoadRemoteInstall = func(ctx context.Context) (domain.RemoteInstallResponse, error) {
		return dashboardRemoteInstallRequest(ctx, repository, domain.RemoteInstallRequest{Operation: domain.RemoteInstallWorkerProbeOperation}, nil)
	}
}
