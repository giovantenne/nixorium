package presentation

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// handleStatusMessage handles the status message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleStatusMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case managedJobsTickMsg:
		if message.id != model.jobs.id {
			return model, nil
		}
		return model, model.loadManagedJobs(message.id)
	case managedJobsMsg:
		if message.id != model.jobs.id {
			return model, nil
		}
		model.jobs.items, model.jobs.err = message.jobs, message.err
		return model, model.scheduleManagedJobsTick()
	case activityResultMsg:
		return model.finishRead(message)
	case dashboardInventoryMsg:
		return model.finishInventory(message)
	case dashboardStatusMsg:
		if message.err == nil {
			model.report = message.report
		} else {
			model.computers.hosts = domain.HostsReport{}
			model.message = "Status refresh failed. Check the laboratory again."
		}
		return model, nil
	case overviewQuietRefreshMsg:
		return model.finishQuietOverviewRefresh(message)
	case overviewRefreshMsg:
		model.busy = ""
		if message.err != nil {
			model.message = "Controller information could not be refreshed. The previous results are still shown."
			return model, nil
		}
		message.report.PXEPreparation = model.report.PXEPreparation
		message.report.Meta = model.report.Meta
		model.report, model.setup = message.report, message.setup
		model.usbReserved = message.usbReserved
		model.observeSetupController(message.setup)
		model.observeRecoveryController(message.recovery)
		model.message = "Controller information refreshed. Client computers were not checked."
		return model, nil
	case dashboardDoctorMsg:
		model.busy = ""
		model.diagnostics.report = message.report
		model.diagnostics.cursor = 0
		if message.err != nil {
			model.message = "Checks could not finish: " + message.err.Error()
		} else {
			model.message = ""
		}
		return model, nil
	case spinner.TickMsg:
		var command tea.Cmd
		model.activitySpinner, command = model.activitySpinner.Update(message)
		return model, command
	case internetObserveMsg:
		return model.finishInternetObservation(message)
	case dashboardServicesMsg:
		model.busy = ""
		model.maintenance.services = message.report
		model.message = ""
		model.screen = dashboardServices
		return model, nil
	case dashboardServiceResultMsg:
		model.busy = ""
		model.maintenance.serviceResult = message.report
		model.message = message.report.Message
		model.screen = dashboardServices
		return model, nil
	case dashboardLogsMsg:
		model.busy = ""
		model.maintenance.logs = message.report
		if model.maintenance.logCursor >= len(message.report.Logs) {
			model.maintenance.logCursor = 0
		}
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardLogs
		return model, nil
	case dashboardLogMsg:
		model.busy = ""
		model.maintenance.logDetail = message.report
		model.maintenance.logScroll = maximumLogScroll(message.report, model.logDetailHeight())
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardLogDetail
		return model, nil
	default:
		return model.handleSettingsMessage(message)
	}
}
