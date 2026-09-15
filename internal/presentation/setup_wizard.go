package presentation

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type settingsField struct {
	id    string
	label string
}

var settingsFields = []settingsField{
	{id: "lab.masterDhcpIp", label: "Current controller DHCP address"},
	{id: "lab.networkBase", label: "Static laboratory network address"},
	{id: "lab.networkPrefixLength", label: "Network prefix length"},
	{id: "lab.pcCount", label: "Number of client computers"},
	{id: "lab.masterHostNumber", label: "Controller host number"},
	{id: "lab.ifaceName", label: "Laboratory network interface"},
	{id: "lab.teacherUser", label: "Teacher user name"},
	{id: "lab.studentUser", label: "Student user name"},
	{id: "lab.homepageUrl", label: "Browser homepage"},
	{id: "lab.studentGitName", label: "Student Git author name"},
	{id: "lab.studentGitEmail", label: "Student Git author email"},
	{id: "lab.adminGitName", label: "Administrator Git author name"},
	{id: "lab.adminGitEmail", label: "Administrator Git author email"},
	{id: "lab.timeZone", label: "Time zone"},
	{id: "lab.defaultLocale", label: "Default locale"},
	{id: "lab.extraLocale", label: "Regional-format locale"},
	{id: "lab.keyboardLayout", label: "Keyboard layout"},
	{id: "lab.consoleKeyMap", label: "Console key map"},
	{id: "lab.veyonNativeHosts", label: "Veyon native hosts (comma-separated, optional)"},
}

type settingsWizardModel struct {
	settings  domain.LabSettingsFile
	drafts    []string
	index     int
	err       string
	accepted  bool
	cancelled bool
	replace   bool
	width     int
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
	drafts := make([]string, len(settingsFields))
	for index, field := range settingsFields {
		drafts[index] = settingFieldValue(settings, field.id)
	}
	return settingsWizardModel{settings: settings, drafts: drafts, replace: true}
}

func (settingsWizardModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (model settingsWizardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.width = size.Width
		return model, nil
	}
	if background, ok := message.(tea.BackgroundColorMsg); ok {
		model.isDark = background.IsDark()
		return model, nil
	}
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return model, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		model.cancelled = true
		return model, tea.Quit
	case "shift+tab", "up", "ctrl+b":
		if model.index > 0 {
			model.index--
			model.err = ""
			model.replace = true
		}
	case "enter":
		candidate, err := setSettingField(model.settings, settingsFields[model.index].id, model.drafts[model.index])
		if err != nil {
			model.err = err.Error()
			return model, nil
		}
		model.settings = candidate
		model.err = ""
		if model.index == len(settingsFields)-1 {
			if issues := candidate.Validate(); len(issues) > 0 {
				model.err = issues[0].Field + ": " + issues[0].Message
				return model, nil
			}
			model.accepted = true
			return model, tea.Quit
		}
		model.index++
		model.replace = true
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
			if model.replace {
				model.drafts[model.index] = ""
				model.replace = false
			}
			model.drafts[model.index] += key.Text
			model.err = ""
		}
	}
	return model, nil
}

func (model settingsWizardModel) View() tea.View {
	field := settingsFields[model.index]
	lines := []string{
		tuiTitle("Nixorium first-run configuration", model.isDark),
		"",
		fmt.Sprintf("Field %d of %d", model.index+1, len(settingsFields)),
		field.label,
		"",
		"> " + model.drafts[model.index] + "█",
	}
	if model.err != "" {
		lines = append(lines, "", tuiError("Invalid: "+model.err, model.isDark))
	}
	lines = append(lines, "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"enter"}, "enter", "continue"),
		tuiHelpBinding([]string{"shift+tab", "up"}, "shift+tab/up", "back"),
		tuiHelpBinding([]string{"esc"}, "esc", "cancel"),
	))
	view := tea.NewView(strings.Join(lines, "\n") + "\n")
	view.AltScreen = true
	return view
}

type configReviewModel struct {
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
