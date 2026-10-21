package presentation

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) setupView() string {
	groups, current := setupJourney(model.setup)
	lines := []string{
		tuiTitle("Laboratory setup", model.isDark),
		fmt.Sprintf("Step %d of %d", current+1, len(groups)),
		"You can leave safely and resume this setup later.",
		"",
	}
	if model.busy != "" {
		lines = append(lines, model.busyView(), "")
	}
	for _, group := range groups {
		label := "○ " + group.title + " · " + group.pending
		switch group.state {
		case domain.SetupStageComplete:
			label = "✓ " + group.title + " · Complete"
		case domain.SetupStageCurrent:
			label = "● " + group.title + " · In progress"
		}
		lines = append(lines, "  "+label)
	}
	if model.setupDetails {
		lines = append(lines, "", tuiSection("Technical steps", model.isDark))
		for _, stage := range model.setup.Stages {
			marker := "○"
			if stage.State == domain.SetupStageComplete {
				marker = "✓"
			} else if stage.State == domain.SetupStageCurrent {
				marker = "●"
			}
			lines = append(lines, fmt.Sprintf("  %s %s", marker, stage.Title))
		}
	}
	lines = append(lines, "")
	if model.setup.State == "ready" {
		lines = append(lines,
			tuiResult("Controller and client systems are ready", true, model.isDark),
			"Enter opens network installation. Any configured computer can boot the installer.",
		)
	} else {
		lines = append(lines,
			tuiSection("Continue setup", model.isDark),
			"  "+setupCurrentTitle(model.setup),
			"  "+setupCurrentAction(model.setup),
			"  Press Enter to continue.",
		)
	}
	backLabel := "Overview"
	if model.areaReturn == dashboardInstallationArea {
		backLabel = "Installation"
	}
	shell := tuiShell{
		path:    []string{"Installation", "Setup"},
		body:    strings.Join(lines, "\n"),
		actions: []tuiAction{{key: "Enter", label: "Continue"}, {key: "t", label: "Technical steps"}, {key: "Esc", label: backLabel}, {key: "q", label: "Quit"}, {key: "F1", label: "Help"}},
	}
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusNeutral, title: model.message}}
	}
	return model.renderShell(shell)
}

func (model dashboardModel) setupKeysView() string {
	path := []string{"Installation", "Setup", "Controller keys"}
	backLabel := "Setup"
	readyLabel := "Save and continue"
	if model.setupKeysReturn == dashboardSettings {
		path = []string{"Maintenance", "Settings", "Advanced", "Controller keys"}
		backLabel = "Settings"
		readyLabel = "Install verified keys"
	}
	lines := []string{
		tuiTitle("Controller keys", model.isDark),
		"These keys authenticate the controller. Private keys stay local and are never added to configuration history.",
		"Existing valid keys are reused and never replaced by this workflow.",
		"",
	}
	if model.busy != "" {
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(append(lines, model.busyView()), "\n"),
			actions: []tuiAction{{key: "F1", label: "Help"}},
		})
	}
	definitions := []struct {
		name, label, purpose string
	}{
		{name: "cache", label: "Binary cache signing", purpose: "Lets client computers verify software served by this controller."},
		{name: "ssh", label: "Administrator SSH", purpose: "Lets the controller manage enrolled computers without a password prompt."},
	}
	states := map[string]domain.KeyMaterialState{}
	for _, state := range model.setupKeys.Keys {
		states[state.Name] = state
	}
	for index, definition := range definitions {
		state := states[definition.name]
		status := "Action required"
		if state.Ready() {
			status = "Existing key — ready"
		} else if state.Problem != "" {
			status += " — " + state.Problem
		}
		lines = append(lines, tuiSelection(definition.label+"  ·  "+status, index == model.setupKeyCursor, model.isDark), "    "+definition.purpose)
	}
	if model.setupKeyImporting {
		name := model.selectedSetupKeyName()
		lines = append(lines,
			"",
			tuiSection("Import existing "+setupKeyShortLabel(name)+" private key", model.isDark),
			"Enter the path to a regular private-key file with mode 0600.",
			"Nixorium validates it, derives the public key, and leaves the source unchanged.",
			"Encrypted or hardware-backed SSH keys are not supported for unattended controller operations.",
			"",
			"> "+model.setupKeyPath+"_",
		)
	} else {
		lines = append(lines, "")
		if model.setupKeys.State == "ready" {
			lines = append(lines, tuiStatus("All controller keys are ready", tuiStatusSuccess, model.isDark))
		} else {
			lines = append(lines,
				"Choose a missing key, then import an existing private key; or create all missing keys.",
			)
		}
	}
	var actions []tuiAction
	if model.setupKeyImporting {
		actions = []tuiAction{{key: "Enter", label: "Import"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	} else if model.setupKeys.State == "ready" {
		actions = []tuiAction{{key: "Enter", label: readyLabel}, {key: "Esc", label: backLabel}, {key: "F1", label: "Help"}}
	} else {
		actions = []tuiAction{{key: "↑/↓", label: "Select"}, {key: "i", label: "Import"}, {key: "c", label: "Create missing"}, {key: "Esc", label: backLabel}, {key: "F1", label: "Help"}}
	}
	shell := tuiShell{
		path:    path,
		body:    strings.Join(lines, "\n"),
		actions: actions,
	}
	if model.message != "" {
		shell.notices = []tuiNotice{{kind: tuiStatusNeutral, title: model.message}}
	}
	return model.renderShell(shell)
}

func (model dashboardModel) selectedSetupKey() (domain.KeyMaterialState, bool) {
	name := model.selectedSetupKeyName()
	for _, state := range model.setupKeys.Keys {
		if state.Name == name {
			return state, true
		}
	}
	return domain.KeyMaterialState{Name: name}, name != ""
}

func (model dashboardModel) selectedSetupKeyName() string {
	names := []string{"cache", "ssh"}
	if model.setupKeyCursor < 0 || model.setupKeyCursor >= len(names) {
		return ""
	}
	return names[model.setupKeyCursor]
}

func setupKeyShortLabel(name string) string {
	switch name {
	case "cache":
		return "cache-signing"
	case "ssh":
		return "administrator SSH"
	default:
		return "controller"
	}
}

func (model dashboardModel) setupKeyActionsAvailable() bool {
	return model.actions.ReconcileSetupKeys != nil && model.actions.SaveSetupConfiguration != nil && model.actions.InstallSetupSecrets != nil
}

func (model dashboardModel) prepareSetupKeys() tea.Cmd {
	return func() tea.Msg {
		keys, err := model.actions.ReconcileSetupKeys()
		if err != nil || keys.State != "ready" {
			return dashboardSetupKeysMsg{keys: keys, keyErr: err}
		}
		save := model.actions.SaveSetupConfiguration()
		if save.HasErrors() {
			return dashboardSetupKeysMsg{keys: keys, save: save}
		}
		return dashboardSetupKeysMsg{keys: keys, save: save, install: model.actions.InstallSetupSecrets()}
	}
}

func firstValidationIssue(issues []domain.ValidationIssue) string {
	if len(issues) == 0 {
		return "Technical detail unavailable."
	}
	return issues[0].Message
}

type setupJourneyGroup struct {
	title   string
	pending string
	ids     []string
	state   domain.SetupStageState
}

func setupJourney(report domain.SetupReport) ([]setupJourneyGroup, int) {
	groups := []setupJourneyGroup{
		{title: "Laboratory settings", pending: "To configure", ids: []string{domain.SetupStageInspectEnvironment, domain.SetupStageNetwork, domain.SetupStageIdentity, domain.SetupStageCredentials, domain.SetupStageKeys, domain.SetupStageValidate, domain.SetupStageReview}},
		{title: "Controller", pending: "To configure", ids: []string{domain.SetupStageApply}},
		{title: "Client system", pending: "To prepare", ids: []string{domain.SetupStageArtifacts}},
	}
	states := map[string]domain.SetupStageState{}
	for _, stage := range report.Stages {
		states[stage.ID] = stage.State
	}
	current := 0
	for index := range groups {
		group := &groups[index]
		group.state = domain.SetupStageComplete
		if len(group.ids) == 0 {
			group.state = domain.SetupStagePending
			continue
		}
		for _, id := range group.ids {
			state, found := states[id]
			if !found && report.State != "ready" {
				group.state = domain.SetupStagePending
			}
			if state == domain.SetupStagePending {
				group.state = domain.SetupStagePending
			}
			if state == domain.SetupStageCurrent {
				group.state = domain.SetupStageCurrent
				current = index
				break
			}
		}
	}
	if report.State == "ready" {
		current = len(groups) - 1
	}
	return groups, current
}

func setupCurrentTitle(report domain.SetupReport) string {
	for _, stage := range report.Stages {
		if stage.State == domain.SetupStageCurrent {
			return stage.Title
		}
	}
	return "Setup is complete"
}

func setupCurrentAction(report domain.SetupReport) string {
	switch report.CurrentStage {
	case domain.SetupStageReview:
		return "Save the generated configuration locally"
	case domain.SetupStageApply:
		return "Review and activate the controller configuration"
	case domain.SetupStageArtifacts:
		return "Prepare installation files and client systems"
	default:
		return "Continue the guided laboratory configuration"
	}
}
