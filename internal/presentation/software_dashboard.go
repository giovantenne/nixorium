package presentation

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) updateSoftware(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if model.software.stage == softwareResult && key.String() == "enter" {
		if model.software.canDistribute(model.controller.result) {
			return model.openSoftwareDeployment()
		}
		if model.software.result.State == "saved" && !model.software.result.HasErrors() && !model.software.result.RecoveryRequired && model.software.result.AffectedController != "" && !controllerVerifiedForSave(model.software.result.Revision, model.controller.result) {
			return model.startSoftwareControllerApply()
		}
	}
	software, intent := model.software.update(key)
	model.software = software
	if intent.setMessage {
		model.message = intent.message
	}
	switch intent.kind {
	case softwareCloseIntent:
		model.screen = dashboardHome
	case softwarePlanIntent:
		return model.startSoftwarePlan(intent.request)
	case softwareSaveIntent:
		if model.actions.SaveSoftware == nil {
			model.message = "Software saving is not available in this deployment."
			return model, nil
		}
		if model.software.result.RecoveryRequired {
			model.busy = "Recovering the local software save"
		} else {
			model.busy = "Saving the software choices you reviewed"
		}
		model.software.applying = true
		plan := model.software.plan
		return model, func() tea.Msg { return dashboardSoftwareApplyMsg{report: model.actions.SaveSoftware(plan)} }
	case softwareControllerIntent:
		if model.software.result.State != "saved" || model.software.result.HasErrors() || model.software.result.RecoveryRequired {
			return model, nil
		}
		if model.actions.PlanController == nil || model.actions.ApplyController == nil {
			model.message = "Controller activation is not available in this deployment. The software selection remains saved."
			return model, nil
		}
		return model.startSoftwareControllerApply()
	case softwareDeployIntent:
		if !model.software.canDistribute(model.controller.result) {
			return model, nil
		}
		return model.openSoftwareDeployment()
	case softwareStateIntent:
		return model.openConfigurationState()
	case softwareProfilesIntent:
		if model.actions.LoadSoftwarePresets == nil {
			model.message = "Software profiles are not available in this deployment. Individual software management remains available."
			return model, nil
		}
		model.busy = "Loading deployment software profiles"
		model.message = ""
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardSoftwarePresetCatalogMsg{report: model.actions.LoadSoftwarePresets(ctx)}
		})
	case softwarePresetPlanIntent:
		if model.actions.PlanSoftwarePreset == nil {
			model.message = "Software profile planning is not available in this deployment."
			return model, nil
		}
		model.busy = "Checking every profile package and its destination"
		model.message = ""
		request := intent.presetRequest
		return model.startRead(func(ctx context.Context) tea.Msg {
			return dashboardSoftwarePresetPlanMsg{report: model.actions.PlanSoftwarePreset(ctx, request)}
		})
	case softwarePresetSaveIntent:
		if model.actions.SaveSoftwarePreset == nil {
			model.message = "Software profile saving is not available in this deployment."
			return model, nil
		}
		if model.software.profileResult.RecoveryRequired {
			model.busy = "Recovering the local software profile save"
		} else {
			model.busy = "Saving the reviewed software profile"
		}
		model.software.applying = true
		plan := model.software.profilePlan
		return model, func() tea.Msg { return dashboardSoftwarePresetApplyMsg{report: model.actions.SaveSoftwarePreset(plan)} }
	}
	return model, nil
}

func (model dashboardModel) startSoftwareControllerApply() (tea.Model, tea.Cmd) {
	if model.actions.PlanController == nil || model.actions.ApplyController == nil {
		model.message = "Controller activation is not available in this deployment. The software selection remains saved."
		return model, nil
	}
	model.busy = "Building and activating the reviewed software on this controller"
	model.software.applying = true
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.controller.result = domain.ControllerRebuildExecutionReport{}
	operation := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), dashboardReadTimeout)
		defer cancel()
		plan := model.actions.PlanController(ctx)
		if plan.HasErrors() {
			return dashboardSoftwareControllerMsg{plan: plan}
		}
		return dashboardSoftwareControllerMsg{plan: plan, report: model.actions.ApplyController(plan)}
	}
	command := model.trackControllerApply(operation)
	return model, command
}

func (model dashboardModel) startSoftwarePlan(request domain.SoftwareChangeRequest) (tea.Model, tea.Cmd) {
	if model.actions.PlanSoftware == nil {
		model.message = "Software planning is not available in this deployment."
		return model, nil
	}
	model.busy = "Checking the package and its destination"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardSoftwarePlanMsg{report: model.actions.PlanSoftware(ctx, request)}
	})
}

func (model dashboardModel) openSoftwareDeployment() (tea.Model, tea.Cmd) {
	if model.report.Meta.Controller.Name == "" {
		return model.loadInventoryThen(func(ready dashboardModel) (tea.Model, tea.Cmd) {
			return ready.openSoftwareDeployment()
		})
	}
	configured := map[string]bool{}
	for _, host := range model.report.Meta.Clients.Hosts {
		configured[host.Name] = true
	}
	chosen := map[string]bool{}
	missing := []string{}
	for _, name := range model.software.result.AffectedClients {
		if configured[name] {
			chosen[name] = true
		} else {
			missing = append(missing, name)
		}
	}
	model.screen = dashboardDeploy
	model.deployment.result = domain.DeploymentExecutionReport{}
	model.deployment.plan = domain.DeploymentPlanReport{}
	model.deployment.progress = domain.DeploymentProgress{}
	model.deployment.recent = nil
	model.deployment.chosen = chosen
	model.deployment.cursor = 0
	model.deployment.context = "Opened from a saved software change. The review deploys the complete current system configuration, not only that package."
	model.deployment.origin = dashboardSoftware
	model.message = ""
	if len(missing) > 0 {
		model.message = "Affected computers no longer in the current inventory were not selected: " + strings.Join(missing, ", ") + ". Review the remaining selection."
	}
	if len(chosen) == 0 {
		model.message = "None of the affected computers are in the current inventory. Return to Software or refresh the laboratory configuration before continuing."
	}
	return model, nil
}

func (model dashboardModel) openConfigurationState() (tea.Model, tea.Cmd) {
	if model.actions.LoadConfigurationState == nil {
		model.message = "Current system state is not available in this deployment."
		return model, nil
	}
	model.screen = dashboardHosts
	model.computers.configurationState = domain.ConfigurationStateReport{}
	model.computers.hosts = domain.HostsReport{}
	model.computers.hostCursor = 0
	model.computers.hostQuery = ""
	model.computers.hostSearching = false
	model.computers.hostDetail = false
	model.computers.hostTechnical = false
	model.busy = "Checking desired and observed system state"
	model.message = ""
	return model.loadConfigurationState()
}

func (model dashboardModel) softwareView() string {
	if model.controller.applying {
		return model.controllerProgressView([]string{"Software", "Controller"})
	}
	context := softwareViewContext{
		width:            model.width,
		height:           model.height,
		dark:             model.isDark,
		message:          model.message,
		busy:             model.busy,
		controllerResult: model.controller.result,
	}
	if model.busy != "" {
		context.busyView = model.busyView()
	}
	return model.renderShell(model.software.view(context))
}
