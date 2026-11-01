package presentation

import (
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

// testModelConfig collects the inputs of newDashboardModel; the zero value is
// an empty report, an empty setup report, no actions and the home screen.
type testModelConfig struct {
	report    domain.StatusReport
	setup     domain.SetupReport
	actions   DashboardActions
	setupMode bool
}

type testModelOption func(*testModelConfig)

func withReport(report domain.StatusReport) testModelOption {
	return func(c *testModelConfig) { c.report = report }
}

func withSetup(setup domain.SetupReport) testModelOption {
	return func(c *testModelConfig) { c.setup = setup }
}

func withActions(actions DashboardActions) testModelOption {
	return func(c *testModelConfig) { c.actions = actions }
}

func withSetupMode() testModelOption {
	return func(c *testModelConfig) { c.setupMode = true }
}

// withReadyLab is the common ready controller with a completed first setup.
func withReadyLab() testModelOption {
	return func(c *testModelConfig) {
		c.report = testDashboardReport("ready")
		c.setup = testSetupReport(true, true, true, true)
	}
}

// newTestModel builds a dashboard model for tests from functional options.
func newTestModel(t testing.TB, opts ...testModelOption) dashboardModel {
	t.Helper()
	var config testModelConfig
	for _, opt := range opts {
		opt(&config)
	}
	return newDashboardModel(config.report, config.setup, config.actions, config.setupMode)
}

// testActions returns an action set with no callbacks, adjusted by overrides.
func testActions(overrides ...func(*DashboardActions)) DashboardActions {
	var actions DashboardActions
	for _, override := range overrides {
		override(&actions)
	}
	return actions
}

func testDashboardReport(mode string) domain.StatusReport {
	report := domain.StatusReport{
		Deployment:     domain.DeploymentStatus{Ready: true},
		PXE:            domain.PXELifecycleState{Mode: mode},
		PXEPreparation: domain.PXEPreparationState{Present: true, Ready: true},
		Services: []domain.ServiceState{
			{Name: "nixorium-harmonia.service", State: "active", Active: true},
		},
	}
	report.Meta.Clients.Count = 2
	report.Meta.Clients.Hosts = []domain.HostMeta{
		{Name: "pc01", IP: "192.0.2.11"},
		{Name: "pc02", IP: "192.0.2.12"},
	}
	report.Meta.Controller.Name = "pc99"
	report.Meta.Controller.DHCPIP = "192.0.2.10"
	report.Meta.Network.Interface = "enp1s0"
	return report
}

func testSetupReport(reviewed, applied, artifacts, _ bool) domain.SetupReport {
	complete := domain.SetupObservation{Complete: true}
	return domain.ReconcileSetup("/deployment", domain.SetupFacts{
		Environment: complete,
		Network:     complete,
		Identity:    complete,
		Credentials: complete,
		Keys:        complete,
		Validation:  complete,
		Review:      domain.SetupObservation{Complete: reviewed},
		Apply:       domain.SetupObservation{Complete: applied},
		Artifacts:   domain.SetupObservation{Complete: artifacts},
	})
}

func testSoftwareCatalogReport() domain.SoftwareCatalogReport {
	return domain.SoftwareCatalogReport{
		SchemaVersion: domain.SoftwareSchemaVersion,
		Operation:     "software-catalog",
		State:         "ready",
		Repository:    "/deployment",
		ManagedFile:   "lab-software.json",
		Clients:       []string{"pc01", "pc02", "pc03"},
		Groups:        map[string][]string{"graphics": {"pc01", "pc02"}},
		Catalog: []domain.SoftwareCatalogItem{
			{ID: "gimp", Label: "GIMP", Summary: "Edit bitmap images", Availability: "available"},
			{ID: "vlc", Label: "VLC", Summary: "Play audio and video", Availability: "available"},
		},
		Packages: []domain.SoftwareDeclaration{{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Origin: "managed"}},
		Issues:   []domain.ValidationIssue{},
	}
}
