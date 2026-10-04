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

func SharePlanText(w io.Writer, p domain.SharePlan) {
	fmt.Fprintf(w, "Send desktop (%s)\n%s\n", p.State, p.Message)
	for _, t := range p.Targets {
		outcome := "will receive the files"
		if !t.Eligible {
			outcome = t.Detail + " Not sent."
		}
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.IP, outcome)
	}
	if p.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", p.ReviewToken)
	}
}

func ShareReportText(w io.Writer, r domain.ShareReport) {
	fmt.Fprintln(w, r.Message)
	for _, t := range r.Targets {
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.State, t.Detail)
	}
}

func ConfirmShare(input io.Reader, output io.Writer, p domain.SharePlan) (bool, error) {
	SharePlanText(output, p)
	fmt.Fprint(output, "Send these files? [y/N] ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}

type shareModel struct {
	cursor, stage int
	chosen        map[string]bool
	plan          domain.SharePlan
	result        domain.ShareReport
}
type sharePlanMsg struct{ plan domain.SharePlan }
type shareApplyMsg struct{ report domain.ShareReport }

func (model dashboardModel) updateShare(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m := &model.share
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
			m.plan = domain.SharePlan{}
		case "enter":
			model.screen = dashboardComputersArea
		}
		return model, nil
	}
	if m.stage == 1 {
		if key.String() == "enter" && !m.plan.HasErrors() && model.actions.ApplyShare != nil {
			plan := m.plan
			model.busy = "Sending the files to the selected computers"
			return model, func() tea.Msg { return shareApplyMsg{model.actions.ApplyShare(plan)} }
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
		if model.actions.PlanShare == nil {
			model.message = "Sending files is unavailable in this session."
			return model, nil
		}
		requested := strings.Join(names, ",")
		model.busy = "Preparing your desktop and checking the computers"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return sharePlanMsg{model.actions.PlanShare(ctx, requested)}
		})
	}
	return model, nil
}

func (model dashboardModel) shareView() string {
	m := model.share
	shell := tuiShell{path: []string{"Computers", "Send desktop"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines := []string{tuiTitle("Send desktop", model.isDark), tuiMuted("Copies the files and folders on your desktop to the students' desktops. A restart empties the student's home.", model.isDark), ""}
	switch m.stage {
	case 1:
		lines = append(lines, "Review", m.plan.Message, "")
		for _, t := range m.plan.Targets {
			outcome := "will receive the files"
			if !t.Eligible {
				outcome = t.Detail + " · not sent"
			}
			lines = append(lines, fmt.Sprintf("%s  %s  %s", t.Name, t.IP, outcome))
		}
		shell.actions = []tuiAction{{key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
		if !m.plan.HasErrors() {
			shell.actions = append([]tuiAction{{key: "Enter", label: "Send"}}, shell.actions...)
		}
	case 2:
		lines = append(lines, m.result.Message, "")
		for _, t := range m.result.Targets {
			lines = append(lines, t.Name+"  "+t.State, t.Detail)
		}
		shell.actions = []tuiAction{{key: "n", label: "New review"}, {key: "Enter", label: "Computers"}, {key: "F1", label: "Help"}}
	default:
		hosts := model.report.Meta.Clients.Hosts
		count := max(1, model.height-16)
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
		shell.actions = []tuiAction{{key: "↑/↓", label: "Move"}, {key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
