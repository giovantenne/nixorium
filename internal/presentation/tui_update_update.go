package presentation

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// handleUpdateMessage handles the update message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleUpdateMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case updateNotificationTickMsg:
		return model, model.checkUpdateNotification()
	case updateNotificationMsg:
		return model.finishUpdateNotification(message)
	case updateNotificationDismissedMsg:
		if message.key == model.updateNotification.Key {
			if message.err == nil {
				model.updateNotification = domain.UpdateNotification{}
			} else {
				model.message = "Could not remember dismissal. Try again with x."
			}
		}
		return model, nil

	case deferredUpdatesMsg:
		model.busy = ""
		model.deferred = deferredModel{status: message.status, loaded: true}
		return model, nil
	case dashboardUpdateCheckMsg:
		model.busy = ""
		model.updates.check = message.report
		model.updates.cursor = 0
		hasStable := false
		stableIndex := 0
		for _, release := range message.report.Development {
			if !release.Downgrade {
				stableIndex++
			}
		}
		for _, release := range message.report.Stable {
			if !release.Downgrade {
				model.updates.cursor = stableIndex
				hasStable = true
				break
			}
		}
		if !hasStable {
			model.updates.prerelease = message.report.CurrentChannel == domain.UpdateChannelPrerelease
			for index, release := range model.availableUpdateReleases() {
				if release.Tag == message.report.CurrentRef {
					model.updates.cursor = index
					break
				}
			}
		}
		model.updates.target = ""
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
		} else {
			model.message = ""
		}
		model.screen = dashboardUpdate
		return model, nil
	case dashboardPackageBaseMsg:
		model.busy = ""
		model.updates.baseStatus = message.report
		model.updates.baseTarget = message.report.Channel
		model.message = operationLogIssues(message.report.Issues)
		return model, nil
	case dashboardUpdatePlanProgressMsg:
		if !model.updates.planning || model.updates.planEvents == nil {
			return model, nil
		}
		model.updates.planProgress = message.progress
		if strings.TrimSpace(message.progress.Detail) != "" {
			model.busy = message.progress.Detail
		}
		return model, waitForActivityEvent(model.read.ctx, model.read.id, model.updates.planEvents)
	case dashboardUpdatePlanMsg:
		model.updates.planning = false
		model.updates.planEvents = nil
		model.busy = ""
		model.updates.plan = message.report
		if !model.updates.packageBase && message.report.Downgrade {
			model.message = "Nixorium downgrades are not available in the TUI. Choose a current or newer release."
			model.screen = dashboardUpdate
			return model, nil
		}
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
			model.screen = dashboardUpdate
			return model, nil
		}
		model.confirmation = ""
		model.updates.scroll = 0
		model.message = ""
		model.screen = dashboardUpdateReview
		return model, nil
	case dashboardUpdateResultMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.updates.applying = false
		model.updates.result = message.report
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardUpdate
		if !message.report.HasErrors() && message.report.Updated {
			model.noteControllerSave(message.report.Revision)
			return model.startUpdateControllerApply()
		}
		return model, nil
	case dashboardUpdateControllerMsg:
		model.busy = ""
		model.controller.applying = false
		model.updates.applying = false
		model.controller.plan = message.plan
		model.controller.result = message.report
		if message.recovery != nil {
			model.observeRecoveryController(*message.recovery)
		}
		model.observeControllerPlan(message.plan)
		model.observeControllerResult(message.report)
		if message.plan.HasErrors() {
			model.message = controllerPlanIssues(message.plan)
		} else {
			model.message = message.report.Message
		}
		model.screen = dashboardUpdate
		if !model.updates.packageBase && !message.plan.HasErrors() && model.updates.result.Updated && !model.updates.result.HasErrors() &&
			!model.updates.result.RecoveryRequired && controllerVerifiedForSave(model.updates.result.Revision, message.report) {
			return model.offerInterfaceReload()
		}
		model.reload = interfaceReloadModel{id: model.reload.id + 1}
		return model, nil
	default:
		return model.handleSetupMessage(message)
	}
}
