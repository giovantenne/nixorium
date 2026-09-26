package presentation

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func InternetPlanText(w io.Writer, p domain.InternetPlan) {
	fmt.Fprintf(w, "Internet: %s (%s)\n%s\n", p.Action, p.State, p.Message)
	for _, t := range p.Targets {
		fmt.Fprintf(w, "  %s  %s  current=%s  available=%t\n", t.Name, t.IP, t.Observed.State, t.Eligible)
	}
	if p.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", p.ReviewToken)
	}
}
func InternetReportText(w io.Writer, r domain.InternetReport) {
	fmt.Fprintln(w, r.Message)
	for _, t := range r.Targets {
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.State, t.Detail)
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
	cursor, stage int
	chosen        map[string]bool
	action        domain.InternetAction
	plan          domain.InternetPlan
	result        domain.InternetReport
	applying      bool
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
		if key.String() == "r" {
			m.stage = 0
			m.plan = domain.InternetPlan{}
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
	switch key.String() {
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(max(0, len(hosts)-1), m.cursor+1)
	case "space":
		if len(hosts) > 0 {
			name := hosts[m.cursor].Name
			m.chosen[name] = !m.chosen[name]
		}
	case "a":
		all := true
		for _, h := range hosts {
			all = all && m.chosen[h.Name]
		}
		for _, h := range hosts {
			m.chosen[h.Name] = !all
		}
	case "tab":
		if m.action == domain.InternetBlock {
			m.action = domain.InternetUnblock
		} else {
			m.action = domain.InternetBlock
		}
	case "enter":
		names := []string{}
		for _, h := range hosts {
			if m.chosen[h.Name] {
				names = append(names, h.Name)
			}
		}
		if len(names) == 0 {
			model.message = "Select at least one client."
			return model, nil
		}
		if model.actions.PlanInternet == nil {
			model.message = "Internet control is unavailable in this session."
			return model, nil
		}
		requested, action := strings.Join(names, ","), m.action
		model.busy = "Checking Internet access on selected clients"
		model.message = ""
		return model, func() tea.Msg { return internetPlanMsg{model.actions.PlanInternet(requested, action)} }
	}
	return model, nil
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
			outcome := "not sent"
			if t.Eligible {
				outcome = t.Observed.State + " → " + m.action.DesiredState()
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
			lines = append(lines, t.Name+"  "+t.State, t.Detail)
		}
		shell.actions = []tuiAction{{key: "r", label: "New review"}, {key: "Enter", label: "Computers"}, {key: "F1", label: "Help"}}
	default:
		lines = append(lines, "Action: "+string(m.action)+" Internet", "")
		hosts := model.report.Meta.Clients.Hosts
		count := max(1, model.height-17)
		start, end := listWindow(len(hosts), m.cursor, count)
		for i := start; i < end; i++ {
			h := hosts[i]
			mark := "[ ]"
			if m.chosen[h.Name] {
				mark = "[x]"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%s %-10s %s", mark, h.Name, h.IP), i == m.cursor, model.isDark))
		}
		if len(hosts) == 0 {
			lines = append(lines, "No client computers configured.")
		}
		if start > 0 || end < len(hosts) {
			lines = append(lines, fmt.Sprintf("Showing %d–%d of %d", start+1, end, len(hosts)))
		}
		shell.actions = []tuiAction{{key: "↑/↓", label: "Move"}, {key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "Tab", label: "Block / unblock"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
