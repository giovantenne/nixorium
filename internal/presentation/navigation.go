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
	rows := max(1, model.height-15)
	if model.height == 0 {
		rows = len(tasks)
	}
	start, end := listWindow(len(tasks), cursor, rows)
	lines := []string{}
	for index := start; index < end; index++ {
		task := tasks[index]
		lines = append(lines, tuiSelection(menuTitle(task.shortcut, task.title), index == cursor, model.isDark))
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
