package presentation

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/giovantenne/nixorium/internal/domain"
)

type DashboardActions struct {
	Refresh         func() (domain.StatusReport, error)
	LoadHosts       func() (domain.HostsReport, error)
	PlanDeployment  func(string) domain.DeploymentPlanReport
	ApplyDeployment func(domain.DeploymentPlanReport) domain.DeploymentExecutionReport
	PreparePXE      func() domain.ActionReport
	PlanPXEStart    func() domain.PXELifecycleReport
	StartPXE        func() domain.PXELifecycleReport
	StopPXE         func() domain.PXELifecycleReport
	RecoverPXE      func() domain.PXELifecycleReport
}

type dashboardScreen int

const (
	dashboardHome dashboardScreen = iota
	dashboardHosts
	dashboardDeploy
	dashboardDeployReview
	dashboardPXE
	dashboardPXEStartReview
)

type dashboardModel struct {
	report       domain.StatusReport
	actions      DashboardActions
	screen       dashboardScreen
	busy         string
	message      string
	confirmation string
	startPlan    domain.PXELifecycleReport
	hosts        domain.HostsReport
	deployCursor int
	deployChosen map[string]bool
	deployPlan   domain.DeploymentPlanReport
	deployResult domain.DeploymentExecutionReport
	deploying    bool
}

type dashboardPlanMsg struct {
	report domain.PXELifecycleReport
}

type dashboardOperationMsg struct {
	message string
	report  domain.StatusReport
	err     error
	screen  dashboardScreen
}

type dashboardHostsMsg struct {
	report domain.HostsReport
	err    error
}

type dashboardDeploymentPlanMsg struct {
	report domain.DeploymentPlanReport
}

type dashboardDeploymentResultMsg struct {
	report domain.DeploymentExecutionReport
}

func RunDashboard(report domain.StatusReport, actions DashboardActions) error {
	_, err := tea.NewProgram(dashboardModel{report: report, actions: actions}).Run()
	return err
}

func (dashboardModel) Init() tea.Cmd { return nil }

func (model dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case dashboardPlanMsg:
		model.busy = ""
		model.startPlan = message.report
		if message.report.HasErrors() {
			model.message = message.report.Message
			model.screen = dashboardPXE
			return model, nil
		}
		if message.report.Mode == "active" {
			model.message = "PXE installation mode is already active."
			model.screen = dashboardPXE
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardPXEStartReview
		return model, nil
	case dashboardOperationMsg:
		model.busy = ""
		model.message = message.message
		if message.err != nil {
			model.message += "; refresh failed: " + message.err.Error()
		} else {
			model.report = message.report
		}
		model.screen = message.screen
		return model, nil
	case dashboardHostsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Computer status refresh failed: " + message.err.Error()
		} else {
			model.hosts = message.report
			model.message = ""
		}
		model.screen = dashboardHosts
		return model, nil
	case dashboardDeploymentPlanMsg:
		model.busy = ""
		model.deployPlan = message.report
		if message.report.HasErrors() {
			model.message = deploymentPlanIssues(message.report)
			model.screen = dashboardDeploy
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardDeployReview
		return model, nil
	case dashboardDeploymentResultMsg:
		model.busy = ""
		model.deploying = false
		model.deployResult = message.report
		model.message = message.report.Message
		model.screen = dashboardDeploy
		return model, nil
	}

	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	if (key.String() == "ctrl+c" || key.String() == "q") && model.deploying {
		model.message = "Deployment is running; wait for its result before closing Nixorium."
		return model, nil
	}
	if key.String() == "ctrl+c" || key.String() == "q" {
		return model, tea.Quit
	}
	if model.busy != "" {
		return model, nil
	}

	switch model.screen {
	case dashboardHome:
		switch key.String() {
		case "d":
			model.screen = dashboardDeploy
			model.message = ""
			model.deployChosen = map[string]bool{}
			model.deployCursor = 0
		case "h":
			model.screen = dashboardHosts
			model.busy = "Checking configured computers"
			model.message = ""
			return model, model.loadHosts()
		case "p", "enter":
			model.screen = dashboardPXE
			model.message = ""
		}
	case dashboardHosts:
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "r":
			model.busy = "Refreshing computer status"
			model.message = ""
			return model, model.loadHosts()
		}
	case dashboardDeploy:
		hosts := model.report.Meta.Clients.Hosts
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "up", "k":
			if model.deployCursor > 0 {
				model.deployCursor--
			}
		case "down", "j":
			if model.deployCursor+1 < len(hosts) {
				model.deployCursor++
			}
		case " ":
			if len(hosts) > 0 {
				if model.deployChosen == nil {
					model.deployChosen = map[string]bool{}
				}
				name := hosts[model.deployCursor].Name
				model.deployChosen[name] = !model.deployChosen[name]
			}
		case "a":
			model.deployChosen = toggleAllDeploymentTargets(hosts, model.deployChosen)
		case "enter":
			requested := selectedDeploymentTargets(hosts, model.deployChosen)
			if requested == "" {
				model.message = "Select at least one computer before reviewing a deployment."
				return model, nil
			}
			model.busy = "Validating revision and selected computers"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardDeploymentPlanMsg{report: model.actions.PlanDeployment(requested)}
			}
		}
	case dashboardDeployReview:
		switch key.String() {
		case "esc":
			model.screen = dashboardDeploy
			model.confirmation = ""
			model.message = "Deployment cancelled; no build or apply was started."
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case " ":
			model.confirmation += " "
		case "enter":
			expected := "DEPLOY " + model.deployPlan.ColmenaSelector
			if model.confirmation != expected {
				model.confirmation = ""
				model.message = "Confirmation did not match; no build or apply was started."
				return model, nil
			}
			model.busy = "Building and applying the reviewed deployment"
			model.deploying = true
			model.confirmation = ""
			model.message = ""
			plan := model.deployPlan
			return model, func() tea.Msg {
				return dashboardDeploymentResultMsg{report: model.actions.ApplyDeployment(plan)}
			}
		default:
			if key.Type == tea.KeyRunes {
				model.confirmation += string(key.Runes)
			}
		}
	case dashboardPXE:
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		case "p":
			model.busy = "Preparing netboot artifacts and client closures"
			model.message = ""
			return model, model.runAction(func() string {
				report := model.actions.PreparePXE()
				return report.Message
			}, dashboardPXE)
		case "s":
			model.busy = "Checking PXE readiness"
			model.message = ""
			return model, func() tea.Msg {
				return dashboardPlanMsg{report: model.actions.PlanPXEStart()}
			}
		case "x":
			model.busy = "Stopping installation mode and restoring networking"
			model.message = ""
			return model, model.runAction(func() string {
				return model.actions.StopPXE().Message
			}, dashboardPXE)
		case "r":
			model.busy = "Recovering normal controller networking"
			model.message = ""
			return model, model.runAction(func() string {
				return model.actions.RecoverPXE().Message
			}, dashboardPXE)
		}
	case dashboardPXEStartReview:
		switch key.String() {
		case "esc":
			model.screen = dashboardPXE
			model.confirmation = ""
			model.message = "PXE start cancelled; networking was not changed."
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case " ":
			model.confirmation += " "
		case "enter":
			if model.confirmation != "START PXE" {
				model.confirmation = ""
				model.message = "Confirmation did not match; networking was not changed."
				return model, nil
			}
			model.busy = "Starting managed PXE installation mode"
			model.confirmation = ""
			model.message = ""
			return model, model.runAction(func() string {
				return model.actions.StartPXE().Message
			}, dashboardPXE)
		default:
			if key.Type == tea.KeyRunes {
				model.confirmation += string(key.Runes)
			}
		}
	}
	return model, nil
}

func (model dashboardModel) runAction(operation func() string, screen dashboardScreen) tea.Cmd {
	return func() tea.Msg {
		message := operation()
		report, err := model.actions.Refresh()
		return dashboardOperationMsg{message: message, report: report, err: err, screen: screen}
	}
}

func (model dashboardModel) loadHosts() tea.Cmd {
	return func() tea.Msg {
		report, err := model.actions.LoadHosts()
		return dashboardHostsMsg{report: report, err: err}
	}
}

func (model dashboardModel) View() string {
	switch model.screen {
	case dashboardHosts:
		return model.hostsView()
	case dashboardDeploy, dashboardDeployReview:
		return model.deployView()
	case dashboardPXE, dashboardPXEStartReview:
		return model.pxeView()
	default:
		return model.homeView()
	}
}

func (model dashboardModel) homeView() string {
	status := "ready"
	if !model.report.Deployment.Ready {
		status = "action required"
	}
	cache := serviceLabel(model.report.Services, "nixorium-harmonia.service")
	lines := []string{
		"Nixorium",
		"",
		"Laboratory",
		fmt.Sprintf("  Configuration        %s", status),
		fmt.Sprintf("  Controller cache     %s", cache),
		fmt.Sprintf("  Installation mode    %s", model.report.PXE.Mode),
		fmt.Sprintf("  Computers            %d configured", model.report.Meta.Clients.Count),
		fmt.Sprintf("  Git worktree         %s", cleanText(model.report.Git.Dirty, model.report.Git.Changes)),
		"",
		"Actions",
		"  h           View computers",
		"  d           Deploy updates",
		"  p / Enter   Install computers over network",
		"",
		"Run `nixorium doctor` for actionable diagnostics.",
		"q: quit",
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) deployView() string {
	lines := []string{"Nixorium — Deploy updates", ""}
	if model.busy != "" {
		lines = append(lines, model.busy+"…")
		if model.deploying {
			lines = append(lines, "", "Nixorium will show the durable log and result when Colmena exits.", "Closing is disabled while this deployment is running.")
		}
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardDeployReview {
		lines = append(lines,
			"Deployment review",
			fmt.Sprintf("  Revision: %s", model.deployPlan.Revision),
			fmt.Sprintf("  Targets:  %s (%d computer(s))", model.deployPlan.ColmenaSelector, len(model.deployPlan.Targets)),
			"  Build every selected configuration before applying it",
			"  Target services may restart; offline computers will fail explicitly",
			"  A failed apply may leave mixed target state; a fresh full retry is safe",
			"",
			fmt.Sprintf("Type DEPLOY %s to continue:", model.deployPlan.ColmenaSelector),
			"> "+model.confirmation+"█",
			"",
			"Esc: cancel",
		)
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}

	hosts := model.report.Meta.Clients.Hosts
	lines = append(lines, "Select computers (Space toggles; a selects all):", "")
	for index, host := range hosts {
		cursor := " "
		if index == model.deployCursor {
			cursor = ">"
		}
		checked := " "
		if model.deployChosen[host.Name] {
			checked = "x"
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %-10s %s", cursor, checked, host.Name, host.IP))
	}
	if len(hosts) == 0 {
		lines = append(lines, "No configured client computers.")
	}
	lines = append(lines, "", "Enter: review selected targets   Esc: back   q: quit")
	if model.deployResult.Operation != "" {
		lines = append(lines,
			"",
			fmt.Sprintf("Last result: %s at phase %s", model.deployResult.State, model.deployResult.Phase),
			fmt.Sprintf("Build complete: %t   Apply complete: %t", model.deployResult.BuildCompleted, model.deployResult.ApplyCompleted),
		)
		if model.deployResult.LogPath != "" {
			lines = append(lines, "Detailed log: "+model.deployResult.LogPath)
		}
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) hostsView() string {
	available, total := hostAvailability(model.hosts.Hosts)
	lines := []string{
		"Nixorium — Computers",
		"",
		fmt.Sprintf("SSH available: %d/%d", available, total),
		fmt.Sprintf("Deployment: %d current, %d outdated, %d unknown", model.hosts.Deployment.Current, model.hosts.Deployment.Outdated, model.hosts.Deployment.Unknown),
		"",
		fmt.Sprintf("  %-10s %-15s %-12s %-11s %-9s", "NAME", "ADDRESS", "NETWORK", "SSH", "DEPLOY"),
	}
	for _, host := range model.hosts.Hosts {
		lines = append(lines, fmt.Sprintf("  %-10s %-15s %-12s %-11s %-9s", host.Name, host.IP, host.Reachability, host.SSH, host.Deployment))
	}
	if model.busy != "" {
		lines = append(lines, "", model.busy+"…")
	} else {
		lines = append(lines, "", "r: refresh   Esc: back   q: quit")
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) pxeView() string {
	preparation := "missing"
	if model.report.PXEPreparation.Present {
		preparation = "stale"
	}
	if model.report.PXEPreparation.Ready {
		preparation = "ready"
	}
	lines := []string{
		"Nixorium — Install computers over network",
		"",
		fmt.Sprintf("Installation mode:  %s", model.report.PXE.Mode),
		fmt.Sprintf("Prepared artifacts: %s", preparation),
		fmt.Sprintf("Interface:          %s", model.report.Meta.Network.Interface),
		fmt.Sprintf("Service address:    %s", model.report.Meta.Controller.DHCPIP),
	}
	if model.busy != "" {
		lines = append(lines, "", model.busy+"…", "", "q: close this view; systemd-owned work continues")
		return strings.Join(lines, "\n") + "\n"
	}
	if model.screen == dashboardPXEStartReview {
		lines = append(lines,
			"",
			"Start review",
			fmt.Sprintf("  Temporarily remove %s", model.startPlan.StaticCIDR),
			fmt.Sprintf("  Serve ProxyDHCP, TFTP, HTTP, and cache via %s", model.startPlan.DHCPAddress),
			"  Institutional DHCP remains authoritative",
			"  `nixorium pxe stop` or reboot recovery restores normal addressing",
			"",
			"Type START PXE to continue:",
			"> "+model.confirmation+"█",
			"",
			"Esc: cancel",
		)
		if model.message != "" {
			lines = append(lines, "", model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines, "", "Actions")
	if model.report.PXE.Mode != "active" {
		lines = append(lines, "  p   Prepare artifacts and client closures", "  s   Start installation mode")
	}
	if model.report.PXE.Mode == "active" || model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required" {
		lines = append(lines, "  x   Stop and restore normal networking")
	}
	lines = append(lines, "  r   Recover normal networking", "  Esc Back", "  q   Quit (active services keep running)")
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func serviceLabel(services []domain.ServiceState, name string) string {
	for _, service := range services {
		if service.Name == name {
			return service.State
		}
	}
	return "unknown"
}

func hostAvailability(hosts []domain.HostStatus) (int, int) {
	available := 0
	for _, host := range hosts {
		if host.SSH == domain.SSHAvailable {
			available++
		}
	}
	return available, len(hosts)
}

func selectedDeploymentTargets(hosts []domain.HostMeta, chosen map[string]bool) string {
	selected := []string{}
	for _, host := range hosts {
		if chosen[host.Name] {
			selected = append(selected, host.Name)
		}
	}
	if len(selected) == len(hosts) && len(hosts) > 0 {
		return "@lab"
	}
	return strings.Join(selected, ",")
}

func toggleAllDeploymentTargets(hosts []domain.HostMeta, chosen map[string]bool) map[string]bool {
	allSelected := len(hosts) > 0
	for _, host := range hosts {
		if !chosen[host.Name] {
			allSelected = false
			break
		}
	}
	result := map[string]bool{}
	if !allSelected {
		for _, host := range hosts {
			result[host.Name] = true
		}
	}
	return result
}

func deploymentPlanIssues(report domain.DeploymentPlanReport) string {
	issues := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		issues = append(issues, issue.Field+": "+issue.Message)
	}
	return strings.Join(issues, "; ")
}
