package presentation

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/giovantenne/nixorium/internal/domain"
)

type dashboardModel struct {
	report domain.StatusReport
}

func RunDashboard(report domain.StatusReport) error {
	_, err := tea.NewProgram(dashboardModel{report: report}).Run()
	return err
}

func (model dashboardModel) Init() tea.Cmd { return nil }

func (model dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "ctrl+c", "esc":
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model dashboardModel) View() string {
	status := "ready"
	if !model.report.Deployment.Ready {
		status = "action required"
	}
	pxe := serviceLabel(model.report.Services, "nixorium-pxe.service")
	cache := serviceLabel(model.report.Services, "nixorium-harmonia.service")
	lines := []string{
		"Nixorium",
		"",
		"Laboratory",
		fmt.Sprintf("  Configuration        %s", status),
		fmt.Sprintf("  Controller cache     %s", cache),
		fmt.Sprintf("  Installation mode    %s", pxe),
		fmt.Sprintf("  Computers            %d configured", model.report.Meta.Clients.Count),
		fmt.Sprintf("  Git worktree         %s", cleanText(model.report.Git.Dirty, model.report.Git.Changes)),
		"",
		"This first management increment is read-only.",
		"Run `nixorium doctor` for actionable diagnostics.",
		"",
		"q: quit",
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
