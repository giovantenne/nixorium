package presentation

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Maintenance → Change network addressing guides the change of laboratory
// network. It shows the change in progress (or a form for a new one), reviews
// each step in the recovery-review style (what was found, what happens,
// confirm), saves it with Enter, and continues with the ordinary controller
// and computer reviews, which are confirmed with Enter as well.

var networkChangeTask = dashboardTask{id: "network", shortcut: "n", title: "Change network addressing", description: "Move the computers and the controller to a new laboratory network, step by step", advanced: true}

type networkChangeModel struct {
	status      domain.NetworkMigrationStatus
	loaded      bool
	input       string
	plan        domain.NetworkMigrationPlan
	reviewing   bool
	acknowledge bool
	result      domain.NetworkMigrationResult
}

type networkChangeStatusMsg struct{ status domain.NetworkMigrationStatus }
type networkChangePlanMsg struct{ plan domain.NetworkMigrationPlan }
type networkChangeResultMsg struct{ result domain.NetworkMigrationResult }

func (model dashboardModel) openNetworkChange() (tea.Model, tea.Cmd) {
	model.screen = dashboardNetworkChange
	model.message = ""
	model.networkChange = networkChangeModel{}
	return model.loadNetworkChange()
}

func (model dashboardModel) loadNetworkChange() (tea.Model, tea.Cmd) {
	if model.actions.LoadNetworkMigration == nil {
		model.message = "The change of laboratory network is not available in this session."
		return model, nil
	}
	model.busy = "Checking the laboratory network and every computer"
	model.networkChange.reviewing = false
	model.networkChange.result = domain.NetworkMigrationResult{}
	load := model.actions.LoadNetworkMigration
	return model.startRead(func(ctx context.Context) tea.Msg { return networkChangeStatusMsg{status: load(ctx)} })
}

func (model dashboardModel) planNetworkChange(action string) (tea.Model, tea.Cmd) {
	if model.actions.PlanNetworkMigration == nil {
		model.message = "The change of laboratory network is not available in this session."
		return model, nil
	}
	if action == domain.NetworkMigrationStart {
		if _, _, err := domain.ParseLabNetwork(model.networkChange.input); err != nil {
			model.message = upperFirstText(err.Error()) + "."
			return model, nil
		}
	}
	model.message = ""
	model.networkChange.reviewing = true
	model.networkChange.plan = domain.NetworkMigrationPlan{Action: action}
	model.busy = "Reviewing the change of laboratory network: checking every computer and the new settings"
	plan, network, acknowledge := model.actions.PlanNetworkMigration, model.networkChange.input, model.networkChange.acknowledge
	return model.startBoundedRead(dashboardBuildTimeout, func(ctx context.Context) tea.Msg {
		return networkChangePlanMsg{plan: plan(ctx, action, network, acknowledge)}
	})
}

func upperFirstText(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

func (model dashboardModel) handleNetworkChangeMessage(message tea.Msg) (dashboardModel, tea.Cmd, bool) {
	switch message := message.(type) {
	case networkChangeStatusMsg:
		model.busy = ""
		model.networkChange.status, model.networkChange.loaded = message.status, true
		return model, nil, true
	case networkChangePlanMsg:
		model.busy = ""
		model.networkChange.plan = message.plan
		return model, nil, true
	case networkChangeResultMsg:
		model.busy = ""
		model.networkChange.reviewing = false
		model.networkChange.result = message.result
		// The saved step changes the configuration: later reviews must read
		// it again.
		model.computers.hosts = domain.HostsReport{}
		model.controller.result = domain.ControllerRebuildExecutionReport{}
		return model, nil, true
	}
	return model, nil, false
}

func (model dashboardModel) updateNetworkChange(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	change := &model.networkChange
	if model.busy != "" {
		return model, nil
	}
	if change.result.Operation != "" {
		return model.updateNetworkChangeResult(key)
	}
	if change.reviewing {
		return model.updateNetworkChangeReview(key)
	}
	status := change.status
	if status.InProgress() {
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardAdministration
			model.message = ""
		case "r":
			return model.loadNetworkChange()
		case "f":
			change.acknowledge = false
			return model.planNetworkChange(domain.NetworkMigrationFinish)
		case "u":
			change.acknowledge = false
			return model.planNetworkChange(domain.NetworkMigrationUndo)
		case "enter":
			switch {
			case !status.ControllerReady:
				return model.networkChangeController()
			case status.CanFinish:
				return model.planNetworkChange(domain.NetworkMigrationFinish)
			case status.Previous+status.Outdated > 0 || status.Waiting+status.Unknown > 0:
				return model.openNetworkChangeDeployment()
			}
		}
		return model, nil
	}
	switch key.String() {
	case "esc":
		model.screen = dashboardAdministration
		model.message = ""
	case "r":
		if change.input == "" {
			return model.loadNetworkChange()
		}
		change.input += "r"
	case "backspace":
		if value := []rune(change.input); len(value) > 0 {
			change.input = string(value[:len(value)-1])
		}
	case "enter":
		if !change.loaded || status.HasErrors() {
			return model, nil
		}
		change.acknowledge = false
		return model.planNetworkChange(domain.NetworkMigrationStart)
	default:
		if text := key.Text; len(text) == 1 && strings.ContainsAny(text, "0123456789./") && len(change.input) < 18 {
			change.input += text
		}
	}
	return model, nil
}

func (model dashboardModel) updateNetworkChangeReview(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	change := &model.networkChange
	plan := change.plan
	switch key.String() {
	case "esc", "left":
		change.reviewing = false
		model.message = "Nothing was saved."
	case "r":
		return model.planNetworkChange(plan.Action)
	case "c":
		if plan.Action == domain.NetworkMigrationStart && len(plan.Off()) > 0 {
			change.acknowledge = !change.acknowledge
			return model.planNetworkChange(plan.Action)
		}
	case "enter":
		if plan.HasErrors() || model.actions.ApplyNetworkMigration == nil {
			return model, nil
		}
		apply := model.actions.ApplyNetworkMigration
		model.busy = "Saving the reviewed step locally"
		return model, func() tea.Msg { return networkChangeResultMsg{result: apply(plan)} }
	}
	return model, nil
}

func (model dashboardModel) updateNetworkChangeResult(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	result := model.networkChange.result
	saved := result.State == "saved" && !result.HasErrors() && !result.RecoveryRequired
	switch key.String() {
	case "esc", "left", "r":
		return model.loadNetworkChange()
	case "enter":
		if !saved {
			return model.loadNetworkChange()
		}
		// Undo: the computers return first; the controller keeps both
		// addresses until its own apply. Otherwise the controller comes
		// first, then the computers.
		if result.Action == domain.NetworkMigrationUndo || controllerVerifiedForSave(result.Revision, model.controller.result) {
			return model.openNetworkChangeDeployment()
		}
		return model.continueSavedConfiguration()
	case "c":
		if saved && result.Action == domain.NetworkMigrationUndo {
			return model.networkChangeController()
		}
	}
	return model, nil
}

// networkChangeController opens the ordinary controller review and returns
// here afterwards.
func (model dashboardModel) networkChangeController() (tea.Model, tea.Cmd) {
	if model.actions.PlanController == nil || model.actions.ApplyController == nil {
		model.message = "Controller application is unavailable in this session."
		return model, nil
	}
	if reason := model.managedJobConflict(); reason != "" {
		model.message = reason
		return model, nil
	}
	model.controller.fromSave, model.controller.saveOrigin, model.controller.autoApply = true, dashboardNetworkChange, false
	model.controller.plan = domain.ControllerRebuildPlanReport{}
	model.controller.result = domain.ControllerRebuildExecutionReport{}
	model.confirmation, model.message = "", ""
	model.busy = "Reviewing the saved configuration for this controller"
	return model.startRead(func(ctx context.Context) tea.Msg {
		return dashboardControllerPlanMsg{report: model.actions.PlanController(ctx)}
	})
}

// openNetworkChangeDeployment opens Update computers with every computer
// selected; its review and Enter confirmation stay unchanged.
func (model dashboardModel) openNetworkChangeDeployment() (tea.Model, tea.Cmd) {
	if model.actions.LoadInventory == nil || model.actions.PlanDeployment == nil {
		model.message = "Computer deployment is unavailable in this session."
		return model, nil
	}
	return model.loadInventoryThen(func(ready dashboardModel) (tea.Model, tea.Cmd) {
		ready.screen = dashboardDeploy
		ready.deployment = deploymentModel{chosen: map[string]bool{}, context: "Change of laboratory network: every computer is selected. A computer still on the other network is reached there and moves with this update; computers that are off can be queued."}
		for _, host := range ready.report.Meta.Clients.Hosts {
			ready.deployment.chosen[host.Name] = true
		}
		ready.confirmation, ready.message, ready.navigation.pageScroll = "", "", 0
		return ready, nil
	})
}

func networkChangeComputerRow(computer domain.NetworkMigrationComputer, dark bool) string {
	kind := tuiStatusNeutral
	switch computer.State {
	case domain.MigrationComputerReachable, domain.MigrationComputerMigrated:
		kind = tuiStatusSuccess
	case domain.MigrationComputerOff, domain.MigrationComputerPrevious, domain.MigrationComputerOutdated, domain.MigrationComputerUnknown:
		kind = tuiStatusAttention
	}
	line := fmt.Sprintf("%-6s %-15s → %-15s ", computer.Name, computer.From, computer.To) + tuiStatus(networkMigrationComputerState(computer), kind, dark)
	if computer.Detail != "" {
		line += "\n" + tuiMuted("       "+computer.Detail, dark)
	}
	return line
}

func (model dashboardModel) networkChangeView() string {
	shell := tuiShell{path: []string{"Maintenance", "Change network addressing"}}
	if model.busy != "" {
		shell.body = model.busyView()
		shell.actions = []tuiAction{{key: "F1", label: "Help"}}
		return model.renderShell(shell)
	}
	change := model.networkChange
	var lines []string
	switch {
	case change.result.Operation != "":
		lines, shell.actions = networkChangeResultLines(change.result, controllerVerifiedForSave(change.result.Revision, model.controller.result), model.isDark)
	case change.reviewing:
		lines, shell.actions = networkChangeReviewLines(change.plan, change.acknowledge, model.isDark)
	case change.status.InProgress():
		lines, shell.actions = networkChangeStatusLines(change.status, model.isDark)
	default:
		lines, shell.actions = networkChangeFormLines(change, model.isDark)
	}
	shell.body = strings.Join(lines, "\n")
	if model.message != "" {
		shell.notices = append(shell.notices, model.messageNotice())
	}
	return model.renderShell(shell)
}

func networkChangeFormLines(change networkChangeModel, dark bool) ([]string, []tuiAction) {
	lines := []string{tuiTitle("Change network addressing", dark),
		tuiMuted("Move every computer and the controller to a new laboratory network. The controller keeps both addresses until you finish, so no computer loses contact.", dark), ""}
	if !change.loaded {
		return append(lines, "The laboratory network has not been read yet."), []tuiAction{{key: "r", label: "Check again"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
	}
	for _, issue := range change.status.Issues {
		lines = append(lines, tuiStatus(issue.Message, tuiStatusFailure, dark))
	}
	if change.status.HasErrors() {
		return lines, []tuiAction{{key: "r", label: "Check again"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
	}
	lines = append(lines, tuiFields(dark, [2]string{"Current laboratory network", change.status.To})...)
	lines = append(lines, "", tuiInputField("New network (address/prefix)", change.input+"_", true, dark),
		tuiMuted("For example 10.1.0.0/24. Every computer keeps its number: pc01 becomes the first address of the new network, and so on.", dark),
		"", tuiMuted("Enter reviews the change; nothing is saved before you confirm it.", dark))
	return lines, []tuiAction{{key: "Enter", label: "Review"}, {key: "Esc", label: "Maintenance"}, {key: "F1", label: "Help"}}
}

func networkChangeStatusLines(status domain.NetworkMigrationStatus, dark bool) ([]string, []tuiAction) {
	controller := tuiStatus("answers on both networks", tuiStatusSuccess, dark)
	if !status.ControllerReady {
		controller = tuiStatus("not applied yet", tuiStatusAttention, dark)
	}
	lines := []string{tuiTitle("Network change in progress", dark), ""}
	lines = append(lines, tuiFields(dark,
		[2]string{"Laboratory network", status.From + " → " + status.To},
		[2]string{"Started", status.StartedAt},
		[2]string{"Controller", status.ControllerFrom + " → " + status.ControllerTo + " · " + controller},
		[2]string{"Computers", fmt.Sprintf("%d moved · %d queued · %d on the previous network · %d to update · %d not answering", status.Migrated, status.Waiting, status.Previous, status.Outdated, status.Unknown)},
	)...)
	lines = append(lines, "")
	for _, computer := range status.Computers {
		lines = append(lines, networkChangeComputerRow(computer, dark))
	}
	for _, issue := range status.Issues {
		lines = append(lines, "", tuiStatus(issue.Message, tuiStatusAttention, dark))
	}
	if status.Next != "" {
		lines = append(lines, "", tuiSection("Next: ", dark)+status.Next)
	}
	lines = append(lines, tuiMuted("You can undo the change until it is finished: the computers return to "+status.From+".", dark))
	actions := []tuiAction{}
	switch {
	case !status.ControllerReady:
		actions = append(actions, tuiAction{key: "Enter", label: "Apply to controller"})
	case status.CanFinish:
		actions = append(actions, tuiAction{key: "Enter", label: "Finish the change"})
	case status.Previous+status.Outdated+status.Waiting+status.Unknown > 0:
		actions = append(actions, tuiAction{key: "Enter", label: "Update computers"})
	}
	if status.CanFinish {
		actions = append(actions, tuiAction{key: "f", label: "Finish"})
	}
	actions = append(actions, tuiAction{key: "u", label: "Undo"}, tuiAction{key: "r", label: "Check again"}, tuiAction{key: "Esc", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
	return lines, actions
}

func networkChangeReviewLines(plan domain.NetworkMigrationPlan, acknowledge bool, dark bool) ([]string, []tuiAction) {
	lines := []string{tuiTitle(networkMigrationTitles[plan.Action], dark), "", tuiSection("What was found", dark)}
	if plan.From != "" {
		lines = append(lines, tuiFields(dark,
			[2]string{"Laboratory network", plan.From + " → " + plan.To},
			[2]string{"Controller", plan.ControllerFrom + " → " + plan.ControllerTo})...)
	}
	for _, computer := range plan.Computers {
		lines = append(lines, networkChangeComputerRow(computer, dark))
	}
	for _, issue := range plan.Issues {
		lines = append(lines, tuiStatus(issue.Message, tuiStatusFailure, dark))
		if issue.Next != nil {
			lines = append(lines, "  "+issue.Next.Line())
		}
	}
	if len(plan.Steps) > 0 {
		lines = append(lines, "", tuiSection("What you can do", dark))
		for index, step := range plan.Steps {
			lines = append(lines, fmt.Sprintf("%d. %s", index+1, step))
		}
	}
	actions := []tuiAction{}
	if !plan.HasErrors() {
		lines = append(lines, "", tuiSection("Confirm", dark), "Enter saves this step locally. No computer changes until you apply to the controller and update the computers, each after its own review.")
		actions = append(actions, tuiAction{key: "Enter", label: "Save"})
	}
	if plan.Action == domain.NetworkMigrationStart && len(plan.Off()) > 0 {
		label := "Continue without them"
		if acknowledge {
			label = "Include them again"
		}
		actions = append(actions, tuiAction{key: "c", label: label})
	}
	return lines, append(actions, tuiAction{key: "r", label: "Check again"}, tuiAction{key: "Esc", label: "Back"}, tuiAction{key: "F1", label: "Help"})
}

func networkChangeResultLines(result domain.NetworkMigrationResult, controllerDone bool, dark bool) ([]string, []tuiAction) {
	saved := result.State == "saved" && !result.HasErrors() && !result.RecoveryRequired
	lines := []string{tuiTitle(networkMigrationTitles[result.Action], dark), "", tuiResult(result.Message, saved, dark)}
	for _, issue := range result.Issues {
		lines = append(lines, tuiStatus(issue.Message, tuiStatusFailure, dark))
	}
	if !saved {
		return lines, []tuiAction{{key: "Enter", label: "Back"}, {key: "F1", label: "Help"}}
	}
	lines = append(lines, "", tuiSection("Next", dark))
	for index, step := range result.Next {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, step))
	}
	if result.Action == domain.NetworkMigrationUndo {
		return lines, []tuiAction{{key: "Enter", label: "Update computers"}, {key: "c", label: "Apply to controller"}, {key: "Esc", label: "Change status"}, {key: "F1", label: "Help"}}
	}
	if controllerDone {
		lines = append(lines, "", tuiStatus("This controller now runs the saved configuration.", tuiStatusSuccess, dark))
		return lines, []tuiAction{{key: "Enter", label: "Update computers"}, {key: "Esc", label: "Change status"}, {key: "F1", label: "Help"}}
	}
	return lines, []tuiAction{{key: "Enter", label: "Apply to this controller"}, {key: "Esc", label: "Change status"}, {key: "F1", label: "Help"}}
}
