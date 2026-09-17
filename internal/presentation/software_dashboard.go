package presentation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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
		case "enter", "esc", "left":
			model.screen = dashboardHome
			model.message = ""
		}
	}
	return model, nil
}

func (model dashboardModel) startSoftwarePlan(request domain.SoftwareChangeRequest) (tea.Model, tea.Cmd) {
	if model.actions.PlanSoftware == nil {
		model.message = "Software planning is not available in this deployment."
		return model, nil
	}
	model.busy = "Validating the software proposal through pinned Nix inputs"
	model.message = ""
	return model, func() tea.Msg { return dashboardSoftwarePlanMsg{report: model.actions.PlanSoftware(request)} }
}

func (model dashboardModel) softwareView() string {
	lines := []string{tuiTitle("Nixorium  /  Add or change software", model.isDark), ""}
	if model.busy != "" {
		return strings.Join(append(lines, model.busyView()), "\n")
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
	return strings.Join(lines, "\n")
}

func (model dashboardModel) softwareCatalogView() []string {
	if model.softwareCatalog.HasErrors() {
		return []string{tuiResult("Software information unavailable", false, model.isDark), model.softwareCatalog.Message, "", "Retry from Interventions after the deployment inputs are available.", "", "esc interventions   ? help"}
	}
	items := model.softwareItems()
	lines := []string{
		softwareModeTabs(model.softwareMode),
		tuiMuted("Configuration choices are separate from applying them to computers.", model.isDark),
		"",
	}
	switch model.softwareMode {
	case softwareConfigured:
		lines = append(lines, tuiSection("Configured software", model.isDark), tuiMuted("Desired software managed by this screen; this is not an observed installed inventory.", model.isDark), "")
	case softwareSearch:
		cursor := ""
		if model.softwareSearching {
			cursor = "_"
		}
		lines = append(lines, tuiSection("Search packages", model.isDark), tuiMuted("Uses this deployment's locked Nix packages and overlays; inputs are never updated.", model.isDark), "", "Package name  "+model.softwareQuery+cursor, "")
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
		lines = append(lines, tuiSection("Suggested software", model.isDark), tuiMuted("A short list of common choices from the same pinned package set.", model.isDark), "")
	}
	start, end := listWindow(len(items), model.softwareCursor, max(4, model.height-17))
	for index := start; index < end; index++ {
		item := items[index]
		marker := "  "
		if index == model.softwareCursor {
			marker = "› "
		}
		status := ""
		if model.softwareMode == softwareConfigured {
			if entry, found := model.softwareDeclaration(item.ID); found {
				status = "  " + softwareScopeLabel(entry.Scope)
			}
		} else if entry, found := model.softwareDeclaration(item.ID); found {
			status = "  ✓ configured for " + softwareScopeLabel(entry.Scope)
		} else if item.Availability != "available" {
			status = "  ! " + item.Availability
		}
		version := ""
		if item.Version != "" {
			version = " · " + item.Version
		}
		lines = append(lines, fmt.Sprintf("%s%-20s%s", marker, item.Label, status), tuiMuted("    "+item.Summary+" · "+item.ID+version, model.isDark))
	}
	if len(items) == 0 && model.softwareMode == softwareConfigured {
		lines = append(lines, "No software is configured through this screen yet.", "", "Open Suggested software or Search packages to add one.")
	}
	primary := "choose scope"
	if model.softwareMode == softwareConfigured {
		primary = "review removal"
	}
	lines = append(lines, "", "Configuration can be prepared while every client is powered off.", "Configured here does not mean applied to a computer.", "Private modules remain untouched and are managed through Advanced tools.", "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"up", "down"}, "↑/↓", "select"),
		tuiHelpBinding([]string{"enter"}, "enter", primary),
		tuiHelpBinding([]string{"r"}, "r", "remove"),
		tuiHelpBinding([]string{"/"}, "/", "search"),
		tuiHelpBinding([]string{"tab"}, "tab", "change view"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
	))
	if model.message != "" {
		lines = append(lines, "", tuiMuted(model.message, model.isDark))
	}
	return lines
}

func (model dashboardModel) softwareScopeView() []string {
	item := model.softwareItem(model.softwareSelected)
	lines := []string{tuiSection("Add "+item.Label, model.isDark), "Choose where this declaration applies. This is not the set of computers deployed today.", ""}
	options := model.softwareScopeOptions()
	for index, option := range options {
		marker := "  "
		if index == model.softwareScopeCursor {
			marker = "› "
		}
		lines = append(lines, marker+option.label)
	}
	if options[model.softwareScopeCursor].scope.Kind == domain.SoftwareScopeClients {
		lines = append(lines, "")
		start, end := listWindow(len(model.softwareCatalog.Clients), model.softwareClientCursor, max(4, model.height-len(lines)-9))
		for index := start; index < end; index++ {
			name := model.softwareCatalog.Clients[index]
			marker := "  "
			if index == model.softwareClientCursor {
				marker = "› "
			}
			checked := "[ ]"
			if model.softwareClients[name] {
				checked = "[x]"
			}
			lines = append(lines, fmt.Sprintf("%s%s %s", marker, checked, name))
		}
	}
	lines = append(lines, "", "Powered-on clients required: none", "Managed file: "+model.softwareCatalog.ManagedFile, "", "↑/↓ move   space select computer   enter review   esc catalog   ? help")
	if model.message != "" {
		lines = append(lines, "", tuiStatus(model.message, tuiStatusAttention, model.isDark))
	}
	return lines
}

func (model dashboardModel) softwareReviewView() []string {
	plan := model.softwarePlan
	action := "Add"
	if !plan.Request.Present {
		action = "Remove"
	}
	item := model.softwareItem(plan.Request.Package)
	lines := []string{tuiSection(action+" "+item.Label+"?", model.isDark), tuiMuted("Package identifier  "+plan.Request.Package, model.isDark), "", "Configuration scope        " + softwareScopeLabel(plan.Request.Scope), fmt.Sprintf("Configured clients affected  %d", len(plan.AffectedClients)), "Managed file               " + plan.ManagedFile, "Powered-on clients         none required", "", tuiStatus("Proposal validated", tuiStatusSuccess, model.isDark), "○ Configuration not saved", "○ System not prepared", "○ No client changed", "", "Only lab-software.json will be replaced and saved locally.", "No build, activation, PXE action, or client deployment is included.", "", "Enter saves this reviewed configuration; Esc cancels.", "", "enter save configuration   esc cancel   F1 help"}
	if plan.AffectedController != "" {
		lines = append(lines[:5], append([]string{"Controller configuration   " + plan.AffectedController + " (not activated by saving)"}, lines[5:]...)...)
	}
	if model.message != "" && model.message != plan.Message {
		lines = append(lines, "", tuiStatus(model.message, tuiStatusAttention, model.isDark))
	}
	return lines
}

func (model dashboardModel) softwareResultView() []string {
	result := model.softwareResult
	switch result.State {
	case "saved":
		if result.AffectedController != "" {
			return []string{tuiResult("Software configuration saved", true, model.isDark), "", "✓ Software selection saved locally", "○ Controller changes not yet applied", "○ No client changed", "", "Apply controller configuration to build and activate these changes.", "Client computers require a separate deployment.", "", "enter interventions   ? help"}
		}
		return []string{tuiResult("Software configuration saved", true, model.isDark), "", "✓ Software selection saved locally", "○ System not prepared", "○ No client changed", "", "You can apply this configuration to selected computers now or later.", "", "enter interventions   ? help"}
	case "unchanged":
		return []string{tuiResult("Software declaration already current", true, model.isDark), "", "✓ The requested declaration is already present", "○ No file changed", "○ No system built or deployed", "", "enter interventions   ? help"}
	case "partial":
		return []string{tuiResult("Software save needs attention", false, model.isDark), result.Message, "", softwareResultIssue(result), "", "No system was built or deployed.", "Retry completes the local save without duplicating the software change.", "", "r retry save   esc interventions   ? help"}
	default:
		return []string{tuiResult("Software declaration was not saved", false, model.isDark), result.Message, "", softwareResultIssue(result), "", "Create a fresh proposal; no system was built or deployed.", "", "esc interventions   ? help"}
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

func softwareModeTabs(mode softwareListMode) string {
	labels := []string{"Configured", "Search packages", "Suggested"}
	parts := make([]string, len(labels))
	for index, label := range labels {
		if index == int(mode) {
			parts[index] = "[" + label + "]"
		} else {
			parts[index] = label
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

func softwareCatalogItemForView(items []domain.SoftwareCatalogItem, id string) (domain.SoftwareCatalogItem, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return domain.SoftwareCatalogItem{ID: id, Label: id}, false
}
