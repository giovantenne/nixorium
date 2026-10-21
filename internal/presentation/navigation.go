package presentation

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func menuTitle(shortcut, title string) string {
	return fmt.Sprintf("[%s] %s", shortcut, title)
}

// All task menus use one row per action, with a stable shortcut column.
// Details follow the focused row without pushing other actions off screen.
func (model dashboardModel) taskMenu(tasks []dashboardTask, cursor int) string {
	if len(tasks) == 0 {
		return ""
	}
	cursor = min(max(0, cursor), len(tasks)-1)
	rows := len(tasks)
	if model.navigation.menuRows > 0 {
		rows = min(rows, model.navigation.menuRows)
	}
	start, end := listWindow(len(tasks), cursor, rows)
	lines := []string{}
	rowWidth := 0
	for _, task := range tasks {
		rowWidth = max(rowWidth, lipgloss.Width(menuTitle(task.shortcut, task.title)))
	}
	if model.width > 0 {
		rowWidth = min(rowWidth, max(1, model.width-10))
	}
	for index := start; index < end; index++ {
		task := tasks[index]
		if task.advanced && (index == 0 || !tasks[index-1].advanced) {
			lines = append(lines, "", tuiMuted("  Advanced", model.isDark))
		}
		pending := strings.HasPrefix(task.id, "pending-")
		if index > start && (pending || strings.HasPrefix(tasks[index-1].id, "pending-")) {
			lines = append(lines, "")
		}
		if pending {
			marker := "  "
			if index == cursor {
				marker = "› "
			}
			lines = append(lines, tuiNoticeText(marker+menuTitle(task.shortcut, task.title), tuiStatusAttention, model.isDark))
		} else if index == cursor {
			lines = append(lines, tuiSelection(lipgloss.NewStyle().Width(rowWidth).Render(menuTitle(task.shortcut, task.title)), true, model.isDark))
		} else {
			lines = append(lines, "  "+tuiShortcut("["+task.shortcut+"]", model.isDark)+" "+task.title)
		}
	}
	if start > 0 || end < len(tasks) {
		lines = append(lines, tuiMuted(fmt.Sprintf("  %d–%d of %d", start+1, end, len(tasks)), model.isDark))
	}
	width := min(110, max(20, model.width-10))
	if model.width == 0 {
		width = 96
	}
	detail := lipgloss.NewStyle().Width(width).Render(tasks[cursor].description)
	lines = append(lines, "", tuiMuted(detail, model.isDark))
	return strings.Join(lines, "\n")
}

func taskHelp(tasks []dashboardTask) []string {
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		lines = append(lines, task.shortcut+"  "+task.title)
	}
	return lines
}

// fitMenu renders a screen whose task menu shows as many rows as the shell
// really has room for: it tries the whole menu first and shrinks the window
// until renderShell no longer needs its scroll hint. Everything around the
// menu (breadcrumb, notices, the wrapped description, footer) is therefore
// counted by the same code that lays it out.
func (model dashboardModel) fitMenu(count int, render func(dashboardModel) string) string {
	if model.height == 0 || count == 0 {
		model.navigation.menuRows = count
		return render(model)
	}
	out := ""
	for rows := count; rows >= 1; rows-- {
		model.navigation.menuRows = rows
		out = render(model)
		if !strings.Contains(out, "scroll · ? help") {
			break
		}
	}
	return out
}
