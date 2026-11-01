package presentation

import (
	tea "charm.land/bubbletea/v2"
)

// handleWindowMessage handles the window message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleWindowMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.height = message.Height
		if model.assistant.initialized {
			model.assistant.input.SetWidth(max(20, model.width-6))
		}
		model.ensureHomeMenu()
		model.homeMenu.setSize(model.width, model.height)
		if model.screen == dashboardLogDetail && model.maintenance.logScroll > maximumLogScroll(model.maintenance.logDetail, model.logDetailHeight()) {
			model.maintenance.logScroll = maximumLogScroll(model.maintenance.logDetail, model.logDetailHeight())
		}
		if model.screen == dashboardUpdateReview && model.updates.scroll > model.maximumUpdateScroll() {
			model.updates.scroll = model.maximumUpdateScroll()
		}
		if model.screen == dashboardSettings {
			model.settings.menu.setSize(model.width, model.height)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settings.passwordMenu.setSize(model.width, model.height)
		}
		if model.screen == dashboardSettingsAccounts {
			model.settings.accountsMenu.setSize(model.width, model.height)
		}
		if model.screen == dashboardSettingsEdit {
			updated, _ := model.settings.editor.Update(message)
			model.settings.editor = updated.(settingsWizardModel)
		}
		return model, nil
	case tea.BackgroundColorMsg:
		model.isDark = message.IsDark()
		model.activitySpinner.Style = newTUISpinner(model.isDark).Style
		selected := model.homeMenu.list.Index()
		model.homeMenu = newDashboardTaskMenu(model.homeAreas(), model.isDark, model.width, model.height)
		model.homeMenu.list.Select(selected)
		if model.screen == dashboardSettings {
			model.settings.menu = newRoutineSettingsMenu(model.isDark, model.width, model.height)
		}
		if model.screen == dashboardSettingsPasswords {
			model.settings.passwordMenu = newRoutinePasswordMenu(model.isDark, model.width, model.height)
		}
		if model.screen == dashboardSettingsAccounts {
			selected := model.settings.accountsMenu.list.Index()
			model.settings.accountsMenu = newRoutineAccountsMenu(model.isDark, model.width, model.height)
			model.settings.accountsMenu.list.Select(selected)
		}
		if model.screen == dashboardSettingsEdit {
			updated, _ := model.settings.editor.Update(message)
			model.settings.editor = updated.(settingsWizardModel)
		}
		return model, nil
	default:
		return model.updateKeyState(message)
	}
}
