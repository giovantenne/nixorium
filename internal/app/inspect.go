package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	minimumFreeBytes = uint64(10 * 1024 * 1024 * 1024)
	sshProbeTimeout  = 500 * time.Millisecond
)

type DoctorOptions struct {
	Full bool
}

type Source interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error)
	GitState(context.Context, string) (domain.GitState, error)
	ServiceState(context.Context, string) domain.ServiceState
	ArtifactState(string, string, string) domain.ArtifactState
	PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState
	InterfaceAddresses(string) ([]string, error)
	AddressOwners(string) ([]string, error)
	FreeBytes(string) (uint64, error)
	CacheKeyState(context.Context, string) (domain.CacheKeyState, error)
	CacheHealth(context.Context, string, int) error
	ListeningPorts() ([]domain.PortUse, error)
	SSHStatus(context.Context, []domain.HostMeta, time.Duration) map[string]domain.SSHProbe
	ControllerBuild(context.Context, string, string) error
	CommandAvailable(string) bool
}

type Inspector struct {
	source Source
	now    func() time.Time
}

func NewInspector(source Source) *Inspector {
	return &Inspector{source: source, now: time.Now}
}

func (i *Inspector) Status(ctx context.Context, repository string) (domain.StatusReport, error) {
	root, err := filepath.Abs(repository)
	if err != nil {
		return domain.StatusReport{}, fmt.Errorf("resolve repository path: %w", err)
	}

	meta, err := i.source.LabMeta(ctx, root)
	if err != nil {
		return domain.StatusReport{}, fmt.Errorf("evaluate labMeta: %w", err)
	}
	deployment, err := i.source.DeploymentStatus(ctx, root)
	if err != nil {
		return domain.StatusReport{}, fmt.Errorf("evaluate deploymentStatus: %w", err)
	}
	gitState, err := i.source.GitState(ctx, root)
	if err != nil {
		return domain.StatusReport{}, fmt.Errorf("inspect Git worktree: %w", err)
	}

	preparation := i.source.PXEPreparation(ctx, root, meta)
	artifacts := []domain.ArtifactState{
		i.source.ArtifactState(root, "kernel", "result-kernel/bzImage"),
		i.source.ArtifactState(root, "initrd", "result-initrd/initrd"),
		i.source.ArtifactState(root, "iPXE script", "result-ipxe/netboot.ipxe"),
		i.source.ArtifactState(root, "iPXE firmware", "assets/ipxe/snponly.efi"),
	}
	if preparation.Ready {
		artifacts = preparation.Artifacts
	}
	listener := i.source.ServiceState(ctx, PXEListenerUnit)
	network := i.source.ServiceState(ctx, PXENetworkUnit)
	pxeState := ObservePXELifecycle(listener, network, preparation)
	report := domain.StatusReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "status",
		GeneratedAt:   i.now().UTC(),
		State:         "ready",
		Repository:    root,
		Meta:          meta,
		Deployment:    deployment,
		Git:           gitState,
		Services: []domain.ServiceState{
			i.source.ServiceState(ctx, HarmoniaUnit),
			listener,
			network,
		},
		PXE:            pxeState,
		PXEPreparation: preparation,
		Artifacts:      artifacts,
	}
	if !deployment.Ready {
		report.State = "action-required"
	}
	if gitState.Dirty {
		report.Warnings = append(report.Warnings, "deployment repository has uncommitted changes")
	}
	if preparation.Present && !preparation.Ready {
		report.Warnings = append(report.Warnings, "managed PXE preparation is stale or invalid: "+preparation.Detail)
	}
	if pxeState.Mode == "degraded" || pxeState.Mode == "recovery-required" {
		report.Warnings = append(report.Warnings, "PXE lifecycle requires recovery: "+pxeState.Detail)
	}
	for _, service := range report.Services {
		if !service.Loaded {
			report.Warnings = append(report.Warnings, service.Name+" is not installed")
		} else if service.Name == "nixorium-harmonia.service" && !service.Active {
			report.Warnings = append(report.Warnings, service.Name+" is inactive")
		}
	}
	return report, nil
}

func (i *Inspector) Hosts(ctx context.Context, repository string) (domain.HostsReport, error) {
	root, err := filepath.Abs(repository)
	if err != nil {
		return domain.HostsReport{}, fmt.Errorf("resolve repository path: %w", err)
	}
	meta, err := i.source.LabMeta(ctx, root)
	if err != nil {
		return domain.HostsReport{}, fmt.Errorf("evaluate labMeta: %w", err)
	}
	hosts := hostStatuses(meta.Clients.Hosts, i.source.SSHStatus(ctx, meta.Clients.Hosts, sshProbeTimeout))
	state := "available"
	for _, host := range hosts {
		if host.SSH != domain.SSHAvailable {
			state = "partial"
			break
		}
	}
	return domain.HostsReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "hosts",
		GeneratedAt:   i.now().UTC(),
		State:         state,
		Repository:    root,
		Hosts:         hosts,
	}, nil
}

func (i *Inspector) Doctor(ctx context.Context, repository string, options DoctorOptions) (domain.DoctorReport, error) {
	status, err := i.Status(ctx, repository)
	if err != nil {
		return domain.DoctorReport{}, err
	}

	report := domain.DoctorReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "doctor",
		GeneratedAt:   i.now().UTC(),
		State:         "healthy",
		Repository:    status.Repository,
	}
	add := func(finding domain.Finding) {
		report.Findings = append(report.Findings, finding)
		if finding.Level == domain.LevelError {
			report.State = "errors"
		} else if finding.Level == domain.LevelWarning && report.State == "healthy" {
			report.State = "warnings"
		}
	}

	add(domain.Finding{ID: "CONFIG-EVAL", Level: domain.LevelOK, Summary: "Typed deployment configuration evaluates", Evidence: fmt.Sprintf("labMeta schema %d, Nixorium %s", status.Meta.SchemaVersion, status.Meta.Version)})
	add(domain.Finding{ID: "NETWORK-SUBNET", Level: domain.LevelOK, Summary: "Static network and host offsets passed typed validation", Evidence: fmt.Sprintf("%s/%d; controller %s", status.Meta.Network.Base, status.Meta.Network.PrefixLength, status.Meta.Controller.StaticIP)})
	if status.Deployment.Ready {
		add(domain.Finding{ID: "DEPLOYMENT-READY", Level: domain.LevelOK, Summary: "Deployment has no reported readiness blockers"})
	} else {
		add(domain.Finding{ID: "DEPLOYMENT-READY", Level: domain.LevelError, Summary: "Deployment is not ready", Evidence: strings.Join(status.Deployment.Issues, "; "), Remediation: "Resolve each deploymentStatus issue before installation or deployment."})
	}

	if status.Git.Dirty {
		add(domain.Finding{ID: "GIT-WORKTREE", Level: domain.LevelWarning, Summary: "Deployment repository has uncommitted changes", Evidence: fmt.Sprintf("%d changed path(s)", status.Git.Changes), Remediation: "Review the Git diff and keep management-generated changes separate from unrelated work."})
	} else {
		add(domain.Finding{ID: "GIT-WORKTREE", Level: domain.LevelOK, Summary: "Deployment repository worktree is clean"})
	}

	i.addNetworkFindings(status, add)
	if status.PXEPreparation.Ready {
		add(domain.Finding{ID: "PXE-PREPARATION", Level: domain.LevelOK, Summary: "Managed PXE artifacts and client closures are current", Evidence: status.PXEPreparation.Detail})
	} else if status.PXEPreparation.Present {
		add(domain.Finding{ID: "PXE-PREPARATION", Level: domain.LevelWarning, Summary: "Managed PXE preparation is stale or invalid", Evidence: status.PXEPreparation.Detail, Remediation: "Review the reported mismatch and run `nixorium pxe prepare` again."})
	} else {
		add(domain.Finding{ID: "PXE-PREPARATION", Level: domain.LevelWarning, Summary: "Managed PXE preparation has not run", Remediation: "Run `nixorium pxe prepare` before starting installation mode."})
	}
	for _, artifact := range status.Artifacts {
		level := domain.LevelOK
		remediation := ""
		summary := artifact.Name + " is present"
		if !artifact.Present {
			level = domain.LevelWarning
			summary = artifact.Name + " is not prepared"
			remediation = "Prepare netboot artifacts before starting PXE installation mode."
		}
		add(domain.Finding{ID: "ARTIFACT-" + identifier(artifact.Name), Level: level, Summary: summary, Evidence: artifact.Path, Remediation: remediation})
	}

	for _, service := range status.Services {
		if !service.Loaded {
			add(domain.Finding{ID: serviceFindingID(service.Name), Level: domain.LevelWarning, Summary: service.Name + " is not installed", Remediation: "Install the managed-service controller module when it becomes available."})
		} else if service.Name == PXEListenerUnit || service.Name == PXENetworkUnit {
			continue
		} else if !service.Active {
			add(domain.Finding{ID: serviceFindingID(service.Name), Level: domain.LevelWarning, Summary: service.Name + " is inactive", Evidence: service.State})
		} else {
			add(domain.Finding{ID: serviceFindingID(service.Name), Level: domain.LevelOK, Summary: service.Name + " is active"})
		}
	}
	switch status.PXE.Mode {
	case "active":
		add(domain.Finding{ID: "PXE-LIFECYCLE", Level: domain.LevelOK, Summary: "PXE installation mode is active", Evidence: status.PXE.Detail})
	case "ready", "stopped":
		add(domain.Finding{ID: "PXE-LIFECYCLE", Level: domain.LevelOK, Summary: "PXE installation mode is " + status.PXE.Mode, Evidence: status.PXE.Detail})
	case "degraded", "recovery-required":
		add(domain.Finding{ID: "PXE-LIFECYCLE", Level: domain.LevelError, Summary: "PXE lifecycle requires recovery", Evidence: status.PXE.Detail, Remediation: "Run `nixorium pxe recover` before starting installation mode again."})
	default:
		add(domain.Finding{ID: "PXE-LIFECYCLE", Level: domain.LevelWarning, Summary: "PXE lifecycle is unavailable", Evidence: status.PXE.Detail})
	}

	i.addCacheKeyFinding(ctx, status.Repository, add)
	i.addCacheHealthFinding(ctx, status, add)
	i.addPortFindings(status, add)

	if free, freeErr := i.source.FreeBytes(status.Repository); freeErr == nil {
		if free < minimumFreeBytes {
			add(domain.Finding{ID: "DISK-FREE", Level: domain.LevelWarning, Summary: "Available disk space may be too low for Nix builds", Evidence: formatGiB(free), Remediation: "Free Nix store or filesystem space before building all clients."})
		} else {
			add(domain.Finding{ID: "DISK-FREE", Level: domain.LevelOK, Summary: "Disk has at least 10 GiB available", Evidence: formatGiB(free)})
		}
	} else {
		add(domain.Finding{ID: "DISK-FREE", Level: domain.LevelWarning, Summary: "Available disk space could not be determined", Evidence: freeErr.Error()})
	}

	for _, command := range []string{"nix", "git", "systemctl", "ssh", "colmena"} {
		if i.source.CommandAvailable(command) {
			add(domain.Finding{ID: "COMMAND-" + strings.ToUpper(command), Level: domain.LevelOK, Summary: command + " is available"})
		} else {
			add(domain.Finding{ID: "COMMAND-" + strings.ToUpper(command), Level: domain.LevelError, Summary: command + " is unavailable", Remediation: "Rebuild the controller with the current Nixorium configuration."})
		}
	}

	hosts := hostStatuses(status.Meta.Clients.Hosts, i.source.SSHStatus(ctx, status.Meta.Clients.Hosts, sshProbeTimeout))
	i.addSSHFinding(hosts, add)
	if options.Full {
		if buildErr := i.source.ControllerBuild(ctx, status.Repository, status.Meta.Controller.Name); buildErr != nil {
			add(domain.Finding{ID: "CONTROLLER-BUILD", Level: domain.LevelError, Summary: "Controller configuration failed to build", Evidence: buildErr.Error(), Remediation: "Review the Nix build error before applying or deploying configuration."})
		} else {
			add(domain.Finding{ID: "CONTROLLER-BUILD", Level: domain.LevelOK, Summary: "Controller configuration builds successfully"})
		}
	}

	return report, nil
}

func (i *Inspector) addNetworkFindings(status domain.StatusReport, add func(domain.Finding)) {
	addresses, addressErr := i.source.InterfaceAddresses(status.Meta.Network.Interface)
	if addressErr != nil {
		add(domain.Finding{ID: "NETWORK-INTERFACE", Level: domain.LevelError, Summary: "Configured network interface is unavailable", Evidence: addressErr.Error(), Remediation: "Choose an existing controller interface before preparing PXE mode."})
	} else {
		add(domain.Finding{ID: "NETWORK-INTERFACE", Level: domain.LevelOK, Summary: "Configured network interface exists", Evidence: strings.Join(addresses, ", ")})
		if containsAddress(addresses, status.Meta.Controller.DHCPIP) {
			add(domain.Finding{ID: "NETWORK-DHCP-IP", Level: domain.LevelOK, Summary: "Configured controller DHCP address is assigned"})
		} else {
			add(domain.Finding{ID: "NETWORK-DHCP-IP", Level: domain.LevelWarning, Summary: "Configured controller DHCP address is not currently assigned", Evidence: "configured: " + status.Meta.Controller.DHCPIP, Remediation: "Review the current DHCP lease and rebuild netboot artifacts after accepting any change."})
		}
	}

	owners, ownerErr := i.source.AddressOwners(status.Meta.Controller.StaticIP)
	if ownerErr != nil {
		add(domain.Finding{ID: "NETWORK-STATIC-IP", Level: domain.LevelWarning, Summary: "Local ownership of the controller static address could not be checked", Evidence: ownerErr.Error()})
	} else if len(owners) == 0 {
		add(domain.Finding{ID: "NETWORK-STATIC-IP", Level: domain.LevelWarning, Summary: "Controller static address is not currently assigned", Evidence: status.Meta.Controller.StaticIP, Remediation: "Restore normal controller networking unless PXE installation mode intentionally removed the address."})
	} else if len(owners) == 1 && owners[0] == status.Meta.Network.Interface {
		add(domain.Finding{ID: "NETWORK-STATIC-IP", Level: domain.LevelOK, Summary: "Controller static address belongs to the configured interface"})
	} else {
		add(domain.Finding{ID: "NETWORK-STATIC-IP", Level: domain.LevelError, Summary: "Controller static address is assigned on an unexpected local interface", Evidence: strings.Join(owners, ", "), Remediation: "Resolve the local address conflict before deployment or PXE mode."})
	}
}

func (i *Inspector) addCacheKeyFinding(ctx context.Context, repository string, add func(domain.Finding)) {
	key, err := i.source.CacheKeyState(ctx, repository)
	if err != nil {
		add(domain.Finding{ID: "CACHE-SIGNING-KEY", Level: domain.LevelError, Summary: "Harmonia signing key could not be verified", Evidence: err.Error(), Remediation: "Inspect the key files without copying private material into Git or logs."})
		return
	}
	if !key.PrivatePresent {
		add(domain.Finding{ID: "CACHE-SIGNING-KEY", Level: domain.LevelWarning, Summary: "Harmonia signing key is missing", Remediation: "Generate the signing key outside Git before starting Harmonia."})
		return
	}
	if key.PrivateMode&0077 != 0 {
		add(domain.Finding{ID: "CACHE-SIGNING-KEY", Level: domain.LevelError, Summary: "Harmonia signing key permissions are too broad", Evidence: fmt.Sprintf("mode %04o", key.PrivateMode), Remediation: "Restrict the signing key to its owner."})
		return
	}
	if !key.PublicPresent {
		add(domain.Finding{ID: "CACHE-SIGNING-KEY", Level: domain.LevelError, Summary: "Harmonia public key is missing", Remediation: "Derive and add only the public key to the private deployment."})
		return
	}
	if !key.Matches {
		add(domain.Finding{ID: "CACHE-SIGNING-KEY", Level: domain.LevelError, Summary: "Harmonia public key does not match the signing key", Remediation: "Derive the public key again; do not overwrite the private key."})
		return
	}
	add(domain.Finding{ID: "CACHE-SIGNING-KEY", Level: domain.LevelOK, Summary: "Harmonia signing/public key pair matches and private permissions are restricted", Evidence: fmt.Sprintf("private mode %04o", key.PrivateMode)})
}

func (i *Inspector) addCacheHealthFinding(ctx context.Context, status domain.StatusReport, add func(domain.Finding)) {
	addresses := []string{status.Meta.Controller.StaticIP}
	if status.Meta.Controller.DHCPIP != status.Meta.Controller.StaticIP && status.Meta.Controller.DHCPIP != "MASTER_DHCP_IP" {
		addresses = append(addresses, status.Meta.Controller.DHCPIP)
	}
	errorsByAddress := []string{}
	for _, address := range addresses {
		if err := i.source.CacheHealth(ctx, address, status.Meta.Network.CachePort); err == nil {
			add(domain.Finding{ID: "CACHE-HEALTH", Level: domain.LevelOK, Summary: "Binary cache responds with valid Nix cache metadata", Evidence: fmt.Sprintf("%s:%d", address, status.Meta.Network.CachePort)})
			return
		} else {
			errorsByAddress = append(errorsByAddress, address+": "+err.Error())
		}
	}
	add(domain.Finding{ID: "CACHE-HEALTH", Level: domain.LevelWarning, Summary: "Binary cache is not reachable", Evidence: strings.Join(errorsByAddress, "; "), Remediation: "Start Harmonia and verify its configured address, port, and signing key."})
}

func (i *Inspector) addPortFindings(status domain.StatusReport, add func(domain.Finding)) {
	uses, err := i.source.ListeningPorts()
	if err != nil {
		add(domain.Finding{ID: "PXE-PORTS", Level: domain.LevelWarning, Summary: "PXE port conflicts could not be checked", Evidence: err.Error()})
		return
	}
	pxeActive := serviceActive(status.Services, "nixorium-pxe.service")
	required := []domain.PortUse{
		{Protocol: "udp", Port: 67},
		{Protocol: "udp", Port: 69},
		{Protocol: "udp", Port: 4011},
		{Protocol: "tcp", Port: status.Meta.Network.PXEHTTPPort},
	}
	occupied := []string{}
	for _, expected := range required {
		if hasPortUse(uses, expected) {
			occupied = append(occupied, fmt.Sprintf("%s/%d", expected.Protocol, expected.Port))
		}
	}
	if !pxeActive && len(occupied) > 0 {
		add(domain.Finding{ID: "PXE-PORTS", Level: domain.LevelError, Summary: "PXE ports are occupied while installation mode is inactive", Evidence: strings.Join(occupied, ", "), Remediation: "Identify and stop conflicting listeners before starting PXE installation mode."})
		return
	}
	if pxeActive && len(occupied) < len(required) {
		add(domain.Finding{ID: "PXE-PORTS", Level: domain.LevelError, Summary: "PXE installation mode is active but not all expected ports are listening", Evidence: strings.Join(occupied, ", "), Remediation: "Inspect nixorium-pxe.service logs."})
		return
	}
	add(domain.Finding{ID: "PXE-PORTS", Level: domain.LevelOK, Summary: "No conflicting PXE listeners were detected", Evidence: strings.Join(occupied, ", ")})
}

func (i *Inspector) addSSHFinding(hosts []domain.HostStatus, add func(domain.Finding)) {
	available := 0
	unavailable := []string{}
	for _, host := range hosts {
		if host.SSH == domain.SSHAvailable {
			available++
		} else {
			unavailable = append(unavailable, host.Name)
		}
	}
	if len(unavailable) == 0 {
		add(domain.Finding{ID: "CLIENT-SSH", Level: domain.LevelOK, Summary: "Every configured client accepts TCP connections on SSH", Evidence: fmt.Sprintf("%d/%d available", available, len(hosts))})
		return
	}
	sort.Strings(unavailable)
	add(domain.Finding{ID: "CLIENT-SSH", Level: domain.LevelWarning, Summary: "SSH is unavailable or unknown on some configured clients", Evidence: fmt.Sprintf("%d/%d available; unavailable or unknown: %s", available, len(hosts), strings.Join(unavailable, ", ")), Remediation: "Power on expected clients and check their static network path before deployment."})
}

func hostStatuses(hosts []domain.HostMeta, probes map[string]domain.SSHProbe) []domain.HostStatus {
	statuses := make([]domain.HostStatus, 0, len(hosts))
	for _, host := range hosts {
		probe, found := probes[host.Name]
		if !found {
			probe = domain.SSHProbe{
				Reachability: domain.ReachabilityUnknown,
				SSH:          domain.SSHUnknown,
				Detail:       "no probe result",
			}
		}
		statuses = append(statuses, domain.HostStatus{
			Name:         host.Name,
			IP:           host.IP,
			Role:         "client",
			Reachability: probe.Reachability,
			SSH:          probe.SSH,
			Detail:       probe.Detail,
		})
	}
	return statuses
}

func containsAddress(addresses []string, expected string) bool {
	for _, address := range addresses {
		if address == expected {
			return true
		}
	}
	return false
}

func serviceActive(services []domain.ServiceState, name string) bool {
	for _, service := range services {
		if service.Name == name {
			return service.Active
		}
	}
	return false
}

func hasPortUse(uses []domain.PortUse, expected domain.PortUse) bool {
	for _, use := range uses {
		if use == expected {
			return true
		}
	}
	return false
}

func identifier(value string) string {
	return strings.ToUpper(strings.ReplaceAll(value, " ", "-"))
}

func serviceFindingID(name string) string {
	name = strings.TrimSuffix(name, ".service")
	name = strings.TrimPrefix(name, "nixorium-")
	return "SERVICE-" + strings.ToUpper(name)
}

func formatGiB(bytes uint64) string {
	return fmt.Sprintf("%.1f GiB available", float64(bytes)/(1024*1024*1024))
}
