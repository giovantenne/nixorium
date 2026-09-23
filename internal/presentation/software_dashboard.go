package presentation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type softwareListMode int

const (
	softwareConfigured softwareListMode = iota
	softwareSearch
	softwareSuggested
)

type softwareStage int

const (
	softwareCatalog softwareStage = iota
	softwareScope
	softwareReview
	softwareResult
	softwareProfiles
	softwareProfilePackages
	softwareProfileScope
	softwareProfileReview
)

type softwareScopeOption struct {
	label string
	scope domain.SoftwareScope
}

// softwareModel owns the transient state of the Software feature. The root
// dashboard keeps one named instance so feature state cannot be mistaken for
// global navigation state.
type softwareModel struct {
	stage                softwareStage
	catalog              domain.SoftwareCatalogReport
	mode                 softwareListMode
	cursor               int
	query                string
	searching            bool
	search               domain.SoftwareSearchReport
	searchID             uint64
	searchBusy           bool
	searchCancel         context.CancelFunc
	selected             string
	scopeCursor          int
	clientCursor         int
	clients              map[string]bool
	plan                 domain.SoftwareChangePlanReport
	result               domain.SoftwareChangeApplyReport
	applying             bool
	presets              domain.SoftwarePresetCatalogReport
	profileCursor        int
	profilePackageCursor int
	profileReviewCursor  int
	profileExcluded      map[string]bool
	profilePlan          domain.SoftwarePresetPlanReport
	profileResult        domain.SoftwarePresetApplyReport
}

type softwareViewContext struct {
	width            int
	height           int
	dark             bool
	message          string
	busy             string
	busyView         string
	controllerResult domain.ControllerRebuildExecutionReport
}

type softwareIntentKind int

const (
	softwareNoIntent softwareIntentKind = iota
	softwareCloseIntent
	softwarePlanIntent
	softwareSaveIntent
	softwareControllerIntent
	softwareDeployIntent
	softwareStateIntent
	softwareProfilesIntent
	softwarePresetPlanIntent
	softwarePresetSaveIntent
)

type softwareIntent struct {
	kind          softwareIntentKind
	request       domain.SoftwareChangeRequest
	presetRequest domain.SoftwarePresetRequest
	message       string
	setMessage    bool
}

type softwareSearchInput struct {
	handled      bool
	delegate     bool
	clearMessage bool
	command      tea.Cmd
}

type softwareMessageResult struct {
	accepted        bool
	message         string
	startController bool
}

func (model softwareModel) open() softwareModel {
	if model.searchCancel != nil {
		model.searchCancel()
	}
	model.stage = softwareCatalog
	model.result = domain.SoftwareChangeApplyReport{}
	model.profileResult = domain.SoftwarePresetApplyReport{}
	model.searchID++
	model.searchCancel = nil
	return model
}

func (model softwareModel) acceptsText() bool {
	return model.searching || model.stage == softwareReview
}

func (model softwareModel) mutating() bool { return model.applying }

func (model *softwareModel) cancelSearch() {
	if model.searchCancel != nil {
		model.searchCancel()
		model.searchCancel = nil
	}
}

func (model softwareModel) reviewing() bool { return model.stage == softwareReview }

func (model softwareModel) searchToken() uint64 { return model.searchID }

func (model softwareModel) currentScopeKind() string {
	options := model.scopeOptions()
	if len(options) == 0 {
		return ""
	}
	return options[min(model.scopeCursor, len(options)-1)].scope.Kind
}

func (model softwareModel) loadCatalog(report domain.SoftwareCatalogReport) (softwareModel, softwareMessageResult) {
	if model.searchCancel != nil {
		model.searchCancel()
		model.searchCancel = nil
	}
	model.catalog = report
	model.cursor = 0
	model.query = ""
	model.searching = false
	model.search = domain.SoftwareSearchReport{}
	model.searchBusy = false
	model.searchID++
	model.mode = softwareSuggested
	model.stage = softwareCatalog
	model.presets = domain.SoftwarePresetCatalogReport{}
	model.profilePlan = domain.SoftwarePresetPlanReport{}
	model.profileResult = domain.SoftwarePresetApplyReport{}
	if len(report.Packages) > 0 {
		model.mode = softwareConfigured
	}
	return model, softwareMessageResult{accepted: true, message: report.Message}
}

func (model softwareModel) startSearch(message dashboardSoftwareSearchStartMsg, search func(context.Context, string) domain.SoftwareSearchReport) (softwareModel, softwareMessageResult, tea.Cmd) {
	if model.stage != softwareCatalog || model.mode != softwareSearch || message.id != model.searchID || message.query != strings.TrimSpace(model.query) || search == nil {
		return model, softwareMessageResult{}, nil
	}
	model.searchBusy = true
	query := message.query
	id := message.id
	searchContext := message.ctx
	command := func() tea.Msg {
		return dashboardSoftwareSearchMsg{id: id, report: search(searchContext, query)}
	}
	return model, softwareMessageResult{accepted: true}, command
}

func (model softwareModel) finishSearch(message dashboardSoftwareSearchMsg) (softwareModel, softwareMessageResult) {
	if model.stage != softwareCatalog || model.mode != softwareSearch || message.id != model.searchID {
		return model, softwareMessageResult{}
	}
	model.searchBusy = false
	if model.searchCancel != nil {
		model.searchCancel()
		model.searchCancel = nil
	}
	model.search = message.report
	model.cursor = 0
	result := softwareMessageResult{accepted: true}
	if message.report.HasErrors() {
		result.message = message.report.Message
	}
	return model, result
}

func (model softwareModel) finishPlan(report domain.SoftwareChangePlanReport) (softwareModel, softwareMessageResult) {
	model.plan = report
	if report.HasErrors() || report.State == "unchanged" {
		if report.Request.Present {
			model.stage = softwareScope
		} else {
			model.stage = softwareCatalog
		}
	} else {
		model.stage = softwareReview
	}
	return model, softwareMessageResult{accepted: true, message: report.Message}
}

func (model softwareModel) finishApply(report domain.SoftwareChangeApplyReport) (softwareModel, softwareMessageResult) {
	model.applying = false
	model.result = report
	model.stage = softwareResult
	return model, softwareMessageResult{
		accepted:        true,
		message:         report.Message,
		startController: !report.HasErrors() && report.State == "saved" && report.AffectedController != "",
	}
}

func (model softwareModel) finishController(plan domain.ControllerRebuildPlanReport, report domain.ControllerRebuildExecutionReport) (softwareModel, softwareMessageResult) {
	model.applying = false
	model.stage = softwareResult
	message := report.Message
	if plan.HasErrors() {
		message = controllerPlanIssues(plan)
	}
	return model, softwareMessageResult{accepted: true, message: message}
}

func (model softwareModel) updateSearchInput(key tea.KeyPressMsg, search func(context.Context, string) domain.SoftwareSearchReport) (softwareModel, softwareSearchInput) {
	if !model.searching {
		return model, softwareSearchInput{}
	}
	changed := false
	switch key.String() {
	case "tab":
		return model.changeMode(1), softwareSearchInput{handled: true, clearMessage: true}
	case "shift+tab":
		return model.changeMode(-1), softwareSearchInput{handled: true, clearMessage: true}
	case "esc":
		model.searching = false
	case "up", "down", "enter":
		model.searching = false
		return model, softwareSearchInput{handled: true, delegate: true}
	case "backspace":
		value := []rune(model.query)
		if len(value) > 0 {
			model.query = string(value[:len(value)-1])
			changed = true
		}
	default:
		if key.Text != "" && len(model.query) < 80 {
			model.query += key.Text
			changed = true
		}
	}
	model.cursor = 0
	result := softwareSearchInput{handled: true}
	if changed {
		result.command = model.scheduleSearch(search)
	}
	return model, result
}

func (model softwareModel) update(key tea.KeyPressMsg) (softwareModel, softwareIntent) {
	switch model.stage {
	case softwareCatalog:
		items := model.items()
		switch key.String() {
		case "esc", "left":
			model.searchID++
			if model.searchCancel != nil {
				model.searchCancel()
				model.searchCancel = nil
			}
			return model, softwareIntent{kind: softwareCloseIntent, setMessage: true}
		case "/":
			model.mode = softwareSearch
			model.searching = true
			model.cursor = 0
		case "tab":
			model = model.changeMode(1)
			return model, softwareIntent{setMessage: true}
		case "shift+tab":
			model = model.changeMode(-1)
			return model, softwareIntent{setMessage: true}
		case "up", "k":
			model.cursor = max(0, model.cursor-1)
		case "down", "j":
			model.cursor = min(max(0, len(items)-1), model.cursor+1)
		case "enter":
			if len(items) == 0 {
				return model, softwareIntent{}
			}
			item := items[min(model.cursor, len(items)-1)]
			if model.mode == softwareConfigured {
				entry, found := model.declaration(item.ID)
				if !found {
					return model, softwareIntent{}
				}
				model.selected = item.ID
				return model, softwareIntent{kind: softwarePlanIntent, request: domain.SoftwareChangeRequest{Package: item.ID, Present: false, Scope: entry.Scope}}
			}
			if item.Availability != "available" {
				return model, softwareIntent{message: softwareAvailabilityMessage(item), setMessage: true}
			}
			model.selected = item.ID
			model.scopeCursor = 0
			model.clientCursor = 0
			model.clients = map[string]bool{}
			model.plan = domain.SoftwareChangePlanReport{}
			model.stage = softwareScope
			return model, softwareIntent{setMessage: true}
		case "r":
			if len(items) == 0 {
				return model, softwareIntent{}
			}
			item := items[min(model.cursor, len(items)-1)]
			entry, found := model.declaration(item.ID)
			if !found {
				return model, softwareIntent{message: item.Label + " is not managed by lab-software.json.", setMessage: true}
			}
			model.selected = item.ID
			return model, softwareIntent{kind: softwarePlanIntent, request: domain.SoftwareChangeRequest{Package: item.ID, Present: false, Scope: entry.Scope}}
		case "v":
			return model, softwareIntent{kind: softwareStateIntent}
		case "p":
			return model, softwareIntent{kind: softwareProfilesIntent}
		}
	case softwareScope:
		options := model.scopeOptions()
		clientOption := len(options) - 1
		switch key.String() {
		case "esc", "left":
			model.stage = softwareCatalog
			return model, softwareIntent{setMessage: true}
		case "up", "k":
			if model.scopeCursor == clientOption && len(model.catalog.Clients) > 0 && model.clientCursor > 0 {
				model.clientCursor--
			} else {
				model.scopeCursor = max(0, model.scopeCursor-1)
			}
		case "down", "j":
			if model.scopeCursor == clientOption && model.clientCursor < len(model.catalog.Clients)-1 {
				model.clientCursor++
			} else {
				model.scopeCursor = min(clientOption, model.scopeCursor+1)
			}
		case "space":
			if model.scopeCursor == clientOption && len(model.catalog.Clients) > 0 {
				name := model.catalog.Clients[model.clientCursor]
				model.clients[name] = !model.clients[name]
			}
		case "enter":
			if len(options) == 0 {
				return model, softwareIntent{}
			}
			scope := options[model.scopeCursor].scope
			if scope.Kind == domain.SoftwareScopeClients {
				for _, name := range model.catalog.Clients {
					if model.clients[name] {
						scope.Clients = append(scope.Clients, name)
					}
				}
				if len(scope.Clients) == 0 {
					return model, softwareIntent{message: "Select at least one configured computer with Space.", setMessage: true}
				}
			}
			return model, softwareIntent{kind: softwarePlanIntent, request: domain.SoftwareChangeRequest{Package: model.selected, Present: true, Scope: scope}}
		}
	case softwareReview:
		switch key.String() {
		case "esc":
			if model.plan.Request.Present {
				model.stage = softwareScope
			} else {
				model.stage = softwareCatalog
			}
			return model, softwareIntent{message: "Software change cancelled; no file changed.", setMessage: true}
		case "enter":
			return model, softwareIntent{kind: softwareSaveIntent}
		}
	case softwareProfiles:
		return model.updateProfiles(key)
	case softwareProfilePackages:
		return model.updateProfilePackages(key)
	case softwareProfileScope:
		return model.updateProfileScope(key)
	case softwareProfileReview:
		return model.updateProfileReview(key)
	case softwareResult:
		switch key.String() {
		case "r":
			if !model.result.RecoveryRequired {
				return model, softwareIntent{}
			}
			if model.profileResult.Operation != "" {
				return model, softwareIntent{kind: softwarePresetSaveIntent}
			}
			return model, softwareIntent{kind: softwareSaveIntent}
		case "a":
			if model.result.AffectedController == "" {
				return model, softwareIntent{}
			}
			return model, softwareIntent{kind: softwareControllerIntent}
		case "d":
			if model.result.State == "saved" && len(model.result.AffectedClients) > 0 {
				return model, softwareIntent{kind: softwareDeployIntent}
			}
		case "v":
			if model.canInspectState() {
				return model, softwareIntent{kind: softwareStateIntent}
			}
		case "enter", "esc", "left":
			return model, softwareIntent{kind: softwareCloseIntent, setMessage: true}
		}
	}
	return model, softwareIntent{}
}

func (model dashboardModel) updateSoftware(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			model.busy = "Saving the reviewed software declaration"
		}
		model.software.applying = true
		plan := model.software.plan
		return model, func() tea.Msg { return dashboardSoftwareApplyMsg{report: model.actions.SaveSoftware(plan)} }
	case softwareControllerIntent:
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
		return model, func() tea.Msg { return dashboardSoftwarePresetCatalogMsg{report: model.actions.LoadSoftwarePresets()} }
	case softwarePresetPlanIntent:
		if model.actions.PlanSoftwarePreset == nil {
			model.message = "Software profile planning is not available in this deployment."
			return model, nil
		}
		model.busy = "Checking every profile package and its destination"
		model.message = ""
		request := intent.presetRequest
		return model, func() tea.Msg {
			return dashboardSoftwarePresetPlanMsg{report: model.actions.PlanSoftwarePreset(request)}
		}
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

func (model softwareModel) canDistribute(controller domain.ControllerRebuildExecutionReport) bool {
	if model.result.State != "saved" || len(model.result.AffectedClients) == 0 {
		return false
	}
	if model.result.AffectedController == "" {
		return true
	}
	return controller.Operation != "" && !controller.HasErrors() && controller.Applied && controller.Verified
}

func (model softwareModel) canInspectState() bool {
	return model.result.State == "saved" || model.result.State == "unchanged"
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
	return model, func() tea.Msg {
		plan := model.actions.PlanController()
		if plan.HasErrors() {
			return dashboardSoftwareControllerMsg{plan: plan}
		}
		return dashboardSoftwareControllerMsg{plan: plan, report: model.actions.ApplyController(plan)}
	}
}

func (model dashboardModel) startSoftwarePlan(request domain.SoftwareChangeRequest) (tea.Model, tea.Cmd) {
	if model.actions.PlanSoftware == nil {
		model.message = "Software planning is not available in this deployment."
		return model, nil
	}
	model.busy = "Checking the package and its destination"
	model.message = ""
	return model, func() tea.Msg { return dashboardSoftwarePlanMsg{report: model.actions.PlanSoftware(request)} }
}

func (model dashboardModel) openSoftwareDeployment() (tea.Model, tea.Cmd) {
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
	return model, model.loadConfigurationState()
}

func (model dashboardModel) softwareView() string {
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

func (model softwareModel) view(context softwareViewContext) tuiShell {
	path := []string{"Software"}
	if model.stage == softwareScope {
		path = append(path, "Scope")
	} else if model.stage == softwareReview {
		path = append(path, "Review")
	} else if model.stage == softwareProfiles {
		path = append(path, "Profiles")
	} else if model.stage == softwareProfilePackages {
		path = append(path, "Profile packages")
	} else if model.stage == softwareProfileScope {
		path = append(path, "Profile scope")
	} else if model.stage == softwareProfileReview {
		path = append(path, "Profile review")
	} else if model.stage == softwareResult {
		path = append(path, "Result")
	}
	lines := []string{}
	if context.busy != "" {
		lines = append(lines, tuiTitle("Software change", context.dark), "", context.busyView)
		return tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			actions: []tuiAction{{key: "F1", label: "Help"}},
		}
	}
	switch model.stage {
	case softwareScope:
		lines = append(lines, model.scopeView(context)...)
	case softwareReview:
		lines = append(lines, model.reviewView(context)...)
	case softwareProfiles:
		lines = append(lines, model.profilesView(context)...)
	case softwareProfilePackages:
		lines = append(lines, model.profilePackagesView(context)...)
	case softwareProfileScope:
		lines = append(lines, model.profileScopeView(context)...)
	case softwareProfileReview:
		lines = append(lines, model.profileReviewView(context)...)
	case softwareResult:
		lines = append(lines, model.resultView(context)...)
	default:
		lines = append(lines, model.catalogView(context)...)
	}
	return tuiShell{
		path:    path,
		body:    strings.Join(lines, "\n"),
		notices: model.notices(context),
		actions: model.actions(context),
	}
}

func (model softwareModel) actions(context softwareViewContext) []tuiAction {
	if model.stage == softwareProfiles || model.stage == softwareProfilePackages || model.stage == softwareProfileScope || model.stage == softwareProfileReview {
		return model.profileActions(context)
	}
	if model.stage == softwareScope {
		actions := []tuiAction{{key: "↑/↓", label: "Select"}}
		options := model.scopeOptions()
		if len(options) > 0 && options[min(model.scopeCursor, len(options)-1)].scope.Kind == domain.SoftwareScopeClients {
			actions = append(actions, tuiAction{key: "Space", label: "Toggle"})
		}
		return append(actions,
			tuiAction{key: "Enter", label: "Review"},
			tuiAction{key: "Esc", label: "Catalog"},
			tuiAction{key: "F1", label: "Help"},
		)
	}
	if model.stage == softwareReview {
		back := "Catalog"
		primary := "Remove"
		if model.plan.Request.Present {
			back = "Scope"
			primary = "Save"
		}
		return []tuiAction{{key: "Enter", label: primary}, {key: "Esc", label: back}, {key: "F1", label: "Help"}}
	}
	if model.stage == softwareResult {
		if model.result.State == "partial" {
			return []tuiAction{{key: "r", label: "Retry save"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
		}
		if model.result.State == "saved" && model.result.AffectedController != "" && (context.controllerResult.Operation == "" || context.controllerResult.HasErrors() || !context.controllerResult.Applied || !context.controllerResult.Verified) {
			return []tuiAction{{key: "a", label: "Retry controller"}, {key: "Enter", label: "Overview"}, {key: "F1", label: "Help"}}
		}
		if model.canDistribute(context.controllerResult) {
			return []tuiAction{{key: "d", label: "Distribute affected computers"}, {key: "v", label: "Check systems"}, {key: "Enter", label: "Later"}, {key: "F1", label: "Help"}}
		}
		if model.canInspectState() {
			return []tuiAction{{key: "v", label: "Check systems"}, {key: "Enter", label: "Overview"}, {key: "F1", label: "Help"}}
		}
		return []tuiAction{{key: "Enter", label: "Overview"}, {key: "F1", label: "Help"}}
	}
	if model.catalog.HasErrors() {
		return []tuiAction{{key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
	}
	if model.searching {
		return []tuiAction{{key: "Type", label: "Search"}, {key: "↑/↓", label: "Results"}, {key: "Tab", label: "Change view"}, {key: "Esc", label: "Stop typing"}, {key: "F1", label: "Help"}}
	}
	actions := []tuiAction{
		{key: "↑/↓", label: "Select"},
	}
	if len(model.items()) > 0 {
		primary := "Choose scope"
		if model.mode == softwareConfigured {
			primary = "Review removal"
		} else {
			items := model.items()
			if items[min(model.cursor, len(items)-1)].Availability != "available" {
				primary = "Explain unavailable"
			}
		}
		actions = append(actions, tuiAction{key: "Enter", label: primary})
	}
	return append(actions,
		tuiAction{key: "p", label: "Add profile"},
		tuiAction{key: "Tab", label: "Change view"},
		tuiAction{key: "/", label: "Search"},
		tuiAction{key: "v", label: "Check systems"},
		tuiAction{key: "Esc", label: "Overview"},
		tuiAction{key: "F1", label: "Help"},
	)
}

func (model softwareModel) notices(context softwareViewContext) []tuiNotice {
	if context.message == "" {
		return nil
	}
	if (model.stage == softwareReview && context.message == model.plan.Message) ||
		(model.stage == softwareProfileReview && context.message == model.profilePlan.Message) {
		return nil
	}
	return []tuiNotice{{kind: tuiStatusAttention, title: context.message}}
}

func (model softwareModel) catalogView(context softwareViewContext) []string {
	if model.catalog.HasErrors() {
		return []string{tuiTitle("Software", context.dark), "", tuiResult("Software information unavailable", false, context.dark), model.catalog.Message, "", "Return after the deployment inputs are available."}
	}
	items := model.items()
	lines := []string{
		tuiTitle("Software", context.dark),
		"",
		softwareModeTabs(model.mode, context.dark),
		tuiMuted("Choose desired software here. Running clients change only when you deploy them.", context.dark),
		"",
	}
	switch model.mode {
	case softwareConfigured:
		lines = append(lines, tuiSection("Selected software", context.dark), tuiMuted("Enter reviews removing the highlighted item. Use Search or Suggestions to add software.", context.dark), "")
	case softwareSearch:
		cursor := ""
		if model.searching {
			cursor = "_"
		}
		lines = append(lines, tuiSection("Search packages", context.dark), tuiMuted("Uses this deployment's locked Nix packages and overlays; inputs are never updated.", context.dark), "", "Package name  "+tuiTitle(model.query+cursor, context.dark), "")
		if model.searchBusy {
			lines = append(lines, "Searching pinned packages…", "")
		} else if model.query == "" {
			lines = append(lines, "Type at least two characters. Use a dotted prefix for nested sets, for example python3Packages.num.", "")
		} else if len(model.query) == 1 {
			lines = append(lines, "Type one more character to start searching.", "")
		} else if domain.ValidateSoftwareSearchQuery(strings.TrimSpace(model.query)) != nil {
			lines = append(lines, "Use only letters, digits, dot, plus, underscore, or hyphen in a package-name search.", "")
		} else if model.search.Operation != "" && len(items) == 0 && !model.search.HasErrors() {
			lines = append(lines, "No matching packages were found in the pinned package set.", "")
		}
	case softwareSuggested:
		lines = append(lines, tuiSection("Suggestions", context.dark), tuiMuted("A short list of common choices from the same pinned package set.", context.dark), "")
	}
	capacity := model.catalogListCapacity(context, lines, len(items))
	start, end := listWindow(len(items), model.cursor, capacity)
	for index := start; index < end; index++ {
		item := items[index]
		status := ""
		if model.mode == softwareConfigured {
			if entry, found := model.declaration(item.ID); found {
				status = "  " + tuiMuted(softwareScopeListLabel(entry.Scope), context.dark)
			}
		} else if entry, found := model.declaration(item.ID); found {
			status = "  " + tuiStatus("configured for "+softwareScopeLabel(entry.Scope), tuiStatusSuccess, context.dark)
		} else if item.Availability != "available" {
			status = "  " + tuiStatus(item.Availability, tuiStatusAttention, context.dark)
		}
		version := ""
		if item.Version != "" {
			version = " · " + item.Version
		}
		label := tuiSelection(fmt.Sprintf("%-20s", item.Label), index == model.cursor, context.dark)
		lines = append(lines, label+status, tuiMuted("    "+item.Summary+" · "+item.ID+version, context.dark))
	}
	if start > 0 || end < len(items) {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d software selections", displayedLineStart(start, len(items)), end, len(items)), context.dark))
	}
	if len(items) == 0 && model.mode == softwareConfigured {
		lines = append(lines, "No software is selected through this screen yet.", "", "Open Suggestions or Search packages to add one.")
	}
	lines = append(lines, "", "This is desired configuration; deploy from Computers to update clients.")
	return lines
}

func (model softwareModel) catalogListCapacity(context softwareViewContext, prefix []string, total int) int {
	if context.height <= 0 {
		return max(1, total)
	}
	width := min(116, max(20, context.width-6))
	if context.width == 0 {
		width = 100
	}
	prefixHeight := lipgloss.Height(lipgloss.NewStyle().Width(width).Render(strings.Join(prefix, "\n")))
	actionHeight := lipgloss.Height(tuiActionBar(context.width, context.dark, model.actions(context)...))
	available := context.height - 4 // frame padding
	available -= 2                  // shell breadcrumb and following blank line
	available -= prefixHeight
	available -= 2 // blank plus the compact desired-configuration footer
	available -= 1 + actionHeight
	available-- // visible-range indicator
	// Each catalog item occupies a title and description row. Reserve the
	// wrapped shell, pagination, explanatory footer and action bar before
	// choosing the item window so the focused row cannot be clipped by frame.
	return max(1, available/2)
}

func (model softwareModel) scopeView(context softwareViewContext) []string {
	item := model.item(model.selected)
	lines := []string{tuiTitle("Add "+item.Label, context.dark), "Choose where this declaration applies. This is not the set of computers deployed today.", ""}
	options := model.scopeOptions()
	for index, option := range options {
		lines = append(lines, tuiSelection(option.label, index == model.scopeCursor, context.dark))
	}
	if options[model.scopeCursor].scope.Kind == domain.SoftwareScopeClients {
		lines = append(lines, "")
		start, end := listWindow(len(model.catalog.Clients), model.clientCursor, max(2, context.height-len(lines)-16))
		for index := start; index < end; index++ {
			name := model.catalog.Clients[index]
			checked := "[ ]"
			if model.clients[name] {
				checked = "[x]"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%s %s", checked, name), index == model.clientCursor, context.dark))
		}
	}
	lines = append(lines, "", "Powered-on clients required: none", "Managed file: "+model.catalog.ManagedFile)
	return lines
}

func (model softwareModel) reviewView(context softwareViewContext) []string {
	plan := model.plan
	action := "Add"
	changeNow := "Save now"
	later := "Later        Deploy clients to install this change"
	if !plan.Request.Present {
		action = "Remove"
		changeNow = "Remove now"
		later = "Later        Deploy clients to remove this software"
	}
	item := model.item(plan.Request.Package)
	lines := []string{
		tuiTitle(action+" "+item.Label+"?", context.dark),
		tuiMuted(plan.Request.Package, context.dark),
		"",
		"Destination  " + softwareScopeLabel(plan.Request.Scope),
		fmt.Sprintf("Clients      %d affected by this declaration", len(plan.AffectedClients)),
		"",
		tuiStatus("Validated against the pinned package set", tuiStatusSuccess, context.dark),
		fmt.Sprintf("%-12s Update %s locally", changeNow, plan.ManagedFile),
		later,
	}
	if plan.AffectedController != "" {
		lines[len(lines)-2] = fmt.Sprintf("%-12s Update %s and rebuild %s", changeNow, plan.ManagedFile, plan.AffectedController)
	}
	if len(plan.AffectedClients) == 0 {
		lines[len(lines)-1] = "Later        No client deployment required"
	}
	return lines
}

func (model softwareModel) resultView(context softwareViewContext) []string {
	if model.profileResult.Operation != "" {
		return model.profileResultView(context)
	}
	result := model.result
	switch result.State {
	case "saved":
		if result.AffectedController != "" {
			if context.controllerResult.Operation != "" && !context.controllerResult.HasErrors() && context.controllerResult.Applied && context.controllerResult.Verified {
				return []string{tuiResult("Software is ready on this controller", true, context.dark), "", "✓ Software selection saved locally", "✓ Controller built, activated, and verified", "○ No client changed", "", "Use Distribute the prepared system when you want clients to receive it."}
			}
			detail := context.message
			if detail == "" {
				detail = "Controller activation did not complete."
			}
			return []string{tuiResult("Software saved; controller needs attention", false, context.dark), "", "✓ Software selection saved locally", "! Controller build or activation did not complete", "○ No client changed", "", detail, "The saved selection is safe; retrying the controller does not duplicate it."}
		}
		return []string{tuiResult("Software configuration saved", true, context.dark), "", "✓ Software selection saved locally", "○ System not prepared", "○ No client changed", "", "You can apply this configuration to selected computers now or later."}
	case "unchanged":
		return []string{tuiResult("Software declaration already current", true, context.dark), "", "✓ The requested declaration is already present", "○ No file changed", "○ No system built or deployed"}
	case "partial":
		return []string{tuiResult("Software save needs attention", false, context.dark), result.Message, "", softwareResultIssue(result), "", "No system was built or deployed.", "Retry completes the local save without duplicating the software change."}
	default:
		return []string{tuiResult("Software declaration was not saved", false, context.dark), result.Message, "", softwareResultIssue(result), "", "Create a fresh proposal; no system was built or deployed."}
	}
}

func softwareResultIssue(result domain.SoftwareChangeApplyReport) string {
	if len(result.Issues) == 0 {
		return "Technical detail unavailable."
	}
	return "Technical detail: " + result.Issues[0].Message
}

func (model softwareModel) items() []domain.SoftwareCatalogItem {
	switch model.mode {
	case softwareConfigured:
		result := make([]domain.SoftwareCatalogItem, 0, len(model.catalog.Packages))
		for _, entry := range model.catalog.Packages {
			result = append(result, model.item(entry.Package))
		}
		return result
	case softwareSearch:
		return append([]domain.SoftwareCatalogItem{}, model.search.Results...)
	default:
		return append([]domain.SoftwareCatalogItem{}, model.catalog.Catalog...)
	}
}

func (model softwareModel) item(id string) domain.SoftwareCatalogItem {
	for _, items := range [][]domain.SoftwareCatalogItem{model.catalog.Catalog, model.search.Results} {
		if item, found := softwareCatalogItemForView(items, id); found {
			return item
		}
	}
	return domain.SoftwareCatalogItem{ID: id, Label: id, Summary: "Managed package from the pinned package set", Availability: "available"}
}

func (model softwareModel) changeMode(offset int) softwareModel {
	mode := (int(model.mode) + offset + 3) % 3
	model.mode = softwareListMode(mode)
	model.cursor = 0
	model.searching = model.mode == softwareSearch
	if model.mode != softwareSearch {
		model.searchID++
		model.searchBusy = false
		if model.searchCancel != nil {
			model.searchCancel()
			model.searchCancel = nil
		}
	}
	return model
}

func (model *softwareModel) scheduleSearch(search func(context.Context, string) domain.SoftwareSearchReport) tea.Cmd {
	if model.searchCancel != nil {
		model.searchCancel()
		model.searchCancel = nil
	}
	model.searchID++
	model.searchBusy = false
	model.search = domain.SoftwareSearchReport{}
	query := strings.TrimSpace(model.query)
	if domain.ValidateSoftwareSearchQuery(query) != nil || search == nil {
		return nil
	}
	model.searchBusy = true
	searchContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	model.searchCancel = cancel
	id := model.searchID
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return dashboardSoftwareSearchStartMsg{id: id, query: query, ctx: searchContext}
	})
}

func softwareModeTabs(mode softwareListMode, dark bool) string {
	labels := []string{"Selected", "Search packages", "Suggestions"}
	parts := make([]string, len(labels))
	for index, label := range labels {
		if index == int(mode) {
			parts[index] = tuiTitle("["+label+"]", dark)
		} else {
			parts[index] = tuiMuted(label, dark)
		}
	}
	return strings.Join(parts, "   ")
}

func softwareAvailabilityMessage(item domain.SoftwareCatalogItem) string {
	switch item.Availability {
	case "blocked-broken":
		return item.Label + " is marked broken in the pinned package set and cannot be selected."
	case "blocked-insecure":
		return item.Label + " is blocked by the pinned package security policy and cannot be selected."
	case "blocked-unfree":
		return item.Label + " is blocked by the deployment's package licensing policy and cannot be selected."
	default:
		return item.Label + " is not available for this deployment platform."
	}
}

func (model softwareModel) declaration(id string) (domain.SoftwareDeclaration, bool) {
	for _, entry := range model.catalog.Packages {
		if entry.Package == id {
			return entry, true
		}
	}
	return domain.SoftwareDeclaration{}, false
}

func (model softwareModel) scopeOptions() []softwareScopeOption {
	result := []softwareScopeOption{{label: "All clients, including future clients", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}}
	if model.catalog.Controller != "" {
		result = append([]softwareScopeOption{
			{label: "This controller and all current or future clients", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}},
			{label: "Only this controller", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeController}},
		}, result...)
	}
	names := make([]string, 0, len(model.catalog.Groups))
	for name := range model.catalog.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, softwareScopeOption{label: "Group " + name + fmt.Sprintf(" (%d clients)", len(model.catalog.Groups[name])), scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: name}})
	}
	if len(model.catalog.Clients) > 0 {
		result = append(result, softwareScopeOption{label: "Selected configured computers", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeClients}})
	}
	return result
}

func softwareScopeLabel(scope domain.SoftwareScope) string {
	switch scope.Kind {
	case domain.SoftwareScopeShared:
		return "this controller and all current or future clients"
	case domain.SoftwareScopeController:
		return "only this controller"
	case domain.SoftwareScopeAllClients:
		return "all clients, including future clients"
	case domain.SoftwareScopeGroup:
		return "group " + scope.Group
	case domain.SoftwareScopeClients:
		return strings.Join(scope.Clients, ", ")
	}
	return "unknown scope"
}

func softwareScopeListLabel(scope domain.SoftwareScope) string {
	if scope.Kind != domain.SoftwareScopeClients {
		return softwareScopeLabel(scope)
	}
	if len(scope.Clients) == 1 {
		return "1 selected client"
	}
	return fmt.Sprintf("%d selected clients", len(scope.Clients))
}

func softwareCatalogItemForView(items []domain.SoftwareCatalogItem, id string) (domain.SoftwareCatalogItem, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return domain.SoftwareCatalogItem{ID: id, Label: id}, false
}
