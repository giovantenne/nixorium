package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSource struct {
	meta         domain.LabMeta
	deployment   domain.DeploymentStatus
	git          domain.GitState
	addresses    []string
	owners       []string
	free         uint64
	key          domain.CacheKeyState
	cacheErr     error
	ports        []domain.PortUse
	ssh          map[string]domain.SSHProbe
	sshCalls     int
	revision     string
	current      map[string]domain.HostSystemProbe
	currentCalls int
	history      domain.DeploymentHistory
	historyErr   error
	buildErr     error
	built        bool
	preparation  domain.PXEPreparationState
	services     map[string]domain.ServiceState
}

func readyFake() *fakeSource {
	source := &fakeSource{
		deployment: domain.DeploymentStatus{Ready: true},
		git:        domain.GitState{Available: true},
		addresses:  []string{"192.0.2.10", "10.0.0.99"},
		owners:     []string{"eth0"},
		free:       20 * 1024 * 1024 * 1024,
		key: domain.CacheKeyState{
			PrivatePresent: true,
			PublicPresent:  true,
			PrivateMode:    0600,
			Matches:        true,
		},
		ssh: map[string]domain.SSHProbe{
			"pc01": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable},
			"pc02": {Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable},
		},
		revision: "0123456789abcdef0123456789abcdef01234567",
		current: map[string]domain.HostSystemProbe{
			"pc01": {SystemPath: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc01-system", Revision: "0123456789abcdef0123456789abcdef01234567"},
			"pc02": {SystemPath: "/nix/store/oldoldoldoldoldoldoldoldoldoldol-pc02-system", Revision: "fedcba9876543210fedcba9876543210fedcba98"},
		},
	}
	source.meta.SchemaVersion = 2
	source.meta.Version = "test"
	source.meta.Controller.Name = "pc99"
	source.meta.Controller.StaticIP = "10.0.0.99"
	source.meta.Controller.DHCPIP = "192.0.2.10"
	source.meta.Network.Interface = "eth0"
	source.meta.Network.CachePort = 5000
	source.meta.Network.PXEHTTPPort = 8080
	source.meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "10.0.0.1"},
		{Name: "pc02", IP: "10.0.0.2"},
	}
	source.history = domain.DeploymentHistory{
		SchemaVersion: domain.SchemaVersion,
		Hosts: map[string]domain.LastSuccessfulDeployment{
			"pc01": {
				Revision:   source.revision,
				SystemPath: source.current["pc01"].SystemPath,
				VerifiedAt: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	return source
}

func (f *fakeSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}

func (f *fakeSource) DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error) {
	return f.deployment, nil
}

func (f *fakeSource) GitState(context.Context, string) (domain.GitState, error) {
	return f.git, nil
}

func (f *fakeSource) GitRevision(context.Context, string) (string, error) { return f.revision, nil }

func (f *fakeSource) ServiceState(_ context.Context, name string) domain.ServiceState {
	if service, found := f.services[name]; found {
		return service
	}
	return domain.ServiceState{Name: name, Loaded: true, State: "inactive"}
}

func (f *fakeSource) ArtifactState(_, name, path string) domain.ArtifactState {
	return domain.ArtifactState{Name: name, Path: path, Present: true}
}

func (f *fakeSource) PXEPreparation(context.Context, string, domain.LabMeta) domain.PXEPreparationState {
	return f.preparation
}

func (f *fakeSource) InterfaceAddresses(string) ([]string, error) {
	if f.addresses == nil {
		return nil, errors.New("missing interface")
	}
	return f.addresses, nil
}

func (f *fakeSource) AddressOwners(string) ([]string, error) { return f.owners, nil }
func (f *fakeSource) FreeBytes(string) (uint64, error)       { return f.free, nil }
func (f *fakeSource) CacheKeyState(context.Context, string) (domain.CacheKeyState, error) {
	return f.key, nil
}
func (f *fakeSource) CacheHealth(context.Context, string, int) error { return f.cacheErr }
func (f *fakeSource) ListeningPorts() ([]domain.PortUse, error)      { return f.ports, nil }
func (f *fakeSource) SSHStatus(context.Context, []domain.HostMeta, time.Duration) map[string]domain.SSHProbe {
	f.sshCalls++
	return f.ssh
}
func (f *fakeSource) CurrentSystems(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.HostSystemProbe {
	f.currentCalls++
	result := make(map[string]domain.HostSystemProbe, len(hosts))
	for _, host := range hosts {
		if probe, found := f.current[host.Name]; found {
			result[host.Name] = probe
		}
	}
	return result
}
func (f *fakeSource) DeploymentHistory(string) (domain.DeploymentHistory, error) {
	return f.history, f.historyErr
}
func (f *fakeSource) ControllerBuild(context.Context, string, string) error {
	f.built = true
	return f.buildErr
}
func (f *fakeSource) CommandAvailable(string) bool { return true }

func TestStatusReportsReadinessAndDirtyTree(t *testing.T) {
	source := readyFake()
	source.deployment = domain.DeploymentStatus{Ready: false, Issues: []string{"missing key"}}
	source.git = domain.GitState{Available: true, Dirty: true, Changes: 2}
	inspector := NewInspector(source)
	inspector.now = func() time.Time { return time.Unix(0, 0) }

	report, err := inspector.Status(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "action-required" {
		t.Fatalf("state = %q, want action-required", report.State)
	}
	if len(report.Warnings) != 2 {
		t.Fatalf("warnings = %v, want dirty-tree and inactive-cache warnings", report.Warnings)
	}
	if report.Warnings[1] != "nixorium-harmonia.service is inactive" {
		t.Fatalf("warnings = %v, want inactive-cache warning", report.Warnings)
	}
	if source.sshCalls != 0 {
		t.Fatalf("cheap status performed %d SSH probe(s)", source.sshCalls)
	}
	if source.currentCalls != 0 {
		t.Fatalf("cheap status performed %d authenticated host observation(s)", source.currentCalls)
	}
}

func TestDoctorDistinguishesErrorsAndWarnings(t *testing.T) {
	source := readyFake()
	source.deployment = domain.DeploymentStatus{Ready: false, Issues: []string{"default password"}}
	source.addresses = []string{"10.0.0.99"}
	source.free = 1024
	source.key.PrivateMode = 0644

	report, err := NewInspector(source).Doctor(context.Background(), ".", DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.HasErrors() || report.State != "errors" {
		t.Fatalf("doctor state = %q, hasErrors = %v", report.State, report.HasErrors())
	}
	assertFinding(t, report.Findings, "DEPLOYMENT-READY", domain.LevelError)
	assertFinding(t, report.Findings, "NETWORK-DHCP-IP", domain.LevelWarning)
	assertFinding(t, report.Findings, "CACHE-SIGNING-KEY", domain.LevelError)
	assertFinding(t, report.Findings, "DISK-FREE", domain.LevelWarning)
}

func TestDoctorFullBuildIsExplicit(t *testing.T) {
	source := readyFake()
	inspector := NewInspector(source)

	report, err := inspector.Doctor(context.Background(), ".", DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if source.built {
		t.Fatal("default doctor unexpectedly built the controller")
	}
	assertNoFinding(t, report.Findings, "CONTROLLER-BUILD")

	report, err = inspector.Doctor(context.Background(), ".", DoctorOptions{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if !source.built {
		t.Fatal("full doctor did not build the controller")
	}
	assertFinding(t, report.Findings, "CONTROLLER-BUILD", domain.LevelOK)
}

func TestDoctorReportsPXEPortConflictAndOfflineHost(t *testing.T) {
	source := readyFake()
	source.ports = []domain.PortUse{{Protocol: "udp", Port: 67}}
	source.ssh["pc02"] = domain.SSHProbe{
		Reachability: domain.ReachabilityUnreachable,
		SSH:          domain.SSHUnknown,
		Detail:       "probe timed out",
	}

	report, err := NewInspector(source).Doctor(context.Background(), ".", DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, report.Findings, "PXE-PORTS", domain.LevelError)
	assertFinding(t, report.Findings, "CLIENT-SSH", domain.LevelWarning)
	if source.sshCalls != 1 {
		t.Fatalf("doctor performed %d SSH probe batches, want one", source.sshCalls)
	}
}

func TestHostsPreservesInventoryOrderAndUnknownProbeResult(t *testing.T) {
	source := readyFake()
	delete(source.ssh, "pc02")

	report, err := NewInspector(source).Hosts(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "partial" || len(report.Hosts) != 2 || report.Hosts[0].Name != "pc01" {
		t.Fatalf("hosts report = %+v, want ordered partial inventory", report)
	}
	if report.Hosts[1].Reachability != domain.ReachabilityUnknown || report.Hosts[1].SSH != domain.SSHUnknown || report.Hosts[1].Detail != "no probe result" {
		t.Fatalf("missing probe status = %+v, want explicit unknown", report.Hosts[1])
	}
	if report.Hosts[0].Deployment != domain.DeploymentCurrent || report.Hosts[1].Deployment != domain.DeploymentUnknown || report.Deployment.Current != 1 || report.Deployment.Unknown != 1 {
		t.Fatalf("deployment reconciliation = %+v / %+v", report.Deployment, report.Hosts)
	}
	if report.DesiredRevision != source.revision || report.Hosts[0].DesiredRevision != source.revision {
		t.Fatalf("desired revision was not propagated: %+v", report)
	}
	if report.Hosts[0].LastSuccessfulDeploy == nil || report.Hosts[0].LastSuccessfulDeploy.Revision != source.revision || report.Hosts[1].LastSuccessfulDeploy != nil {
		t.Fatalf("last successful deployments = %+v", report.Hosts)
	}
}

func TestHostsReportsOutdatedAuthenticatedSystem(t *testing.T) {
	source := readyFake()
	report, err := NewInspector(source).Hosts(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if report.Deployment.Current != 1 || report.Deployment.Outdated != 1 || report.Deployment.Unknown != 0 {
		t.Fatalf("deployment summary = %+v", report.Deployment)
	}
	if report.Hosts[1].Deployment != domain.DeploymentOutdated || report.Hosts[1].CurrentRevision == report.Hosts[1].DesiredRevision {
		t.Fatalf("outdated host = %+v", report.Hosts[1])
	}
	if source.currentCalls != 1 {
		t.Fatalf("authenticated observation batches = %d, want one", source.currentCalls)
	}
}

func TestHostsPreservesLiveStateWhenDeploymentHistoryIsInvalid(t *testing.T) {
	source := readyFake()
	source.historyErr = errors.New("invalid history")
	report, err := NewInspector(source).Hosts(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "partial" || !strings.Contains(report.HistoryDetail, "invalid history") {
		t.Fatalf("history warning = %+v", report)
	}
	if report.Deployment.Current != 1 || report.Deployment.Outdated != 1 {
		t.Fatalf("live state was lost: %+v", report.Deployment)
	}
}

func TestDoctorReportsStaleManagedPXEPreparation(t *testing.T) {
	source := readyFake()
	source.preparation = domain.PXEPreparationState{
		Present: true,
		Detail:  "prepared revision differs from deployment revision",
	}

	report, err := NewInspector(source).Doctor(context.Background(), ".", DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, report.Findings, "PXE-PREPARATION", domain.LevelWarning)
}

func TestStatusAndDoctorReportPXERecoveryState(t *testing.T) {
	source := readyFake()
	source.services = map[string]domain.ServiceState{
		PXENetworkUnit: {Name: PXENetworkUnit, Loaded: true, Active: true, State: "active"},
	}
	status, err := NewInspector(source).Status(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if status.PXE.Mode != "recovery-required" {
		t.Fatalf("PXE mode = %q, want recovery-required", status.PXE.Mode)
	}
	doctor, err := NewInspector(source).Doctor(context.Background(), ".", DoctorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFinding(t, doctor.Findings, "PXE-LIFECYCLE", domain.LevelError)
}

func assertFinding(t *testing.T, findings []domain.Finding, id string, level domain.Level) {
	t.Helper()
	for _, finding := range findings {
		if finding.ID == id {
			if finding.Level != level {
				t.Fatalf("%s level = %s, want %s", id, finding.Level, level)
			}
			return
		}
	}
	t.Fatalf("finding %s not found", id)
}

func assertNoFinding(t *testing.T, findings []domain.Finding, id string) {
	t.Helper()
	for _, finding := range findings {
		if finding.ID == id {
			t.Fatalf("unexpected finding %s", id)
		}
	}
}
