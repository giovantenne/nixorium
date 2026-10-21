package presentation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func InternetPlanText(w io.Writer, p domain.InternetPlan) {
	fmt.Fprintf(w, "Internet: %s (%s)\n%s\n", p.Action, p.State, p.Message)
	for _, t := range p.Targets {
		outcome := "not reachable; not sent"
		if t.Eligible {
			outcome = "now " + internetStateWords(t.Observed.State) + "; will be " + internetStateWords(p.Action.DesiredState())
		}
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.IP, outcome)
	}
	if p.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", p.ReviewToken)
	}
}
func InternetReportText(w io.Writer, r domain.InternetReport) {
	fmt.Fprintln(w, r.Message)
	for _, t := range r.Targets {
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, internetStateWords(t.State), t.Detail)
	}
}
func ConfirmInternet(input io.Reader, output io.Writer, p domain.InternetPlan) (bool, error) {
	InternetPlanText(output, p)
	fmt.Fprint(output, "Apply this change? [y/N] ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}

type internetModel struct {
	hostPicker
	stage    int
	action   domain.InternetAction
	plan     domain.InternetPlan
	result   domain.InternetReport
	applying bool
	// observed is the last read-only Internet check of this session.
	observed   map[string]string
	observedAt time.Time
}
type internetPlanMsg struct{ plan domain.InternetPlan }
type internetApplyMsg struct{ report domain.InternetReport }

func (model dashboardModel) updateInternet(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m := &model.internet
	hosts := model.report.Meta.Clients.Hosts
	if key.String() == "esc" {
		if m.stage == 1 {
			m.stage = 0
		} else {
			model.screen = dashboardComputersArea
		}
		model.message = ""
		return model, nil
	}
	if m.stage == 2 {
		if key.String() == "h" && internetUnconfirmed(m.result) {
			return model.openComputerTask("h")
		}
		if key.String() == "n" {
			// A new review starts with nothing selected, so "a All"
			// selects everyone instead of clearing the last selection.
			m.stage = 0
			m.plan = domain.InternetPlan{}
			m.clear()
		}
		if key.String() == "enter" {
			model.screen = dashboardComputersArea
		}
		return model, nil
	}
	if m.stage == 1 {
		if key.String() == "enter" && !m.plan.HasErrors() && model.actions.ApplyInternet != nil {
			plan := m.plan
			m.applying = true
			model.busy = "Applying and verifying Internet access"
			return model, func() tea.Msg { return internetApplyMsg{model.actions.ApplyInternet(plan)} }
		}
		return model, nil
	}
	if m.pick(key.String(), hosts) {
		return model, nil
	}
	switch key.String() {
	case "r":
		return model.checkInternetState()
	case "n":
		if m.observedAt.IsZero() {
			model.message = "No observation yet: press r to check Internet access first."
			return model, nil
		}
		// Select the computers the current action would change.
		wanted := "enabled"
		if m.action == domain.InternetUnblock {
			wanted = "blocked"
		}
		chosen := map[string]bool{}
		for _, h := range hosts {
			if m.observed[h.Name] == wanted {
				chosen[h.Name] = true
			}
		}
		if len(chosen) == 0 {
			model.message = "No client would change: none had " + internetStateWords(wanted) + " at the last check."
			return model, nil
		}
		m.chosen = chosen
		model.message = ""
	case "tab":
		if m.action == domain.InternetBlock {
			m.action = domain.InternetUnblock
		} else {
			m.action = domain.InternetBlock
		}
	case "enter":
		requested := m.requested(hosts)
		if requested == "" {
			model.message = "Select at least one client."
			return model, nil
		}
		if model.actions.PlanInternet == nil {
			model.message = "Internet control is unavailable in this session."
			return model, nil
		}
		action := m.action
		model.busy = "Checking Internet access on selected clients"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return internetPlanMsg{model.actions.PlanInternet(ctx, requested, action)}
		})
	}
	return model, nil
}
func internetUnconfirmed(report domain.InternetReport) bool {
	for _, target := range report.Targets {
		if target.State == "unconfirmed" {
			return true
		}
	}
	return false
}

func (model dashboardModel) internetView() string {
	m := model.internet
	shell := tuiShell{path: []string{"Computers", "Internet access"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines := []string{tuiTitle("Internet access", model.isDark), tuiMuted("Internet returns after reboot. Lab access remains available.", model.isDark), ""}
	switch m.stage {
	case 1:
		lines = append(lines, "Review: "+string(m.action)+" Internet", m.plan.Message, "")
		for _, t := range m.plan.Targets {
			outcome := "not reachable · not sent"
			if t.Eligible {
				outcome = internetStateWords(t.Observed.State) + " → " + internetStateWords(m.action.DesiredState())
			}
			lines = append(lines, fmt.Sprintf("%s  %s  %s", t.Name, t.IP, outcome))
		}
		shell.actions = []tuiAction{{key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		if !m.plan.HasErrors() {
			shell.actions = append([]tuiAction{{key: "Enter", label: "Apply"}}, shell.actions...)
		}
	case 2:
		lines = append(lines, m.result.Message, "")
		for _, t := range m.result.Targets {
			lines = append(lines, t.Name+"  "+internetStateWords(t.State), t.Detail)
		}
		shell.actions = []tuiAction{{key: "n", label: "New review"}}
		if internetUnconfirmed(m.result) {
			shell.actions = append(shell.actions, tuiAction{key: "h", label: "Check computers"})
		}
		shell.actions = append(shell.actions, tuiAction{key: "Enter", label: "Computers"}, tuiAction{key: "F1", label: "Help"})
	default:
		lines = append(lines, "Action: "+string(m.action)+" Internet", tuiMuted(observationHeading(m.observedAt), model.isDark), "")
		hosts := model.report.Meta.Clients.Hosts
		lines = append(lines, m.rows(hosts, max(1, model.height-ctaRows(model.height)-17), model.isDark, func(h domain.HostMeta) string {
			return fmt.Sprintf("%-10s %-15s %s", h.Name, h.IP, m.stateLabel(h.Name))
		})...)
		shell.actions = []tuiAction{{key: "↑/↓", label: "Move"}, {key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "n", label: "Those to change"}, {key: "r", label: "Check state"}, {key: "Tab", label: "Block / unblock"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
