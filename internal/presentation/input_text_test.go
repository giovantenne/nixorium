package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAppendInputTextKeepsPrintableCharactersWithinTheLimit(t *testing.T) {
	if got := appendInputText("ab", "c\nd\te\u200bf", 10); got != "abcdef" {
		t.Fatalf("control and invisible characters kept: %q", got)
	}
	if got := appendInputText("abc", "defgh", 5); got != "abcde" {
		t.Fatalf("limit ignored: %q", got)
	}
	if got := appendInputText("a", "é", 2); got != "a" {
		t.Fatalf("a character cut across the limit: %q", got)
	}
	if got := dropLastRune("caffè"); got != "caff" {
		t.Fatalf("last character not removed whole: %q", got)
	}
}

func TestPasteGoesIntoTheSendFilesPath(t *testing.T) {
	model := newTestModel(t, withReadyLab(), withActions(DashboardActions{
		PlanShare:  func(context.Context, string, string) domain.SharePlan { return domain.SharePlan{} },
		ApplyShare: func(domain.SharePlan) domain.ShareReport { return domain.ShareReport{} },
	}))
	opened, _ := model.openComputerTask("s")
	model = opened.(dashboardModel)
	model.share.path = ""
	updated, _ := model.Update(tea.PasteMsg{Content: "  /home/teacher/Lesson plans/q?\n"})
	model = updated.(dashboardModel)
	if model.share.path != "/home/teacher/Lesson plans/q?" {
		t.Fatalf("pasted path %q", model.share.path)
	}
	// Past the path step a paste is ignored.
	model.share.stage = shareStageComputers
	updated, _ = model.Update(tea.PasteMsg{Content: "extra"})
	if got := updated.(dashboardModel).share.path; got != "/home/teacher/Lesson plans/q?" {
		t.Fatalf("paste changed the path outside its step: %q", got)
	}
}

func TestPasteGoesIntoTheComputerSearch(t *testing.T) {
	model := newTestModel(t, withReadyLab())
	model.screen = dashboardHosts
	model.computers.hostSearching = true
	model.computers.hostCursor = 3
	updated, _ := model.Update(tea.PasteMsg{Content: "pc0\n1"})
	model = updated.(dashboardModel)
	if model.computers.hostQuery != "pc01" || model.computers.hostCursor != 0 {
		t.Fatalf("search %q cursor %d", model.computers.hostQuery, model.computers.hostCursor)
	}
	model.computers.hostSearching = false
	model.computers.hostQuery = ""
	updated, _ = model.Update(tea.PasteMsg{Content: "pc01"})
	if got := updated.(dashboardModel).computers.hostQuery; got != "" {
		t.Fatalf("paste reached a closed search: %q", got)
	}
}

func TestPasteGoesIntoTheSoftwareSearchAndSearchesOnce(t *testing.T) {
	model := newTestModel(t, withReadyLab(), withActions(DashboardActions{
		SearchSoftware: func(context.Context, string) domain.SoftwareSearchReport { return domain.SoftwareSearchReport{} },
	}))
	model.screen = dashboardSoftware
	model.software.mode = softwareSearch
	model.software.searching = true
	updated, command := model.Update(tea.PasteMsg{Content: "inkscape"})
	model = updated.(dashboardModel)
	if model.software.query != "inkscape" || command == nil {
		t.Fatalf("query %q, search scheduled %v", model.software.query, command != nil)
	}
	updated, _ = model.Update(tea.PasteMsg{Content: strings.Repeat("x", 200)})
	model = updated.(dashboardModel)
	if len(model.software.query) != softwareQueryMax {
		t.Fatalf("query length %d", len(model.software.query))
	}
	updated, command = model.Update(tea.PasteMsg{Content: "more"})
	if command != nil || updated.(dashboardModel).software.query != model.software.query {
		t.Fatal("a paste that adds nothing started another search")
	}
}

func TestPasteGoesIntoTheKeyPathAndNetworkAddress(t *testing.T) {
	model := newTestModel(t, withReadyLab())
	model.screen = dashboardSetupKeys
	model.keys.importing = true
	updated, _ := model.Update(tea.PasteMsg{Content: "/home/admin/.ssh/id_ed25519\n"})
	model = updated.(dashboardModel)
	if model.keys.path != "/home/admin/.ssh/id_ed25519" {
		t.Fatalf("key path %q", model.keys.path)
	}

	model = newTestModel(t, withReadyLab())
	model.screen = dashboardNetworkChange
	model.networkChange.loaded = true
	updated, _ = model.Update(tea.PasteMsg{Content: "10.42.0.0/24 abc"})
	model = updated.(dashboardModel)
	if model.networkChange.input != "10.42.0.0/24" {
		t.Fatalf("network address %q", model.networkChange.input)
	}
}
