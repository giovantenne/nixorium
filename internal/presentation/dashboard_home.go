package presentation

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	{id: "restore", shortcut: "r", title: "Restore computers", description: "Reapply the intended system or reinstall from scratch"},
	{id: "software", shortcut: "w", title: "Add or change software", description: "Open the supported workflow and current advanced path"},
	{id: "deploy", shortcut: "d", title: "Distribute the prepared system", description: "Update only the computers selected for this intervention"},
	{id: "pxe", shortcut: "p", title: "Install or reinstall computers", description: "Prepare and control network installation"},
	{id: "update", shortcut: "u", title: "Update Nixorium", description: "Choose from releases fetched from the configured upstream"},
	{id: "admin", shortcut: "a", title: "Advanced tools", description: "Inventory, settings, revisions, services, logs and diagnostics"},
}

var administrationTasks = []dashboardTask{
	{id: "hosts", shortcut: "h", title: "Computer inventory", description: "Explicitly check reachability and deployed configuration"},
	{id: "settings", shortcut: "e", title: "Change settings", description: "Network, accounts, regional values, browser, Git, and Veyon"},
	{id: "controller", shortcut: "c", title: "Rebuild controller", description: "Review and activate the committed controller revision"},
	{id: "services", shortcut: "s", title: "Manage services", description: "Inspect services or restart the signed cache"},
	{id: "git", shortcut: "g", title: "Review Git changes", description: "Inspect and commit selected safe deployment files"},
	{id: "logs", shortcut: "l", title: "View operation logs", description: "Recent outcomes and bounded deployment log tails"},
	{id: "diagnostics", shortcut: "i", title: "Diagnostics", description: "Check the lab and see recovery instructions"},
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
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Bold(true).PaddingLeft(2)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().PaddingLeft(2)
	menu := list.New(items, delegate, dashboardMenuWidth(width), dashboardMenuHeight(height))
	menu.Title = "Interventions"
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
	lines := []string{
		tuiTitle("Nixorium  /  Computer laboratory", model.isDark),
		"",
	}
	if model.report.PXE.Mode == "recovery-required" {
		lines = append(lines,
			tuiStatus("Controller network recovery required", tuiStatusAttention, model.isDark),
			"A previous installation session must be reconciled before normal networking can be trusted.",
			"Press p to open installation recovery.",
			"",
		)
	} else if model.report.PXE.Mode == "active" {
		lines = append(lines,
			tuiStatus("Network installation is active", tuiStatusAttention, model.isDark),
			"Press p to continue installation or restore normal controller networking.",
			"",
		)
	}
	lines = append(lines,
		tuiTitle("What do you want to do?", model.isDark),
		tuiMuted("Choose an intervention. Computers are checked only when the selected task needs them.", model.isDark),
	)
	lines = append(lines, "")
	if model.busy != "" {
		lines = append(lines, model.busyView(), "")
	}
	for index, item := range dashboardTasks {
		marker := "  "
		if index == menu.list.Index() {
			marker = "› "
		}
		label := marker + item.title
		if index == menu.list.Index() {
			label = tuiTitle(label, model.isDark)
		}
		lines = append(lines, label, tuiMuted("    "+item.description, model.isDark), "")
	}
	if model.message != "" {
		lines = append(lines, "", tuiMuted(model.message, model.isDark))
	}
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"up", "down"}, "↑/↓", "select"),
		tuiHelpBinding([]string{"enter"}, "enter", "open"),
		tuiHelpBinding([]string{"?"}, "?", "help"),
		tuiHelpBinding([]string{"q"}, "q", "quit"),
	))
	return strings.Join(lines, "\n") + "\n"
}
