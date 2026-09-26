package presentation

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// DemoFrame is a deterministic frame exported from the real dashboard
// renderer. The website replays these frames; it does not reproduce the TUI
// state machine in JavaScript.
type DemoFrame struct {
	Label      string `json:"label"`
	DurationMS int    `json:"durationMs"`
	Text       string `json:"text"`
	ANSI       string `json:"ansi"`
}

type DemoScenario struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Frames      []DemoFrame `json:"frames"`
}

type DemoBundle struct {
	SchemaVersion int            `json:"schemaVersion"`
	SourceCommit  string         `json:"sourceCommit"`
	SourceDate    string         `json:"sourceDate"`
	Terminal      string         `json:"terminal"`
	Synthetic     bool           `json:"synthetic"`
	Scenarios     []DemoScenario `json:"scenarios"`
}

var demoANSI = regexp.MustCompile(`(?:\x1b\][^\x07]*(?:\x07|\x1b\\))|(?:\x1b\[[0-?]*[ -/]*[@-~])`)

const demoServiceAddress = "192.168.1.123"

func RenderDemoBundle(sourceCommit, sourceDate string) DemoBundle {
	return RenderDemoBundleAtSize(sourceCommit, sourceDate, 120, 30)
}

// RenderDemoBundleAtSize renders the deterministic fixture at a supported
// terminal size. Documentation uses 120x30; tests cover all supported layouts.
func RenderDemoBundleAtSize(sourceCommit, sourceDate string, width, height int) DemoBundle {
	return DemoBundle{
		SchemaVersion: 2,
		SourceCommit:  sourceCommit,
		SourceDate:    sourceDate,
		Terminal:      fmt.Sprintf("%dx%d", width, height),
		Synthetic:     true,
		Scenarios: []DemoScenario{
			renderSoftwareDeploymentDemo(sourceCommit, width, height),
			renderInstallationDemo(sourceCommit, width, height),
			renderShutdownDemo(sourceCommit, width, height),
			renderSoftwareProfileDemo(sourceCommit, width, height),
			renderUSBInstallationDemo(sourceCommit, width, height),
		},
	}
}

type demoRecorder struct {
	model  dashboardModel
	frames []DemoFrame
}

func newDemoRecorder(actions DashboardActions, revision string, width, height int) demoRecorder {
	model := newDashboardModel(demoStatus("ready", revision), demoSetupReady(), actions, false)
	model.width, model.height, model.isDark = width, height, true
	model.homeMenu = newDashboardTaskMenu(true, model.width, model.height)
	model.ensureActivitySpinner()
	return demoRecorder{model: model}
}

func (r *demoRecorder) capture(label string, durationMS int) {
	ansi := strings.TrimRight(r.model.View().Content, " \n")
	text := demoANSI.ReplaceAllString(ansi, "")
	r.frames = append(r.frames, DemoFrame{Label: label, DurationMS: durationMS, Text: text, ANSI: ansi})
}

func (r *demoRecorder) key(key tea.KeyPressMsg) tea.Cmd {
	updated, command := r.model.Update(key)
	r.model = updated.(dashboardModel)
	return command
}

func (r *demoRecorder) message(message tea.Msg) tea.Cmd {
	updated, command := r.model.Update(message)
	r.model = updated.(dashboardModel)
	return command
}

func (r *demoRecorder) command(command tea.Cmd) tea.Cmd {
	if command == nil {
		panic("demo transition did not return its expected command")
	}
	return r.message(command())
}

func demoText(value string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: value} }
func demoCode(value rune) tea.KeyPressMsg   { return tea.KeyPressMsg{Code: value} }

func (r *demoRecorder) pressAndCapture(key tea.KeyPressMsg, label string, durationMS int) {
	r.key(key)
	r.capture(label, durationMS)
}

func (r *demoRecorder) typeAndCapture(value, label string) {
	characters := []rune(value)
	for index, character := range characters {
		key := demoText(string(character))
		if character == ' ' {
			key = demoCode(tea.KeySpace)
		}
		r.key(key)
		duration := 110
		if index == len(characters)-1 {
			duration = 700
		}
		r.capture(label, duration)
	}
}

func renderSoftwareDeploymentDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	catalog := demoSoftwareCatalog()
	actions.LoadSoftware = func() domain.SoftwareCatalogReport { return catalog }
	actions.SearchSoftware = func(_ context.Context, query string) domain.SoftwareSearchReport {
		return domain.SoftwareSearchReport{
			SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-search", State: "ready", Repository: "/demo/lab", Query: query,
			Results: []domain.SoftwareCatalogItem{{ID: "inkscape", Label: "Inkscape", Summary: "Create and edit vector graphics", Version: "1.4.2", Availability: "available"}},
			Issues:  []domain.ValidationIssue{},
		}
	}
	actions.PlanSoftware = func(request domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
		confirmation := "SAVE"
		if !request.Present {
			confirmation = "REMOVE"
		}
		return domain.SoftwareChangePlanReport{
			SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-plan", State: "ready", Repository: "/demo/lab", ManagedFile: "lab-software.json",
			Request: request, AffectedClients: []string{"pc01", "pc02", "pc03", "pc04", "pc05"}, ReviewToken: "sha256:demo", Confirmation: confirmation, Issues: []domain.ValidationIssue{},
		}
	}
	actions.SaveSoftware = func(plan domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
		return domain.SoftwareChangeApplyReport{
			SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-change-save", State: "saved", Repository: plan.Repository, ManagedFile: plan.ManagedFile,
			Request: plan.Request, AffectedClients: plan.AffectedClients, Revision: revision, Message: "Software selection saved locally.", Issues: []domain.ValidationIssue{},
		}
	}
	actions.PlanDeployment = func(requested string) domain.DeploymentPlanReport {
		return domain.DeploymentPlanReport{
			SchemaVersion: domain.SchemaVersion, Operation: "deploy-plan", State: "ready", Repository: "/demo/lab", Requested: requested,
			Revision: revision, ColmenaSelector: requested, Targets: demoDeploymentTargets(), BuildFirst: true, Issues: []domain.ValidationIssue{},
		}
	}

	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1200)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to Installation", 550)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to Software", 750)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.key(demoText("/"))
	r.capture("Open Software directly in Search", 900)
	r.typeAndCapture("inkscape", "Type the package name")
	r.message(dashboardSoftwareSearchMsg{id: r.model.software.searchToken(), report: actions.SearchSoftware(context.Background(), "inkscape")})
	r.capture("Find Inkscape in the pinned package set", 1900)
	r.pressAndCapture(demoCode(tea.KeyEnter), "Choose Inkscape from Search", 1900)
	for r.model.software.currentScopeKind() != domain.SoftwareScopeAllClients {
		r.pressAndCapture(demoCode(tea.KeyDown), "Move through declaration scopes", 550)
	}
	r.capture("Choose all current and future clients", 2400)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Review what saving changes", 3000)
	r.command(r.key(demoCode(tea.KeyEnter)))

	r.model.screen = dashboardDeploy
	r.model.deployment.result = domain.DeploymentExecutionReport{}
	r.model.deployment.chosen = map[string]bool{"pc01": true, "pc02": true, "pc03": true, "pc04": true, "pc05": true}
	r.model.deployment.context = "Opened from a saved software change. Review deploys the complete current configuration."
	r.capture("Open contextual client selection", 1800)
	r.model.deployment.context = ""
	r.command(r.command(r.key(demoCode(tea.KeyEnter))))
	r.capture("Review deployment to all five current clients", 2100)
	r.typeAndCapture("DEPLOY", "Type the one-word deployment confirmation")
	deployCommand := r.key(demoCode(tea.KeyEnter))
	r.capture("Press Enter to start deployment", 650)
	if deployCommand == nil {
		panic("deployment confirmation did not start the synthetic operation")
	}

	r.model.screen = dashboardDeploy
	r.model.deployment.applying = true
	r.model.busy = "Building and applying the reviewed deployment"
	r.model.deployment.started = time.Now()
	r.model.deployment.progress = domain.DeploymentProgress{Phase: domain.DeploymentPhaseBuild, Completed: 2, Total: 4, Activity: "Building all five clients from the reviewed revision"}
	r.model.deployment.recent = []string{"Validated clean revision", "Building all five clients from the reviewed revision"}
	r.capture("Build the selected configuration", 1900)
	r.model.deployment.progress = domain.DeploymentProgress{Phase: domain.DeploymentPhaseApply, Completed: 3, Total: 4, TargetCurrent: 3, TargetTotal: 5, Activity: "Activating pc03 over SSH"}
	r.model.deployment.recent = append(r.model.deployment.recent, "Activating clients over SSH · 3 of 5")
	r.capture("Apply to every selected client", 1900)
	r.model.deployment.progress = domain.DeploymentProgress{Phase: domain.DeploymentPhaseVerify, Completed: 4, Total: 4, TargetCurrent: 5, TargetTotal: 5, Activity: "Authenticated all five client system states"}
	r.model.deployment.recent = append(r.model.deployment.recent, "Authenticated all five client system states")
	r.capture("Verify the observed client state", 2000)
	r.message(dashboardDeploymentResultMsg{report: domain.DeploymentExecutionReport{
		SchemaVersion: domain.SchemaVersion, Operation: "deploy-apply", State: "completed", Repository: "/demo/lab", Requested: "@lab", Revision: revision, ColmenaSelector: "@lab",
		Targets: demoDeploymentTargets(), Phase: domain.DeploymentPhaseComplete, BuildCompleted: true, ApplyCompleted: true,
		Verification: domain.DeploymentVerificationSummary{Attempted: 5, Verified: 5, Recorded: 5}, LogPath: "/demo/state/deploy-lab.log", Message: "All five clients report the reviewed revision.", Issues: []domain.ValidationIssue{},
	}})
	r.capture("Deployment completed and verified", 3500)

	return DemoScenario{ID: "software-all-clients", Title: "Add one package to every client", Description: "Search the pinned package set for Inkscape, save it for every current and future client, then explicitly deploy and verify all five configured PCs.", Frames: r.frames}
}

func renderSoftwareProfileDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	catalog := demoSoftwareCatalog()
	catalog.Controller = "pc99"
	profiles := domain.SoftwarePresetCatalog{
		SchemaVersion: domain.SoftwarePresetSchemaVersion,
		DefaultPreset: "essential",
		Presets: []domain.SoftwarePreset{
			{ID: "essential", Label: "Essential", Description: "Browser, terminal, fonts, npm, Pi and OpenCode", Packages: []string{"chromium", "ghostty", "liberation_ttf", "nodejs", "opencode", "pi-coding-agent"}},
			{ID: "general-education", Label: "General education", Description: "Documents, spelling, web access and media playback", Packages: []string{"chromium", "libreoffice-qt", "nodejs", "opencode", "pi-coding-agent", "vlc"}},
			{ID: "programming", Label: "Programming", Description: "Editors and common development toolchains", Packages: []string{"git", "nodejs", "opencode", "pi-coding-agent", "python3", "vscode"}},
			{ID: "graphics", Label: "Graphics and illustration", Description: "Raster, vector and digital painting tools", Packages: []string{"gimp", "inkscape", "krita", "nodejs", "opencode", "pi-coding-agent"}},
			{ID: "multimedia", Label: "Audio and video", Description: "Audio editing, screen recording and video production", Packages: []string{"audacity", "nodejs", "obs-studio", "opencode", "pi-coding-agent", "vlc"}},
			{ID: "cad-3d", Label: "CAD and 3D modelling", Description: "Parametric CAD, modelling and rendering", Packages: []string{"blender", "freecad", "nodejs", "opencode", "pi-coding-agent"}},
			{ID: "stem", Label: "STEM and scientific computing", Description: "Numerical, plotting and symbolic mathematics tools", Packages: []string{"gnuplot", "maxima", "nodejs", "octave", "opencode", "pi-coding-agent"}},
		},
	}
	actions.LoadSoftware = func() domain.SoftwareCatalogReport { return catalog }
	actions.LoadSoftwarePresets = func() domain.SoftwarePresetCatalogReport {
		return domain.SoftwarePresetCatalogReport{SchemaVersion: domain.SoftwarePresetSchemaVersion, Operation: "software-presets", State: "ready", Repository: "/demo/lab", Catalog: &profiles, Fingerprint: "sha256:demo-profiles", Issues: []domain.ValidationIssue{}}
	}
	actions.PlanSoftwarePreset = func(request domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport {
		preset := profiles.Presets[0]
		selected := []domain.SoftwareCatalogItem{}
		additions := []domain.SoftwareDeclaration{}
		excluded := map[string]bool{}
		for _, packageID := range request.Exclude {
			excluded[packageID] = true
		}
		for _, packageID := range preset.Packages {
			if excluded[packageID] {
				continue
			}
			selected = append(selected, domain.SoftwareCatalogItem{ID: packageID, Label: packageID, Summary: "Pinned package", Availability: "available"})
			additions = append(additions, domain.SoftwareDeclaration{Package: packageID, Scope: request.Scope})
		}
		return domain.SoftwarePresetPlanReport{
			SchemaVersion: domain.SoftwarePresetSchemaVersion, Operation: "software-preset-plan", State: "ready", Repository: "/demo/lab", ManagedFile: "lab-software.json",
			Request: request, Preset: preset, SelectedPackages: selected, Additions: additions,
			AffectedController: "pc99", AffectedClients: catalog.Clients, ReviewToken: "sha256:demo-profile-review", Confirmation: "ADD PROFILE", Issues: []domain.ValidationIssue{},
		}
	}
	actions.SaveSoftwarePreset = func(plan domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport {
		return domain.SoftwarePresetApplyReport{
			SchemaVersion: domain.SoftwarePresetSchemaVersion, Operation: "software-preset-save", State: "saved", Repository: plan.Repository, ManagedFile: plan.ManagedFile,
			Request: plan.Request, Preset: plan.Preset, Existing: plan.Existing, Additions: plan.Additions,
			AffectedController: plan.AffectedController, AffectedClients: plan.AffectedClients, Revision: revision, Issues: []domain.ValidationIssue{},
		}
	}
	actions.PlanController = func() domain.ControllerRebuildPlanReport {
		return domain.ControllerRebuildPlanReport{State: "ready", Controller: "pc99", Revision: revision, Issues: []domain.ValidationIssue{}}
	}
	actions.ApplyController = func(plan domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
		return domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Controller: plan.Controller, Revision: plan.Revision, Applied: true, Verified: true, Issues: []domain.ValidationIssue{}}
	}

	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1000)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to Installation", 450)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to Software", 650)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Open Software", 900)
	r.command(r.key(demoText("p")))
	r.capture("Choose a deployment-owned profile", 2200)
	r.pressAndCapture(demoCode(tea.KeyEnter), "Review Essential packages", 1800)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move to the terminal package", 350)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move to the fonts package", 350)
	r.pressAndCapture(demoCode(tea.KeySpace), "Exclude the fonts package", 1500)
	r.pressAndCapture(demoCode(tea.KeyEnter), "Choose where missing packages apply", 1800)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Review the complete profile addition", 2600)
	controllerCommand := r.command(r.key(demoCode(tea.KeyEnter)))
	r.command(controllerCommand)
	r.capture("Controller verified; clients remain unchanged", 2600)

	return DemoScenario{ID: "software-profile", Title: "Add a software profile as one reviewed change", Description: "Choose a deployment-owned profile, exclude one package, preserve existing scopes, save all missing declarations together, and verify the controller before offering a separate client deployment.", Frames: r.frames}
}

func demoDeploymentTargets() []domain.DeploymentTarget {
	return []domain.DeploymentTarget{
		{Name: "pc01", IP: "10.42.0.11"},
		{Name: "pc02", IP: "10.42.0.12"},
		{Name: "pc03", IP: "10.42.0.13"},
		{Name: "pc04", IP: "10.42.0.14"},
		{Name: "pc05", IP: "10.42.0.15"},
	}
}

func renderInstallationDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	actions.LoadSettings = func() (domain.LabSettingsFile, error) { return demoSettings(), nil }
	actions.Refresh = func() (domain.StatusReport, error) {
		report := demoStatus("stopped", revision)
		report.PXEPreparation.Ready = false
		return report, nil
	}
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1100)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to Installation", 700)
	r.key(demoCode(tea.KeyEnter))
	r.capture("Open Installation", 1500)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Choose network boot", 1500)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Review laboratory network settings", 2300)
	r.model.settings.editor = r.model.settings.editor.moveToField(4)
	r.capture("Configure five client computers", 2300)

	r.model.screen = dashboardPXE
	r.model.installation.flow = true
	r.model.installation.stage = 3
	r.model.controller.applying = true
	r.model.controller.started = time.Now()
	r.model.busy = "Building and activating the laboratory controller"
	controllerPhases := []struct {
		phase    string
		current  int
		state    string
		activity string
		label    string
	}{
		{phase: "build", current: 1, state: "running", activity: "Building the reviewed controller configuration", label: "Build the controller configuration"},
		{phase: "activate", current: 2, state: "running", activity: "Activating the reviewed controller configuration", label: "Activate the controller configuration"},
		{phase: "verify", current: 3, state: "running", activity: "Verifying the active controller system", label: "Verify the active controller"},
		{phase: "complete", current: 4, state: "completed", activity: "Controller activation and verification completed", label: "Complete controller activation and verification"},
	}
	for _, phase := range controllerPhases {
		r.model.controller.progress = domain.OperationProgress{SchemaVersion: 1, Operation: "controller-apply", State: phase.state, Phase: phase.phase, Current: phase.current, Total: 4, Recent: []string{phase.activity}}
		if phase.state == "completed" {
			r.model.installation.stage = 4
		}
		r.capture(phase.label, 1200)
	}

	r.model.controller.applying = false
	r.model.installation.stage = 4
	r.model.installation.pxePreparing = true
	r.model.installation.pxeStarted = time.Now()
	r.model.busy = "Preparing client systems and network installation files"
	r.model.installation.pxeProgress = domain.OperationProgress{SchemaVersion: 1, Operation: "pxe-prepare", State: "running", Phase: "clients", Current: 4, Total: 6, Recent: []string{"Building configured client systems from the reviewed revision"}}
	r.capture("Prepare client systems and netboot files", 3000)
	r.model.installation.pxeProgress.Phase = "publish"
	r.model.installation.pxeProgress.Current = 5
	r.model.installation.pxeProgress.Recent = []string{"Publishing immutable artifacts for the LAN installer"}
	r.capture("Publish the prepared LAN artifacts", 2200)
	r.model.installation.pxeProgress.State = "completed"
	r.model.installation.pxeProgress.Phase = "complete"
	r.model.installation.pxeProgress.Current = 6
	r.model.installation.pxeProgress.Recent = []string{"Client systems and network installation files are ready"}
	r.capture("Complete client and netboot preparation", 1500)

	r.model.installation.pxePreparing = false
	r.model.busy = ""
	r.model.installation.stage = 5
	r.model.report.PXEPreparation.Ready = true
	r.capture("Complete every preparation step", 1800)
	r.model.screen = dashboardPXEStartReview
	r.model.installation.startPlan = domain.PXELifecycleReport{SchemaVersion: 1, Operation: "pxe-start-plan", State: "ready", Mode: "ready", Interface: "enp1s0", DHCPAddress: demoServiceAddress, StaticCIDR: "10.42.0.99/24"}
	r.capture("Review the temporary network impact", 3300)
	r.typeAndCapture("START", "Type the one-word network-impact confirmation")
	startCommand := r.key(demoCode(tea.KeyEnter))
	r.capture("Press Enter to start network installation", 650)
	if startCommand == nil {
		panic("PXE confirmation did not start the synthetic operation")
	}

	r.model.screen = dashboardPXE
	r.model.busy = ""
	r.model.installation.stage = 6
	r.model.report.PXE.Mode = "active"
	r.model.report.PXEPreparation.Ready = true
	r.model.message = "Network installation is active."
	r.capture("Complete every controller-side installation step", 1700)
	r.model.installation.flow = false
	r.capture("Follow the installation steps on each client", 4200)
	return DemoScenario{ID: "installation", Title: "Prepare and start network installation", Description: "Review lab settings, prepare configured clients, inspect the controller network change, then start PXE. Disk identity and erasure are confirmed later on each client console.", Frames: r.frames}
}

func renderUSBInstallationDemo(revision string, width, height int) DemoScenario {
	r := newDemoRecorder(demoActions(), revision, width, height)
	r.capture("Overview", 900)
	r.model.screen = dashboardInstallationArea
	r.model.installation = installationModel{flow: true}
	r.model.installationAreaCursor = 1
	r.capture("Choose USB over SSH", 1800)
	r.model.screen = dashboardUSBInstall
	r.model.installation.method = domain.RemoteInstallUSBSSH
	r.model.installation.remote = remoteInstallationModel{stage: remoteInstallSelectHost}
	r.capture("Choose the Nixorium client identity", 1800)
	r.model.installation.remote.host = "pc01"
	r.model.installation.remote.stage = remoteInstallPreparing
	r.model.busy = "Building the selected client closure and immutable installer bundle"
	r.capture("Prepare only the selected client", 2200)
	r.model.busy = ""
	r.model.installation.remote.stage = remoteInstallConsole
	r.model.installation.remote.address = "192.168.1.141"
	r.capture("Enter the live IPv4 address", 2200)
	r.model.installation.remote.stage = remoteInstallFingerprint
	r.model.installation.remote.fingerprint = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	r.capture("Compare the automatically observed fingerprint", 2800)
	r.model.installation.remote.confirmation = "MATCH"
	r.capture("Confirm the physical fingerprint match", 1800)
	r.model.installation.remote.confirmation = ""
	r.model.installation.remote.stage = remoteInstallPassword
	r.model.installation.remote.password = strings.Repeat("x", 12)
	r.model.installation.remote.formField = 2
	r.capture("Enter only the temporary password", 2200)

	preparation := domain.RemoteInstallPreparation{
		OperationID: "0123456789abcdef0123456789abcdef", DeploymentRevision: revision,
		BundlePath:      "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-remote-installer-bundle",
		SystemPath:      "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-pc01-demo",
		Host:            domain.RemoteInstallHost{Name: "pc01", Interface: "enp1s0", LiveIP: "192.168.1.141", StaticIP: "10.42.0.11"},
		Cache:           domain.RemoteInstallCache{URL: "http://10.42.0.99:5000"},
		HostFingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Facts: domain.RemoteMachineFacts{Disks: []domain.RemoteDisk{
			{Path: "/dev/sda", SizeBytes: 8 << 30, Model: "USB ISO", Serial: "USB-DEMO", ExclusionReasons: []string{"boot-media"}},
			{Path: "/dev/nvme0n1", SizeBytes: 128 << 30, Model: "Demo NVMe", Serial: "NVME-DEMO", WWN: "demo-wwn", Eligible: true},
		}},
	}
	r.model.installation.remote.password = ""
	r.model.installation.remote.operationID = preparation.OperationID
	r.model.installation.remote.stage = remoteInstallBootstrap
	r.model.busy = "Verifying signed cache access, importing the installer bundle and probing disks"
	r.capture("Verify the pinned ISO identity and signed cache", 2400)
	r.model.busy = ""
	r.model.installation.remote.stage = remoteInstallSelectDisk
	r.model.installation.remote.diskCursor = 1
	r.model.installation.remote.response = domain.RemoteInstallResponse{
		State: "prepared", OperationID: preparation.OperationID,
		Session: &domain.RemoteInstallSession{OperationID: preparation.OperationID, State: "prepared", Preparation: &preparation},
	}
	r.capture("Select an eligible disk explicitly", 2600)

	plan := domain.RemoteInstallPlanReport{
		State: "ready", OperationID: preparation.OperationID, Host: preparation.Host, Disk: preparation.Facts.Disks[1],
		Revision: revision, BundlePath: preparation.BundlePath, SystemPath: preparation.SystemPath, CacheURL: preparation.Cache.URL,
		ReviewToken: "sha256:" + strings.Repeat("c", 64), Confirmation: "ERASE /dev/nvme0n1 FOR pc01",
	}
	r.model.installation.remote.stage = remoteInstallReview
	r.model.installation.remote.plan = plan
	r.capture("Review physical identity, logical identity and disk", 3200)
	r.model.installation.remote.confirmation = plan.Confirmation
	r.capture("Type the disk-bound destructive confirmation", 2200)
	r.model.installation.remote.confirmation = ""
	r.model.installation.remote.stage = remoteInstallApplying
	r.model.busy = "Revalidating the reviewed identity and dispatching the independent installer job"
	r.capture("Dispatch the independent installer job", 2400)
	r.model.busy = ""
	r.model.installation.remote.stage = remoteInstallResult
	r.model.installation.remote.response = domain.RemoteInstallResponse{
		State: "ready-to-reboot", OperationID: preparation.OperationID,
		Execution: &domain.RemoteInstallExecutionReport{OperationID: preparation.OperationID, Phase: domain.RemoteInstallPhaseReadyToReboot, MutationStarted: true, DiskMayBeModified: true, Installed: true, LogID: "usb-install-0123456789abcdef0123456789abcdef.log"},
		Message:   "installation completed; remove or deprioritize the USB medium before reboot",
	}
	r.capture("Show installed state before reboot", 3000)
	r.model.installation.remote.stage = remoteInstallConfirmReboot
	r.capture("Authorize reboot separately", 2400)
	r.model.installation.remote.stage = remoteInstallResult
	r.model.installation.remote.response = domain.RemoteInstallResponse{
		State: "verified", OperationID: preparation.OperationID,
		Execution: &domain.RemoteInstallExecutionReport{OperationID: preparation.OperationID, Phase: domain.RemoteInstallPhasePostBootVerify, MutationStarted: true, DiskMayBeModified: true, Installed: true, RebootRequested: true, BootVerified: true},
		Message:   "verified pc01 at 10.42.0.11 with the reviewed revision and system closure",
	}
	r.capture("Verify the installed identity after reboot", 3400)
	return DemoScenario{
		ID: "installation-usb", Title: "Install one computer from the official USB ISO",
		Description: "Observe the Ed25519 key automatically, confirm its physical-console fingerprint, select one Nixorium identity and disk, install from the signed controller cache, then authorize reboot and verify the exact system.",
		Frames:      r.frames,
	}
}

func renderShutdownDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	actions.PlanShutdown = func(requested string, _ domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
		return domain.ShutdownPlanReport{
			SchemaVersion: domain.SchemaVersion, Operation: "shutdown-plan", State: "ready", Repository: "/demo/lab", Requested: requested, Policy: domain.ShutdownProtectUnknown,
			Targets: []domain.ShutdownTargetPlan{
				{Name: "pc02", IP: "10.42.0.12", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionActive, Eligible: true, Detail: "Interactive student session active; unsaved work may be lost"},
				{Name: "pc04", IP: "10.42.0.14", Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: domain.ShutdownSessionIdle, Eligible: true},
				{Name: "pc05", IP: "10.42.0.15", Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnavailable, Session: domain.ShutdownSessionUnknown, Eligible: false, Detail: "No management connection"},
			},
			Eligible: 2, ReviewToken: "sha256:demo", Confirmation: "SHUTDOWN", Message: "pc02 and pc04 are eligible; pc02 has an active session.", Issues: []domain.ValidationIssue{},
		}
	}
	actions.ApplyShutdown = func(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
		return domain.ShutdownApplyReport{
			SchemaVersion: domain.SchemaVersion, Operation: "shutdown-apply", State: "partial", Repository: plan.Repository, Requested: plan.Requested, Policy: plan.Policy,
			Targets:  []domain.ShutdownTargetOutcome{{Name: "pc02", State: "accepted", Detail: "Operating-system request accepted with an active session"}, {Name: "pc04", State: "accepted", Detail: "Operating-system request accepted"}, {Name: "pc05", State: "not-sent", Detail: "Unreachable; nothing queued"}},
			Accepted: 2, NotSent: 1, Unconfirmed: 0, Message: "Requests were accepted for pc02 and pc04; physical power state was not observed.", Issues: []domain.ValidationIssue{},
		}
	}

	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1200)
	r.key(demoText("c"))
	r.capture("Open Computers", 1300)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move to client distribution", 450)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move to Shut down computers", 700)
	r.key(demoCode(tea.KeyEnter))
	r.capture("Choose client computers", 1800)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to pc02", 500)
	r.pressAndCapture(demoCode(tea.KeySpace), "Select pc02", 550)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to pc03", 350)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to pc04", 500)
	r.pressAndCapture(demoCode(tea.KeySpace), "Select pc04", 550)
	r.pressAndCapture(demoCode(tea.KeyDown), "Move the cursor to pc05", 500)
	r.pressAndCapture(demoCode(tea.KeySpace), "Select pc05", 700)
	r.capture("Select pc02, pc04, and pc05; controller absent", 2200)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Active sessions will shut down; unreachable clients are not sent", 3300)
	r.typeAndCapture("SHUTDOWN", "Type the one-word confirmation")
	shutdownCommand := r.key(demoCode(tea.KeyEnter))
	r.capture("Press Enter to send the reviewed requests", 650)
	r.command(shutdownCommand)
	r.capture("Report accepted and not-sent outcomes", 3900)
	return DemoScenario{ID: "shutdown", Title: "Review a controlled shutdown", Description: "The controller is excluded. An active session produces a visible data-loss warning but still receives the reviewed request; an unreachable client receives nothing, and the result does not claim physical power-off.", Frames: r.frames}
}

func demoActions() DashboardActions {
	fail := func(name string) { panic("demo callback invoked without a synthetic implementation: " + name) }
	return DashboardActions{
		RunningVersion: "demo",
		LoadInitial: func() (domain.StatusReport, domain.SetupReport, error) {
			fail("LoadInitial")
			return domain.StatusReport{}, domain.SetupReport{}, nil
		},
		LoadDoctor:    func() (domain.DoctorReport, error) { fail("LoadDoctor"); return domain.DoctorReport{}, nil },
		Refresh:       func() (domain.StatusReport, error) { fail("Refresh"); return domain.StatusReport{}, nil },
		LoadSetup:     func() domain.SetupReport { fail("LoadSetup"); return domain.SetupReport{} },
		LoadSetupKeys: func() domain.KeyReconcileReport { fail("LoadSetupKeys"); return domain.KeyReconcileReport{} },
		ReconcileSetupKeys: func() (domain.KeyReconcileReport, error) {
			fail("ReconcileSetupKeys")
			return domain.KeyReconcileReport{}, nil
		},
		ImportSetupKey: func(string, string) (domain.KeyImportReport, error) {
			fail("ImportSetupKey")
			return domain.KeyImportReport{}, nil
		},
		SaveSetupConfiguration: func() domain.ConfigurationSaveReport {
			fail("SaveSetupConfiguration")
			return domain.ConfigurationSaveReport{}
		},
		InstallSetupSecrets: func() domain.ActionReport { fail("InstallSetupSecrets"); return domain.ActionReport{} },
		LoadHosts:           func() (domain.HostsReport, error) { fail("LoadHosts"); return domain.HostsReport{}, nil },
		LoadConfigurationState: func() domain.ConfigurationStateReport {
			fail("LoadConfigurationState")
			return domain.ConfigurationStateReport{}
		},
		LoadSoftware: func() domain.SoftwareCatalogReport { fail("LoadSoftware"); return domain.SoftwareCatalogReport{} },
		SearchSoftware: func(context.Context, string) domain.SoftwareSearchReport {
			fail("SearchSoftware")
			return domain.SoftwareSearchReport{}
		},
		PlanSoftware: func(domain.SoftwareChangeRequest) domain.SoftwareChangePlanReport {
			fail("PlanSoftware")
			return domain.SoftwareChangePlanReport{}
		},
		SaveSoftware: func(domain.SoftwareChangePlanReport) domain.SoftwareChangeApplyReport {
			fail("SaveSoftware")
			return domain.SoftwareChangeApplyReport{}
		},
		LoadSoftwarePresets: func() domain.SoftwarePresetCatalogReport {
			fail("LoadSoftwarePresets")
			return domain.SoftwarePresetCatalogReport{}
		},
		PlanSoftwarePreset: func(domain.SoftwarePresetRequest) domain.SoftwarePresetPlanReport {
			fail("PlanSoftwarePreset")
			return domain.SoftwarePresetPlanReport{}
		},
		SaveSoftwarePreset: func(domain.SoftwarePresetPlanReport) domain.SoftwarePresetApplyReport {
			fail("SaveSoftwarePreset")
			return domain.SoftwarePresetApplyReport{}
		},
		PlanShutdown: func(string, domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
			fail("PlanShutdown")
			return domain.ShutdownPlanReport{}
		},
		ApplyShutdown: func(domain.ShutdownPlanReport) domain.ShutdownApplyReport {
			fail("ApplyShutdown")
			return domain.ShutdownApplyReport{}
		},
		PlanDeployment: func(string) domain.DeploymentPlanReport { fail("PlanDeployment"); return domain.DeploymentPlanReport{} },
		ApplyDeployment: func(domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
			fail("ApplyDeployment")
			return domain.DeploymentExecutionReport{}
		},
		PlanController: func() domain.ControllerRebuildPlanReport {
			fail("PlanController")
			return domain.ControllerRebuildPlanReport{}
		},
		ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
			fail("ApplyController")
			return domain.ControllerRebuildExecutionReport{}
		},
		LoadControllerProgress: func() (domain.OperationProgress, error) {
			fail("LoadControllerProgress")
			return domain.OperationProgress{}, nil
		},
		LoadServices:   func() domain.ServicesReport { fail("LoadServices"); return domain.ServicesReport{} },
		RestartService: func(string) domain.ServiceActionReport { fail("RestartService"); return domain.ServiceActionReport{} },
		LoadLogs:       func() domain.OperationLogsReport { fail("LoadLogs"); return domain.OperationLogsReport{} },
		LoadLog:        func(string) domain.OperationLogReport { fail("LoadLog"); return domain.OperationLogReport{} },
		LoadGitReview:  func() domain.GitReviewReport { fail("LoadGitReview"); return domain.GitReviewReport{} },
		PlanGitCommit:  func(string) domain.GitCommitPlanReport { fail("PlanGitCommit"); return domain.GitCommitPlanReport{} },
		ApplyGitCommit: func(domain.GitCommitPlanReport) domain.GitCommitReport {
			fail("ApplyGitCommit")
			return domain.GitCommitReport{}
		},
		CheckUpdate: func() domain.UpdateCheckReport { fail("CheckUpdate"); return domain.UpdateCheckReport{} },
		PlanUpdate:  func(string, bool, bool) domain.UpdatePlanReport { fail("PlanUpdate"); return domain.UpdatePlanReport{} },
		PlanUpdateWithProgress: func(string, bool, bool, func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
			fail("PlanUpdateWithProgress")
			return domain.UpdatePlanReport{}
		},
		SaveUpdate: func(domain.UpdatePlanReport) domain.UpdateApplyReport {
			fail("SaveUpdate")
			return domain.UpdateApplyReport{}
		},
		LoadSettings: func() (domain.LabSettingsFile, error) { fail("LoadSettings"); return domain.LabSettingsFile{}, nil },
		PlanSettings: func(domain.LabSettingsFile) domain.ConfigPlanReport {
			fail("PlanSettings")
			return domain.ConfigPlanReport{}
		},
		SaveSettings: func(domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigurationSaveReport {
			fail("SaveSettings")
			return domain.ConfigurationSaveReport{}
		},
		ChangePassword: func(string, domain.LabSettingsFile, *os.File, io.Writer) (domain.LabSettingsFile, error) {
			fail("ChangePassword")
			return domain.LabSettingsFile{}, nil
		},
		PreparePXE: func() domain.ActionReport { fail("PreparePXE"); return domain.ActionReport{} },
		LoadPXEProgress: func() (domain.OperationProgress, error) {
			fail("LoadPXEProgress")
			return domain.OperationProgress{}, nil
		},
		PlanPXEStart: func() domain.PXELifecycleReport { fail("PlanPXEStart"); return domain.PXELifecycleReport{} },
		StartPXE:     func() domain.PXELifecycleReport { fail("StartPXE"); return domain.PXELifecycleReport{} },
		StopPXE:      func() domain.PXELifecycleReport { fail("StopPXE"); return domain.PXELifecycleReport{} },
		RecoverPXE:   func() domain.PXELifecycleReport { fail("RecoverPXE"); return domain.PXELifecycleReport{} },
		PrepareRemoteInstall: func(string) (domain.RemoteInstallResponse, error) {
			fail("PrepareRemoteInstall")
			return domain.RemoteInstallResponse{}, nil
		},
		ObserveRemoteInstall: func(string) (string, error) {
			fail("ObserveRemoteInstall")
			return "", nil
		},
		BootstrapRemoteInstall: func(string, string, string, []byte) (domain.RemoteInstallResponse, error) {
			fail("BootstrapRemoteInstall")
			return domain.RemoteInstallResponse{}, nil
		},
		RemoteInstallRequest: func(domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			fail("RemoteInstallRequest")
			return domain.RemoteInstallResponse{}, nil
		},
		LoadRemoteInstall: func() (domain.RemoteInstallResponse, error) {
			return domain.RemoteInstallResponse{State: "ready"}, nil
		},
	}
}

func demoStatus(mode, revision string) domain.StatusReport {
	report := domain.StatusReport{SchemaVersion: domain.SchemaVersion, Operation: "status", State: "ready", Repository: "/demo/lab", Deployment: domain.DeploymentStatus{Ready: true}, PXE: domain.PXELifecycleState{Mode: mode}, PXEPreparation: domain.PXEPreparationState{Present: true, Ready: true, Revision: revision, DHCPAddress: demoServiceAddress}}
	report.Meta.SchemaVersion = domain.SchemaVersion
	report.Meta.Version = "demo"
	report.Meta.DeploymentMode = "laboratory"
	report.Meta.Controller.Name = "pc99"
	report.Meta.Controller.Number = 99
	report.Meta.Controller.StaticIP = "10.42.0.99"
	report.Meta.Controller.DHCPIP = demoServiceAddress
	report.Meta.Network.Base = "10.42.0.0"
	report.Meta.Network.PrefixLength = 24
	report.Meta.Network.Interface = "enp1s0"
	report.Meta.Clients.Count = 5
	for index := 1; index <= 5; index++ {
		report.Meta.Clients.Hosts = append(report.Meta.Clients.Hosts, domain.HostMeta{Name: fmt.Sprintf("pc%02d", index), IP: fmt.Sprintf("10.42.0.%d", 10+index), Interface: "enp1s0"})
	}
	return report
}

func demoSetupReady() domain.SetupReport {
	complete := domain.SetupObservation{Complete: true}
	return domain.ReconcileSetup("/demo/lab", domain.SetupFacts{Environment: complete, Network: complete, Identity: complete, Credentials: complete, Keys: complete, Validation: complete, Review: complete, Apply: complete, Artifacts: complete})
}

func demoSoftwareCatalog() domain.SoftwareCatalogReport {
	return domain.SoftwareCatalogReport{
		Controller: "pc99", SchemaVersion: domain.SoftwareSchemaVersion, Operation: "software-catalog", State: "ready", Repository: "/demo/lab", ManagedFile: "lab-software.json",
		Clients: []string{"pc01", "pc02", "pc03", "pc04", "pc05"}, Groups: map[string][]string{"graphics": {"pc01", "pc02"}},
		Catalog:  []domain.SoftwareCatalogItem{{ID: "inkscape", Label: "Inkscape", Summary: "Create and edit vector graphics", Version: "1.4.2", Availability: "available"}, {ID: "gimp", Label: "GIMP", Summary: "Edit bitmap images", Version: "3.0.4", Availability: "available"}},
		Packages: []domain.SoftwareDeclaration{{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Origin: "managed"}}, Issues: []domain.ValidationIssue{},
	}
}

func demoSettings() domain.LabSettingsFile {
	return domain.LabSettingsFile{SchemaVersion: domain.SettingsSchemaVersion, Lab: domain.LabSettings{
		DeploymentMode: "controller", MasterDHCPIP: demoServiceAddress, NetworkBase: "10.42.0.0", NetworkPrefix: 24, PCCount: 5, MasterHostNumber: 99,
		InterfaceName: "enp1s0", TeacherUser: "teacher", StudentUser: "student", TeacherPassword: domain.DefaultPasswordHash, StudentPassword: domain.DefaultPasswordHash,
		AdminPassword: domain.DefaultPasswordHash, HomepageURL: "https://school.example/", StudentGitName: "Student", StudentGitEmail: "student@example.invalid",
		AdminGitName: "Lab Administrator", AdminGitEmail: "admin@example.invalid", TimeZone: "Europe/Rome", DefaultLocale: "en_US.UTF-8", ExtraLocale: "it_IT.UTF-8",
		KeyboardLayout: "it", ConsoleKeyMap: "it2", VeyonNativeHosts: []string{},
	}}
}
