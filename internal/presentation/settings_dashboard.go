package presentation

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type SettingsPasswordAction func(string, domain.LabSettingsFile, *os.File, io.Writer) (domain.LabSettingsFile, error)

type routineSettingsGroup struct {
	id          string
	label       string
	description string
	fields      []settingsField
}

type routineSettingsGroupItem struct {
	index int
	group routineSettingsGroup
}

func (item routineSettingsGroupItem) Title() string       { return item.group.label }
func (item routineSettingsGroupItem) Description() string { return item.group.description }
func (item routineSettingsGroupItem) FilterValue() string {
	return item.group.label + " " + item.group.description
}

var routineSettingsGroups = []routineSettingsGroup{
	{
		id:          "network",
		label:       "Network",
		description: "Controller/client interfaces, address, and laboratory subnet",
		fields: []settingsField{
			{id: "lab.controllerIfaceName", group: "Network", label: "Controller network interface (optional override)"},
			{id: "lab.clientIfaceName", group: "Network", label: "Default client network interface (optional override)"},
			{id: "lab.ifaceName", group: "Network", label: "Fallback network interface"},
			{id: "lab.masterDhcpIp", group: "Network", label: "Current controller DHCP address"},
			{id: "lab.networkBase", group: "Network", label: "Static laboratory network address"},
			{id: "lab.networkPrefixLength", group: "Network", label: "Network prefix length"},
		},
	},
	{
		id:          "computers",
		label:       "Computers",
		description: "Client count and controller host number",
		fields: []settingsField{
			{id: "lab.pcCount", group: "Computers", label: "Number of client computers"},
			{id: "lab.masterHostNumber", group: "Computers", label: "Controller host number"},
		},
	},
	{
		id:          "accounts",
		label:       "Accounts",
		description: "Teacher and student account names",
		fields: []settingsField{
			{id: "lab.teacherUser", group: "Accounts", label: "Teacher user name"},
			{id: "lab.studentUser", group: "Accounts", label: "Student user name"},
		},
	},
	{
		id:          "regional",
		label:       "Regional",
		description: "Time zone and keyboard layout",
		fields: []settingsField{
			{id: "lab.timeZone", group: "Regional", label: "Time zone", choices: timeZoneChoices},
			{id: "lab.keyboardLayout", group: "Regional", label: "Keyboard layout", choices: keyboardChoices},
		},
	},
	{
		id:          "browser",
		label:       "Browser",
		description: "Default classroom homepage",
		fields: []settingsField{
			{id: "lab.homepageUrl", group: "Browser", label: "Browser homepage"},
		},
	},
	{
		id:          "git",
		label:       "Git",
		description: "Student and administrator commit identity",
		fields: []settingsField{
			{id: "lab.studentGitName", group: "Git", label: "Student Git author name"},
			{id: "lab.studentGitEmail", group: "Git", label: "Student Git author email"},
			{id: "lab.adminGitName", group: "Git", label: "Administrator Git author name"},
			{id: "lab.adminGitEmail", group: "Git", label: "Administrator Git author email"},
		},
	},
	{
		id:          "veyon",
		label:       "Veyon",
		description: "Native classroom-control hosts",
		fields: []settingsField{
			{id: "lab.veyonNativeHosts", group: "Veyon", label: "Veyon native hosts (comma-separated, optional)"},
		},
	},
}

type routineSettingsMenu struct {
	list list.Model
}

type routinePasswordChoice struct {
	id          string
	label       string
	description string
}

func (item routinePasswordChoice) Title() string       { return item.label }
func (item routinePasswordChoice) Description() string { return item.description }
func (item routinePasswordChoice) FilterValue() string { return item.label }

var routinePasswordChoices = []routinePasswordChoice{
	{id: "admin", label: "Administrator", description: "Controller administrator account"},
	{id: "teacher", label: "Teacher", description: "Classroom teacher account"},
	{id: "student", label: "Student", description: "Classroom student account"},
}

type routinePasswordMenu struct {
	list        list.Model
	initialized bool
}

func newRoutineSettingsMenu(isDark bool, width, height int) routineSettingsMenu {
	items := make([]list.Item, 0, len(routineSettingsGroups))
	for index, group := range routineSettingsGroups {
		items = append(items, routineSettingsGroupItem{index: index, group: group})
	}
	delegate := tuiListDelegate(isDark)
	menu := list.New(items, delegate, settingsMenuWidth(width), settingsMenuHeight(height))
	menu.Title = "Settings categories"
	menu.SetShowTitle(false)
	menu.SetShowStatusBar(false)
	menu.SetShowHelp(false)
	menu.Styles = list.DefaultStyles(isDark)
	menu.Help.Styles = help.DefaultStyles(isDark)
	return routineSettingsMenu{list: menu}
}

func settingsMenuWidth(width int) int {
	return max(28, min(108, width-10))
}

func settingsMenuHeight(height int) int {
	height -= 13
	if height < 4 {
		return 4
	}
	if height > 20 {
		return 20
	}
	return height
}

func (menu *routineSettingsMenu) setSize(width, height int) {
	menu.list.SetSize(settingsMenuWidth(width), settingsMenuHeight(height))
}

func (menu routineSettingsMenu) selected() (routineSettingsGroup, bool) {
	item, ok := menu.list.SelectedItem().(routineSettingsGroupItem)
	if !ok || item.index < 0 || item.index >= len(routineSettingsGroups) {
		return routineSettingsGroup{}, false
	}
	return routineSettingsGroups[item.index], true
}

func (menu *routineSettingsMenu) selectGroup(id string) {
	for index, group := range routineSettingsGroups {
		if group.id == id {
			menu.list.Select(index)
			return
		}
	}
}

func (menu routineSettingsMenu) update(message tea.Msg) (routineSettingsMenu, tea.Cmd) {
	updated, command := menu.list.Update(message)
	menu.list = updated
	return menu, command
}

func newRoutinePasswordMenu(isDark bool, width, height int) routinePasswordMenu {
	items := make([]list.Item, 0, len(routinePasswordChoices))
	for _, choice := range routinePasswordChoices {
		items = append(items, choice)
	}
	delegate := tuiListDelegate(isDark)
	menu := list.New(items, delegate, settingsMenuWidth(width), settingsMenuHeight(height))
	menu.Title = "Password account"
	menu.SetShowTitle(false)
	menu.SetShowStatusBar(false)
	menu.SetShowHelp(false)
	menu.SetFilteringEnabled(false)
	menu.Styles = list.DefaultStyles(isDark)
	menu.Help.Styles = help.DefaultStyles(isDark)
	return routinePasswordMenu{list: menu, initialized: true}
}

func (menu *routinePasswordMenu) setSize(width, height int) {
	if !menu.initialized {
		return
	}
	menu.list.SetSize(settingsMenuWidth(width), settingsMenuHeight(height))
}

func (menu routinePasswordMenu) selected() (routinePasswordChoice, bool) {
	if !menu.initialized {
		return routinePasswordChoice{}, false
	}
	item, ok := menu.list.SelectedItem().(routinePasswordChoice)
	return item, ok
}

func (menu routinePasswordMenu) update(message tea.Msg) (routinePasswordMenu, tea.Cmd) {
	if !menu.initialized {
		return menu, nil
	}
	updated, command := menu.list.Update(message)
	menu.list = updated
	return menu, command
}

type settingsPasswordCommand struct {
	action    SettingsPasswordAction
	account   string
	settings  domain.LabSettingsFile
	input     *os.File
	output    io.Writer
	inputErr  error
	candidate domain.LabSettingsFile
}

func (command *settingsPasswordCommand) SetStdin(reader io.Reader) {
	input, ok := reader.(*os.File)
	if !ok {
		command.inputErr = errors.New("password input is not a terminal file")
		return
	}
	command.input = input
}

func (command *settingsPasswordCommand) SetStdout(writer io.Writer) { command.output = writer }
func (command *settingsPasswordCommand) SetStderr(io.Writer)        {}

func (command *settingsPasswordCommand) Run() error {
	if command.inputErr != nil {
		return command.inputErr
	}
	if command.input == nil || command.output == nil {
		return errors.New("password terminal input and output are required")
	}
	candidate, err := command.action(command.account, command.settings, command.input, command.output)
	command.candidate = candidate
	if err != nil {
		return err
	}
	return nil
}

func (model dashboardModel) settingsView() string {
	title := "Nixorium — Settings"
	intro := "Choose one area to edit. Values are validated before any file changes."
	backLabel := "back"
	if model.settingsReturn == dashboardSetup {
		title = "Nixorium — First setup / Laboratory settings"
		intro = "Complete the required laboratory settings here, then return to setup."
		backLabel = "setup"
	}
	if model.busy != "" {
		return tuiTitle(title, model.isDark) + "\n\n" + model.busyView() + "\n"
	}
	switch model.screen {
	case dashboardSettingsEdit:
		return model.settingsEditor.View().Content
	case dashboardSettingsPasswords:
		return model.settingsPasswordsView()
	case dashboardSettingsReview:
		return model.settingsReviewView()
	}
	lines := []string{tuiTitle(title, model.isDark), ""}
	if model.settingsResult.Operation != "" {
		success := !model.settingsResult.HasErrors() && (model.settingsResult.State == "saved" || model.settingsResult.State == "unchanged")
		title := "Configuration needs attention"
		if success {
			title = "Configuration saved"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Changed fields: %d", model.settingsResult.State, len(model.settingsResult.Changes)),
		)
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		returnLabel := "dashboard"
		if model.settingsReturn == dashboardSetup {
			returnLabel = "setup"
		}
		bindings := []key.Binding{tuiHelpBinding([]string{"e"}, "e", "edit more")}
		if model.settingsResult.RecoveryRequired {
			bindings = append(bindings, tuiHelpBinding([]string{"r"}, "r", "retry save"))
		}
		bindings = append(bindings, tuiHelpBinding([]string{"enter"}, "enter", returnLabel))
		lines = append(lines, "", tuiHelp(model.width, model.isDark, bindings...))
		return strings.Join(lines, "\n") + "\n"
	}
	lines = append(lines,
		intro,
		"",
		model.settingsMenu.list.View(),
		"",
		"enter edit   / search   p passwords   esc "+backLabel+"   ? help",
	)
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) settingsPasswordsView() string {
	if model.settingsReturn == dashboardSetup {
		lines := []string{
			tuiTitle("Nixorium — First setup / Account passwords", model.isDark),
			"",
			"Enter the administrator, teacher, and student passwords in one protected session.",
			"After all three are accepted, Nixorium validates the complete laboratory configuration once.",
			"",
			"Enter: collect all passwords   Esc: cancel setup configuration",
		}
		if model.message != "" {
			lines = append(lines, "", "Result: "+model.message)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	lines := []string{
		tuiTitle("Nixorium — Change password", model.isDark),
		"",
		"Choose the account whose password you want to change.",
		"Your password stays hidden while you type.",
		"",
		model.settingsPasswordMenu.list.View(),
		"",
		"Enter: change selected password   Esc: back",
	}
	if model.message != "" {
		lines = append(lines, "", "Result: "+model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (model dashboardModel) settingsReviewView() string {
	lines := []string{
		tuiTitle("Nixorium — Settings review", model.isDark),
		"",
		"Validated managed-setting changes",
	}
	for _, change := range model.settingsPlan.Changes {
		before := change.Before
		after := change.After
		if change.Sensitive {
			before = "configured"
			after = "updated"
		}
		lines = append(lines, fmt.Sprintf("  %s: %v → %v", change.Field, before, after))
	}
	lines = append(lines,
		"",
		"Only lab-settings.json will be replaced atomically.",
		"Existing unrelated files and changes are preserved.",
		"Saving does not rebuild or change any computer.",
		"",
		"Save these settings?",
		tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"y"}, "y", "save"),
			tuiHelpBinding([]string{"n", "esc"}, "n/esc", "cancel"),
		),
	)
	if model.message != "" {
		lines = append(lines, "", model.message)
	}
	return strings.Join(lines, "\n") + "\n"
}

func settingsIssueMessage(issues []domain.ValidationIssue) string {
	if len(issues) == 0 {
		return "settings operation failed"
	}
	return issues[0].Field + ": " + issues[0].Message
}
