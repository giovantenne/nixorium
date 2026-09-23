package presentation

import (
	"context"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSoftwareModelOwnsCatalogNavigation(t *testing.T) {
	model := softwareModel{
		catalog: domain.SoftwareCatalogReport{
			Controller: "pc99",
			Clients:    []string{"pc01", "pc02"},
			Packages: []domain.SoftwareDeclaration{{
				Package: "gimp",
				Scope:   domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
			}},
		},
	}

	items := model.items()
	if len(items) != 1 || items[0].ID != "gimp" {
		t.Fatalf("configured items = %#v", items)
	}
	options := model.scopeOptions()
	if len(options) != 4 || options[0].scope.Kind != domain.SoftwareScopeShared || options[1].scope.Kind != domain.SoftwareScopeController {
		t.Fatalf("scope options = %#v", options)
	}

	model = model.changeMode(1)
	if model.mode != softwareSearch || !model.searching || model.cursor != 0 {
		t.Fatalf("search mode = %#v", model)
	}
	model = model.changeMode(1)
	if model.mode != softwareSuggested || model.searching {
		t.Fatalf("suggested mode = %#v", model)
	}
}

func TestSoftwareModelSchedulesOnlyValidSearch(t *testing.T) {
	model := softwareModel{query: "p"}
	calls := 0
	search := func(context.Context, string) domain.SoftwareSearchReport {
		calls++
		return domain.SoftwareSearchReport{}
	}
	if command := model.scheduleSearch(search); command != nil || calls != 0 || model.searchBusy {
		t.Fatalf("short query scheduled search: command=%v calls=%d busy=%t", command != nil, calls, model.searchBusy)
	}

	model.query = "python3Packages.num"
	if command := model.scheduleSearch(search); command == nil || calls != 0 || !model.searchBusy || model.searchID == 0 {
		t.Fatalf("valid query not scheduled: command=%v calls=%d busy=%t id=%d", command != nil, calls, model.searchBusy, model.searchID)
	}
}
