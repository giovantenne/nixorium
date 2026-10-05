package presentation

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type dashboardTask struct {
	id          string
	shortcut    string
	title       string
	description string
	// advanced tasks are listed after a separator in their menu.
	advanced bool
}

func (task dashboardTask) Title() string       { return menuTitle(task.shortcut, task.title) }
func (task dashboardTask) Description() string { return task.description }
func (task dashboardTask) FilterValue() string { return task.title + " " + task.description }

var dashboardTasks = []dashboardTask{
	{id: "computers", shortcut: "c", title: "Computers", description: "Inventory, updating computers, Internet access and power"},
	{id: "installation", shortcut: "n", title: "Installation", description: "Configure the lab and install computers by PXE or the official USB ISO over SSH"},
	{id: "software", shortcut: "w", title: "Software", description: "Review configured choices or search this lab's pinned packages"},
	{id: "admin", shortcut: "a", title: "Maintenance", description: "Settings, controller updates, services, revisions, logs and diagnostics"},
}

var computersAreaTasks = []dashboardTask{
	{id: "hosts", shortcut: "h", title: "Computer inventory", description: "Check reachability and compare observed systems with the intended revision"},
	{id: "deploy", shortcut: "d", title: "Update computers", description: "Make the selected client computers run the saved configuration"},
	{id: "shutdown", shortcut: "x", title: "Power controls", description: "Shut down or restart selected client computers after review"},
	{id: "internet", shortcut: "i", title: "Internet access", description: "Temporarily block or restore Internet on selected clients"},
}

var classroomComputerTasks = []dashboardTask{
	computersAreaTasks[0],
	computersAreaTasks[2],
	computersAreaTasks[3],
}

var lockScreensTask = dashboardTask{id: "lock", shortcut: "l", title: "Lock screens", description: "Lock or unlock the screens of selected students' computers"}

var sendDesktopTask = dashboardTask{id: "share", shortcut: "s", title: "Send files", description: "Copy a file or folder to the desktops of selected students' computers"}

var classroomViewTask = dashboardTask{id: "view", shortcut: "v", title: "Classroom view", description: "See, control and lock the students' screens, show yours, send your desktop"}

func (model dashboardModel) availableComputerTasks() []dashboardTask {
	tasks := computersAreaTasks
	if model.actions.ClassroomMode {
		tasks = classroomComputerTasks
	}
	if model.actions.PlanLock != nil {
		tasks = append(append([]dashboardTask{}, tasks...), lockScreensTask)
	}
	if model.actions.PlanShare != nil {
		tasks = append(append([]dashboardTask{}, tasks...), sendDesktopTask)
	}
	if model.actions.OpenClassroomView != nil {
		tasks = append(append([]dashboardTask{}, tasks...), classroomViewTask)
	}
	return tasks
}

var installationAreaTasks = []dashboardTask{
	{id: "pxe", shortcut: "p", title: "Network boot (PXE)", description: "Prepare, start or finish network installation; recover interrupted networking here"},
	{id: "usb", shortcut: "u", title: "USB over SSH", description: "Install one physically identified computer using the official Minimal ISO"},
}

// Maintenance lists frequent tasks first; advanced tools follow in a
// visually separate group so the default selection is never destructive.
var administrationTasks = []dashboardTask{
	{id: "settings", shortcut: "e", title: "Change settings", description: "Network, accounts, regional values, browser, Git and controller keys"},
	{id: "diagnostics", shortcut: "i", title: "Diagnostics", description: "Check the lab and see recovery instructions"},
	{id: "package-base", shortcut: "b", title: "Update system and packages", description: "Refresh the NixOS base or review a channel migration"},
	{id: "update", shortcut: "u", title: "Update Nixorium", description: "Choose master or a release fetched from the configured upstream"},
	{id: "logs", shortcut: "l", title: "View operation logs", description: "Recent outcomes and bounded deployment log tails"},
	{id: "backup", shortcut: "y", title: "Back up the controller", description: "Encrypted copy of the configuration, its history and the private keys"},
	{id: "cleanup", shortcut: "f", title: "Free disk space", description: "Remove old system versions on the controller and clients, after review", advanced: true},
	{id: "controller", shortcut: "c", title: "Apply to controller", description: "Make this controller run the saved configuration, after review", advanced: true},
	{id: "services", shortcut: "s", title: "Controller services", description: "Check software delivery services or restart the signed cache when troubleshooting", advanced: true},
	{id: "git", shortcut: "g", title: "Review Git changes", description: "Inspect and commit selected safe deployment files", advanced: true},
	{id: "template-reset", shortcut: "t", title: "Reset deployment template", description: "Replace local customizations from the locked upstream; preserve settings and keys with a backup", advanced: true},
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
	model.syncHomeTasks()
	menu := model.homeMenu
	if len(menu.list.Items()) == 0 {
		menu = newDashboardTaskMenu(model.isDark, model.width, model.height)
	}
	if model.initialError {
		return model.safeModeView()
	}
	if model.initializing {
		return model.renderShell(tuiShell{
			path:    []string{"Overview"},
			body:    model.busyView() + "\n\n" + tuiMuted("Reading the saved laboratory configuration and setup state…", model.isDark),
			actions: []tuiAction{{key: "q", label: "Quit"}, {key: "F1", label: "Help"}},
		})
	}
	lines := []string{
		tuiTitle("Laboratory overview", model.isDark),
		tuiMuted("Choose an area. Observed state is loaded only when the selected task needs it.", model.isDark),
		"",
	}
	notices := []tuiNotice{}
	// Rows appear only when something needs action; an empty list says nothing.
	if len(model.pendingTasks()) > 0 {
		lines = append(lines, "Needs attention — press the number to open it", "")
	}
	if model.busy != "" {
		lines = append(lines, model.busyView(), "")
	}
	lines = append(lines, model.taskMenu(model.overviewTasks(), menu.list.Index()))
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	actions := []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "r", label: "Refresh local state"}, {key: "F1", label: "Help"}, {key: "q", label: "Quit"}}
	if model.actions.LoadManagedJobs != nil && !model.actions.ClassroomMode {
		actions = append([]tuiAction{{key: "v", label: "View progress"}}, actions...)
	}
	return model.renderShell(tuiShell{
		path:    []string{"Overview"},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: actions,
	})
}

func setupOverviewNotice(setup domain.SetupReport) (tuiNotice, bool) {
	if setup.State == "ready" || setup.State == "unchecked" {
		return tuiNotice{}, false
	}
	notice := tuiNotice{kind: tuiStatusAttention}
	switch setup.CurrentStage {
	case domain.SetupStageNetwork, domain.SetupStageIdentity, domain.SetupStageCredentials, domain.SetupStageKeys, domain.SetupStageValidate:
		notice.title = "Computer installation is not configured yet"
		notice.detail = "Choose Installation to complete the laboratory configuration."
	case domain.SetupStageReview:
		notice.title = "Managed configuration has uncommitted changes"
		notice.detail = "Open Maintenance > Review Git changes to review and save them."
	case domain.SetupStageApply:
		notice.title = "Saved configuration needs applying to this controller"
		notice.detail = "Open Maintenance > Apply to controller for a fresh review."
	default:
		// Network boot files are needed only for PXE installation; a lab
		// installed from USB never prepares them. Installation > Network boot
		// asks for them when it is used.
		return tuiNotice{}, false
	}
	return notice, true
}

func (model dashboardModel) areaView(path, title, description string, tasks []dashboardTask, cursor int) string {
	lines := []string{tuiTitle(title, model.isDark), tuiMuted(description, model.isDark), ""}
	lines = append(lines, model.taskMenu(tasks, cursor))
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusNeutral, title: model.message})
	}
	actions := []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
	if model.actions.ClassroomMode && path == "Computers" {
		actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Open"}, {key: "F1", label: "Help"}, {key: "q", label: "Quit"}}
	}
	return model.renderShell(tuiShell{
		path:    []string{path},
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: actions,
	})
}

func (model dashboardModel) computersAreaView() string {
	title := "Manage client computers"
	description := "Observed state is loaded only by Computer inventory or an operation that needs it."
	if model.actions.ClassroomMode {
		title = "Classroom controls"
		description = "Check computers, control temporary Internet access, or review a shutdown or restart. Administrative configuration is not available here."
	}
	return model.areaView(
		"Computers",
		title,
		description,
		model.availableComputerTasks(),
		model.computers.areaCursor,
	)
}
