package presentation

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/giovantenne/nixorium/internal/domain"
)

type DashboardActions struct {
	Refresh      func() (domain.StatusReport, error)
	LoadHosts    func() (domain.HostsReport, error)
	PreparePXE   func() domain.ActionReport
	PlanPXEStart func() domain.PXELifecycleReport
	StartPXE     func() domain.PXELifecycleReport
	StopPXE      func() domain.PXELifecycleReport
	RecoverPXE   func() domain.PXELifecycleReport
}

type dashboardScreen int

const (
	dashboardHome dashboardScreen = iota
	dashboardHosts
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
	}

	key, ok := message.(tea.KeyMsg)
	if !ok {
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
		"  p / Enter   Install computers over network",
		"",
		"Run `nixorium doctor` for actionable diagnostics.",
		"q: quit",
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) hostsView() string {
	available, total := hostAvailability(model.hosts.Hosts)
	lines := []string{
		"Nixorium — Computers",
		"",
		fmt.Sprintf("SSH available: %d/%d", available, total),
		"",
		fmt.Sprintf("  %-10s %-15s %-12s %-11s", "NAME", "ADDRESS", "NETWORK", "SSH"),
	}
	for _, host := range model.hosts.Hosts {
		lines = append(lines, fmt.Sprintf("  %-10s %-15s %-12s %-11s", host.Name, host.IP, host.Reachability, host.SSH))
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
