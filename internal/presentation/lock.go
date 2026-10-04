package presentation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func lockWords(locked bool) string {
	if locked {
		return "locked"
	}
	return "unlocked"
}

func LockPlanText(w io.Writer, p domain.LockPlan) {
	fmt.Fprintf(w, "Lock screens: %s (%s)\n%s\n", p.Action, p.State, p.Message)
	for _, t := range p.Targets {
		outcome := t.Detail
		if t.Eligible {
			outcome = "now " + lockWords(t.Locked) + "; will be " + lockWords(p.Action.Locked())
		}
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.IP, outcome)
	}
	if p.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", p.ReviewToken)
	}
}

func LockReportText(w io.Writer, r domain.LockReport) {
	fmt.Fprintln(w, r.Message)
	for _, t := range r.Targets {
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.State, t.Detail)
	}
}

func ConfirmLock(input io.Reader, output io.Writer, p domain.LockPlan) (bool, error) {
	LockPlanText(output, p)
	fmt.Fprint(output, "Apply this change? [y/N] ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}

type lockModel struct {
	cursor, stage int
	chosen        map[string]bool
	action        domain.LockAction
	plan          domain.LockPlan
	result        domain.LockReport
}
type lockPlanMsg struct{ plan domain.LockPlan }
type lockApplyMsg struct{ report domain.LockReport }

func (model dashboardModel) updateLock(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m := &model.lock
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
		switch key.String() {
		case "n":
			m.stage = 0
			m.plan = domain.LockPlan{}
		case "enter":
			model.screen = dashboardComputersArea
		}
		return model, nil
	}
	if m.stage == 1 {
		if key.String() == "enter" && !m.plan.HasErrors() && model.actions.ApplyLock != nil {
			plan := m.plan
			model.busy = "Changing the selected screens"
			return model, func() tea.Msg { return lockApplyMsg{model.actions.ApplyLock(plan)} }
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
		if m.action == domain.LockOn {
			m.action = domain.LockOff
		} else {
			m.action = domain.LockOn
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
		if model.actions.PlanLock == nil {
			model.message = "Locking screens is unavailable in this session."
			return model, nil
		}
		requested, action := strings.Join(names, ","), m.action
		model.busy = "Checking the selected computers"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return lockPlanMsg{model.actions.PlanLock(ctx, requested, action)}
		})
	}
	return model, nil
}

func (model dashboardModel) lockView() string {
	m := model.lock
	shell := tuiShell{path: []string{"Computers", "Lock screens"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines := []string{tuiTitle("Lock screens", model.isDark), tuiMuted("A locked screen shows \"Eyes on the teacher\" and ignores keyboard and mouse. Logging out or restarting unlocks it.", model.isDark), ""}
	switch m.stage {
	case 1:
		lines = append(lines, "Review: "+string(m.action)+" screens", m.plan.Message, "")
		for _, t := range m.plan.Targets {
			outcome := t.Detail + " · not sent"
			if t.Eligible {
				outcome = lockWords(t.Locked) + " → " + lockWords(m.action.Locked())
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
		shell.actions = []tuiAction{{key: "n", label: "New review"}, {key: "Enter", label: "Computers"}, {key: "F1", label: "Help"}}
	default:
		lines = append(lines, "Action: "+string(m.action)+" screens", "")
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
		shell.actions = []tuiAction{{key: "↑/↓", label: "Move"}, {key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "Tab", label: "Lock / unlock"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
