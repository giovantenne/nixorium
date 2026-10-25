package presentation

import (
	"context"

	"github.com/giovantenne/nixorium/internal/domain"
)

// MenuEntry is one selectable row of a TUI menu. Path holds the labels from the
// top-level area down to the entry, exactly as the TUI shows them.
type MenuEntry struct {
	Path        []string
	Key         string
	Description string
}

// MenuSection is the menu tree of one TUI session kind.
type MenuSection struct {
	Title   string
	Entries []MenuEntry
}

// MenuMap walks the task and group tables the dashboard renders and returns
// the administrator menu and the teacher (classroom) dashboard. It never opens
// a screen and has no side effects, so the output is deterministic.
func MenuMap() []MenuSection {
	return []MenuSection{
		{Title: "Administrator menu", Entries: administratorMenuEntries()},
		{Title: "Teacher dashboard", Entries: teacherMenuEntries()},
	}
}

// menuMapActions wires every optional callback so every optional menu row is
// listed. The callbacks are never invoked.
func menuMapActions(classroom bool) DashboardActions {
	actions := DashboardActions{
		ClassroomMode: classroom,
		Assistant:     domain.AssistantConfig{Enabled: true},
		OpenClassroomView: func(context.Context) (string, error) {
			return "", nil
		},
		PlanLock:  func(context.Context, string, domain.LockAction) domain.LockPlan { return domain.LockPlan{} },
		PlanShare: func(context.Context, string, string) domain.SharePlan { return domain.SharePlan{} },
	}
	if !classroom {
		actions.LoadDeferredUpdates = func(context.Context) domain.DeferredUpdateStatus { return domain.DeferredUpdateStatus{} }
	}
	return actions
}

func menuMapModel(classroom bool) dashboardModel {
	return newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, menuMapActions(classroom), false)
}

func taskEntries(path []string, tasks []dashboardTask) []MenuEntry {
	entries := make([]MenuEntry, 0, len(tasks))
	for _, task := range tasks {
		entries = append(entries, MenuEntry{Path: append(append([]string{}, path...), task.title), Key: task.shortcut, Description: task.description})
	}
	return entries
}

func administratorMenuEntries() []MenuEntry {
	model := menuMapModel(false)
	var entries []MenuEntry
	for _, area := range model.homeAreas() {
		entries = append(entries, MenuEntry{Path: []string{area.title}, Key: area.shortcut, Description: area.description})
		switch area.id {
		case "computers":
			entries = append(entries, taskEntries([]string{area.title}, model.availableComputerTasks())...)
		case "installation":
			entries = append(entries, taskEntries([]string{area.title}, installationAreaTasks)...)
		case "admin":
			for _, task := range administrationTasks {
				entries = append(entries, taskEntries([]string{area.title}, []dashboardTask{task})...)
				if task.id == "settings" {
					entries = append(entries, settingsMenuEntries([]string{area.title, task.title})...)
				}
			}
		}
	}
	return entries
}

func settingsMenuEntries(path []string) []MenuEntry {
	var entries []MenuEntry
	for _, group := range routineSettingsGroups {
		entries = append(entries, MenuEntry{Path: append(append([]string{}, path...), group.label), Key: group.shortcut, Description: group.description})
		if group.id != "accounts" {
			continue
		}
		accounts := append(append([]string{}, path...), group.label)
		for _, choice := range routineAccountsChoices {
			entries = append(entries, MenuEntry{Path: append(append([]string{}, accounts...), choice.label), Key: choice.shortcut, Description: choice.description})
			if choice.id != "passwords" {
				continue
			}
			for _, account := range routinePasswordChoices {
				entries = append(entries, MenuEntry{Path: append(append([]string{}, accounts...), choice.label, account.label), Key: account.id[:1], Description: account.description})
			}
		}
	}
	return entries
}

func teacherMenuEntries() []MenuEntry {
	model := menuMapModel(true)
	return taskEntries([]string{"Computers"}, model.availableComputerTasks())
}
