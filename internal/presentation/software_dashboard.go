package presentation

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

type softwareScopeOption struct {
	label string
	scope domain.SoftwareScope
}

func (model dashboardModel) updateSoftware(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.screen {
	case dashboardSoftware:
		items := model.filteredSoftwareCatalog()
		switch key.String() {
		case "esc", "left":
			if model.softwareQuery != "" {
				model.softwareQuery = ""
				model.softwareCursor = 0
				return model, nil
			}
			model.screen = dashboardHome
			model.message = ""
		case "/":
			model.softwareSearching = true
		case "up", "k":
			model.softwareCursor = max(0, model.softwareCursor-1)
		case "down", "j":
			model.softwareCursor = min(max(0, len(items)-1), model.softwareCursor+1)
		case "enter":
			if len(items) == 0 {
				return model, nil
			}
			model.softwareSelected = items[min(model.softwareCursor, len(items)-1)].ID
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
			model.confirmation = ""
			model.message = "Software change cancelled; no file changed."
			model.screen = dashboardSoftwareScope
		case "backspace":
			value := []rune(model.confirmation)
			if len(value) > 0 {
				model.confirmation = string(value[:len(value)-1])
			}
		case "space":
			model.confirmation += " "
		case "enter":
			if model.confirmation != model.softwarePlan.Confirmation {
				model.confirmation = ""
				model.message = "Confirmation did not match; lab-software.json was not changed."
				return model, nil
			}
			if model.actions.ApplySoftware == nil {
				model.message = "Software apply is not available in this deployment."
				return model, nil
			}
			model.busy = "Saving the reviewed software declaration"
			model.softwareApplying = true
			model.confirmation = ""
			plan := model.softwarePlan
			return model, func() tea.Msg { return dashboardSoftwareApplyMsg{report: model.actions.ApplySoftware(plan)} }
		default:
			if key.Text != "" {
				model.confirmation += key.Text
			}
		}
	case dashboardSoftwareResult:
		switch key.String() {
		case "g":
			if model.actions.LoadGitReview == nil {
				model.message = "Git review is not available in this deployment."
				return model, nil
			}
			model.screen = dashboardGitReview
			model.busy = "Reviewing the saved software declaration"
			model.message = ""
			return model, func() tea.Msg { return dashboardGitReviewMsg{report: model.actions.LoadGitReview()} }
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
		return []string{tuiResult("Software catalog unavailable", false, model.isDark), model.softwareCatalog.Message, "", "The package name cannot be entered manually because it would bypass pinned-input validation.", "", "esc interventions   ? help"}
	}
	items := model.filteredSoftwareCatalog()
	lines := []string{tuiSection("Supported client software", model.isDark), tuiMuted("Resolved from the laboratory's pinned package set; searching does not update inputs.", model.isDark), ""}
	if model.softwareSearching || model.softwareQuery != "" {
		lines = append(lines, "Search  / "+model.softwareQuery+"_", "")
	}
	start, end := listWindow(len(items), model.softwareCursor, max(4, model.height-17))
	for index := start; index < end; index++ {
		item := items[index]
		marker := "  "
		if index == model.softwareCursor {
			marker = "› "
		}
		configured := ""
		if entry, found := model.softwareDeclaration(item.ID); found {
			configured = "  ✓ " + softwareScopeLabel(entry.Scope)
		}
		lines = append(lines, fmt.Sprintf("%s%-20s%s", marker, item.Label, configured), tuiMuted("    "+item.Summary+" · "+item.ID, model.isDark))
	}
	if len(items) == 0 {
		lines = append(lines, "No supported software matches this search.")
	}
	lines = append(lines, "", "Configuration can be prepared while every client is powered off.", "Declared does not mean committed, built, or distributed.", "Private modules remain untouched and are managed through Advanced tools.", "", tuiHelp(model.width, model.isDark,
		tuiHelpBinding([]string{"up", "down"}, "↑/↓", "select"),
		tuiHelpBinding([]string{"enter"}, "enter", "scope"),
		tuiHelpBinding([]string{"r"}, "r", "remove"),
		tuiHelpBinding([]string{"/"}, "/", "search"),
		tuiHelpBinding([]string{"esc"}, "esc", "back"),
	))
	if model.message != "" {
		lines = append(lines, "", tuiMuted(model.message, model.isDark))
	}
	return lines
}

func (model dashboardModel) softwareScopeView() []string {
	item, _ := softwareCatalogItemForView(model.softwareCatalog.Catalog, model.softwareSelected)
	lines := []string{tuiSection("Add "+item.Label, model.isDark), "Choose where this declaration applies. This is not the set of computers deployed today.", ""}
	options := model.softwareScopeOptions()
	for index, option := range options {
		marker := "  "
		if index == model.softwareScopeCursor {
			marker = "› "
		}
		lines = append(lines, marker+option.label)
	}
	if model.softwareScopeCursor == len(options)-1 {
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
	item, _ := softwareCatalogItemForView(model.softwareCatalog.Catalog, plan.Request.Package)
	lines := []string{tuiSection(action+" "+item.Label+"?", model.isDark), tuiMuted("Package identifier  "+plan.Request.Package, model.isDark), "", "Configuration scope        " + softwareScopeLabel(plan.Request.Scope), fmt.Sprintf("Configured clients affected  %d", len(plan.AffectedClients)), "Managed file               " + plan.ManagedFile, "Powered-on clients         none required", "", tuiStatus("Proposal validated", tuiStatusSuccess, model.isDark), "○ Revision not saved", "○ System not prepared", "○ No client changed", "", "Only lab-software.json will be replaced atomically.", "No commit, build, activation, PXE action, or deployment is included.", "", tuiSection("Type "+plan.Confirmation+" to continue:", model.isDark), "> " + model.confirmation + "_", "", "enter save declaration   esc cancel   F1 help"}
	if model.message != "" && model.message != plan.Message {
		lines = append(lines, "", tuiStatus(model.message, tuiStatusAttention, model.isDark))
	}
	return lines
}

func (model dashboardModel) softwareResultView() []string {
	result := model.softwareResult
	switch result.State {
	case "applied":
		return []string{tuiResult("Software declaration saved", true, model.isDark), "", "✓ lab-software.json updated", "○ Git revision not saved", "○ System not prepared", "○ No client changed", "", "Review and commit the declaration before preparing or distributing systems.", "", "g review Git changes   enter interventions   ? help"}
	case "unchanged":
		return []string{tuiResult("Software declaration already current", true, model.isDark), "", "✓ The requested declaration is already present", "○ No file changed", "○ No system built or deployed", "", "enter interventions   ? help"}
	case "partial":
		return []string{tuiResult("Software save needs attention", false, model.isDark), result.Message, "", softwareResultIssue(result), "", "No system was built or deployed.", "Inspect the file and Git review before creating another proposal.", "", "g review Git changes   esc interventions   ? help"}
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

func (model dashboardModel) filteredSoftwareCatalog() []domain.SoftwareCatalogItem {
	query := strings.ToLower(model.softwareQuery)
	result := []domain.SoftwareCatalogItem{}
	for _, item := range model.softwareCatalog.Catalog {
		if strings.Contains(strings.ToLower(item.ID+" "+item.Label+" "+item.Summary), query) {
			result = append(result, item)
		}
	}
	return result
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
	names := make([]string, 0, len(model.softwareCatalog.Groups))
	for name := range model.softwareCatalog.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, softwareScopeOption{label: "Group " + name + fmt.Sprintf(" (%d clients)", len(model.softwareCatalog.Groups[name])), scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: name}})
	}
	return append(result, softwareScopeOption{label: "Selected configured computers", scope: domain.SoftwareScope{Kind: domain.SoftwareScopeClients}})
}

func softwareScopeLabel(scope domain.SoftwareScope) string {
	switch scope.Kind {
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
