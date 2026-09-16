package presentation

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type settingsField struct {
	id      string
	group   string
	label   string
	choices []settingsChoice
}

type settingsChoice struct {
	value       string
	label       string
	description string
}

type settingsChoiceItem settingsChoice

func (item settingsChoiceItem) Title() string       { return item.label }
func (item settingsChoiceItem) Description() string { return item.description }
func (item settingsChoiceItem) FilterValue() string { return item.label + " " + item.value }

const customSettingsChoice = "__custom__"

var timeZoneChoices = []settingsChoice{
	{value: "America/New_York", label: "America/New_York", description: "US Eastern time"},
	{value: "America/Chicago", label: "America/Chicago", description: "US Central time"},
	{value: "America/Denver", label: "America/Denver", description: "US Mountain time"},
	{value: "America/Los_Angeles", label: "America/Los_Angeles", description: "US Pacific time"},
	{value: "America/Phoenix", label: "America/Phoenix", description: "Arizona"},
	{value: "America/Anchorage", label: "America/Anchorage", description: "Alaska"},
	{value: "Pacific/Honolulu", label: "Pacific/Honolulu", description: "Hawaii"},
	{value: "Europe/Rome", label: "Europe/Rome", description: "Italy"},
	{value: "Europe/London", label: "Europe/London", description: "United Kingdom"},
	{value: "Europe/Paris", label: "Europe/Paris", description: "France"},
	{value: "Europe/Berlin", label: "Europe/Berlin", description: "Germany"},
	{value: "Europe/Madrid", label: "Europe/Madrid", description: "Spain"},
	{value: "UTC", label: "UTC", description: "Coordinated Universal Time"},
}

var localeChoices = []settingsChoice{
	{value: "en_US.UTF-8", label: "English — United States", description: "en_US.UTF-8"},
	{value: "it_IT.UTF-8", label: "Italiano — Italia", description: "it_IT.UTF-8"},
	{value: "en_GB.UTF-8", label: "English — United Kingdom", description: "en_GB.UTF-8"},
	{value: "fr_FR.UTF-8", label: "Français — France", description: "fr_FR.UTF-8"},
	{value: "de_DE.UTF-8", label: "Deutsch — Deutschland", description: "de_DE.UTF-8"},
	{value: "es_ES.UTF-8", label: "Español — España", description: "es_ES.UTF-8"},
}

var keyboardChoices = []settingsChoice{
	{value: "us", label: "US English", description: "XKB layout: us"},
	{value: "it", label: "Italian", description: "XKB layout: it"},
	{value: "gb", label: "UK English", description: "XKB layout: gb"},
	{value: "fr", label: "French", description: "XKB layout: fr"},
	{value: "de", label: "German", description: "XKB layout: de"},
	{value: "es", label: "Spanish", description: "XKB layout: es"},
}

var consoleKeyMapChoices = []settingsChoice{
	{value: "us", label: "US English", description: "Console keymap: us"},
	{value: "it2", label: "Italian", description: "Console keymap: it2"},
	{value: "uk", label: "UK English", description: "Console keymap: uk"},
	{value: "fr", label: "French", description: "Console keymap: fr"},
	{value: "de", label: "German", description: "Console keymap: de"},
	{value: "es", label: "Spanish", description: "Console keymap: es"},
}

var settingsFields = []settingsField{
	{id: "lab.ifaceName", group: "Network", label: "Laboratory network interface"},
	{id: "lab.masterDhcpIp", group: "Network", label: "Current controller DHCP address"},
	{id: "lab.networkBase", group: "Network", label: "Static laboratory network address"},
	{id: "lab.networkPrefixLength", group: "Network", label: "Network prefix length"},
	{id: "lab.pcCount", group: "Laboratory", label: "Number of client computers"},
	{id: "lab.masterHostNumber", group: "Laboratory", label: "Controller host number"},
	{id: "lab.teacherUser", group: "Accounts", label: "Teacher user name"},
	{id: "lab.studentUser", group: "Accounts", label: "Student user name"},
	{id: "lab.timeZone", group: "Regional settings", label: "Time zone", choices: timeZoneChoices},
	{id: "lab.defaultLocale", group: "Regional settings", label: "System language and locale", choices: localeChoices},
	{id: "lab.extraLocale", group: "Regional settings", label: "Regional formats", choices: localeChoices},
	{id: "lab.keyboardLayout", group: "Regional settings", label: "Desktop keyboard layout", choices: keyboardChoices},
	{id: "lab.consoleKeyMap", group: "Regional settings", label: "Console keyboard layout", choices: consoleKeyMapChoices},
	{id: "lab.homepageUrl", group: "Preferences", label: "Browser homepage"},
	{id: "lab.veyonNativeHosts", group: "Classroom", label: "Veyon native hosts (comma-separated, optional)"},
}

type settingsWizardModel struct {
	helpOpen  bool
	settings  domain.LabSettingsFile
	fields    []settingsField
	title     string
	drafts    []string
	index     int
	err       string
	accepted  bool
	cancelled bool
	replace   bool
	custom    bool
	selector  list.Model
	width     int
	height    int
	isDark    bool
}

func RunSettingsWizard(settings domain.LabSettingsFile) (domain.LabSettingsFile, bool, error) {
	model := newSettingsWizardModel(settings)
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return settings, false, err
	}
	result, ok := final.(settingsWizardModel)
	if !ok {
		return settings, false, fmt.Errorf("unexpected settings wizard result %T", final)
	}
	return result.settings, result.accepted && !result.cancelled, nil
}

func newSettingsWizardModel(settings domain.LabSettingsFile) settingsWizardModel {
	return newSettingsEditorModel(settings, settingsFields, "Nixorium first-run configuration")
}

func newSettingsEditorModel(settings domain.LabSettingsFile, fields []settingsField, title string) settingsWizardModel {
	drafts := make([]string, len(fields))
	for index, field := range fields {
		drafts[index] = settingFieldValue(settings, field.id)
	}
	model := settingsWizardModel{settings: settings, fields: fields, title: title, drafts: drafts, replace: true, width: 80, height: 24}
	model.prepareCurrentField()
	return model
}

func (model *settingsWizardModel) prepareCurrentField() {
	model.replace = true
	model.custom = false
	field := model.fields[model.index]
	if len(field.choices) == 0 {
		return
	}
	items := make([]list.Item, 0, len(field.choices)+2)
	current := model.drafts[model.index]
	selected := 0
	found := false
	for index, choice := range field.choices {
		items = append(items, settingsChoiceItem(choice))
		if choice.value == current {
			selected = index
			found = true
		}
	}
	if current != "" && !found {
		items = append([]list.Item{settingsChoiceItem{
			value:       current,
			label:       current,
			description: "Current custom value",
		}}, items...)
		selected = 0
	}
	items = append(items, settingsChoiceItem{
		value:       customSettingsChoice,
		label:       "Custom…",
		description: "Enter another validated value",
	})
	delegate := tuiListDelegate(model.isDark)
	model.selector = list.New(items, delegate, model.choiceWidth(), model.choiceHeight())
	model.selector.Title = field.label
	model.selector.SetShowTitle(false)
	model.selector.SetShowStatusBar(false)
	model.selector.SetShowHelp(false)
	model.selector.Styles = list.DefaultStyles(model.isDark)
	model.selector.Help.Styles = help.DefaultStyles(model.isDark)
	model.selector.Select(selected)
}

func (model settingsWizardModel) choiceWidth() int {
	if model.width < 40 {
		return 40
	}
	return model.width
}

func (model settingsWizardModel) choiceHeight() int {
	height := model.height - 8
	if height < 8 {
		return 8
	}
	if height > 16 {
		return 16
	}
	return height
}

func (model settingsWizardModel) moveToField(index int) settingsWizardModel {
	model.index = index
	model.err = ""
	model.prepareCurrentField()
	return model
}

func (settingsWizardModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (model settingsWizardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		if model.helpOpen {
			if key.String() == "esc" || key.String() == "f1" || key.String() == "?" {
				model.helpOpen = false
			}
			return model, nil
		}
		if key.String() == "f1" || (key.String() == "?" && len(model.fields[model.index].choices) > 0 && !model.custom && model.selector.FilterState() != list.Filtering) {
			model.helpOpen = true
			return model, nil
		}
	}
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.width = size.Width
		model.height = size.Height
		if len(model.fields[model.index].choices) > 0 && !model.custom {
			model.selector.SetSize(model.choiceWidth(), model.choiceHeight())
		}
		return model, nil
	}
	if background, ok := message.(tea.BackgroundColorMsg); ok {
		model.isDark = background.IsDark()
		if len(model.fields[model.index].choices) > 0 && !model.custom {
			model.prepareCurrentField()
		}
		return model, nil
	}
	field := model.fields[model.index]
	if len(field.choices) > 0 && !model.custom {
		if key, ok := message.(tea.KeyPressMsg); ok {
			if key.String() == "ctrl+c" {
				model.cancelled = true
				return model, tea.Quit
			}
			return model.updateChoiceField(key)
		}
		updated, command := model.selector.Update(message)
		model.selector = updated
		return model, command
	}
	if paste, ok := message.(tea.PasteMsg); ok {
		model = model.appendDraft(paste.Content)
		return model, nil
	}
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return model, nil
	}
	if key.String() == "ctrl+c" {
		model.cancelled = true
		return model, tea.Quit
	}
	switch key.String() {
	case "esc":
		if model.custom {
			model.custom = false
			model.err = ""
			model.prepareCurrentField()
			return model, nil
		}
		model.cancelled = true
		return model, tea.Quit
	case "shift+tab", "up", "left", "ctrl+b":
		if model.index > 0 {
			model = model.moveToField(model.index - 1)
		}
	case "enter":
		return model.acceptCurrentField()
	case "backspace":
		model.replace = false
		value := []rune(model.drafts[model.index])
		if len(value) > 0 {
			model.drafts[model.index] = string(value[:len(value)-1])
		}
		model.err = ""
	case "ctrl+u":
		model.drafts[model.index] = ""
		model.err = ""
		model.replace = false
	default:
		if key.Text != "" {
			model = model.appendDraft(key.Text)
		}
	}
	return model, nil
}

func (model settingsWizardModel) appendDraft(value string) settingsWizardModel {
	if model.replace {
		model.drafts[model.index] = ""
		model.replace = false
	}
	model.drafts[model.index] += value
	model.err = ""
	return model
}

func (model settingsWizardModel) updateChoiceField(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "shift+tab", "left", "ctrl+b":
		if model.selector.FilterState() == list.Filtering || model.selector.FilterState() == list.FilterApplied {
			updated, command := model.selector.Update(key)
			model.selector = updated
			return model, command
		}
		if model.index > 0 {
			model = model.moveToField(model.index - 1)
		}
		return model, nil
	case "esc":
		if model.selector.FilterState() == list.Filtering || model.selector.FilterState() == list.FilterApplied {
			updated, command := model.selector.Update(key)
			model.selector = updated
			return model, command
		}
		model.cancelled = true
		return model, tea.Quit
	case "enter":
		if model.selector.FilterState() == list.Filtering {
			updated, command := model.selector.Update(key)
			model.selector = updated
			return model, command
		}
		selected, ok := model.selector.SelectedItem().(settingsChoiceItem)
		if !ok {
			model.err = "select a value"
			return model, nil
		}
		if selected.value == customSettingsChoice {
			model.custom = true
			model.replace = true
			model.err = ""
			return model, nil
		}
		model.drafts[model.index] = selected.value
		return model.acceptCurrentField()
	default:
		updated, command := model.selector.Update(key)
		model.selector = updated
		return model, command
	}
}

func (model settingsWizardModel) acceptCurrentField() (tea.Model, tea.Cmd) {
	field := model.fields[model.index]
	candidate, err := setSettingField(model.settings, field.id, model.drafts[model.index])
	if err != nil {
		model.err = err.Error()
		return model, nil
	}
	model.settings = candidate
	model.err = ""
	if model.index == len(model.fields)-1 {
		if issues := candidate.Validate(); len(issues) > 0 {
			model.err = issues[0].Field + ": " + issues[0].Message
			return model, nil
		}
		model.accepted = true
		return model, tea.Quit
	}
	model = model.moveToField(model.index + 1)
	return model, nil
}

func (model settingsWizardModel) View() tea.View {
	if model.helpOpen {
		return tea.NewView(tuiTitle("First setup · Keyboard help", model.isDark) + "\n\nEnter continues after validation.\nShift Tab returns to the previous question.\nEsc cancels; existing settings are preserved.\n/ searches suggested values.\n\nEsc / F1 closes help.")
	}
	field := model.fields[model.index]
	lines := []string{
		tuiTitle(model.title, model.isDark),
		"",
		fmt.Sprintf("Step %d of %d — %s", model.index+1, len(model.fields), field.group),
		field.label,
	}
	if len(field.choices) > 0 && !model.custom {
		lines = append(lines, "", "Choose a suggested value, or press / to filter.", "", model.selector.View())
		lines = append(lines, "", "↑/↓ choose   enter continue   / search   esc cancel   F1 help")
	} else {
		if model.custom {
			lines = append(lines, "", "Custom value; it will be validated before continuing.")
		}
		lines = append(lines, "", "> "+model.drafts[model.index]+"█")
	}
	if model.err != "" {
		lines = append(lines, "", tuiError("Invalid: "+model.err, model.isDark))
	}
	if len(field.choices) == 0 || model.custom {
		backLabel := "cancel"
		if model.custom {
			backLabel = "suggestions"
		}
		lines = append(lines, "", tuiHelp(model.width, model.isDark,
			tuiHelpBinding([]string{"enter"}, "enter", "continue"),
			tuiHelpBinding([]string{"shift+tab", "up"}, "shift+tab/up", "previous"),
			tuiHelpBinding([]string{"esc"}, "esc", backLabel),
		))
	}
	view := tea.NewView(strings.Join(lines, "\n") + "\n")
	view.AltScreen = true
	return view
}

type configReviewModel struct {
	helpOpen  bool
	plan      domain.ConfigPlanReport
	git       domain.GitState
	accepted  bool
	cancelled bool
	width     int
	isDark    bool
}

func RunConfigReview(plan domain.ConfigPlanReport, git domain.GitState) (bool, error) {
	final, err := tea.NewProgram(configReviewModel{plan: plan, git: git}).Run()
	if err != nil {
		return false, err
	}
	result, ok := final.(configReviewModel)
	if !ok {
		return false, fmt.Errorf("unexpected configuration review result %T", final)
	}
	return result.accepted && !result.cancelled, nil
}

func (configReviewModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (model configReviewModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		if key.String() == "f1" || key.String() == "?" {
			model.helpOpen = !model.helpOpen
			return model, nil
		}
		if model.helpOpen && key.String() != "ctrl+c" {
			if key.String() == "esc" {
				model.helpOpen = false
			}
			return model, nil
		}
	}
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.width = size.Width
		return model, nil
	}
	if background, ok := message.(tea.BackgroundColorMsg); ok {
		model.isDark = background.IsDark()
		return model, nil
	}
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch strings.ToLower(key.String()) {
		case "y":
			model.accepted = true
			return model, tea.Quit
		case "n", "esc", "ctrl+c":
			model.cancelled = true
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model configReviewModel) View() tea.View {
	if model.helpOpen {
		view := tea.NewView(tuiTitle("Configuration review help", model.isDark) + "\n\nReview the changes before applying.\n\ny  Apply the reviewed managed settings\nn / Esc  Cancel without changing files\n\nEsc / F1  Close help\n")
		view.AltScreen = true
		return view
	}
	lines := []string{tuiTitle("Review configuration", model.isDark), ""}
	if len(model.plan.Changes) == 0 {
		lines = append(lines, "No managed settings will change.")
	}
	for _, change := range model.plan.Changes {
		lines = append(lines, fmt.Sprintf("%s: %v -> %v", change.Field, change.Before, change.After))
	}
	lines = append(lines, "")
	if model.git.Dirty {
		generated, unexpected := classifyExistingChanges(model.git.Paths)
		if len(generated) > 0 {
			lines = append(lines, "Setup-generated public key changes already present:", "  "+strings.Join(generated, "\n  "), "")
		}
		if len(unexpected) > 0 {
			lines = append(lines, "Warning: other existing worktree changes will not be touched:", "  "+strings.Join(unexpected, "\n  "), "")
		}
	}
	lines = append(lines, "Apply only these managed settings?", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"y"}, "y", "apply"),
		tuiHelpBinding([]string{"n", "esc"}, "n/esc", "cancel"),
	))
	view := tea.NewView(strings.Join(lines, "\n") + "\n")
	view.AltScreen = true
	return view
}

func classifyExistingChanges(paths []string) (generated, unexpected []string) {
	known := map[string]bool{
		"keys/cache-public-key":     true,
		"keys/admin-ssh.pub":        true,
		"keys/veyon-public-key.pem": true,
	}
	for _, path := range paths {
		if known[path] {
			generated = append(generated, path)
		} else {
			unexpected = append(unexpected, path)
		}
	}
	return generated, unexpected
}

func settingFieldValue(settings domain.LabSettingsFile, field string) string {
	switch field {
	case "lab.masterDhcpIp":
		return settings.Lab.MasterDHCPIP
	case "lab.networkBase":
		return settings.Lab.NetworkBase
	case "lab.networkPrefixLength":
		return strconv.Itoa(settings.Lab.NetworkPrefix)
	case "lab.pcCount":
		return strconv.Itoa(settings.Lab.PCCount)
	case "lab.masterHostNumber":
		return strconv.Itoa(settings.Lab.MasterHostNumber)
	case "lab.ifaceName":
		return settings.Lab.InterfaceName
	case "lab.teacherUser":
		return settings.Lab.TeacherUser
	case "lab.studentUser":
		return settings.Lab.StudentUser
	case "lab.homepageUrl":
		return settings.Lab.HomepageURL
	case "lab.studentGitName":
		return settings.Lab.StudentGitName
	case "lab.studentGitEmail":
		return settings.Lab.StudentGitEmail
	case "lab.adminGitName":
		return settings.Lab.AdminGitName
	case "lab.adminGitEmail":
		return settings.Lab.AdminGitEmail
	case "lab.timeZone":
		return settings.Lab.TimeZone
	case "lab.defaultLocale":
		return settings.Lab.DefaultLocale
	case "lab.extraLocale":
		return settings.Lab.ExtraLocale
	case "lab.keyboardLayout":
		return settings.Lab.KeyboardLayout
	case "lab.consoleKeyMap":
		return settings.Lab.ConsoleKeyMap
	case "lab.veyonNativeHosts":
		return strings.Join(settings.Lab.VeyonNativeHosts, ",")
	default:
		return ""
	}
}

func setSettingField(settings domain.LabSettingsFile, field, value string) (domain.LabSettingsFile, error) {
	value = strings.TrimSpace(value)
	parseInteger := func() (int, error) {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, errors.New("must be an integer")
		}
		return parsed, nil
	}
	switch field {
	case "lab.masterDhcpIp":
		settings.Lab.MasterDHCPIP = value
	case "lab.networkBase":
		settings.Lab.NetworkBase = value
	case "lab.networkPrefixLength":
		parsed, err := parseInteger()
		if err != nil {
			return settings, err
		}
		settings.Lab.NetworkPrefix = parsed
	case "lab.pcCount":
		parsed, err := parseInteger()
		if err != nil {
			return settings, err
		}
		settings.Lab.PCCount = parsed
	case "lab.masterHostNumber":
		parsed, err := parseInteger()
		if err != nil {
			return settings, err
		}
		settings.Lab.MasterHostNumber = parsed
	case "lab.ifaceName":
		settings.Lab.InterfaceName = value
	case "lab.teacherUser":
		settings.Lab.TeacherUser = value
	case "lab.studentUser":
		settings.Lab.StudentUser = value
	case "lab.homepageUrl":
		settings.Lab.HomepageURL = value
	case "lab.studentGitName":
		settings.Lab.StudentGitName = value
	case "lab.studentGitEmail":
		settings.Lab.StudentGitEmail = value
	case "lab.adminGitName":
		settings.Lab.AdminGitName = value
	case "lab.adminGitEmail":
		settings.Lab.AdminGitEmail = value
	case "lab.timeZone":
		settings.Lab.TimeZone = value
	case "lab.defaultLocale":
		settings.Lab.DefaultLocale = value
	case "lab.extraLocale":
		settings.Lab.ExtraLocale = value
	case "lab.keyboardLayout":
		settings.Lab.KeyboardLayout = value
	case "lab.consoleKeyMap":
		settings.Lab.ConsoleKeyMap = value
	case "lab.veyonNativeHosts":
		settings.Lab.VeyonNativeHosts = []string{}
		for _, host := range strings.Split(value, ",") {
			if host = strings.TrimSpace(host); host != "" {
				settings.Lab.VeyonNativeHosts = append(settings.Lab.VeyonNativeHosts, host)
			}
		}
	default:
		return settings, errors.New("unsupported setting")
	}
	for _, issue := range settings.Validate() {
		if issue.Field == field {
			return settings, errors.New(issue.Message)
		}
	}
	return settings, nil
}
