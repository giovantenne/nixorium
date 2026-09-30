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
	model.busy = "Searching editor extensions in the pinned package set"
	search := model.actions.SearchSoftware
	return model, boundedReadCommand(ctx, activityID, func(ctx context.Context) tea.Msg {
		return dashboardWorkspaceSearchMsg{id: id, query: query, report: search(ctx, workspaceExtensionPrefix+query)}
	})
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
		model.message = fmt.Sprintf("No packaged extension matches %q in the pinned package set.", message.query)
		return model, nil
	}
	w.choice = slices.Index(w.choices, found[0])
	model.message = fmt.Sprintf("%d packaged extension(s) match %q and were added to the list. Space selects.", len(found), message.query)
	return model, nil
}
