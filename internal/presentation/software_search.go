package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

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

func (model softwareModel) updateSearchInput(key tea.KeyPressMsg, search func(context.Context, string) domain.SoftwareSearchReport) (softwareModel, softwareSearchInput) {
	if !model.searching {
		return model, softwareSearchInput{}
	}
	changed := false
	switch key.String() {
	case "f2", "f3", "f4":
		target := int(key.String()[1] - '2')
		return model.changeMode(target - int(model.mode)), softwareSearchInput{handled: true, clearMessage: true}
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
		if query := dropLastRune(model.query); query != model.query {
			model.query = query
			changed = true
		}
	default:
		if query := appendInputText(model.query, key.Text, softwareQueryMax); query != model.query {
			model.query = query
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
