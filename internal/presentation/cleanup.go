package presentation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Free disk space: remove old system versions after review (ADR 0022).

func CleanupPlanText(w io.Writer, p domain.CleanupPlanReport) {
	fmt.Fprintf(w, "Free disk space: %s\n%s\n", p.State, p.Message)
	fmt.Fprintf(w, "Each computer keeps its newest %d system versions plus the one running and the one it started with.\n", p.KeepGenerations)
	for _, t := range p.Targets {
		fmt.Fprintf(w, "  %s  %s\n", cleanupTargetName(t), cleanupTargetSummary(t))
	}
	for _, issue := range p.Issues {
		fmt.Fprintf(w, "  %s: %s\n", issue.Field, issue.Message)
	}
	if p.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", p.ReviewToken)
	}
}

func CleanupReportText(w io.Writer, r domain.CleanupApplyReport) {
	fmt.Fprintln(w, r.Message)
	for _, t := range r.Targets {
		fmt.Fprintf(w, "  %s  %s\n", t.Name, cleanupOutcomeSummary(t))
		if t.TechnicalDetail != "" {
			fmt.Fprintf(w, "    %s\n", t.TechnicalDetail)
		}
	}
}

func ConfirmCleanup(input io.Reader, output io.Writer, p domain.CleanupPlanReport) (bool, error) {
	CleanupPlanText(output, p)
	fmt.Fprintf(output, "Rollback will be possible only to the kept versions. Type %s to continue: ", p.Confirmation)
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	return strings.TrimSpace(line) == p.Confirmation, nil
}

func cleanupTargetName(t domain.CleanupTargetPlan) string {
	if t.Controller {
		return t.Name + " (controller)"
	}
	return t.Name
}

func cleanupTargetSummary(t domain.CleanupTargetPlan) string {
	if !t.Eligible {
		return t.Detail
	}
	return fmt.Sprintf("remove %d old version(s) %s; keep %s; %s free now", len(t.Remove), cleanupNumbers(t.Remove), cleanupKept(t.Keep), humanBytes(t.FreeBytes))
}

func cleanupOutcomeSummary(t domain.CleanupTargetOutcome) string {
	switch t.State {
	case "cleaned":
		summary := fmt.Sprintf("done: %s", t.Detail)
		if t.FreeAfter > 0 {
			summary += fmt.Sprintf(" · free space %s → %s", humanBytes(t.FreeBefore), humanBytes(t.FreeAfter))
		}
		return summary
	case "unchanged":
		return "nothing to remove"
	case "not-sent":
		return "not sent: " + t.Detail
	}
	return "not confirmed: " + t.Detail
}

func cleanupNumbers(numbers []int) string {
	sorted := slices.Clone(numbers)
	slices.Sort(sorted)
	words := make([]string, 0, len(sorted))
	for _, number := range sorted {
		words = append(words, fmt.Sprint(number))
	}
	if len(words) > 8 {
		words = append(words[:3], "…", words[len(words)-2], words[len(words)-1])
	}
	return "(" + strings.Join(words, " ") + ")"
}

func cleanupKept(keep []domain.CleanupGeneration) string {
	if len(keep) == 0 {
		return "none"
	}
	numbers := make([]int, 0, len(keep))
	for _, generation := range keep {
		numbers = append(numbers, generation.Number)
	}
	slices.Sort(numbers)
	return fmt.Sprintf("%d (%d–%d)", len(numbers), numbers[0], numbers[len(numbers)-1])
}

type cleanupModel struct {
	cursor, stage int
	chosen        map[string]bool
	plan          domain.CleanupPlanReport
	result        domain.CleanupApplyReport
	confirmation  string
}

type cleanupPlanMsg struct{ plan domain.CleanupPlanReport }
type cleanupApplyMsg struct{ report domain.CleanupApplyReport }

// cleanupRows lists the controller first, then every client.
func (model dashboardModel) cleanupRows() []domain.HostMeta {
	rows := []domain.HostMeta{{Name: model.report.Meta.Controller.Name}}
	return append(rows, model.report.Meta.Clients.Hosts...)
}

func (model dashboardModel) openCleanup() (tea.Model, tea.Cmd) {
	if model.report.Meta.Controller.Name == "" {
		return model.loadInventoryThen(func(ready dashboardModel) (tea.Model, tea.Cmd) {
			return ready.openCleanup()
		})
	}
	model.screen = dashboardCleanup
	model.cleanup = cleanupModel{chosen: map[string]bool{model.report.Meta.Controller.Name: true}}
	model.message = ""
	return model, nil
}

func (model dashboardModel) updateCleanup(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m := &model.cleanup
	rows := model.cleanupRows()
	switch m.stage {
	case 1:
		switch key.String() {
		case "esc":
			m.stage, m.confirmation = 0, ""
			model.message = "Nothing was removed."
		case "backspace":
			value := []rune(m.confirmation)
			if len(value) > 0 {
				m.confirmation = string(value[:len(value)-1])
			}
		case "enter":
			if m.plan.State != "ready" || m.plan.HasErrors() || model.actions.ApplyCleanup == nil {
				return model, nil
			}
			if m.confirmation != m.plan.Confirmation {
				m.confirmation = ""
				model.message = "Confirmation did not match; nothing was removed."
				return model, nil
			}
			plan := m.plan
			m.confirmation = ""
			model.busy = "Removing old system versions; this can take several minutes"
			model.message = ""
			return model, func() tea.Msg { return cleanupApplyMsg{model.actions.ApplyCleanup(plan)} }
		default:
			if key.Text != "" && key.Text != " " {
				m.confirmation += key.Text
			}
		}
		return model, nil
	case 2:
		switch key.String() {
		case "n":
			m.stage, m.plan, m.result = 0, domain.CleanupPlanReport{}, domain.CleanupApplyReport{}
		case "enter", "esc":
			model.screen = dashboardAdministration
		}
		model.message = ""
		return model, nil
	}
	switch key.String() {
	case "esc", "left":
		model.screen = dashboardAdministration
		model.message = ""
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(len(rows)-1, m.cursor+1)
	case "space":
		name := rows[m.cursor].Name
		m.chosen[name] = !m.chosen[name]
	case "a":
		all := true
		for _, row := range rows {
			all = all && m.chosen[row.Name]
		}
		for _, row := range rows {
			m.chosen[row.Name] = !all
		}
	case "enter":
		names := []string{}
		for index, row := range rows {
			if !m.chosen[row.Name] {
				continue
			}
			if index == 0 {
				names = append(names, domain.CleanupControllerTarget)
			} else {
				names = append(names, row.Name)
			}
		}
		if len(names) == 0 {
			model.message = "Select at least one computer."
			return model, nil
		}
		if model.actions.PlanCleanup == nil {
			model.message = "Freeing disk space is not available in this session."
			return model, nil
		}
		requested := strings.Join(names, ",")
		model.busy = "Checking old system versions on the selected computers"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return cleanupPlanMsg{model.actions.PlanCleanup(ctx, requested)}
		})
	}
	return model, nil
}

func (model dashboardModel) cleanupView() string {
	m := model.cleanup
	shell := tuiShell{path: []string{"Maintenance", "Free disk space"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	lines := []string{
		tuiTitle("Free disk space", model.isDark),
		tuiMuted(fmt.Sprintf("Removes old system versions. Each computer keeps its newest %d, the one running and the one it started with.", domain.CleanupKeepGenerations), model.isDark),
		"",
	}
	switch m.stage {
	case 1:
		lines = append(lines, m.plan.Message, "")
		for _, t := range m.plan.Targets {
			lines = append(lines, fmt.Sprintf("%-18s %s", cleanupTargetName(t), cleanupTargetSummary(t)))
		}
		for _, issue := range m.plan.Issues {
			lines = append(lines, "", issue.Message)
		}
		shell.actions = []tuiAction{{key: "Esc", label: "Back"}, {key: "F1", label: "Help"}}
		if m.plan.State == "ready" && !m.plan.HasErrors() {
			lines = append(lines, "",
				"Afterwards you can go back only to the kept versions. Computers that are off are not cleaned.",
				tuiSection("Type "+m.plan.Confirmation+" to continue:", model.isDark), "> "+m.confirmation+"_")
			shell.actions = append([]tuiAction{{key: "Enter", label: "Free space"}}, shell.actions...)
		}
	case 2:
		lines = append(lines, tuiResult(m.result.Message, !m.result.HasErrors(), model.isDark), "")
		for _, t := range m.result.Targets {
			lines = append(lines, t.Name+"  "+cleanupOutcomeSummary(t))
		}
		shell.actions = []tuiAction{{key: "Enter", label: "Maintenance"}, {key: "n", label: "New review"}, {key: "F1", label: "Help"}}
	default:
		rows := model.cleanupRows()
		count := max(1, model.height-17)
		start, end := listWindow(len(rows), m.cursor, count)
		for i := start; i < end; i++ {
			mark := "[ ]"
			if m.chosen[rows[i].Name] {
				mark = "[x]"
			}
			label := rows[i].Name
			if i == 0 {
				label += " (controller)"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%s %-18s %s", mark, label, rows[i].IP), i == m.cursor, model.isDark))
		}
		if start > 0 || end < len(rows) {
			lines = append(lines, fmt.Sprintf("Showing %d–%d of %d", start+1, end, len(rows)))
		}
		shell.actions = []tuiAction{{key: "↑/↓", label: "Move"}, {key: "Space", label: "Select"}, {key: "a", label: "All"}, {key: "Enter", label: "Review"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
	}
	return model.renderShell(shell)
}
