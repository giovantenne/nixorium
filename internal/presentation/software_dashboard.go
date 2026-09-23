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

type softwareScopeOption struct {
	label string
	scope domain.SoftwareScope
}

// softwareDashboardState owns the transient state of the Software feature.
// Embedding it keeps the existing screen code readable while removing feature
// details from the root model's global navigation and window state.
type softwareDashboardState struct {
	softwareCatalog      domain.SoftwareCatalogReport
	softwareMode         softwareListMode
	softwareCursor       int
	softwareQuery        string
	softwareSearching    bool
	softwareSearch       domain.SoftwareSearchReport
	softwareSearchID     uint64
	softwareSearchBusy   bool
	softwareSearchCancel context.CancelFunc
	softwareSelected     string
	softwareScopeCursor  int
	softwareClientCursor int
	softwareClients      map[string]bool
	softwarePlan         domain.SoftwareChangePlanReport
	softwareResult       domain.SoftwareChangeApplyReport
	softwareApplying     bool
}

func (model dashboardModel) updateSoftware(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardSoftware:
		items := model.softwareItems()
		switch key.String() {
		case "esc", "left":
			model.softwareSearchID++
			if model.softwareSearchCancel != nil {
				model.softwareSearchCancel()
				model.softwareSearchCancel = nil
			}
			model.screen = dashboardHome
			model.message = ""
		case "/":
			model.softwareMode = softwareSearch
			model.softwareSearching = true
			model.softwareCursor = 0
		case "tab":
			model = model.changeSoftwareMode(1)
		case "shift+tab":
			model = model.changeSoftwareMode(-1)
		case "up", "k":
			model.softwareCursor = max(0, model.softwareCursor-1)
		case "down", "j":
			model.softwareCursor = min(max(0, len(items)-1), model.softwareCursor+1)
		case "enter":
			if len(items) == 0 {
				return model, nil
			}
			item := items[min(model.softwareCursor, len(items)-1)]
			if model.softwareMode == softwareConfigured {
				entry, found := model.softwareDeclaration(item.ID)
				if !found {
					return model, nil
				}
				model.softwareSelected = item.ID
				return model.startSoftwarePlan(domain.SoftwareChangeRequest{Package: item.ID, Present: false, Scope: entry.Scope})
			}
			if item.Availability != "available" {
				model.message = softwareAvailabilityMessage(item)
				return model, nil
			}
			model.softwareSelected = item.ID
			model.softwareScopeCursor = 0
			model.softwareClientCursor = 0
			model.softwareClients = map[string]bool{}
			model.softwarePlan = domain.SoftwareChangePlanReport{}
			model.message = ""
			model.screen = dashboardSoftwareScope
		case "r":
			if len(items) == 0 {
				return model, nil
			}
			item := items[min(model.softwareCursor, len(items)-1)]
			entry, found := model.softwareDeclaration(item.ID)
			if !found {
				model.message = item.Label + " is not managed by lab-software.json."
				return model, nil
			}
			model.softwareSelected = item.ID
			return model.startSoftwarePlan(domain.SoftwareChangeRequest{Package: item.ID, Present: false, Scope: entry.Scope})
		}
	case dashboardSoftwareScope:
		options := model.softwareScopeOptions()
		clientOption := len(options) - 1
		switch key.String() {
		case "esc", "left":
			model.screen = dashboardSoftware
			model.message = ""
		case "up", "k":
			if model.softwareScopeCursor == clientOption && len(model.softwareCatalog.Clients) > 0 && model.softwareClientCursor > 0 {
				model.softwareClientCursor--
			} else {
				model.softwareScopeCursor = max(0, model.softwareScopeCursor-1)
			}
		case "down", "j":
			if model.softwareScopeCursor == clientOption && model.softwareClientCursor < len(model.softwareCatalog.Clients)-1 {
				model.softwareClientCursor++
			} else {
				model.softwareScopeCursor = min(clientOption, model.softwareScopeCursor+1)
			}
		case "space":
			if model.softwareScopeCursor == clientOption && len(model.softwareCatalog.Clients) > 0 {
				name := model.softwareCatalog.Clients[model.softwareClientCursor]
				model.softwareClients[name] = !model.softwareClients[name]
			}
		case "enter":
			if len(options) == 0 {
				return model, nil
			}
			scope := options[model.softwareScopeCursor].scope
			if scope.Kind == domain.SoftwareScopeClients {
				for _, name := range model.softwareCatalog.Clients {
					if model.softwareClients[name] {
						scope.Clients = append(scope.Clients, name)
					}
				}
				if len(scope.Clients) == 0 {
					model.message = "Select at least one configured computer with Space."
					return model, nil
				}
			}
			return model.startSoftwarePlan(domain.SoftwareChangeRequest{Package: model.softwareSelected, Present: true, Scope: scope})
		}
	case dashboardSoftwareReview:
		switch key.String() {
		case "esc":
			model.message = "Software change cancelled; no file changed."
			if model.softwarePlan.Request.Present {
				model.screen = dashboardSoftwareScope
			} else {
				model.screen = dashboardSoftware
			}
		case "enter":
			if model.actions.SaveSoftware == nil {
				model.message = "Software saving is not available in this deployment."
				return model, nil
			}
			model.busy = "Saving the reviewed software declaration"
			model.softwareApplying = true
			plan := model.softwarePlan
			return model, func() tea.Msg { return dashboardSoftwareApplyMsg{report: model.actions.SaveSoftware(plan)} }
		}
	case dashboardSoftwareResult:
		switch key.String() {
		case "r":
			if !model.softwareResult.RecoveryRequired || model.actions.SaveSoftware == nil {
				return model, nil
			}
			model.busy = "Recovering the local software save"
			model.softwareApplying = true
			plan := model.softwarePlan
			return model, func() tea.Msg { return dashboardSoftwareApplyMsg{report: model.actions.SaveSoftware(plan)} }
		case "a":
			if model.softwareResult.AffectedController == "" || model.actions.PlanController == nil || model.actions.ApplyController == nil {
				return model, nil
			}
			return model.startSoftwareControllerApply()
		case "enter", "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		}
	}
	return model, nil
}

func (model dashboardModel) startSoftwareControllerApply() (tea.Model, tea.Cmd) {
	if model.actions.PlanController == nil || model.actions.ApplyController == nil {
		model.message = "Controller activation is not available in this deployment. The software selection remains saved."
		return model, nil
	}
	model.busy = "Building and activating the reviewed software on this controller"
	model.softwareApplying = true
	model.controllerPlan = domain.ControllerRebuildPlanReport{}
	model.controllerResult = domain.ControllerRebuildExecutionReport{}
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

func (model dashboardModel) softwareView() string {
	path := []string{"Software"}
	if model.screen == dashboardSoftwareScope {
		path = append(path, "Scope")
	} else if model.screen == dashboardSoftwareReview {
		path = append(path, "Review")
	} else if model.screen == dashboardSoftwareResult {
		path = append(path, "Result")
	}
	lines := []string{}
	if model.busy != "" {
		lines = append(lines, tuiTitle("Software change", model.isDark), "", model.busyView())
		return model.renderShell(tuiShell{
			path:    path,
			body:    strings.Join(lines, "\n"),
			actions: []tuiAction{{key: "F1", label: "Help"}},
		})
	}
	switch model.screen {
	case dashboardSoftwareScope:
		lines = append(lines, model.softwareScopeView()...)
	case dashboardSoftwareReview:
		lines = append(lines, model.softwareReviewView()...)
	case dashboardSoftwareResult:
		lines = append(lines, model.softwareResultView()...)
	default:
		lines = append(lines, model.softwareCatalogView()...)
	}
	notices := model.softwareNotices()
	return model.renderShell(tuiShell{
		path:    path,
		body:    strings.Join(lines, "\n"),
		notices: notices,
		actions: model.softwareActions(),
	})
}

func (model dashboardModel) softwareActions() []tuiAction {
	if model.screen == dashboardSoftwareScope {
		actions := []tuiAction{{key: "↑/↓", label: "Select"}}
		options := model.softwareScopeOptions()
		if len(options) > 0 && options[min(model.softwareScopeCursor, len(options)-1)].scope.Kind == domain.SoftwareScopeClients {
			actions = append(actions, tuiAction{key: "Space", label: "Toggle"})
		}
		return append(actions,
			tuiAction{key: "Enter", label: "Review"},
			tuiAction{key: "Esc", label: "Catalog"},
			tuiAction{key: "F1", label: "Help"},
		)
	}
	if model.screen == dashboardSoftwareReview {
		back := "Catalog"
		primary := "Remove"
		if model.softwarePlan.Request.Present {
			back = "Scope"
			primary = "Save"
		}
		return []tuiAction{{key: "Enter", label: primary}, {key: "Esc", label: back}, {key: "F1", label: "Help"}}
	}
	if model.screen == dashboardSoftwareResult {
		if model.softwareResult.State == "partial" {
			return []tuiAction{{key: "r", label: "Retry save"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
		}
		if model.softwareResult.State == "saved" && model.softwareResult.AffectedController != "" && (model.controllerResult.Operation == "" || model.controllerResult.HasErrors() || !model.controllerResult.Applied || !model.controllerResult.Verified) {
			return []tuiAction{{key: "a", label: "Retry controller"}, {key: "Enter", label: "Overview"}, {key: "F1", label: "Help"}}
		}
		return []tuiAction{{key: "Enter", label: "Overview"}, {key: "F1", label: "Help"}}
	}
	if model.softwareCatalog.HasErrors() {
		return []tuiAction{{key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
	}
	if model.softwareSearching {
		return []tuiAction{{key: "Type", label: "Search"}, {key: "↑/↓", label: "Results"}, {key: "Tab", label: "Change view"}, {key: "Esc", label: "Stop typing"}, {key: "F1", label: "Help"}}
	}
	actions := []tuiAction{
		{key: "↑/↓", label: "Select"},
	}
	if len(model.softwareItems()) > 0 {
		primary := "Choose scope"
		if model.softwareMode == softwareConfigured {
			primary = "Review removal"
		} else {
			items := model.softwareItems()
			if items[min(model.softwareCursor, len(items)-1)].Availability != "available" {
				primary = "Explain unavailable"
			}
		}
		actions = append(actions, tuiAction{key: "Enter", label: primary})
	}
	return append(actions,
		tuiAction{key: "Tab", label: "Change view"},
		tuiAction{key: "/", label: "Search"},
		tuiAction{key: "Esc", label: "Overview"},
		tuiAction{key: "F1", label: "Help"},
	)
}

func (model dashboardModel) softwareNotices() []tuiNotice {
	if model.message == "" {
		return nil
	}
	if model.screen == dashboardSoftwareReview && model.message == model.softwarePlan.Message {
		return nil
	}
	return []tuiNotice{{kind: tuiStatusAttention, title: model.message}}
}

func (model dashboardModel) softwareCatalogView() []string {
	if model.softwareCatalog.HasErrors() {
		return []string{tuiTitle("Software", model.isDark), "", tuiResult("Software information unavailable", false, model.isDark), model.softwareCatalog.Message, "", "Return after the deployment inputs are available."}
	}
	items := model.softwareItems()
	lines := []string{
		tuiTitle("Software", model.isDark),
		"",
		softwareModeTabs(model.softwareMode, model.isDark),
		tuiMuted("Choose desired software here. Running clients change only when you deploy them.", model.isDark),
		"",
	}
	switch model.softwareMode {
	case softwareConfigured:
		lines = append(lines, tuiSection("Selected software", model.isDark), tuiMuted("Enter reviews removing the highlighted item. Use Search or Suggestions to add software.", model.isDark), "")
	case softwareSearch:
		cursor := ""
		if model.softwareSearching {
			cursor = "_"
		}
		lines = append(lines, tuiSection("Search packages", model.isDark), tuiMuted("Uses this deployment's locked Nix packages and overlays; inputs are never updated.", model.isDark), "", "Package name  "+tuiTitle(model.softwareQuery+cursor, model.isDark), "")
		if model.softwareSearchBusy {
			lines = append(lines, "Searching pinned packages…", "")
		} else if model.softwareQuery == "" {
			lines = append(lines, "Type at least two characters. Use a dotted prefix for nested sets, for example python3Packages.num.", "")
		} else if len(model.softwareQuery) == 1 {
			lines = append(lines, "Type one more character to start searching.", "")
		} else if domain.ValidateSoftwareSearchQuery(strings.TrimSpace(model.softwareQuery)) != nil {
			lines = append(lines, "Use only letters, digits, dot, plus, underscore, or hyphen in a package-name search.", "")
		} else if model.softwareSearch.Operation != "" && len(items) == 0 && !model.softwareSearch.HasErrors() {
			lines = append(lines, "No matching packages were found in the pinned package set.", "")
		}
	case softwareSuggested:
		lines = append(lines, tuiSection("Suggestions", model.isDark), tuiMuted("A short list of common choices from the same pinned package set.", model.isDark), "")
	}
	capacity := model.softwareCatalogListCapacity(lines, len(items))
	start, end := listWindow(len(items), model.softwareCursor, capacity)
	for index := start; index < end; index++ {
		item := items[index]
		status := ""
		if model.softwareMode == softwareConfigured {
			if entry, found := model.softwareDeclaration(item.ID); found {
				status = "  " + tuiMuted(softwareScopeListLabel(entry.Scope), model.isDark)
			}
		} else if entry, found := model.softwareDeclaration(item.ID); found {
			status = "  " + tuiStatus("configured for "+softwareScopeLabel(entry.Scope), tuiStatusSuccess, model.isDark)
		} else if item.Availability != "available" {
			status = "  " + tuiStatus(item.Availability, tuiStatusAttention, model.isDark)
		}
		version := ""
		if item.Version != "" {
			version = " · " + item.Version
		}
		label := tuiSelection(fmt.Sprintf("%-20s", item.Label), index == model.softwareCursor, model.isDark)
		lines = append(lines, label+status, tuiMuted("    "+item.Summary+" · "+item.ID+version, model.isDark))
	}
	if start > 0 || end < len(items) {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d software selections", displayedLineStart(start, len(items)), end, len(items)), model.isDark))
	}
	if len(items) == 0 && model.softwareMode == softwareConfigured {
		lines = append(lines, "No software is selected through this screen yet.", "", "Open Suggestions or Search packages to add one.")
	}
	lines = append(lines, "", "This is desired configuration; deploy from Computers to update clients.")
	return lines
}

func (model dashboardModel) softwareCatalogListCapacity(prefix []string, total int) int {
	if model.height <= 0 {
		return max(1, total)
	}
	width := min(116, max(20, model.width-6))
	if model.width == 0 {
		width = 100
	}
	prefixHeight := lipgloss.Height(lipgloss.NewStyle().Width(width).Render(strings.Join(prefix, "\n")))
	actionHeight := lipgloss.Height(tuiActionBar(model.width, model.isDark, model.softwareActions()...))
	available := model.height - 4 // frame padding
	available -= 2                // shell breadcrumb and following blank line
	available -= prefixHeight
	available -= 2 // blank plus the compact desired-configuration footer
	available -= 1 + actionHeight
	available-- // visible-range indicator
	// Each catalog item occupies a title and description row. Reserve the
	// wrapped shell, pagination, explanatory footer and action bar before
	// choosing the item window so the focused row cannot be clipped by frame.
	return max(1, available/2)
}

func (model dashboardModel) softwareScopeView() []string {
	item := model.softwareItem(model.softwareSelected)
	lines := []string{tuiTitle("Add "+item.Label, model.isDark), "Choose where this declaration applies. This is not the set of computers deployed today.", ""}
	options := model.softwareScopeOptions()
	for index, option := range options {
		lines = append(lines, tuiSelection(option.label, index == model.softwareScopeCursor, model.isDark))
	}
	if options[model.softwareScopeCursor].scope.Kind == domain.SoftwareScopeClients {
		lines = append(lines, "")
		start, end := listWindow(len(model.softwareCatalog.Clients), model.softwareClientCursor, max(2, model.height-len(lines)-16))
		for index := start; index < end; index++ {
			name := model.softwareCatalog.Clients[index]
			checked := "[ ]"
			if model.softwareClients[name] {
				checked = "[x]"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%s %s", checked, name), index == model.softwareClientCursor, model.isDark))
		}
	}
	lines = append(lines, "", "Powered-on clients required: none", "Managed file: "+model.softwareCatalog.ManagedFile)
	return lines
}

func (model dashboardModel) softwareReviewView() []string {
	plan := model.softwarePlan
	action := "Add"
	changeNow := "Save now"
	later := "Later        Deploy clients to install this change"
	if !plan.Request.Present {
		action = "Remove"
		changeNow = "Remove now"
		later = "Later        Deploy clients to remove this software"
	}
	item := model.softwareItem(plan.Request.Package)
	lines := []string{
		tuiTitle(action+" "+item.Label+"?", model.isDark),
		tuiMuted(plan.Request.Package, model.isDark),
		"",
		"Destination  " + softwareScopeLabel(plan.Request.Scope),
		fmt.Sprintf("Clients      %d affected by this declaration", len(plan.AffectedClients)),
		"",
		tuiStatus("Validated against the pinned package set", tuiStatusSuccess, model.isDark),
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

func (model dashboardModel) softwareResultView() []string {
	result := model.softwareResult
	switch result.State {
	case "saved":
		if result.AffectedController != "" {
			if model.controllerResult.Operation != "" && !model.controllerResult.HasErrors() && model.controllerResult.Applied && model.controllerResult.Verified {
				return []string{tuiResult("Software is ready on this controller", true, model.isDark), "", "✓ Software selection saved locally", "✓ Controller built, activated, and verified", "○ No client changed", "", "Use Distribute the prepared system when you want clients to receive it."}
			}
			detail := model.message
			if detail == "" {
				detail = "Controller activation did not complete."
			}
			return []string{tuiResult("Software saved; controller needs attention", false, model.isDark), "", "✓ Software selection saved locally", "! Controller build or activation did not complete", "○ No client changed", "", detail, "The saved selection is safe; retrying the controller does not duplicate it."}
		}
		return []string{tuiResult("Software configuration saved", true, model.isDark), "", "✓ Software selection saved locally", "○ System not prepared", "○ No client changed", "", "You can apply this configuration to selected computers now or later."}
	case "unchanged":
		return []string{tuiResult("Software declaration already current", true, model.isDark), "", "✓ The requested declaration is already present", "○ No file changed", "○ No system built or deployed"}
	case "partial":
		return []string{tuiResult("Software save needs attention", false, model.isDark), result.Message, "", softwareResultIssue(result), "", "No system was built or deployed.", "Retry completes the local save without duplicating the software change."}
	default:
		return []string{tuiResult("Software declaration was not saved", false, model.isDark), result.Message, "", softwareResultIssue(result), "", "Create a fresh proposal; no system was built or deployed."}
	}
}

func softwareResultIssue(result domain.SoftwareChangeApplyReport) string {
	if len(result.Issues) == 0 {
		return "Technical detail unavailable."
	}
	return "Technical detail: " + result.Issues[0].Message
}

func (model dashboardModel) softwareItems() []domain.SoftwareCatalogItem {
	switch model.softwareMode {
	case softwareConfigured:
		result := make([]domain.SoftwareCatalogItem, 0, len(model.softwareCatalog.Packages))
		for _, entry := range model.softwareCatalog.Packages {
			result = append(result, model.softwareItem(entry.Package))
		}
		return result
	case softwareSearch:
		return append([]domain.SoftwareCatalogItem{}, model.softwareSearch.Results...)
	default:
		return append([]domain.SoftwareCatalogItem{}, model.softwareCatalog.Catalog...)
	}
}

func (model dashboardModel) softwareItem(id string) domain.SoftwareCatalogItem {
	for _, items := range [][]domain.SoftwareCatalogItem{model.softwareCatalog.Catalog, model.softwareSearch.Results} {
		if item, found := softwareCatalogItemForView(items, id); found {
			return item
		}
	}
	return domain.SoftwareCatalogItem{ID: id, Label: id, Summary: "Managed package from the pinned package set", Availability: "available"}
}

func (model dashboardModel) changeSoftwareMode(offset int) dashboardModel {
	mode := (int(model.softwareMode) + offset + 3) % 3
	model.softwareMode = softwareListMode(mode)
	model.softwareCursor = 0
	model.message = ""
	model.softwareSearching = model.softwareMode == softwareSearch
	if model.softwareMode != softwareSearch {
		model.softwareSearchID++
		model.softwareSearchBusy = false
		if model.softwareSearchCancel != nil {
			model.softwareSearchCancel()
			model.softwareSearchCancel = nil
		}
	}
	return model
}

func (model *dashboardModel) scheduleSoftwareSearch() tea.Cmd {
	if model.softwareSearchCancel != nil {
		model.softwareSearchCancel()
		model.softwareSearchCancel = nil
	}
	model.softwareSearchID++
	model.softwareSearchBusy = false
	model.softwareSearch = domain.SoftwareSearchReport{}
	query := strings.TrimSpace(model.softwareQuery)
	if domain.ValidateSoftwareSearchQuery(query) != nil || model.actions.SearchSoftware == nil {
		return nil
	}
	model.softwareSearchBusy = true
	searchContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	model.softwareSearchCancel = cancel
	id := model.softwareSearchID
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

func (model dashboardModel) softwareDeclaration(id string) (domain.SoftwareDeclaration, bool) {
	for _, entry := range model.softwareCatalog.Packages {
		if entry.Package == id {
			return entry, true
		}
	}
	return domain.SoftwareDeclaration{}, false
}

func (model dashboardModel) softwareScopeOptions() []softwareScopeOption {
	result := []softwareScopeOption{{label: "All clients, including future clients", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}}
	if model.softwareCatalog.Controller != "" {
		result = append([]softwareScopeOption{
			{label: "This controller and all current or future clients", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}},
			{label: "Only this controller", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeController}},
		}, result...)
	}
	names := make([]string, 0, len(model.softwareCatalog.Groups))
	for name := range model.softwareCatalog.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, softwareScopeOption{label: "Group " + name + fmt.Sprintf(" (%d clients)", len(model.softwareCatalog.Groups[name])), scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: name}})
	}
	if len(model.softwareCatalog.Clients) > 0 {
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
