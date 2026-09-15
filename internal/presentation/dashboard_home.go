package presentation

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type dashboardTask struct {
	id          string
	shortcut    string
	title       string
	description string
}

func (task dashboardTask) Title() string       { return task.title + "  [" + task.shortcut + "]" }
func (task dashboardTask) Description() string { return task.description }
func (task dashboardTask) FilterValue() string { return task.title + " " + task.description }

var dashboardTasks = []dashboardTask{
	{id: "hosts", shortcut: "h", title: "View computers", description: "Reachability and deployed configuration"},
	{id: "deploy", shortcut: "d", title: "Deploy updates", description: "Build and update selected computers"},
	{id: "pxe", shortcut: "p", title: "Install computers over network", description: "Prepare, start, stop, or recover installation mode"},
	{id: "settings", shortcut: "e", title: "Change settings", description: "Network, accounts, regional values, browser, Git, and Veyon"},
	{id: "controller", shortcut: "c", title: "Rebuild controller", description: "Review and activate the committed controller revision"},
	{id: "services", shortcut: "s", title: "Manage services", description: "Inspect services or restart the signed cache"},
	{id: "git", shortcut: "g", title: "Review Git changes", description: "Inspect and commit selected safe deployment files"},
	{id: "logs", shortcut: "l", title: "View operation logs", description: "Recent outcomes and bounded deployment log tails"},
	{id: "update", shortcut: "u", title: "Update Nixorium", description: "Review and apply an explicit upstream release"},
}

type dashboardTaskMenu struct {
	list list.Model
}

func newDashboardTaskMenu(isDark bool, width, height int) dashboardTaskMenu {
	items := make([]list.Item, 0, len(dashboardTasks))
	for _, task := range dashboardTasks {
		items = append(items, task)
	}
	delegate := list.NewDefaultDelegate()
	delegate.Styles = list.NewDefaultItemStyles(isDark)
	delegate.SetSpacing(0)
	menu := list.New(items, delegate, dashboardMenuWidth(width), dashboardMenuHeight(height))
	menu.Title = "Laboratory tasks"
	menu.SetShowTitle(false)
	menu.SetShowStatusBar(false)
	menu.SetShowHelp(false)
	menu.SetFilteringEnabled(false)
	menu.Styles = list.DefaultStyles(isDark)
	menu.Help.Styles = help.DefaultStyles(isDark)
	return dashboardTaskMenu{list: menu}
}

func dashboardMenuWidth(width int) int {
	if width < 48 {
		return 48
	}
	if width > 88 {
		return 88
	}
	return width
}

func dashboardMenuHeight(height int) int {
	height -= 13
	if height < 7 {
		return 7
	}
	if height > 18 {
		return 18
	}
	return height
}

func (menu *dashboardTaskMenu) setSize(width, height int) {
	menu.list.SetSize(dashboardMenuWidth(width), dashboardMenuHeight(height))
}

func (menu dashboardTaskMenu) selected() (dashboardTask, bool) {
	task, ok := menu.list.SelectedItem().(dashboardTask)
	return task, ok
}

func (menu dashboardTaskMenu) update(message tea.Msg) (dashboardTaskMenu, tea.Cmd) {
	updated, command := menu.list.Update(message)
	menu.list = updated
	return menu, command
}

func (model dashboardModel) homeView() string {
	menu := model.homeMenu
	if len(menu.list.Items()) == 0 {
		menu = newDashboardTaskMenu(model.isDark, model.width, model.height)
	}
	configuration := "action required"
	configurationKind := tuiStatusAttention
	if model.report.Deployment.Ready {
		configuration = "ready"
		configurationKind = tuiStatusSuccess
	}
	cache := serviceLabel(model.report.Services, "nixorium-harmonia.service")
	cacheKind := tuiStatusAttention
	if cache == "active" || cache == "healthy" {
		cacheKind = tuiStatusSuccess
	}
	pxeKind := tuiStatusNeutral
	if model.report.PXE.Mode == "active" {
		pxeKind = tuiStatusAttention
	} else if model.report.PXE.Mode == "degraded" || model.report.PXE.Mode == "recovery-required" {
		pxeKind = tuiStatusFailure
	}
	git := cleanText(model.report.Git.Dirty, model.report.Git.Changes)
	gitKind := tuiStatusSuccess
	if model.report.Git.Dirty {
		gitKind = tuiStatusAttention
	}
	lines := []string{
		tuiTitle("Nixorium", model.isDark),
		tuiMuted("Laboratory control center", model.isDark),
		"",
		tuiSection("Status", model.isDark),
		fmt.Sprintf("  %-20s %s", "Configuration", tuiStatus(configuration, configurationKind, model.isDark)),
		fmt.Sprintf("  %-20s %s", "Controller cache", tuiStatus(cache, cacheKind, model.isDark)),
		fmt.Sprintf("  %-20s %s", "Installation mode", tuiStatus(model.report.PXE.Mode, pxeKind, model.isDark)),
		fmt.Sprintf("  %-20s %d configured", "Computers", model.report.Meta.Clients.Count),
		fmt.Sprintf("  %-20s %s", "Git worktree", tuiStatus(git, gitKind, model.isDark)),
	}
	if model.setup.State != "" && model.setup.State != "ready" {
		lines = append(lines,
			"",
			tuiResult("Next: finish first setup", false, model.isDark),
			"  "+setupCurrentTitle(model.setup),
			"  Press Enter to continue",
		)
	} else {
		lines = append(lines, "", tuiSection("Tasks", model.isDark), menu.list.View())
	}
	if model.message != "" {
		lines = append(lines, "", tuiResult(model.message, true, model.isDark))
	}
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"up", "down"}, "↑/↓", "select"),
		tuiHelpBinding([]string{"enter"}, "enter", "open"),
		tuiHelpBinding([]string{"q", "ctrl+c"}, "q", "quit"),
	))
	return strings.Join(lines, "\n") + "\n"
}
