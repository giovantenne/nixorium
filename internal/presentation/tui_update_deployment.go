package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// handleDeploymentMessage handles the deployment message family and forwards other messages
// to the next family handler.
func (model dashboardModel) handleDeploymentMessage(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case dashboardConfigurationStateMsg:
		model.busy = ""
		model.computers.configurationState = message.report
		model.computers.hosts = message.report.Clients
		model.computers.hostCursor = 0
		model.message = ""
		model.screen = dashboardHosts
		return model, nil
	case deploymentWaitingSelectionMsg:
		return model.applyWaitingSelection(message.status), nil
	case dashboardDeploymentUSBMsg:
		return model.handleDeploymentUSBMessage(message)
	case dashboardDeploymentPlanMsg:
		model.busy = ""
		model.deployment.plan = message.report
		model.deployment.queueOff = len(deploymentOffComputers(message.report)) > 0
		if message.report.HasErrors() {
			model.message = deploymentPlanIssues(message.report)
			model.screen = dashboardDeploy
			return model, nil
		}
		model.deployment.confirmation = ""
		model.deployment.chosen = map[string]bool{}
		for _, target := range message.report.Targets {
			model.deployment.chosen[target.Name] = true
		}
		model.message = ""
		model.screen = dashboardDeployReview
		return model, nil
	case dashboardDeploymentResultMsg:
		if model.deployment.cancel != nil {
			model.deployment.cancel()
			model.deployment.cancel = nil
		}
		model.deployment.stopReview = false
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.deployment.applying = false
		model.deployment.events = nil
		model.deployment.result = message.report
		model.message = message.report.Message
		model.screen = dashboardDeploy
		if message.report.HasErrors() && message.report.Phase == domain.DeploymentPhasePreflight &&
			!message.report.BuildCompleted && !message.report.ApplyCompleted && model.actions.LoadRemoteInstall != nil {
			names := make([]string, 0, len(message.report.Targets))
			for _, target := range message.report.Targets {
				names = append(names, target.Name)
			}
			if len(names) > 0 {
				return model.checkDeploymentUSB(strings.Join(names, ","), true)
			}
		}
		// An apply that left its pending record goes straight to the review
		// of that record, with the failure shown above the plan.
		if message.report.RecoveryRequired && model.actions.PlanDeploymentRecovery != nil {
			next, command := model.openDeploymentRecovery()
			opened := next.(dashboardModel)
			opened.recovery.review.failure = message.report.Message
			opened.recovery.review.returnTo, opened.recovery.review.returnMessage, opened.recovery.review.returnSet = dashboardDeploy, message.report.Message, true
			return opened, tea.Batch(command, model.observeRecoveryInBackground())
		}
		// Updates queued for switched-off computers appear on the overview now,
		// not only after reopening Nixorium.
		return model, model.observeRecoveryInBackground()
	case dashboardDeploymentProgressMsg:
		if !model.deployment.applying || model.deployment.events == nil {
			return model, nil
		}
		model.deployment.progress = message.progress
		if message.progress.Activity != "" {
			model.deployment.recent = appendBoundedActivity(model.deployment.recent, message.progress.Activity, 5)
		}
		return model, waitForDeploymentEvent(model.deployment.events)
	case dashboardControllerPlanMsg:
		model.busy = ""
		model.controller.plan = message.report
		model.controller.result = domain.ControllerRebuildExecutionReport{}
		model.observeControllerPlan(message.report)
		if model.installation.flow {
			if message.report.HasErrors() {
				return model.failComputerInstallation(controllerPlanIssues(message.report))
			}
			if message.report.Current {
				model.busy = "Checking installation prerequisites"
				return model.loadSetup()
			}
			model.busy = "Building and activating the laboratory controller"
			model.controller.applying = true
			model.controller.progress = domain.OperationProgress{}
			model.controller.started = time.Now().UTC()
			model.controller.progressID++
			plan := message.report
			operation := func() tea.Msg {
				report := model.actions.ApplyController(plan)
				status := domain.StatusReport{}
				var err error
				if model.actions.Refresh != nil {
					ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
					defer cancel()
					status, err = model.actions.Refresh(ctx)
				}
				return dashboardControllerResultMsg{report: report, status: status, statusErr: err}
			}
			return model, tea.Batch(operation, scheduleControllerProgressTick(model.controller.progressID))
		}
		if model.controller.autoApply {
			// Assistant path: a clean, applicable review is applied without
			// another key press. Anything else stops on the ordinary screens.
			if message.report.HasErrors() || !model.controller.fromSave {
				model.controller.autoApply = false
			} else if controllerPlanCurrent(message.report) {
				model.controller.autoApply, model.controller.fromSave = false, false
				model.screen = model.controller.saveOrigin
				return model.openSavedDeployment(true)
			} else if message.report.State == "ready" {
				model.message = ""
				model.screen = dashboardControllerReview
				return model.startReviewedControllerApply()
			} else {
				model.controller.autoApply = false
			}
		}
		if message.report.HasErrors() {
			model.message = controllerPlanIssues(message.report)
			model.screen = dashboardController
			if model.controller.fromSave {
				model.screen = model.controller.saveOrigin
				model.controller.fromSave = false
			}
			return model, nil
		}
		model.confirmation = ""
		model.message = ""
		model.screen = dashboardControllerReview
		return model, nil
	case dashboardControllerResultMsg:
		model.computers.hosts = domain.HostsReport{}
		model.busy = ""
		model.controller.applying = false
		model.controller.result = message.report
		if message.recovery != nil {
			model.observeRecoveryController(*message.recovery)
		}
		model.observeControllerResult(message.report)
		model.message = message.report.Message
		model.controller.details = false
		switch {
		case message.refreshLater:
		case message.statusErr != nil:
			model.message += "; dashboard refresh failed: " + message.statusErr.Error()
		default:
			model.report = message.status
		}
		if model.installation.flow {
			model.screen = dashboardPXE
			if message.report.HasErrors() || !message.report.Applied || !message.report.Verified {
				return model.failComputerInstallation("Controller activation failed: " + strings.TrimPrefix(message.report.Message, "controller build or activation failed: "))
			}
			model.busy = "Checking installation prerequisites"
			if model.actions.LoadControllerProgress != nil {
				next, read := model.loadSetup()
				return next, tea.Batch(model.loadControllerProgress(model.controller.progressID), read)
			}
			return model.loadSetup()
		}
		model.screen = dashboardController
		if model.controller.fromSave {
			if model.controller.autoApply {
				origin := model.controller.saveOrigin
				model.controller.autoApply, model.controller.fromSave = false, false
				model.screen = origin
				if saved, revision := model.saveFollowupState(); saved && controllerVerifiedForSave(revision, message.report) {
					return model.openSavedDeployment(true)
				}
				// Not verified: stay on the controller result with its
				// recovery actions and never continue to the clients.
				model.screen = dashboardController
			} else {
				model.screen = model.controller.saveOrigin
				model.controller.fromSave = false
			}
		}
		model.controller.autoApply = false
		if model.screen == dashboardNetworkChange && model.networkChange.result.Operation == "" {
			// Back to the change in progress: read it again, the controller
			// now carries other addresses.
			message := model.message
			next, read := model.loadNetworkChange()
			updated := next.(dashboardModel)
			updated.message = message
			return updated, read
		}
		model, reload := model.offerControllerInterfaceReload()
		if model.actions.LoadControllerProgress != nil {
			return model, tea.Batch(model.loadControllerProgress(model.controller.progressID), reload)
		}
		return model, reload
	case controllerStatusRefreshMsg:
		// Another operation or read started since: its own result is newer.
		if message.id != model.controller.progressID || model.busy != "" || model.controller.applying ||
			(model.screen != dashboardController && model.screen != dashboardControllerReview) {
			// Other screens (installation, Overview) read their own state.
			return model, nil
		}
		if message.err != nil {
			if model.screen == dashboardController && model.message != "" {
				model.message += "; the dashboard could not be refreshed: " + message.err.Error()
			}
			return model, nil
		}
		model.report = message.status
		return model, nil
	case dashboardControllerProgressTickMsg:
		if !model.controller.applying || message.id != model.controller.progressID || model.actions.LoadControllerProgress == nil {
			return model, nil
		}
		return model, model.loadControllerProgress(message.id)
	case dashboardControllerProgressMsg:
		if message.id != model.controller.progressID {
			return model, nil
		}
		if message.err != nil {
			model.controller.progressFailures++
		} else {
			model.controller.progressFailures = 0
		}
		model.controller.progressUnavailable = model.controller.progressFailures >= controllerProgressMissesShown
		if message.err == nil && (model.controller.started.IsZero() || !message.progress.StartedAt.Before(model.controller.started)) {
			model.controller.progress = message.progress
		}
		if model.controller.applying {
			return model, scheduleControllerProgressTick(message.id)
		}
		return model, nil
	default:
		return model.handleStatusMessage(message)
	}
}
