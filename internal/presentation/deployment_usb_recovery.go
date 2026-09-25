package presentation

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// This view composes the existing worker callbacks. Only the worker may verify
// identity and release a reservation; resuming always creates a new review.
type deploymentUSBRecovery struct {
	requested string
	response  domain.RemoteInstallResponse
	detail    string
	failed    bool
	details   bool
}

type dashboardDeploymentUSBMsg struct {
	id           uint64
	verify       bool
	afterFailure bool
	response     domain.RemoteInstallResponse
	err          error
}

func (recovery deploymentUSBRecovery) canVerify() bool {
	session := recovery.response.Session
	return session != nil && recovery.response.OperationID == session.OperationID &&
		session.RebootRequested && domain.RemoteInstallDiskCompleted(*session)
}

func (recovery deploymentUSBRecovery) host() string {
	if session := recovery.response.Session; session != nil {
		if session.Preparation != nil {
			return sanitizeRemoteInstallText(session.Preparation.Host.Name)
		}
		if session.Artifacts != nil {
			return sanitizeRemoteInstallText(session.Artifacts.HostName)
		}
	}
	return "the computer"
}

func (recovery deploymentUSBRecovery) hasSession() bool {
	return recovery.response.Session != nil && recovery.response.OperationID != "" &&
		recovery.response.OperationID == recovery.response.Session.OperationID
}

func (model dashboardModel) planSelectedDeployment(requested string) (tea.Model, tea.Cmd) {
	model.deployment.usbRecovery = nil
	model.deployment.confirmation = ""
	model.deployment.plan = domain.DeploymentPlanReport{}
	model.deployment.result = domain.DeploymentExecutionReport{}
	model.screen = dashboardDeploy
	if model.actions.PlanDeployment == nil {
		model.busy = ""
		model.message = "Deployment planning is not available in this deployment."
		return model, nil
	}
	model.busy = "Validating revision and selected computers"
	model.message = ""
	return model, func() tea.Msg { return dashboardDeploymentPlanMsg{report: model.actions.PlanDeployment(requested)} }
}

func (model dashboardModel) checkDeploymentUSB(requested string, afterFailure bool) (tea.Model, tea.Cmd) {
	model.screen = dashboardDeploy
	model.deployment.confirmation = ""
	model.deployment.plan = domain.DeploymentPlanReport{}
	model.deployment.usbRecovery = &deploymentUSBRecovery{requested: requested}
	model.message = ""
	model.busy = "Checking for an unfinished USB installation"
	model.deployment.usbRequestID++
	id := model.deployment.usbRequestID
	return model, func() tea.Msg {
		response, err := model.actions.LoadRemoteInstall()
		return dashboardDeploymentUSBMsg{id: id, afterFailure: afterFailure, response: response, err: err}
	}
}

func (model dashboardModel) handleDeploymentUSBMessage(message dashboardDeploymentUSBMsg) (tea.Model, tea.Cmd) {
	if model.deployment.usbRecovery == nil || message.id != model.deployment.usbRequestID || model.screen != dashboardDeploy {
		return model, nil
	}
	model.busy = ""
	recovery := *model.deployment.usbRecovery
	model.deployment.usbRecovery = &recovery
	if message.verify {
		response := message.response
		if message.err == nil && response.State == "verified" && response.OperationID == recovery.response.OperationID &&
			response.Session != nil && response.Session.OperationID == response.OperationID && response.Session.BootVerified {
			// Check the reservation again, including a new operation started by
			// another controller client, before preparing a fresh deployment.
			return model.checkDeploymentUSB(recovery.requested, false)
		}
		// Keep the original, bound session when transport/cleanup errors omit
		// it, so retry cannot lose its identity or target another installation.
		recovery.failed = true
		recovery.detail = sanitizeRemoteInstallText(response.Message)
		if message.err != nil {
			recovery.detail = sanitizeRemoteInstallText(message.err.Error())
		}
		if recovery.detail == "" {
			recovery.detail = "The worker did not confirm verification and cleanup."
		}
		return model, nil
	}
	if message.err == nil && message.response.State == "ready" && message.response.OperationID == "" && message.response.Session == nil {
		if message.afterFailure {
			// A preflight error unrelated to USB must keep its original result.
			model.deployment.usbRecovery = nil
			model.message = model.deployment.result.Message
			return model, nil
		}
		return model.planSelectedDeployment(recovery.requested)
	}
	recovery.response = message.response
	if message.err != nil {
		recovery.response = domain.RemoteInstallResponse{}
		recovery.detail = sanitizeRemoteInstallText(message.err.Error())
	} else if !recovery.hasSession() {
		recovery.detail = "The controller did not return a usable installation record. " + sanitizeRemoteInstallText(message.response.Message)
	}
	return model, nil
}

func (model dashboardModel) updateDeploymentUSBRecovery(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	recovery := *model.deployment.usbRecovery
	model.deployment.usbRecovery = &recovery
	switch key.String() {
	case "esc", "left":
		model.deployment.usbRecovery = nil
		model.deployment.result = domain.DeploymentExecutionReport{}
		model.deployment.confirmation = ""
		model.message = "Selection kept; no deployment was started."
	case "d":
		recovery.details = !recovery.details
	case "r":
		return model.checkDeploymentUSB(recovery.requested, false)
	case "enter":
		if recovery.canVerify() && model.actions.RemoteInstallRequest != nil {
			model.busy = "Verifying the installed system on " + recovery.host()
			model.deployment.usbRequestID++
			id := model.deployment.usbRequestID
			request := domain.RemoteInstallRequest{Operation: domain.RemoteInstallVerifyOperation, OperationID: recovery.response.OperationID}
			return model, func() tea.Msg {
				response, err := model.actions.RemoteInstallRequest(request)
				return dashboardDeploymentUSBMsg{id: id, verify: true, response: response, err: err}
			}
		}
		if !recovery.hasSession() {
			return model.checkDeploymentUSB(recovery.requested, false)
		}
		return model.openDeploymentUSBInstallation()
	case "i":
		if recovery.hasSession() {
			return model.openDeploymentUSBInstallation()
		}
	}
	return model, nil
}

func (model dashboardModel) openDeploymentUSBInstallation() (tea.Model, tea.Cmd) {
	recovery := model.deployment.usbRecovery
	bootstrapError := ""
	if model.installation.remote.operationID == recovery.response.OperationID {
		bootstrapError = model.installation.remote.bootstrapError
	}
	model.installation.remote = remoteInstallationModel{
		stage: remoteInstallResult, operationID: recovery.response.OperationID,
		response: recovery.response, returnToDeployment: true, bootstrapError: bootstrapError,
	}
	model.screen = dashboardUSBInstall
	model.message = "Return to the deployment with Esc after resolving this installation."
	model.busy = "Refreshing the existing USB installation"
	return model.remoteInstallCommand("status", domain.RemoteInstallRequest{
		Operation: domain.RemoteInstallStatusOperation, OperationID: recovery.response.OperationID,
	})
}

func (model dashboardModel) deploymentUSBRecoveryView(shell tuiShell) string {
	recovery := model.deployment.usbRecovery
	shell.path = append(shell.path, "Finish installation")
	lines := []string{tuiTitle("Finish the previous installation", model.isDark), "", "Selected for deployment: " + recovery.requested, ""}
	if model.busy != "" {
		shell.body = strings.Join(append(lines, model.busyView()), "\n")
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	primary := "Retry check"
	notice := tuiNotice{kind: tuiStatusAttention, title: "Installation status could not be checked", detail: "Retry the check. If it still fails, open Details for assistance."}
	if recovery.hasSession() {
		primary = "Open installation"
		notice.title = "An unfinished USB installation is blocking deployment"
		notice.detail = "Open the installation to check its progress and available recovery actions."
		lines = append(lines, "Computer: "+recovery.host())
		if recovery.canVerify() && model.actions.RemoteInstallRequest != nil {
			primary = "Verify " + recovery.host() + " and resume"
			lines = append(lines, "The system is installed; its final check is pending.")
			notice.title = "Ready to check the installed computer"
			notice.detail = "Turn it on, boot from the installed disk and check the network cable."
			if recovery.failed {
				primary = "Retry verification"
				notice.title = "Verification did not complete"
				notice.detail += " If retry fails, open Details for assistance."
			}
		}
	}
	lines = append(lines, "", "After recovery, review and confirm the deployment again.")
	if recovery.details {
		lines = append(lines, "", tuiSection("Technical details", model.isDark),
			fmt.Sprintf("Operation: %s", sanitizeRemoteInstallText(recovery.response.OperationID)),
			"State: "+sanitizeRemoteInstallText(recovery.response.State),
			sanitizeRemoteInstallText(recovery.response.Message), recovery.detail)
	}
	shell.body = strings.Join(lines, "\n")
	shell.notices = []tuiNotice{notice}
	shell.actions = []tuiAction{{key: "Enter", label: primary}, {key: "Esc", label: "Selection"}, {key: "d", label: "Details"}, {key: "r", label: "Refresh"}}
	if recovery.hasSession() {
		shell.actions = append(shell.actions, tuiAction{key: "i", label: "Installation"})
	}
	shell.actions = append(shell.actions, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(shell)
}
