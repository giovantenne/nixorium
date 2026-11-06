package presentation

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

type softwareViewContext struct {
	width            int
	height           int
	dark             bool
	message          string
	busy             string
	busyView         string
	controllerResult domain.ControllerRebuildExecutionReport
}

func (model softwareModel) view(context softwareViewContext) tuiShell {
	path := []string{"Software"}
	if model.stage == softwareDetails {
		path = append(path, "Details")
	} else if model.stage == softwareScope {
		path = append(path, "Computers")
	} else if model.stage == softwareReview {
		path = append(path, "Review")
	} else if model.stage == softwareProfiles {
		path = append(path, "Profiles")
	} else if model.stage == softwareProfilePackages {
		path = append(path, "Profile packages")
	} else if model.stage == softwareProfileScope {
		path = append(path, "Profile computers")
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
	case softwareDetails:
		lines = append(lines, model.detailsView(context)...)
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
	if model.stage == softwareDetails {
		if domain.IsBaseSoftware(model.selected) {
			return []tuiAction{{key: "Esc", label: "Configured software"}, {key: "F1", label: "Help"}}
		}
		return []tuiAction{{key: "c", label: "Change computers"}, {key: "x", label: "Remove"}, {key: "Esc", label: "Configured software"}, {key: "F1", label: "Help"}}
	}
	if model.stage == softwareScope {
		actions := []tuiAction{{key: "↑/↓", label: "Select"}}
		options := model.scopeOptions()
		if len(options) > 0 && options[min(model.scopeCursor, len(options)-1)].scope.Kind == domain.SoftwareScopeClients {
			actions = append(actions, tuiAction{key: "Space", label: "Toggle"})
		}
		back := "Software"
		if model.changingScope {
			back = "Details"
		}
		return append(actions,
			tuiAction{key: "Enter", label: "Review"},
			tuiAction{key: "Esc", label: back},
			tuiAction{key: "F1", label: "Help"},
		)
	}
	if model.stage == softwareReview {
		back := "Details"
		primary := "Remove"
		if model.plan.Request.Present {
			back = "Choose computers"
			primary = "Save"
		}
		return []tuiAction{{key: "Enter", label: primary}, {key: "Esc", label: back}, {key: "F1", label: "Help"}}
	}
	if model.stage == softwareResult {
		if model.result.State == "partial" {
			return []tuiAction{{key: "s", label: "Retry save"}, {key: "Esc", label: "Overview"}, {key: "F1", label: "Help"}}
		}
		if model.result.State == "saved" && !model.result.HasErrors() && !model.result.RecoveryRequired && model.result.AffectedController != "" && !controllerVerifiedForSave(model.result.Revision, context.controllerResult) {
			return []tuiAction{{key: "Enter", label: "Retry controller"}, {key: "a", label: "Retry controller"}, {key: "Esc", label: "Later"}, {key: "F1", label: "Help"}}
		}
		if model.canDistribute(context.controllerResult) {
			return []tuiAction{{key: "Enter", label: "Update affected computers"}, {key: "v", label: "Check systems"}, {key: "Esc", label: "Later"}, {key: "F1", label: "Help"}}
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
		actions := []tuiAction{{key: "Type", label: "Search"}, {key: "↑/↓", label: "Results"}}
		if len(model.items()) > 0 {
			actions = append(actions, tuiAction{key: "Enter", label: "Choose computers"})
		}
		return append(actions, tuiAction{key: "Tab", label: "Change view"}, tuiAction{key: "Esc", label: "Stop typing"}, tuiAction{key: "F1", label: "Help"})
	}
	actions := []tuiAction{
		{key: "↑/↓", label: "Select"},
	}
	if len(model.items()) > 0 {
		primary := "Choose computers"
		if model.mode == softwareConfigured {
			primary = "View details"
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
	// The result page already explains successful saves and their next step.
	if model.stage == softwareResult && !model.result.HasErrors() &&
		(model.result.State == "saved" || model.result.State == "unchanged") &&
		!model.result.RecoveryRequired && context.message == model.result.Message {
		return nil
	}
	return []tuiNotice{{kind: tuiStatusAttention, title: context.message}}
}

func (model softwareModel) catalogView(context softwareViewContext) []string {
	if model.catalog.HasErrors() {
		return []string{tuiTitle("Software", context.dark), "", tuiResult("Software information unavailable", false, context.dark), model.catalog.Message, "", "Return after the deployment inputs are available."}
	}
	items := model.items()
	lines := []string{softwareModeTabs(model.mode, context.dark), ""}
	switch model.mode {
	case softwareConfigured:
		lines = append(lines, tuiTitle("Configured software", context.dark),
			tuiMuted("The saved choices, not a check of what is installed. Enter shows a package; F3 finds more.", context.dark), "")
	case softwareSearch:
		cursor := ""
		if model.searching {
			cursor = "_"
		}
		query := model.query + cursor
		input := tuiInputField("Package name", query, model.searching, context.dark)
		if model.query == "" {
			input += tuiMuted("e.g. firefox", context.dark)
		}
		lines = append(lines, tuiTitle("Search for software", context.dark),
			tuiMuted("Choose a package, then its computers; you review before saving.", context.dark), "",
			input, "")
		if model.searchBusy {
			lines = append(lines, tuiNoticeText("Searching available packages…", tuiStatusNeutral, context.dark), "")
		} else if model.query == "" {
			lines = append(lines, "Type at least two characters.", "")
		} else if len(model.query) == 1 {
			lines = append(lines, "Type one more character to start searching.", "")
		} else if domain.ValidateSoftwareSearchQuery(strings.TrimSpace(model.query)) != nil {
			lines = append(lines, "Use only letters, digits, dot, plus, underscore, or hyphen in a package-name search.", "")
		} else if model.search.Operation != "" && len(items) == 0 && !model.search.HasErrors() {
			lines = append(lines, "No matches in the package versions used by this laboratory.", "")
		}
	case softwareSuggested:
		lines = append(lines, tuiTitle("Suggested software", context.dark), tuiMuted("Choose a package, then its computers; you review before saving.", context.dark), "")
	}
	capacity := model.catalogListCapacity(context, lines, len(items))
	start, end := listWindow(len(items), model.cursor, capacity)
	for index := start; index < end; index++ {
		item := items[index]
		// Keep names and destinations on separate lines, including long package
		// identifiers. Full metadata and exact destinations remain in Details.
		width := min(112, max(20, context.width-10))
		if context.width == 0 {
			width = 100
		}
		title := item.Label
		if item.Version != "" {
			title += " · " + item.Version
		}
		detail := item.Summary
		if model.mode == softwareConfigured {
			if entry, found := model.declaration(item.ID); found {
				// What the package is, then where it goes: identifiers such as
				// mysql-workbench mean little on their own.
				detail = "Installed on " + softwareScopeLabel(entry.Scope)
				if summary := strings.TrimSpace(item.Summary); summary != "" {
					detail = summary + " · " + strings.ToLower(detail[:1]) + detail[1:]
				}
			}
		} else if entry, found := model.declaration(item.ID); found {
			detail = "Already installed on " + softwareScopeLabel(entry.Scope)
		} else if item.Availability != "available" {
			detail = softwareAvailabilityLabel(item.Availability)
		}
		lines = append(lines,
			tuiSelection(ansi.Truncate(title, width, "…"), index == model.cursor, context.dark),
			"    "+tuiMuted(ansi.Truncate(detail, width-2, "…"), context.dark))
	}
	if start > 0 || end < len(items) {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d packages", displayedLineStart(start, len(items)), end, len(items)), context.dark))
	}
	if len(items) == 0 && model.mode == softwareConfigured {
		lines = append(lines, "No packages have been added through this screen.", "", "Press F3 to find software, or F4 for suggestions.")
	}
	lines = append(lines, "", tuiMuted("Saved choices reach the client computers with Computers → Update computers.", context.dark))
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
	if !compactTerminal(context.height) && context.height > 0 {
		actionHeight += 2 // the next-step button border
	}
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

func (model softwareModel) detailsView(context softwareViewContext) []string {
	item := model.item(model.selected)
	entry, _ := model.declaration(model.selected)
	version := ""
	if item.Version != "" {
		version = " · " + item.Version
	}
	lines := []string{tuiTitle(item.Label, context.dark)}
	if item.ID != item.Label || version != "" {
		lines = append(lines, tuiMuted("Package "+item.ID+version, context.dark))
	}
	if item.Summary != "" {
		lines = append(lines, tuiMuted(item.Summary, context.dark))
	}
	lines = append(lines, "")
	lines = append(lines, tuiFields(context.dark, [2]string{"Installed on", softwareScopeLabel(entry.Scope)})...)
	lines = append(lines, "")
	if domain.IsBaseSoftware(item.ID) {
		return append(lines, "Every laboratory computer always has it: it cannot be removed or limited to some computers.")
	}
	if softwareScopeAffectsController(entry.Scope) {
		lines = append(lines, "A change is applied to this controller right after saving.")
	}
	return append(lines, "Client computers change only when you update them from Computers → Update computers.")
}

func (model softwareModel) scopeView(context softwareViewContext) []string {
	item := model.item(model.selected)
	title := "Add " + item.Label
	if model.changingScope {
		title = "Change where " + item.Label + " applies"
	}
	lines := []string{tuiTitle(title, context.dark), "Choose which computers should receive this package.", ""}
	options := model.scopeOptions()
	for index, option := range options {
		lines = append(lines, tuiSelection(option.label, index == model.scopeCursor, context.dark))
	}
	if options[model.scopeCursor].scope.Kind == domain.SoftwareScopeClients {
		lines = append(lines, "")
		start, end := listWindow(len(model.catalog.Clients), model.clientCursor, max(2, context.height-ctaRows(context.height)-len(lines)-16))
		for index := start; index < end; index++ {
			name := model.catalog.Clients[index]
			checked := "[ ]"
			if model.clients[name] {
				checked = "[x]"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%s %s", checked, name), index == model.clientCursor, context.dark))
		}
	}
	lines = append(lines, "", "Client computers can be switched off while you save these choices.")
	return lines
}

func (model softwareModel) reviewView(context softwareViewContext) []string {
	plan := model.plan
	item := model.item(plan.Request.Package)
	title, now, later := "Add "+item.Label+"?", "the software choices are saved", "Update computers installs it on the client computers"
	if !plan.Request.Present {
		title, later = "Remove "+item.Label+"?", "Update computers removes it from the client computers"
	} else if model.changingScope {
		title = "Change where " + item.Label + " is installed?"
	}
	if plan.AffectedController != "" {
		now += " and this controller (" + plan.AffectedController + ") is rebuilt"
	}
	if len(plan.AffectedClients) == 0 {
		later = "no client computer needs updating"
	}
	lines := []string{tuiTitle(title, context.dark)}
	if item.ID != item.Label {
		lines = append(lines, tuiMuted("Package "+item.ID, context.dark))
	}
	lines = append(lines, "")
	lines = append(lines, tuiFields(context.dark,
		[2]string{"Installed on", softwareScopeLabel(plan.Request.Scope)},
		[2]string{"Client computers", countNoun(len(plan.AffectedClients), "computer")},
		[2]string{"Now", now},
		[2]string{"Later", later},
	)...)
	lines = append(lines, "", tuiStatus("Available in this laboratory’s software versions", tuiStatusSuccess, context.dark))
	if plan.AffectedController != "" && !plan.Request.Present {
		lines = append(lines, "", tuiStatus("Saving removes it from this controller right away", tuiStatusAttention, context.dark))
	}
	return lines
}

func (model softwareModel) resultView(context softwareViewContext) []string {
	result := model.result
	verified := controllerVerifiedForSave(result.Revision, context.controllerResult)
	title := "Software save needs attention"
	if result.State == "saved" && !result.HasErrors() && !result.RecoveryRequired {
		title = "Software configuration saved"
		if result.AffectedController != "" && verified {
			title = "Software is ready on this controller"
		} else if result.AffectedController != "" {
			title = "Software saved; controller needs attention"
		}
	} else if result.State == "unchanged" {
		title = "Software choices are already saved"
	}
	success := result.State == "unchanged" ||
		(result.State == "saved" && !result.HasErrors() && !result.RecoveryRequired && (result.AffectedController == "" || verified))
	lines := []string{tuiResult(title, success, context.dark), ""}
	lines = append(lines, saveStatusLines(result.State, result.RecoveryRequired, result.AffectedController != "", len(result.AffectedClients) > 0, verified)...)
	if model.profileResult.Operation != "" {
		lines = append(lines, "", fmt.Sprintf("%s: %s added; existing destinations kept.", model.profileResult.Preset.Label, countNoun(len(model.profileResult.Additions), "package")))
	}
	if result.RecoveryRequired || result.State == "partial" {
		lines = append(lines, "", result.Message, "No system was built or deployed.", "Retry completes the local save without duplicating the change.", softwareResultIssue(result))
	} else if result.State == "saved" {
		if result.AffectedController != "" && !verified {
			lines = append(lines, "", "The configuration remains saved; retrying the controller does not duplicate it.")
		} else if len(result.AffectedClients) > 0 {
			lines = append(lines, "", "The computers get this change when they are updated. Nothing has changed on them yet.")
		} else {
			lines = append(lines, "", "No client deployment is required.")
		}
	} else if result.State == "unchanged" {
		lines = append(lines, "", "No file changed; live system state has not been checked.")
	} else if result.HasErrors() {
		lines = append(lines, "", result.Message, softwareResultIssue(result))
	}
	return lines
}

func softwareResultIssue(result domain.SoftwareChangeApplyReport) string {
	if len(result.Issues) == 0 {
		return "Technical detail unavailable."
	}
	return "Technical detail: " + result.Issues[0].Message
}

func softwareModeTabs(mode softwareListMode, dark bool) string {
	labels := []string{"Configured", "Search", "Suggested"}
	parts := make([]string, len(labels))
	for index, label := range labels {
		label = menuTitle(fmt.Sprintf("F%d", index+2), label)
		// The page title below is the violet heading; the current tab is
		// marked by weight and a marker, also without colour.
		if index == int(mode) {
			parts[index] = tuiSection("▸ "+label, dark)
		} else {
			parts[index] = tuiMuted("  "+label, dark)
		}
	}
	return strings.Join(parts, "   ")
}

func softwareAvailabilityLabel(availability string) string {
	switch availability {
	case "blocked-broken":
		return "Unavailable: this package version is marked as broken"
	case "blocked-insecure":
		return "Unavailable: known security problems"
	case "blocked-unfree":
		return "Unavailable: restricted by this laboratory’s license settings"
	default:
		return "Unavailable for this laboratory’s system"
	}
}

func softwareAvailabilityMessage(item domain.SoftwareCatalogItem) string {
	switch item.Availability {
	case "blocked-broken":
		return item.Label + " is marked as broken in this laboratory’s package version. Choose another package."
	case "blocked-insecure":
		return item.Label + " has known security problems and is blocked. Choose another package."
	case "blocked-unfree":
		return item.Label + " is blocked by this laboratory’s license settings."
	default:
		return item.Label + " is not available for this deployment platform."
	}
}

// countNoun writes "1 client" or "3 clients".
func countNoun(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// softwareScopeLabel is the one wording for where software applies, used in
// lists, details, choices and reviews.
func softwareScopeLabel(scope domain.SoftwareScope) string {
	switch scope.Kind {
	case domain.SoftwareScopeShared:
		return "this controller and all clients, including future ones"
	case domain.SoftwareScopeController:
		return "only this controller"
	case domain.SoftwareScopeAllClients:
		return "all clients, including future ones"
	case domain.SoftwareScopeGroup:
		return "group " + scope.Group
	case domain.SoftwareScopeClients:
		if len(scope.Clients) > 0 && len(scope.Clients) <= 3 {
			return strings.Join(scope.Clients, ", ")
		}
		return countNoun(len(scope.Clients), "chosen client")
	}
	return "unknown destination"
}

func softwareCatalogItemForView(items []domain.SoftwareCatalogItem, id string) (domain.SoftwareCatalogItem, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return domain.SoftwareCatalogItem{ID: id, Label: id}, false
}
