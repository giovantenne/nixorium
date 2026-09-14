package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
)

const CacheServiceID = "cache"
const CacheRestartUnit = "nixorium-restart-cache.service"

type ServiceManagementSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	ServiceState(context.Context, string) domain.ServiceState
	PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState
	CacheHealth(context.Context, string, int) error
	ControlSystemUnit(context.Context, string, string) error
}

type ServiceManager struct {
	source ServiceManagementSource
}

func NewServiceManager(source ServiceManagementSource) *ServiceManager {
	return &ServiceManager{source: source}
}

func (m *ServiceManager) Status(ctx context.Context, repository string) domain.ServicesReport {
	report := domain.ServicesReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "services",
		State:         "healthy",
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return serviceStatusIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return serviceStatusIssue(report, "configuration", fmt.Sprintf("evaluate labMeta: %v", err))
	}

	preparation := m.source.PXEPreparation(ctx, root, meta)
	cache := m.cacheStatus(ctx, meta, preparation)
	listener := m.source.ServiceState(ctx, PXEListenerUnit)
	network := m.source.ServiceState(ctx, PXENetworkUnit)
	pxe := ObservePXELifecycle(listener, network, preparation)
	pxeHealthy := pxe.Mode == "stopped" || pxe.Mode == "ready" || pxe.Mode == "active"
	pxeService := domain.ManagedService{
		ID:       "pxe",
		Name:     "PXE installation mode",
		Purpose:  "On-demand client installation listeners and transactional network state",
		Mode:     "on-demand",
		State:    pxe.Mode,
		Healthy:  pxeHealthy,
		Detail:   pxe.Detail,
		Actions:  []string{},
		Commands: []string{"nixorium pxe prepare", "nixorium pxe start", "nixorium pxe stop", "nixorium pxe recover"},
		Units:    []domain.ServiceState{listener, network},
	}
	report.Services = []domain.ManagedService{cache, pxeService}
	if !cache.Healthy || !pxeService.Healthy {
		report.State = "degraded"
	}
	return report
}

func (m *ServiceManager) Restart(ctx context.Context, repository, serviceID string) domain.ServiceActionReport {
	status := m.Status(ctx, repository)
	report := domain.ServiceActionReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "service-restart",
		State:         "blocked",
		Repository:    status.Repository,
		Action:        "restart",
		Service:       serviceID,
		RetrySafe:     true,
		Issues:        append([]domain.ValidationIssue{}, status.Issues...),
	}
	if serviceID != CacheServiceID {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "service", Message: "only the binary cache may be restarted through service management"})
		report.Message = "service restart was not started"
		return report
	}
	report.Unit = CacheRestartUnit
	if len(status.Issues) > 0 || len(status.Services) == 0 {
		report.Message = "service restart preflight failed; no action was started"
		return report
	}
	report.Previous = status.Services[0].Units[0]
	if !report.Previous.Loaded {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "service", Message: "binary cache unit is not installed"})
		report.Message = "service restart was not started"
		return report
	}
	if err := m.source.ControlSystemUnit(ctx, "start", CacheRestartUnit); err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("binary cache restart failed: %v; inspect the unit journal and retry", err)
		return report
	}
	after := m.Status(ctx, status.Repository)
	if len(after.Services) > 0 && len(after.Services[0].Units) > 0 {
		report.Current = after.Services[0].Units[0]
	}
	if len(after.Issues) > 0 || len(after.Services) == 0 || !after.Services[0].Healthy {
		report.State = "failed"
		report.Message = "binary cache restart completed but health verification failed; inspect the unit journal and retry"
		return report
	}
	report.Verified = true
	report.State = "completed"
	report.Message = "binary cache restarted and verified healthy"
	return report
}

func (m *ServiceManager) cacheStatus(ctx context.Context, meta domain.LabMeta, preparation domain.PXEPreparationState) domain.ManagedService {
	unit := m.source.ServiceState(ctx, HarmoniaUnit)
	service := domain.ManagedService{
		ID:      CacheServiceID,
		Name:    "Binary cache",
		Purpose: "Serve signed Nix closures to offline-first clients",
		Mode:    "persistent",
		State:   unit.State,
		Actions: []string{"restart"},
		Units:   []domain.ServiceState{unit},
	}
	switch {
	case !unit.Loaded:
		service.Detail = "managed cache unit is not installed"
	case !unit.Active:
		service.Detail = "managed cache unit is not active"
	default:
		addresses := []string{}
		if preparation.Ready && preparation.DHCPAddress != "" {
			addresses = append(addresses, preparation.DHCPAddress)
		}
		if meta.Controller.StaticIP != preparation.DHCPAddress || !preparation.Ready {
			addresses = append(addresses, meta.Controller.StaticIP)
		}
		if meta.Controller.DHCPIP != meta.Controller.StaticIP && meta.Controller.DHCPIP != preparation.DHCPAddress {
			addresses = append(addresses, meta.Controller.DHCPIP)
		}
		for _, address := range addresses {
			if m.source.CacheHealth(ctx, address, meta.Network.CachePort) == nil {
				service.State = "healthy"
				service.Healthy = true
				service.Detail = fmt.Sprintf("managed cache unit is active and HTTP-ready at %s:%d", address, meta.Network.CachePort)
				return service
			}
		}
		service.State = "unhealthy"
		service.Detail = "cache process is active but its HTTP readiness check failed"
	}
	return service
}

func serviceStatusIssue(report domain.ServicesReport, field, message string) domain.ServicesReport {
	report.State = "degraded"
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}
