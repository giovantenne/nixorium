package app

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const remoteBundleSafetyMultiplier = 2
const remoteInstallHeadroomBytes = 2 * 1024 * 1024 * 1024

type RemoteInstallDispatch struct {
	Accepted  bool
	Uncertain bool
	LogID     string
	Receipt   domain.RemoteInstallReceipt
}

type RemoteInstallSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	CurrentRevision(context.Context, string) (string, error)
	ClientOperationActive() (bool, error)
	ServiceState(context.Context, string) domain.ServiceState
	RemoteIdentityReachable(context.Context, string) (bool, error)
	ReserveRemoteInstall(context.Context, domain.RemoteInstallPlan, string) (domain.RemoteInstallReservation, error)
	RevalidateRemoteInstall(context.Context, domain.RemoteInstallPreparation, domain.RemoteInstallPlan) error
	DispatchRemoteInstall(context.Context, domain.RemoteInstallPlan) (RemoteInstallDispatch, error)
}

type RemoteInstallManager struct {
	source RemoteInstallSource
	now    func() time.Time
}

func NewRemoteInstallManager(source RemoteInstallSource) *RemoteInstallManager {
	return &RemoteInstallManager{source: source, now: time.Now}
}

func (m *RemoteInstallManager) Plan(ctx context.Context, preparation domain.RemoteInstallPreparation, diskPath string) domain.RemoteInstallPlanReport {
	report := domain.RemoteInstallPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "usb-install-plan",
		State:         "blocked",
		Method:        domain.RemoteInstallUSBSSH,
		OperationID:   preparation.OperationID,
		Host:          preparation.Host,
		Revision:      preparation.DeploymentRevision,
		BundlePath:    preparation.BundlePath,
		SystemPath:    preparation.SystemPath,
		CacheURL:      preparation.Cache.URL,
		Issues:        []domain.ValidationIssue{},
		Preparation:   preparation,
	}
	root, err := filepath.Abs(preparation.Repository)
	if err != nil {
		return remotePlanIssue(report, "repository", fmt.Sprintf("resolve deployment repository: %v", err))
	}
	report.Repository = root
	if root != preparation.Repository {
		return remotePlanIssue(report, "repository", "prepared repository must already be canonical")
	}
	if len(preparation.Issues) > 0 {
		return remotePlanIssue(report, "preparation", "remote installation preparation contains unresolved issues")
	}
	if preparation.PreparedAt.IsZero() || preparation.OperationID == "" || preparation.Facts.BootID == "" {
		return remotePlanIssue(report, "preparation", "remote installation preparation is incomplete")
	}
	if err := domain.ValidateRemoteMachineFacts(preparation.Facts); err != nil {
		return remotePlanIssue(report, "facts", err.Error())
	}
	if preparation.BundleClosureBytes == 0 || preparation.SystemClosureBytes == 0 || !remoteStorePath(preparation.BundlePath) {
		return remotePlanIssue(report, "preparation", "prepared closure measurements are incomplete")
	}
	if preparation.BundleClosureBytes > ^uint64(0)/remoteBundleSafetyMultiplier {
		return remotePlanIssue(report, "resources", "remote installer closure measurement overflows")
	}
	requiredLiveBytes := preparation.BundleClosureBytes * remoteBundleSafetyMultiplier
	if preparation.Facts.StoreAvailableBytes < requiredLiveBytes || preparation.Facts.MemoryAvailableBytes < requiredLiveBytes {
		return remotePlanIssue(report, "resources", fmt.Sprintf("live installer needs at least %d bytes free in memory and store", requiredLiveBytes))
	}
	if preparation.SystemClosureBytes > ^uint64(0)-remoteInstallHeadroomBytes {
		return remotePlanIssue(report, "resources", "system closure measurement overflows")
	}

	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return remotePlanIssue(report, "configuration", fmt.Sprintf("evaluate client inventory: %v", err))
	}
	configured, found := remoteConfiguredHost(meta, preparation.Host.Name)
	if !found || configured.IP != preparation.Host.StaticIP || configured.Interface != preparation.Host.Interface {
		return remotePlanIssue(report, "host", "prepared host identity, address, or interface differs from deployment inventory")
	}
	if preparation.Host.LiveIP == meta.Controller.StaticIP || preparation.Host.LiveIP == meta.Controller.DHCPIP {
		return remotePlanIssue(report, "host.liveIp", "live client address conflicts with a controller address")
	}
	if !remoteInterfaceHasAddress(preparation.Facts, preparation.Host.Interface, preparation.Host.LiveIP) {
		return remotePlanIssue(report, "host.interface", "declared client interface does not carry the observed live address")
	}

	revision, err := m.source.CurrentRevision(ctx, root)
	if err != nil || revision != preparation.DeploymentRevision {
		return remotePlanIssue(report, "revision", "deployment revision differs from the immutable preparation")
	}
	active, err := m.source.ClientOperationActive()
	if err != nil {
		return remotePlanIssue(report, "operation", "inspect controller operation gate: "+err.Error())
	}
	if active {
		return remotePlanIssue(report, "operation", "another controller or client operation is active or reserved")
	}
	if conflict := remotePXEConflict(m.source, ctx); conflict != "" {
		return remotePlanIssue(report, "installation", conflict)
	}
	if reachable, probeErr := m.source.RemoteIdentityReachable(ctx, preparation.Host.StaticIP); probeErr != nil {
		return remotePlanIssue(report, "host.staticIp", "could not exclude a duplicate static identity: "+probeErr.Error())
	} else if reachable {
		return remotePlanIssue(report, "host.staticIp", "configured static identity is already reachable")
	}

	disk, found := remotePreparedDisk(preparation.Facts.Disks, diskPath)
	if !found {
		return remotePlanIssue(report, "disk", "selected disk is absent from the prepared inventory")
	}
	report.Disk = disk
	if !disk.Eligible || len(disk.ExclusionReasons) != 0 {
		return remotePlanIssue(report, "disk", "selected disk is excluded: "+strings.Join(disk.ExclusionReasons, ", "))
	}
	if disk.SizeBytes < preparation.SystemClosureBytes+remoteInstallHeadroomBytes {
		return remotePlanIssue(report, "disk", "selected disk lacks measured closure capacity and installation headroom")
	}

	report.Plan = domain.RemoteInstallPlan{
		SchemaVersion:      domain.RemoteInstallSchemaVersion,
		OperationID:        preparation.OperationID,
		BootID:             preparation.Facts.BootID,
		DeploymentRevision: preparation.DeploymentRevision,
		SystemPath:         preparation.SystemPath,
		Host:               preparation.Host,
		Cache:              preparation.Cache,
		Disk:               remotePlanDisk(disk),
		AdminPublicKey:     preparation.AdminPublicKey,
		HostKeyPublic:      preparation.HostKeyPublic,
	}
	if err := domain.ValidateRemoteInstallPlan(report.Plan); err != nil {
		return remotePlanIssue(report, "plan", err.Error())
	}
	report.State = "ready"
	report.ExpiresAt = m.now().UTC().Add(domain.RemoteInstallReviewWindow)
	report.ReviewToken = domain.RemoteInstallReviewToken(report)
	report.Confirmation = fmt.Sprintf("ERASE %s FOR %s", disk.Path, preparation.Host.Name)
	report.Message = "Review the verified live machine, logical identity, cache, revision, and exact disk before applying."
	return report
}

func (m *RemoteInstallManager) Apply(ctx context.Context, plan domain.RemoteInstallPlanReport, expectedToken string) domain.RemoteInstallExecutionReport {
	report := domain.RemoteInstallExecutionReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "usb-install-apply",
		State:         "blocked",
		OperationID:   plan.OperationID,
		Phase:         domain.RemoteInstallPhaseRevalidate,
		Issues:        []domain.ValidationIssue{},
	}
	if plan.HasErrors() || expectedToken == "" || expectedToken != plan.ReviewToken || expectedToken != domain.RemoteInstallReviewToken(plan) {
		return remoteExecutionIssue(report, "review", "remote installation review token does not match the frozen plan")
	}
	if !m.now().UTC().Before(plan.ExpiresAt) {
		return remoteExecutionIssue(report, "expiry", "remote installation review expired before apply")
	}
	if err := domain.ValidateRemoteInstallPlan(plan.Plan); err != nil {
		return remoteExecutionIssue(report, "plan", err.Error())
	}
	if conflict := remotePXEConflict(m.source, ctx); conflict != "" {
		return remoteExecutionIssue(report, "installation", conflict)
	}
	revision, err := m.source.CurrentRevision(ctx, plan.Repository)
	if err != nil || revision != plan.Revision {
		return remoteExecutionIssue(report, "revision", "deployment revision changed after review")
	}
	meta, err := m.source.LabMeta(ctx, plan.Repository)
	if err != nil {
		return remoteExecutionIssue(report, "configuration", "client inventory could not be rechecked")
	}
	configured, found := remoteConfiguredHost(meta, plan.Host.Name)
	if !found || configured.IP != plan.Host.StaticIP || configured.Interface != plan.Host.Interface {
		return remoteExecutionIssue(report, "host", "client inventory changed after review")
	}
	if reachable, probeErr := m.source.RemoteIdentityReachable(ctx, plan.Host.StaticIP); probeErr != nil || reachable {
		return remoteExecutionIssue(report, "host.staticIp", "static client identity can no longer be proven unused")
	}

	reservation, err := m.source.ReserveRemoteInstall(ctx, plan.Plan, expectedToken)
	if err != nil {
		return remoteExecutionIssue(report, "operation", err.Error())
	}
	resolved := false
	defer func() {
		if resolved {
			_ = reservation.ReleaseResolved()
		}
	}()
	if err := m.source.RevalidateRemoteInstall(ctx, plan.Preparation, plan.Plan); err != nil {
		resolved = true
		return remoteExecutionIssue(report, "revalidate", err.Error())
	}
	dispatch, err := m.source.DispatchRemoteInstall(ctx, plan.Plan)
	if err != nil {
		report.LogID = dispatch.LogID
		if dispatch.Uncertain {
			report.State = "reconciliation-required"
			report.Phase = domain.RemoteInstallPhaseReconciliationRequired
			report.DispatchUncertain = true
			report.DiskMayBeModified = true
			report.Message = "Apply dispatch may have reached the live client; reconcile the same operation ID before any retry."
			return report
		}
		resolved = true
		return remoteExecutionIssue(report, "dispatch", err.Error())
	}
	if !dispatch.Accepted {
		resolved = true
		return remoteExecutionIssue(report, "dispatch", "remote helper did not accept the reviewed operation")
	}
	report.State = dispatch.Receipt.State
	report.Phase = dispatch.Receipt.Phase
	report.MutationStarted = dispatch.Receipt.MutationStarted
	report.DiskMayBeModified = dispatch.Receipt.DiskMayBeModified
	report.Installed = dispatch.Receipt.Installed
	report.LogID = dispatch.LogID
	report.Message = dispatch.Receipt.Message
	if dispatch.Receipt.OperationID != plan.OperationID {
		report.State = "reconciliation-required"
		report.Phase = domain.RemoteInstallPhaseReconciliationRequired
		report.DispatchUncertain = true
		report.Message = "Remote receipt identity does not match the dispatched operation."
	}
	return report
}

func remoteConfiguredHost(meta domain.LabMeta, name string) (domain.HostMeta, bool) {
	for _, host := range meta.Clients.Hosts {
		if host.Name == name {
			return host, true
		}
	}
	return domain.HostMeta{}, false
}

func remoteInterfaceHasAddress(facts domain.RemoteMachineFacts, name, address string) bool {
	for _, networkInterface := range facts.Interfaces {
		if networkInterface.Name == name && slices.Contains(networkInterface.Addresses, address) {
			return true
		}
	}
	return false
}

func remotePreparedDisk(disks []domain.RemoteDisk, path string) (domain.RemoteDisk, bool) {
	for _, disk := range disks {
		if disk.Path == path {
			return disk, true
		}
	}
	return domain.RemoteDisk{}, false
}

func remotePlanDisk(disk domain.RemoteDisk) domain.RemoteDisk {
	disk.Eligible = false
	disk.ExclusionReasons = nil
	return disk
}

func remotePXEConflict(source RemoteInstallSource, ctx context.Context) string {
	listener := source.ServiceState(ctx, PXEListenerUnit)
	network := source.ServiceState(ctx, PXENetworkUnit)
	observed := ObservePXELifecycle(listener, network, domain.PXEPreparationState{})
	switch observed.Mode {
	case "active":
		return "network installation is active; stop it before starting USB installation"
	case "degraded", "recovery-required":
		return "controller network or PXE installation state requires recovery"
	default:
		return ""
	}
}

func remotePlanIssue(report domain.RemoteInstallPlanReport, field, message string) domain.RemoteInstallPlanReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}

func remoteExecutionIssue(report domain.RemoteInstallExecutionReport, field, message string) domain.RemoteInstallExecutionReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	report.Message = message
	return report
}

func remoteStorePath(value string) bool {
	return domain.ValidStorePath(value)
}
