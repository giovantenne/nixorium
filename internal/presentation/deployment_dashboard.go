package presentation

import (
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
	deploymentApplyIntent
	deploymentLogsIntent
)

type deploymentIntent struct {
	kind        deploymentIntentKind
	requested   string
	destination dashboardScreen
	message     string
}

func (model deploymentModel) update(screen dashboardScreen, key tea.KeyPressMsg, hosts []domain.HostMeta, restoreMode bool) (deploymentModel, deploymentIntent) {
	if screen == dashboardDeploy && model.result.Operation != "" {
		switch key.String() {
		case "enter", "esc", "left":
			return model, deploymentIntent{kind: deploymentCloseIntent, destination: dashboardComputersArea}
		case "l":
			return model, deploymentIntent{kind: deploymentLogsIntent}
		case "r":
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
			if restoreMode {
				destination = dashboardRestore
			} else if model.context != "" {
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
	deployment, intent := model.deployment.update(model.screen, key, model.report.Meta.Clients.Hosts, model.restoreMode)
	model.deployment = deployment
	if intent.message != "" {
		model.message = intent.message
	}
	switch intent.kind {
	case deploymentCloseIntent:
		model.screen = intent.destination
		if intent.message == "" {
			model.message = ""
		}
		if intent.destination == dashboardRestore {
			model.restoreMode = false
		}
	case deploymentPlanIntent:
		if model.actions.PlanDeployment == nil {
			model.message = "Deployment planning is not available in this deployment."
			return model, nil
		}
		model.busy = "Validating revision and selected computers"
		model.message = ""
		requested := intent.requested
		return model, func() tea.Msg { return dashboardDeploymentPlanMsg{report: model.actions.PlanDeployment(requested)} }
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
		return model, startDeployment(model.actions.ApplyDeployment, plan, events)
	case deploymentLogsIntent:
		if model.actions.LoadLogs == nil {
			model.message = "Operation logs are not available in this deployment."
			return model, nil
		}
		model.busy = "Loading operation logs"
		model.message = ""
		return model, func() tea.Msg { return dashboardLogsMsg{report: model.actions.LoadLogs()} }
	}
	return model, nil
}
