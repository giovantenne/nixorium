package presentation

import (
	tea "charm.land/bubbletea/v2"
)

// handleOperationsMessage handles the operations message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleOperationsMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case cleanupPlanMsg:
		model.busy = ""
		model.cleanup.plan = message.plan
		model.cleanup.stage = 1
		model.cleanup.confirmation = ""
		model.message = ""
		return model, nil
	case cleanupApplyMsg:
		model.busy = ""
		model.cleanup.result = message.report
		model.cleanup.stage = 2
		model.message = ""
		return model, nil
	case internetPlanMsg:
		model.busy = ""
		model.internet.plan = message.plan
		model.internet.stage = 1
		model.message = ""
		return model, nil
	case sharePlanMsg:
		model.busy = ""
		model.share.plan = message.plan
		model.share.stage = shareStageReview
		model.message = ""
		return model, nil
	case shareApplyMsg:
		model.busy = ""
		model.share.result = message.report
		model.share.stage = shareStageResult
		model.message = ""
		return model, nil
	case lockPlanMsg:
		model.busy = ""
		model.lock.plan = message.plan
		model.lock.stage = 1
		model.message = ""
		return model, nil
	case lockApplyMsg:
		model.busy = ""
		model.lock.result = message.report
		model.lock.stage = 2
		model.message = ""
		return model, nil
	case internetApplyMsg:
		model.busy = ""
		model.internet.applying = false
		model.internet.result = message.report
		model.internet.stage = 2
		model.message = ""
		return model, nil
	case dashboardShutdownPlanMsg:
		model.busy = ""
		model.shutdown.plan = message.report
		model.message = message.report.Message
		if len(message.report.Targets) == 0 {
			model.screen = dashboardShutdown
			return model, nil
		}
		model.shutdown.confirmation = ""
		model.screen = dashboardShutdownReview
		return model, nil
	case dashboardShutdownApplyMsg:
		model.busy = ""
		model.shutdown.applying = false
		model.shutdown.result = message.report
		model.shutdown.technical = false
		model.message = message.report.Message
		model.screen = dashboardShutdownResult
		return model, nil
	case dashboardWakePlanMsg:
		model.busy = ""
		model.shutdown.wakePlan = message.plan
		model.message = message.plan.Message
		if len(message.plan.Targets) == 0 {
			model.screen = dashboardShutdown
			return model, nil
		}
		model.screen = dashboardShutdownReview
		return model, nil
	case dashboardWakeApplyMsg:
		model.busy = ""
		model.shutdown.applying = false
		model.shutdown.wakeResult = message.report
		model.message = ""
		model.screen = dashboardShutdownResult
		return model, nil
	default:
		return model.handleWindowMessage(message)
	}
}
