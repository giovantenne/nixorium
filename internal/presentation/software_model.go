package presentation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
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
	// softwareDetails shows one selected package with explicit actions.
	softwareDetails
)

type softwareScopeOption struct {
	label string
	scope domain.SoftwareScope
}

// softwareModel owns the transient state of the Software feature. The root
// dashboard keeps one named instance so feature state cannot be mistaken for
// global navigation state.
type softwareModel struct {
	stage        softwareStage
	catalog      domain.SoftwareCatalogReport
	mode         softwareListMode
	cursor       int
	query        string
	searching    bool
	search       domain.SoftwareSearchReport
	searchID     uint64
	searchBusy   bool
	searchCancel context.CancelFunc
	selected     string
	// changingScope marks a scope stage opened from package details.
	changingScope        bool
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
	result := softwareMessageResult{accepted: true}
	if report.HasErrors() {
		result.message = report.Message
	}
	return model, result
}

func (model softwareModel) finishPlan(report domain.SoftwareChangePlanReport) (softwareModel, softwareMessageResult) {
	model.plan = report
	if report.HasErrors() || report.State == "unchanged" {
		if report.Request.Present {
			model.stage = softwareScope
		} else if _, found := model.declaration(report.Request.Package); found {
			model.stage = softwareDetails
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

func (model softwareModel) update(key tea.KeyPressMsg) (softwareModel, softwareIntent) {
	switch model.stage {
	case softwareCatalog:
		items := model.items()
		switch key.String() {
		case "f2", "f3", "f4":
			target := int(key.String()[1] - '2')
			return model.changeMode(target - int(model.mode)), softwareIntent{setMessage: true}
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
				if _, found := model.declaration(item.ID); !found {
					return model, softwareIntent{}
				}
				model.selected = item.ID
				model.stage = softwareDetails
				return model, softwareIntent{setMessage: true}
			}
			if item.Availability != "available" {
				return model, softwareIntent{message: softwareAvailabilityMessage(item), setMessage: true}
			}
			model.selected = item.ID
			model.changingScope = false
			model.scopeCursor = 0
			model.clientCursor = 0
			model.clients = map[string]bool{}
			model.plan = domain.SoftwareChangePlanReport{}
			model.stage = softwareScope
			return model, softwareIntent{setMessage: true}
		case "v":
			return model, softwareIntent{kind: softwareStateIntent}
		case "p":
			return model, softwareIntent{kind: softwareProfilesIntent}
		}
	case softwareDetails:
		entry, found := model.declaration(model.selected)
		switch key.String() {
		case "esc", "left":
			model.stage = softwareCatalog
			return model, softwareIntent{setMessage: true}
		case "c":
			if found && !domain.IsBaseSoftware(model.selected) {
				model = model.startScopeChange(entry.Scope)
			}
			return model, softwareIntent{setMessage: true}
		case "x":
			if !found || domain.IsBaseSoftware(model.selected) {
				return model, softwareIntent{}
			}
			return model, softwareIntent{kind: softwarePlanIntent, request: domain.SoftwareChangeRequest{Package: model.selected, Present: false, Scope: entry.Scope}}
		}
	case softwareScope:
		switch key.String() {
		case "esc", "left":
			model.stage = softwareCatalog
			if model.changingScope {
				model.stage = softwareDetails
			}
			return model, softwareIntent{setMessage: true}
		case "enter":
			scope, notice := model.selectedScope()
			if notice != "" {
				return model, softwareIntent{message: notice, setMessage: true}
			}
			return model, softwareIntent{kind: softwarePlanIntent, request: domain.SoftwareChangeRequest{Package: model.selected, Present: true, Scope: scope}}
		default:
			model = model.updateScopeSelection(key)
		}
	case softwareReview:
		switch key.String() {
		case "esc":
			if model.plan.Request.Present {
				model.stage = softwareScope
			} else if _, found := model.declaration(model.selected); found {
				model.stage = softwareDetails
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
		case "s":
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

func (model softwareModel) canDistribute(controller domain.ControllerRebuildExecutionReport) bool {
	if model.result.State != "saved" || model.result.HasErrors() || model.result.RecoveryRequired || len(model.result.AffectedClients) == 0 {
		return false
	}
	if model.result.AffectedController == "" {
		return true
	}
	return controllerVerifiedForSave(model.result.Revision, controller)
}

func (model softwareModel) canInspectState() bool {
	return model.result.State == "saved" || model.result.State == "unchanged"
}

// startScopeChange opens the scope choice for a selected package with its
// current scope preselected.
func (model softwareModel) startScopeChange(current domain.SoftwareScope) softwareModel {
	model.changingScope = true
	model.scopeCursor, model.clientCursor = 0, 0
	model.clients = map[string]bool{}
	model.plan = domain.SoftwareChangePlanReport{}
	for index, option := range model.scopeOptions() {
		if option.scope.Kind == current.Kind && option.scope.Group == current.Group {
			model.scopeCursor = index
		}
	}
	for _, name := range current.Clients {
		model.clients[name] = true
	}
	model.stage = softwareScope
	return model
}

func softwareScopeAffectsController(scope domain.SoftwareScope) bool {
	return scope.Kind == domain.SoftwareScopeShared || scope.Kind == domain.SoftwareScopeController
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
	return domain.SoftwareCatalogItem{ID: id, Label: id, Availability: "available"}
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

func (model softwareModel) declaration(id string) (domain.SoftwareDeclaration, bool) {
	for _, entry := range model.catalog.Packages {
		if entry.Package == id {
			return entry, true
		}
	}
	return domain.SoftwareDeclaration{}, false
}

func (model softwareModel) updateScopeSelection(key tea.KeyPressMsg) softwareModel {
	clientOption := len(model.scopeOptions()) - 1
	switch key.String() {
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
	}
	return model
}

func (model softwareModel) selectedScope() (domain.SoftwareScope, string) {
	scope := model.scopeOptions()[model.scopeCursor].scope
	if scope.Kind == domain.SoftwareScopeClients {
		for _, name := range model.catalog.Clients {
			if model.clients[name] {
				scope.Clients = append(scope.Clients, name)
			}
		}
		if len(scope.Clients) == 0 {
			return scope, "Select at least one configured computer with Space."
		}
	}
	return scope, ""
}

func (model softwareModel) scopeOptions() []softwareScopeOption {
	option := func(scope domain.SoftwareScope) softwareScopeOption {
		label := softwareScopeLabel(scope)
		return softwareScopeOption{label: strings.ToUpper(label[:1]) + label[1:], scope: scope}
	}
	result := []softwareScopeOption{
		option(domain.SoftwareScope{Kind: domain.SoftwareScopeShared}),
		option(domain.SoftwareScope{Kind: domain.SoftwareScopeController}),
		option(domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}),
	}
	names := make([]string, 0, len(model.catalog.Groups))
	for name := range model.catalog.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, softwareScopeOption{label: fmt.Sprintf("Group %s (%s)", name, countNoun(len(model.catalog.Groups[name]), "client")), scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: name}})
	}
	if len(model.catalog.Clients) > 0 {
		result = append(result, softwareScopeOption{label: "Chosen clients…", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeClients}})
	}
	return result
}
