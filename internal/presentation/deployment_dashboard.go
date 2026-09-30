package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type deploymentIntentKind int

const (
	deploymentNoIntent deploymentIntentKind = iota
	deploymentCloseIntent
	deploymentPlanIntent
	deploymentReachableIntent
	deploymentApplyIntent
	deploymentLogsIntent
)

type deploymentIntent struct {
	kind        deploymentIntentKind
	requested   string
	destination dashboardScreen
	message     string
}

func (model deploymentModel) update(screen dashboardScreen, key tea.KeyPressMsg, hosts []domain.HostMeta) (deploymentModel, deploymentIntent) {
	if screen == dashboardDeploy && model.result.Operation != "" {
		switch key.String() {
		case "enter", "esc", "left":
			return model, deploymentIntent{kind: deploymentCloseIntent, destination: dashboardComputersArea}
		case "l":
			return model, deploymentIntent{kind: deploymentLogsIntent}
		case "n":
			if model.result.RecoveryRequired {
				return model, deploymentIntent{}
			}
			model.result = domain.DeploymentExecutionReport{}
			model.progress = domain.DeploymentProgress{}
			model.recent = nil
			return model, deploymentIntent{}
		}
		return model, deploymentIntent{}
	}

	switch screen {
	case dashboardDeploy:
		switch key.String() {
		case "esc", "left":
			destination := dashboardHome
			if model.context != "" {
				destination = dashboardSoftware
				model.context = ""
			}
			return model, deploymentIntent{kind: deploymentCloseIntent, destination: destination}
		case "up", "k":
			model.cursor = max(0, model.cursor-1)
		case "down", "j":
			model.cursor = min(max(0, len(hosts)-1), model.cursor+1)
		case "space":
			if len(hosts) > 0 {
				if model.chosen == nil {
					model.chosen = map[string]bool{}
				}
				name := hosts[model.cursor].Name
				model.chosen[name] = !model.chosen[name]
			}
		case "a":
			model.chosen = toggleAllDeploymentTargets(hosts, model.chosen)
		case "enter":
			requested := selectedDeploymentTargets(hosts, model.chosen)
			if model.context != "" {
				requested = strings.Join(selectedDeploymentTargetNames(hosts, model.chosen), ",")
			}
			if requested == "" {
				return model, deploymentIntent{message: "Select at least one computer before reviewing a deployment."}
			}
			return model, deploymentIntent{kind: deploymentPlanIntent, requested: requested}
		}
	case dashboardDeployReview:
		switch key.String() {
		case "f2":
			if model.plan.ReachableRequested != "" {
				model.confirmation = ""
				return model, deploymentIntent{kind: deploymentReachableIntent}
			}
		case "esc":
			model.confirmation = ""
			return model, deploymentIntent{kind: deploymentCloseIntent, destination: dashboardDeploy, message: "Deployment cancelled; no build or apply was started."}
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != "DEPLOY" {
				model.confirmation = ""
				return model, deploymentIntent{message: "Confirmation did not match; no build or apply was started."}
			}
			model.applying = true
			model.progress = domain.DeploymentProgress{}
			model.recent = nil
			model.started = time.Now()
			model.confirmation = ""
			return model, deploymentIntent{kind: deploymentApplyIntent}
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	}
	return model, deploymentIntent{}
}

func (model dashboardModel) updateDeployment(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.deployment.usbRecovery != nil {
		return model.updateDeploymentUSBRecovery(key)
	}
	if model.screen == dashboardDeploy && model.deployment.result.Operation == "" && !model.deployment.applying {
		switch key.String() {
		case "r":
			return model.checkComputersThen()
		case "n":
			chosen, message := model.selectComputers(model.deployment.chosen, hostNeedsUpdate, "No computer needed an update at the last check.")
			model.deployment.chosen, model.message = chosen, message
			return model, nil
		}
	}
	deployment, intent := model.deployment.update(model.screen, key, model.report.Meta.Clients.Hosts)
	model.deployment = deployment
	if intent.message != "" {
		model.message = intent.message
	}
	switch intent.kind {
	case deploymentReachableIntent:
		if model.actions.PlanReachableDeployment == nil {
			model.message = "Reachable-only planning is not available in this session."
			return model, nil
		}
		// Retain the original plan until the new read succeeds. Cancelling this
		// read returns to that review with its confirmation cleared, never a
		// partly edited selector or the authorization from the larger plan.
		previous := model.deployment.plan
		model.busy = "Creating a fresh review for the reachable computers"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardDeploymentPlanMsg{report: model.actions.PlanReachableDeployment(ctx, previous)}
		})
	case deploymentCloseIntent:
		model.screen = intent.destination
		if intent.message == "" {
			model.message = ""
		}
	case deploymentPlanIntent:
		if model.actions.LoadRemoteInstall != nil {
			// Keep exactly the selected identities if the inventory changes
			// while an unfinished installation is being recovered.
			requested := strings.Join(selectedDeploymentTargetNames(model.report.Meta.Clients.Hosts, model.deployment.chosen), ",")
			return model.checkDeploymentUSB(requested, false)
		}
		return model.planSelectedDeployment(intent.requested)
	case deploymentApplyIntent:
		if model.actions.ApplyDeployment == nil {
			model.deployment.applying = false
			model.message = "Deployment is not available in this deployment."
			return model, nil
		}
		model.busy = "Building and applying the reviewed deployment"
		model.message = ""
		plan := model.deployment.plan
		events := make(chan tea.Msg)
		model.deployment.events = events
		ctx, cancel := context.WithCancel(context.Background())
		model.deployment.cancel = cancel
		model.deployment.stopReview, model.deployment.stopRequested = false, false
		return model, startDeployment(ctx, model.actions.ApplyDeployment, plan, events)
	case deploymentLogsIntent:
		if model.actions.LoadLogs == nil {
			model.message = "Operation logs are not available in this deployment."
			return model, nil
		}
		model.busy = "Loading operation logs"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg { return dashboardLogsMsg{report: model.actions.LoadLogs(ctx)} })
	}
	return model, nil
}

func deploymentAvailabilityLabel(plan domain.DeploymentPlanReport, name, ip string) string {
	for _, observed := range plan.Availability {
		if observed.Name != name || observed.IP != ip {
			continue
		}
		switch {
		case observed.Reachability == domain.ReachabilityUnreachable:
			return "Not reached at last check"
		case observed.Reachability == domain.ReachabilityReachable && observed.SSH == domain.SSHAvailable:
			return "Reachable at last check"
		case observed.Reachability == domain.ReachabilityReachable:
			return "Reachable; SSH port unavailable"
		default:
			return "Availability unknown"
		}
	}
	return "Not checked; review to probe"
}

// deploymentSelectionLabel prefers the availability probed by the last review
// and otherwise shows the last computer observation of this session.
func deploymentSelectionLabel(plan domain.DeploymentPlanReport, host domain.HostMeta, observed domain.HostStatus, found bool) string {
	for _, availability := range plan.Availability {
		if availability.Name == host.Name && availability.IP == host.IP {
			return deploymentAvailabilityLabel(plan, host.Name, host.IP)
		}
	}
	return deploymentStateLabel(observed, found)
}
