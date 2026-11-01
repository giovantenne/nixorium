package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// handleSettingsMessage handles the settings message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleSettingsMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case dashboardInventoryMsg:
		return model.finishInventory(message)
	case dashboardSettingsMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Settings could not be loaded: " + message.err.Error()
			if model.installation.flow {
				model.installation.flow = false
				model.installation.failed = false
			}
			if model.settings.returnScreen == dashboardSetup {
				model.screen = dashboardSetup
			} else {
				model.screen = dashboardHome
			}
			return model, nil
		}
		model.settings.current = message.settings
		model.settings.repair = message.repair
		model.settings.addresses = message.addresses
		model.settings.menu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
		model.settings.fromAccounts = false
		if model.installation.startingLabSetup {
			model.installation.startingLabSetup = false
			// Both installation methods reuse complete saved settings; the
			// installation preflight still runs after Continue. Guided first
			// setup (no method yet) always shows the form.
			method := model.installation.method
			if (method == "pxe" || method == domain.RemoteInstallUSBSSH) && message.settings.InstallationSettingsComplete() {
				model.installation.savedSummary = true
				model.screen, model.message = dashboardSettings, ""
				return model, nil
			}
			model.settings.current.Lab.DeploymentMode = "laboratory"
			if model.settings.current.Lab.PCCount == 0 {
				model.settings.current.Lab.PCCount = 20
			}
			model.settings.collectPasswords = !domain.CredentialsFromSettings(model.settings.current).Ready()
			model.settings.editor = model.newSettingsEditor(model.settings.current, installationSettingsFields, "Installation / Laboratory settings")
			model.message = ""
			model.screen = dashboardSettingsEdit
			return model, nil
		}
		if model.settings.returnScreen == dashboardSetup {
			model.settings.editor = model.newSettingsEditor(model.settings.current, settingsFields, "First setup / Laboratory settings")
			model.message = ""
			model.screen = dashboardSettingsEdit
			return model, nil
		}
		model.message = ""
		model.screen = dashboardSettings
		return model, nil
	case dashboardSettingsPlanMsg:
		model.busy = ""
		model.settings.editor = settingsWizardModel{}
		model.settings.plan = message.report
		model.settings.saveAnyway = false
		failure := ""
		if message.report.HasErrors() {
			failure = "The proposed settings did not pass validation: " + settingsIssueMessage(message.report.Issues)
		} else if refusal := message.report.NetworkPreflight.SaveRefusal(); model.installation.flow && refusal != "" {
			// Installation saves without a separate review, so a failed
			// network check returns to the form instead.
			failure = "The network check refused these settings: " + refusal
		}
		if failure != "" {
			model.message = failure
			if model.settings.returnScreen == dashboardSetup || model.installation.flow {
				title := "First setup / Laboratory settings"
				if model.installation.flow {
					title = "Installation / Laboratory settings"
				}
				fields := settingsFields
				if model.installation.flow {
					fields = installationSettingsFields
				}
				model.settings.editor = model.newSettingsEditor(model.settings.candidate, fields, title)
				model.settings.editor.index = len(model.settings.editor.fields) - 1
				model.settings.editor.accepted = false
				model.settings.editor.err = model.message
				model.settings.editor.prepareCurrentField()
				model.screen = dashboardSettingsEdit
			} else {
				model.screen = dashboardSettings
			}
			return model, nil
		}
		if model.installation.flow {
			if model.actions.SaveSettings == nil {
				return model.failComputerInstallation("Laboratory settings cannot be saved in this session.")
			}
			model.installation.stage = 1
			model.busy = "Saving the validated laboratory settings"
			model.settings.applying = true
			model.message = ""
			candidate := model.settings.candidate
			plan := model.settings.plan
			return model, func() tea.Msg {
				return dashboardSettingsApplyMsg{report: model.actions.SaveSettings(candidate, plan)}
			}
		}
		if len(message.report.Changes) == 0 {
			model.message = "No managed settings changed."
			model.screen = dashboardSettings
			return model, nil
		}
		model.message = ""
		model.screen = dashboardSettingsReview
		return model, nil
	case dashboardSettingsPasswordMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Password change failed: " + message.err.Error()
			if (model.settings.returnScreen == dashboardSetup || model.installation.flow) && message.candidate.SchemaVersion != 0 {
				model.settings.candidate = message.candidate
			}
			model.settings.passwordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
			model.screen = dashboardSettingsPasswords
			return model, nil
		}
		model.settings.candidate = message.candidate
		model.settings.collectPasswords = false
		model.busy = "Validating the password change through Nix"
		model.message = ""
		candidate := model.settings.candidate
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardSettingsPlanMsg{report: model.actions.PlanSettings(ctx, candidate)}
		})
	case dashboardSettingsApplyMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.settings.applying = false
		model.settings.result = message.report
		if !message.report.HasErrors() && message.report.State == "saved" {
			model.noteControllerSave(message.report.Revision)
			model.settings.current = model.settings.candidate
			model.message = message.report.Message
		} else if message.report.State == "unchanged" {
			model.message = message.report.Message
		} else {
			model.message = message.report.Message
			if model.message == "" {
				model.message = "Configuration save failed: " + settingsIssueMessage(message.report.Issues)
			}
		}
		if model.installation.flow {
			if message.report.HasErrors() || (message.report.State != "saved" && message.report.State != "unchanged") {
				return model.failComputerInstallation(message.report.Message)
			}
			model.settings.current = model.settings.candidate
			model.settings.returnScreen = dashboardHome
			model.screen = dashboardPXE
			model.busy = "Checking installation prerequisites"
			model.message = ""
			if model.actions.LoadSetup == nil {
				return model.failComputerInstallation("Installation prerequisite checks are not available in this session.")
			}
			return model.loadSetup()
		}
		if model.settings.returnScreen == dashboardSetup && !message.report.HasErrors() && (message.report.State == "saved" || message.report.State == "unchanged") {
			model.settings.returnScreen = dashboardHome
			model.settings.collectPasswords = false
			model.screen = dashboardSetup
			model.busy = "Continuing first setup"
			model.message = ""
			if model.actions.LoadSetup != nil {
				return model.loadSetup()
			}
			model.busy = ""
			return model, nil
		}
		model.screen = dashboardSettings
		return model, nil
	default:
		return model.handleSoftwareMessage(message)
	}
}
