package presentation

import (
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// handleBackupMessage handles the backup message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleBackupMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case recoveryPlanMsg:
		model.busy = ""
		if model.recovery.review.kind == "deploy" {
			model.recovery.review.deployPlan = message.deploy
			model.recovery.review.usbHost = message.usbHost
		} else {
			model.recovery.review.resetPlan = message.reset
		}
		return model, nil
	case gitBackupPlanMsg:
		model.busy = ""
		if message.err != nil {
			model.message = message.err.Error()
			return model, nil
		}
		model.backup.plan = message.plan
		model.backup.reviewed = true
		model.backup.field = 0
		return model, nil
	case backupResultMsg:
		model.busy = ""
		model.backup.result, model.backup.done = message.report, true
		return model, nil
	case backupKeysMsg:
		model.busy = ""
		if message.report.HasErrors() {
			model.message = "The keys could not be installed: " + message.report.Message + ". Try again, or run nixorium setup install-secrets."
			return model, nil
		}
		model.backup.adoptResult.State = "completed"
		model.backup.adoptResult.Issues = nil
		model.backup.adoptResult.Message = "This controller now manages the restored lab and has its keys."
		return model, nil
	case backupAdoptMsg:
		model.busy = ""
		model.backup.adopted, model.backup.adoptResult = true, message.report
		if message.report.State == "completed" || message.report.State == "partial" {
			// Everything observed so far described the previous lab.
			model.report = domain.StatusReport{}
		}
		return model, nil
	case backupSyncMsg:
		return model.finishBackupSync(message)
	case recoveryResultMsg:
		model.busy = ""
		model.recovery.review.done = true
		model.recovery.review.deployResult, model.recovery.review.resetResult = message.deploy, message.reset
		return model, nil
	case recoveryMsg:
		model.busy = ""
		model.observeRecoveryController(message.report)
		model.recovery.cursor = min(model.recovery.cursor, max(0, len(message.report.Conditions)-1))
		return model, nil
	case recoveryObservedMsg:
		model.observeRecoveryController(message.report)
		return model, nil
	case dashboardGitReviewMsg:
		model.busy = ""
		model.maintenance.gitReview = message.report
		model.maintenance.gitScroll = 0
		model.message = operationLogIssues(message.report.Issues)
		model.screen = dashboardGitReview
		return model, nil
	case dashboardGitCommitPlanMsg:
		model.busy = ""
		model.maintenance.gitCommitPlan = message.report
		if message.report.HasErrors() {
			model.message = operationLogIssues(message.report.Issues)
			model.screen = dashboardGitCommitSelect
			return model, nil
		}
		model.confirmation = ""
		model.maintenance.gitScroll = 0
		model.message = ""
		model.screen = dashboardGitCommitReview
		return model, nil
	case dashboardGitUndoPlanMsg:
		model.busy = ""
		model.maintenance.gitUndoPlan = message.report
		model.confirmation = ""
		model.maintenance.gitScroll = 0
		model.message = ""
		model.screen = dashboardGitUndoReview
		return model, nil
	case dashboardGitUndoResultMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		if !message.report.HasErrors() && message.report.Committed {
			model.noteControllerSave(message.report.Revision)
		}
		model.maintenance.gitUndoResult = message.report
		model.maintenance.gitCommitResult = domain.GitCommitReport{}
		model.maintenance.gitReview = message.review
		model.maintenance.gitScroll = 0
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardGitReview
		return model, nil
	case dashboardGitCommitResultMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		if !message.report.HasErrors() && message.report.Committed {
			model.noteControllerSave(message.report.Revision)
		}
		model.maintenance.gitCommitResult = message.report
		model.maintenance.gitUndoResult = domain.GitUndoReport{}
		model.maintenance.gitReview = message.review
		model.maintenance.gitScroll = 0
		model.confirmation = ""
		model.message = message.report.Message
		model.screen = dashboardGitReview
		return model, nil
	default:
		return model.handleWorkspaceMessage(message)
	}
}
