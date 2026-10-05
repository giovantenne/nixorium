package presentation

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Extension search reuses the package search of the deployment's pinned
// package set; "vscode-extensions.<text>" matches publisher.name everywhere.
const workspaceExtensionPrefix = "vscode-extensions."

type dashboardWorkspaceSearchMsg struct {
	id     uint64
	query  string
	report domain.SoftwareSearchReport
}

// dashboardWorkspaceMarketplaceMsg carries one proposal (add) or one result
// per existing pin (update check).
type dashboardWorkspaceMarketplaceMsg struct {
	id      uint64
	update  bool
	reports []domain.WorkspaceMarketplaceReport
}

// workspaceMarketplaceField is edited together with the extension list.
var workspaceMarketplaceField = workspaceField{group: "VSCode", label: "Marketplace pins", path: []string{"vscode", "marketplace"}, kind: "marketplace"}

func workspaceQueryCharacter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(".+_-", r)
}

// editSearch edits the query; Enter is handled by the dashboard, which owns
// the bounded read that runs the search.
func (model *workspaceModel) editSearch(key tea.KeyPressMsg) string {
	switch key.String() {
	case "esc":
		model.searching, model.query = false, ""
		return "Search closed; the selection is unchanged."
	case "backspace":
		if model.query != "" {
			model.query = model.query[:len(model.query)-1]
		}
	default:
		for _, r := range key.Text {
			if r == ' ' {
				r = '-'
			}
			if workspaceQueryCharacter(r) && len(model.query) < 64 {
				model.query += string(r)
			}
		}
	}
	return ""
}

func (model dashboardModel) startWorkspaceSearch() (tea.Model, tea.Cmd) {
	if model.workspace.searchMode == "marketplace" {
		return model.startWorkspaceMarketplace([]string{model.workspace.query}, false)
	}
	query := strings.Trim(strings.ToLower(model.workspace.query), ".")
	if model.actions.SearchSoftware == nil {
		model.message = "Extension search is not available in this session."
		return model, nil
	}
	if len(query) < 2 {
		model.message = "Type at least two characters of the extension name."
		return model, nil
	}
	model.workspace.cancelRead()
	ctx, activityID := model.beginRead(dashboardReadTimeout)
	model.workspace.cancel = model.read.cancel
	id := model.workspace.requestID
	model.message = ""
	model.busy = "Searching extensions available for this laboratory"
	search := model.actions.SearchSoftware
	return model, boundedReadCommand(ctx, activityID, func(ctx context.Context) tea.Msg {
		return dashboardWorkspaceSearchMsg{id: id, query: query, report: search(ctx, workspaceExtensionPrefix+query)}
	})
}

// startWorkspaceMarketplace downloads into the controller store only. The
// bounded read has the build limit because a package may be large.
func (model dashboardModel) startWorkspaceMarketplace(ids []string, update bool) (tea.Model, tea.Cmd) {
	if model.actions.ResolveMarketplace == nil {
		model.message = "Marketplace extensions are not available in this session."
		return model, nil
	}
	if !update {
		id, err := domain.NormalizeWorkspaceMarketplaceID(ids[0])
		if err != nil {
			model.message = err.Error()
			return model, nil
		}
		ids = []string{id}
	}
	model.workspace.cancelRead()
	ctx, activityID := model.beginRead(dashboardBuildTimeout)
	model.workspace.cancel = model.read.cancel
	id := model.workspace.requestID
	model.message = ""
	model.busy = "Asking the Marketplace and downloading into the controller's Nix store"
	resolve := model.actions.ResolveMarketplace
	return model, boundedReadCommand(ctx, activityID, func(ctx context.Context) tea.Msg {
		reports := []domain.WorkspaceMarketplaceReport{}
		for _, extension := range ids {
			reports = append(reports, resolve(ctx, extension))
		}
		return dashboardWorkspaceMarketplaceMsg{id: id, update: update, reports: reports}
	})
}

func (model dashboardModel) finishWorkspaceMarketplace(message dashboardWorkspaceMarketplaceMsg) (tea.Model, tea.Cmd) {
	w := &model.workspace
	if model.screen != dashboardWorkspace || message.id != w.requestID || w.stage != workspaceFieldEdit || w.field.kind != "extensions" {
		return model, nil
	}
	w.cancelRead()
	model.busy = ""
	failure := func(report domain.WorkspaceMarketplaceReport) string {
		items := []string{}
		for _, issue := range report.Issues {
			items = append(items, issue.Message)
		}
		return report.ID + ": " + strings.Join(items, " ")
	}
	if !message.update {
		report := message.reports[0]
		w.searching, w.query = false, ""
		if report.HasErrors() {
			model.message = "Nothing was added. " + failure(report)
			return model, nil
		}
		w.pending = report.Candidate
		return model, nil
	}
	lines := []string{}
	for _, report := range message.reports {
		switch {
		case report.HasErrors():
			lines = append(lines, failure(report))
		case *w.marketplace[report.ID].Version == *report.Candidate.Entry.Version:
			lines = append(lines, report.ID+" is current ("+*report.Candidate.Entry.Version+")")
		default:
			lines = append(lines, report.ID+": "+*w.marketplace[report.ID].Version+" → "+*report.Candidate.Entry.Version)
			w.marketplace[report.ID] = report.Candidate.Entry
			w.choiceNotes[report.ID] = "Marketplace " + *report.Candidate.Entry.Version
		}
	}
	model.message = strings.Join(lines, "; ") + ". Keep the draft, then review and save to use newer versions."
	return model, nil
}

// confirmMarketplace adds a reviewed download to the draft selection.
func (model *workspaceModel) confirmMarketplace(key tea.KeyPressMsg) string {
	switch key.String() {
	case "esc":
		model.pending = nil
		return "Not added; the draft is unchanged. The download stays in the Nix store until garbage collection."
	case "enter":
		entry := model.pending.Entry
		id := entry.ID()
		model.marketplace[id] = entry
		model.choiceNotes[id] = "Marketplace " + *entry.Version
		if !slices.Contains(model.choices, id) {
			model.choices = append(model.choices, id)
		}
		if !slices.Contains(model.selected, id) {
			model.selected = append(model.selected, id)
		}
		model.inherit = false
		model.choice = slices.Index(model.choices, id)
		missing := []string{}
		for _, dependency := range model.pending.Dependencies {
			if !slices.Contains(model.selected, dependency) {
				missing = append(missing, dependency)
			}
		}
		model.pending = nil
		message := id + " selected at this exact version; press Enter to keep it in the draft."
		if len(missing) != 0 {
			message += " Also select what it depends on: " + strings.Join(missing, ", ") + "."
		}
		return message
	}
	return ""
}

func (w workspaceModel) marketplaceView(width int) []string {
	c := w.pending
	lines := []string{
		"Marketplace extension (not yet in the draft)",
		"",
		workspaceShort(c.Entry.ID()+"  "+*c.Entry.Version+"  "+c.DisplayName, width),
		workspaceShort(c.Description, width),
		"",
		"Editor requirement: " + safeWorkspaceText(c.Engine) + " · configured VS Code " + safeWorkspaceText(c.EditorVersion),
	}
	if c.Entry.Platform != nil {
		lines = append(lines, "Package for: "+*c.Entry.Platform)
	}
	if len(c.Dependencies) != 0 {
		lines = append(lines, workspaceShort("Needs extensions: "+strings.Join(c.Dependencies, ", "), width))
	}
	if len(c.Pack) != 0 {
		lines = append(lines, workspaceShort("Recommends (pack): "+strings.Join(c.Pack, ", "), width))
	}
	lines = append(lines, "", "This exact extension version will run as the student. It is third-party code, not tested by Nixorium.")
	if c.Native {
		lines = append(lines, "Contains native programs that are not adapted to NixOS: they often fail to start.")
	}
	lines = append(lines, "Try it on one computer before the whole classroom.")
	return lines
}

func (model dashboardModel) finishWorkspaceSearch(message dashboardWorkspaceSearchMsg) (tea.Model, tea.Cmd) {
	w := &model.workspace
	if model.screen != dashboardWorkspace || message.id != w.requestID || w.stage != workspaceFieldEdit || w.field.kind != "extensions" {
		return model, nil
	}
	w.cancelRead()
	model.busy = ""
	if message.report.HasErrors() {
		items := []string{"The extension search failed; the selection is unchanged."}
		for _, issue := range message.report.Issues {
			items = append(items, issue.Message)
		}
		model.message = strings.Join(items, " ")
		return model, nil
	}
	found := []string{}
	for _, item := range message.report.Results {
		id, ok := strings.CutPrefix(item.ID, workspaceExtensionPrefix)
		// Only extensions selectable by one lowercase publisher.name identifier.
		if !ok || item.Availability != "available" || strings.Count(id, ".") != 1 || strings.ToLower(id) != id {
			continue
		}
		found = append(found, id)
		w.choiceNotes[id] = strings.TrimSpace(item.Version + " · " + item.Summary)
		if !slices.Contains(w.choices, id) {
			w.choices = append(w.choices, id)
		}
	}
	w.searching, w.query = false, ""
	if len(found) == 0 {
		model.message = fmt.Sprintf("No extension matching %q is available in this laboratory’s package versions.", message.query)
		return model, nil
	}
	w.choice = slices.Index(w.choices, found[0])
	model.message = fmt.Sprintf("%s matching %q added to the list. Space selects.", countNoun(len(found), "packaged extension"), message.query)
	return model, nil
}
