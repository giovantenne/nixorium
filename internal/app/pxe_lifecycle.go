package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

const PXEListenerUnit = "nixorium-pxe.service"
const PXENetworkUnit = "nixorium-pxe-network.service"
const PXERecoverUnit = "nixorium-pxe-recover.service"
const HarmoniaUnit = "nixorium-harmonia.service"

type PXELifecycleSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error)
	GitState(context.Context, string) (domain.GitState, error)
	PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState
	ServiceState(context.Context, string) domain.ServiceState
	InterfaceAddresses(string) ([]string, error)
	CacheHealth(context.Context, string, int) error
	ControlSystemUnit(context.Context, string, string) error
}

type PXELifecycle struct {
	source PXELifecycleSource
}

func NewPXELifecycle(source PXELifecycleSource) PXELifecycle {
	return PXELifecycle{source: source}
}

func ObservePXELifecycle(listener, network domain.ServiceState, preparation domain.PXEPreparationState) domain.PXELifecycleState {
	result := domain.PXELifecycleState{Listener: listener, Network: network}
	switch {
	case !listener.Loaded || !network.Loaded:
		result.Mode = "unavailable"
		result.Detail = "managed PXE units are not installed"
	case listener.Active && network.Active:
		result.Mode = "active"
		result.Detail = "listeners and transactional network are active"
	case listener.Active:
		result.Mode = "degraded"
		result.Detail = "listeners are active without the network unit"
	case network.Active:
		result.Mode = "recovery-required"
		result.Detail = "network transition is active without listeners"
	case listener.State == "failed" || network.State == "failed":
		result.Mode = "degraded"
		result.Detail = "a managed PXE unit is failed"
	case preparation.Ready:
		result.Mode = "ready"
		result.Detail = "current prepared artifacts are ready"
	default:
		result.Mode = "stopped"
		result.Detail = "installation mode is stopped"
	}
	return result
}

func (m PXELifecycle) PlanStart(ctx context.Context, repository string) domain.PXELifecycleReport {
	report, meta, addresses := m.startPreflight(ctx, repository)
	if report.HasErrors() {
		return report
	}
	if report.Mode == "active" {
		report.Message = "PXE installation mode is already active"
		return report
	}
	if !containsIP(addresses, meta.Controller.StaticIP) {
		return failPXEReport(report, missingControllerAddressMessage(meta))
	}
	report.Message = fmt.Sprintf("will remove %s temporarily and serve PXE from %s", report.StaticCIDR, report.DHCPAddress)
	return report
}

func (m PXELifecycle) Start(ctx context.Context, repository string) domain.PXELifecycleReport {
	report, meta, addresses := m.startPreflight(ctx, repository)
	report.Operation = "pxe-start"
	if report.HasErrors() {
		return report
	}
	if report.Mode == "active" {
		report.State = "completed"
		report.Message = "PXE installation mode was already active"
		return report
	}
	if !containsIP(addresses, meta.Controller.StaticIP) {
		return failPXEReport(report, missingControllerAddressMessage(meta))
	}
	if err := m.source.ControlSystemUnit(ctx, "start", PXEListenerUnit); err != nil {
		cleanupErrors := m.stopUnits(ctx)
		message := err.Error()
		if len(cleanupErrors) > 0 {
			message += "; cleanup: " + strings.Join(cleanupErrors, "; ")
		}
		return failPXEReport(m.observeReport(report, ctx), message)
	}

	report = m.observeReport(report, ctx)
	addresses, err := m.source.InterfaceAddresses(meta.Network.Interface)
	if err != nil {
		return m.failStartedPXE(ctx, report, "PXE started but interface verification failed: "+err.Error())
	}
	if report.Mode != "active" || !containsIP(addresses, report.DHCPAddress) || containsIP(addresses, meta.Controller.StaticIP) {
		return m.failStartedPXE(ctx, report, "PXE units did not reach a consistent active network state")
	}
	if err := m.source.CacheHealth(ctx, report.DHCPAddress, meta.Network.CachePort); err != nil {
		return m.failStartedPXE(ctx, report, "PXE started but cache readiness failed: "+err.Error())
	}
	report.State = "completed"
	report.Message = fmt.Sprintf("PXE installation mode active on %s via %s", meta.Network.Interface, report.DHCPAddress)
	return report
}

func (m PXELifecycle) Stop(ctx context.Context, repository string) domain.PXELifecycleReport {
	report := m.baseReport(ctx, repository, "pxe-stop")
	errors := m.stopUnits(ctx)
	report = m.observeReport(report, ctx)
	if pxeUnitsActive(report) {
		errors = append(errors, "PXE listener or network unit is still active")
	}
	m.verifyRestoredAddress(ctx, repository, &report, &errors)
	if len(errors) > 0 {
		return failPXEReport(report, strings.Join(errors, "; "))
	}
	report.State = "completed"
	report.Mode = "stopped"
	report.Message = "PXE installation mode stopped and normal addressing restored"
	return report
}

func (m PXELifecycle) Recover(ctx context.Context, repository string) domain.PXELifecycleReport {
	report := m.baseReport(ctx, repository, "pxe-recover")
	errors := m.stopUnits(ctx)
	if err := m.source.ControlSystemUnit(ctx, "start", PXERecoverUnit); err != nil {
		errors = append(errors, err.Error())
	}
	report = m.observeReport(report, ctx)
	if pxeUnitsActive(report) {
		errors = append(errors, "PXE listener or network unit is still active after recovery")
	}
	m.verifyRestoredAddress(ctx, repository, &report, &errors)
	if len(errors) > 0 {
		return failPXEReport(report, strings.Join(errors, "; "))
	}
	report.State = "completed"
	report.Mode = "stopped"
	report.Message = "PXE recovery completed; installation mode is stopped"
	return report
}

func (m PXELifecycle) startPreflight(ctx context.Context, repository string) (domain.PXELifecycleReport, domain.LabMeta, []string) {
	report := domain.PXELifecycleReport{SchemaVersion: domain.SchemaVersion, Operation: "pxe-start-plan", State: "ready"}
	meta, err := m.source.LabMeta(ctx, repository)
	if err != nil {
		return failPXEReport(report, "evaluate lab metadata: "+err.Error()), meta, nil
	}
	report.Interface = meta.Network.Interface
	report.DHCPAddress = meta.Controller.DHCPIP
	report.StaticCIDR = fmt.Sprintf("%s/%d", meta.Controller.StaticIP, meta.Network.PrefixLength)

	deployment, err := m.source.DeploymentStatus(ctx, repository)
	if err != nil {
		return failPXEReport(report, "evaluate deployment readiness: "+err.Error()), meta, nil
	}
	if !deployment.Ready {
		return failPXEReport(report, "deployment is not ready: "+strings.Join(deployment.Issues, "; ")), meta, nil
	}
	gitState, err := m.source.GitState(ctx, repository)
	if err != nil {
		return failPXEReport(report, "inspect Git worktree: "+err.Error()), meta, nil
	}
	if gitState.Dirty {
		return failPXEReport(report, "deployment worktree must be clean before starting PXE"), meta, nil
	}
	report.Preparation = m.source.PXEPreparation(ctx, repository, meta)
	if !report.Preparation.Ready {
		return failPXEReport(report, "managed PXE preparation is not current: "+report.Preparation.Detail), meta, nil
	}
	report.DHCPAddress = report.Preparation.DHCPAddress
	if report.DHCPAddress == "" {
		return failPXEReport(report, "managed PXE preparation does not record a controller address"), meta, nil
	}

	report = m.observeReport(report, ctx)
	if report.Mode == "unavailable" {
		return failPXEReport(report, "managed PXE units are not installed"), meta, nil
	}
	if report.Mode == "degraded" || report.Mode == "recovery-required" {
		return failPXEReport(report, report.Message+"; run `nixorium pxe recover`"), meta, nil
	}
	harmonia := m.source.ServiceState(ctx, HarmoniaUnit)
	report.Services = append([]domain.ServiceState{harmonia}, report.Services...)
	if !harmonia.Loaded || !harmonia.Active {
		return failPXEReport(report, "Harmonia cache service is not active"), meta, nil
	}
	if err := m.source.CacheHealth(ctx, report.DHCPAddress, meta.Network.CachePort); err != nil {
		return failPXEReport(report, "Harmonia cache readiness failed: "+err.Error()), meta, nil
	}
	addresses, err := m.source.InterfaceAddresses(meta.Network.Interface)
	if err != nil {
		return failPXEReport(report, "inspect configured interface: "+err.Error()), meta, nil
	}
	if !containsIP(addresses, report.DHCPAddress) {
		return failPXEReport(report, fmt.Sprintf("prepared controller address %s is not assigned to %s; run `nixorium pxe prepare` again", report.DHCPAddress, meta.Network.Interface)), meta, addresses
	}
	if report.Mode == "active" && containsIP(addresses, meta.Controller.StaticIP) {
		return failPXEReport(report, "PXE units are active but the static address is still assigned; run `nixorium pxe recover`"), meta, addresses
	}
	report.State = "ready"
	return report, meta, addresses
}

func (m PXELifecycle) baseReport(ctx context.Context, repository, operation string) domain.PXELifecycleReport {
	report := domain.PXELifecycleReport{SchemaVersion: domain.SchemaVersion, Operation: operation, State: "ready"}
	if meta, err := m.source.LabMeta(ctx, repository); err == nil {
		report.Interface = meta.Network.Interface
		report.DHCPAddress = meta.Controller.DHCPIP
		report.StaticCIDR = fmt.Sprintf("%s/%d", meta.Controller.StaticIP, meta.Network.PrefixLength)
		report.Preparation = m.source.PXEPreparation(ctx, repository, meta)
		if report.Preparation.DHCPAddress != "" {
			report.DHCPAddress = report.Preparation.DHCPAddress
		}
	}
	return m.observeReport(report, ctx)
}

func (m PXELifecycle) observeReport(report domain.PXELifecycleReport, ctx context.Context) domain.PXELifecycleReport {
	listener := m.source.ServiceState(ctx, PXEListenerUnit)
	network := m.source.ServiceState(ctx, PXENetworkUnit)
	observed := ObservePXELifecycle(listener, network, report.Preparation)
	report.Mode = observed.Mode
	report.Services = []domain.ServiceState{listener, network}
	report.Message = observed.Detail
	return report
}

func (m PXELifecycle) stopUnits(ctx context.Context) []string {
	errors := []string{}
	if err := m.source.ControlSystemUnit(ctx, "stop", PXEListenerUnit); err != nil {
		errors = append(errors, err.Error())
	}
	if err := m.source.ControlSystemUnit(ctx, "stop", PXENetworkUnit); err != nil {
		errors = append(errors, err.Error())
	}
	return errors
}

func (m PXELifecycle) failStartedPXE(ctx context.Context, report domain.PXELifecycleReport, message string) domain.PXELifecycleReport {
	cleanupErrors := m.stopUnits(ctx)
	if len(cleanupErrors) > 0 {
		message += "; cleanup: " + strings.Join(cleanupErrors, "; ")
	}
	return failPXEReport(m.observeReport(report, ctx), message)
}

func (m PXELifecycle) verifyRestoredAddress(ctx context.Context, repository string, report *domain.PXELifecycleReport, errors *[]string) {
	meta, err := m.source.LabMeta(ctx, repository)
	if err != nil {
		return
	}
	addresses, err := m.source.InterfaceAddresses(meta.Network.Interface)
	if err != nil {
		*errors = append(*errors, "verify restored interface: "+err.Error())
		return
	}
	if !containsIP(addresses, meta.Controller.StaticIP) {
		*errors = append(*errors, fmt.Sprintf("normal controller address %s is still missing on %s; apply the current controller configuration (Maintenance > Rebuild controller), then run Diagnostics", meta.Controller.StaticIP, meta.Network.Interface))
	}
	report.Interface = meta.Network.Interface
	if report.Preparation.DHCPAddress != "" {
		report.DHCPAddress = report.Preparation.DHCPAddress
	} else {
		report.DHCPAddress = meta.Controller.DHCPIP
	}
	report.StaticCIDR = fmt.Sprintf("%s/%d", meta.Controller.StaticIP, meta.Network.PrefixLength)
}

func missingControllerAddressMessage(meta domain.LabMeta) string {
	return fmt.Sprintf("normal controller address %s is not assigned to %s; apply the current controller configuration (Maintenance > Rebuild controller), then run Diagnostics. Recovery is only for an interrupted PXE transition", meta.Controller.StaticIP, meta.Network.Interface)
}

func containsIP(addresses []string, expected string) bool {
	for _, address := range addresses {
		if address == expected {
			return true
		}
	}
	return false
}

func failPXEReport(report domain.PXELifecycleReport, message string) domain.PXELifecycleReport {
	report.State = "failed"
	report.Message = message
	return report
}

func pxeUnitsActive(report domain.PXELifecycleReport) bool {
	for _, service := range report.Services {
		if service.Active {
			return true
		}
	}
	return false
}
