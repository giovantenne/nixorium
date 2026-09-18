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
	{id: "computers", shortcut: "c", title: "Computers", description: "Inventory, distribute, restore or shut down client computers"},
	{id: "installation", shortcut: "n", title: "Installation", description: "Configure the lab, prepare netboot and guide computer installation"},
	{id: "software", shortcut: "w", title: "Software", description: "Review configured choices or search this lab's pinned packages"},
	{id: "admin", shortcut: "a", title: "Maintenance", description: "Settings, controller updates, services, revisions, logs and diagnostics"},
}

var computersAreaTasks = []dashboardTask{
	{id: "hosts", shortcut: "h", title: "Computer inventory", description: "Check reachability and compare observed systems with the intended revision"},
	{id: "deploy", shortcut: "d", title: "Distribute the prepared system", description: "Update only the computers selected for this intervention"},
	{id: "restore", shortcut: "r", title: "Restore computers", description: "Reapply the intended system or reinstall from scratch"},
	{id: "shutdown", shortcut: "x", title: "Shut down computers", description: "Send reviewed power-off requests to selected clients only"},
}

var installationAreaTasks = []dashboardTask{
	{id: "install", shortcut: "n", title: "Install a computer", description: "Guided workflow: choose its configured identity, start PXE and verify the installation"},
	{id: "pxe", shortcut: "p", title: "PXE mode and network recovery", description: "Advanced controls for network boot and interrupted controller networking"},
}

var administrationTasks = []dashboardTask{
	{id: "update", shortcut: "u", title: "Update Nixorium", description: "Choose master or a release fetched from the configured upstream"},
	{id: "settings", shortcut: "e", title: "Change settings", description: "Network, accounts, regional values, browser, Git, and Veyon"},
	{id: "controller", shortcut: "c", title: "Rebuild controller", description: "Review and activate the committed controller revision"},
	{id: "services", shortcut: "s", title: "Controller services", description: "Check software delivery services or restart the signed cache when troubleshooting"},
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
	if model.initialError {
		return renderTUIShell(tuiShell{
			path: []string{"Overview"},
			body: "The saved laboratory state is not available yet.",
			notices: []tuiNotice{{
				kind:   tuiStatusFailure,
				title:  "The laboratory could not be opened",
				detail: model.message + " No configuration or computer was changed.",
			}},
			actions: []tuiAction{{key: "Enter", label: "Try again"}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}},
		}, model.width, model.isDark)
	}
	if model.initializing {
		return renderTUIShell(tuiShell{
			path:    []string{"Overview"},
			body:    model.busyView() + "\n\n" + tuiMuted("Reading the saved laboratory configuration and setup state…", model.isDark),
			actions: []tuiAction{{key: "q", label: "Quit"}, {key: "F1", label: "Help"}},
		}, model.width, model.isDark)
	}
	lines := []string{
		tuiTitle("Laboratory overview", model.isDark),
		tuiMuted("Choose an area. Observed state is loaded only when the selected task needs it.", model.isDark),
		"",
	}
	notices := []tuiNotice{}
	if model.report.PXE.Mode == "recovery-required" {
		notices = append(notices, tuiNotice{
			kind:   tuiStatusAttention,
			title:  "Controller network recovery required",
			detail: "Open Installation to reconcile the previous session before trusting normal networking.",
		})
	} else if model.report.PXE.Mode == "active" {
		notices = append(notices, tuiNotice{
			kind:   tuiStatusAttention,
			title:  "Network installation is active",
			detail: "Open Installation to continue or restore normal controller networking.",
		})
	}
	if model.setup.State != "ready" {
		notices = append(notices, tuiNotice{
			kind:   tuiStatusAttention,
			title:  "Computer installation is not configured yet",
			detail: "Choose Installation when you are ready to configure the laboratory.",
		})
	}
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
		lines = append(lines, label, tuiMuted("    "+item.description, model.isDark))
	}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return renderTUIShell(tuiShell{
		path:    []string{"Overview"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "?", label: "Help"}, {key: "q", label: "Quit"}},
	}, model.width, model.isDark)
}

func (model dashboardModel) areaView(path, title, description string, tasks []dashboardTask, cursor int) string {
	lines := []string{tuiTitle(title, model.isDark), tuiMuted(description, model.isDark), ""}
	for index, task := range tasks {
		marker := "  "
		if index == cursor {
			marker = "› "
		}
		label := marker + task.title
		if index == cursor {
			label = tuiTitle(label, model.isDark)
		}
		lines = append(lines, label, tuiMuted("    "+task.description, model.isDark))
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	return renderTUIShell(tuiShell{
		path:    []string{path},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "Esc", label: "Overview"}, {key: "?", label: "Help"}},
	}, model.width, model.isDark)
}

func (model dashboardModel) computersAreaView() string {
	return model.areaView(
		"Computers",
		"Manage client computers",
		"Observed state is loaded only by Computer inventory or an operation that needs it.",
		computersAreaTasks,
		model.computersAreaCursor,
	)
}

func (model dashboardModel) installationAreaView() string {
	return model.areaView(
		"Installation",
		"Install client computers",
		"Prepare and control network installation without mixing it with routine distribution.",
		installationAreaTasks,
		model.installationAreaCursor,
	)
}
