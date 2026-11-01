package presentation

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Single-line fields written by hand (the Send files path, the computer and
// software searches, the key path, the network address) share these helpers,
// so a typed key and a pasted text are filtered the same way.

// appendInputText adds the printable characters of text to value without
// growing it past limit bytes. Control characters, line breaks and invisible
// format characters are dropped.
func appendInputText(value, text string, limit int) string {
	for _, character := range text {
		if len(value)+len(string(character)) > limit {
			break
		}
		if unicode.IsPrint(character) && !unicode.In(character, unicode.Cf) {
			value += string(character)
		}
	}
	return value
}

// dropLastRune removes the last character of value.
func dropLastRune(value string) string {
	characters := []rune(value)
	if len(characters) == 0 {
		return value
	}
	return string(characters[:len(characters)-1])
}

const (
	sharePathLimit   = 4096
	hostSearchLimit  = 128
	softwareQueryMax = 80
	setupKeyPathMax  = 4096
	networkInputMax  = 18
)

// pasteIntoInput inserts pasted text into the single-line field that
// currently accepts typing. It reports false when no such field is active, so
// the paste is ignored like any other key the screen does not use.
func (model dashboardModel) pasteIntoInput(paste tea.PasteMsg) (dashboardModel, tea.Cmd, bool) {
	text := strings.TrimSpace(paste.Content)
	switch {
	case model.screen == dashboardShare && model.share.stage == shareStagePath:
		model.share.path = appendInputText(model.share.path, text, sharePathLimit)
		model.message = ""
		return model, nil, true
	case model.screen == dashboardHosts && model.computers.hostSearching:
		model.computers.hostQuery = appendInputText(model.computers.hostQuery, text, hostSearchLimit)
		model.computers.hostCursor = 0
		return model, nil, true
	case model.screen == dashboardSoftware && model.software.searching:
		query := appendInputText(model.software.query, text, softwareQueryMax)
		if query == model.software.query {
			return model, nil, true
		}
		model.software.query = query
		model.software.cursor = 0
		return model, model.software.scheduleSearch(model.actions.SearchSoftware), true
	case model.screen == dashboardSetupKeys && model.keys.importing:
		model.keys.path = appendInputText(model.keys.path, text, setupKeyPathMax)
		return model, nil, true
	case model.screen == dashboardNetworkChange && model.networkChangeEditing():
		var digits strings.Builder
		for _, character := range text {
			if strings.ContainsRune("0123456789./", character) {
				digits.WriteRune(character)
			}
		}
		model.networkChange.input = appendInputText(model.networkChange.input, digits.String(), networkInputMax)
		return model, nil, true
	}
	return model, nil, false
}
