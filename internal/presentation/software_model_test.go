package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

func TestSoftwareModelRendersItsOwnWorkflowStage(t *testing.T) {
	model := softwareModel{
		stage: softwareReview,
		catalog: domain.SoftwareCatalogReport{Catalog: []domain.SoftwareCatalogItem{{
			ID: "gimp", Label: "GIMP", Availability: "available",
		}}},
		plan: domain.SoftwareChangePlanReport{
			ManagedFile: "lab-software.json",
			Request: domain.SoftwareChangeRequest{
				Package: "gimp", Present: true,
				Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
			},
			AffectedClients: []string{"pc01"},
		},
	}

	shell := model.view(softwareViewContext{width: 80, height: 24})
	if strings.Join(shell.path, "/") != "Software/Review" {
		t.Fatalf("path = %#v", shell.path)
	}
	if !strings.Contains(shell.body, "Package checked against this laboratory’s software versions") {
		t.Fatalf("review body = %q", shell.body)
	}
	if len(shell.actions) < 2 || shell.actions[0].label != "Save" || shell.actions[1].label != "Computers" {
		t.Fatalf("review actions = %#v", shell.actions)
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

func TestSoftwareModelEmitsTypedIntentWithoutExecutingActions(t *testing.T) {
	model := softwareModel{
		stage: softwareCatalog,
		mode:  softwareSuggested,
		catalog: domain.SoftwareCatalogReport{Catalog: []domain.SoftwareCatalogItem{{
			ID: "gimp", Label: "GIMP", Availability: "available",
		}}},
	}

	model, intent := model.update(keyPress("enter"))
	if intent.kind != softwareNoIntent || model.stage != softwareScope || model.selected != "gimp" {
		t.Fatalf("catalog transition: model=%#v intent=%#v", model, intent)
	}
	model, intent = model.update(keyPress("enter"))
	if intent.kind != softwarePlanIntent || intent.request.Package != "gimp" || !intent.request.Present {
		t.Fatalf("scope transition: model=%#v intent=%#v", model, intent)
	}

	model.stage = softwareReview
	model.plan = domain.SoftwareChangePlanReport{Request: intent.request}
	_, intent = model.update(keyPress("enter"))
	if intent.kind != softwareSaveIntent {
		t.Fatalf("review intent = %#v", intent)
	}
}

func keyPress(value string) tea.KeyPressMsg {
	if value == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Text: value}
}

// Catalog loading is a read, not an action needing a success/warning notice.
// Failures must still reach the operator instead of looking like an empty list.
func TestSoftwareCatalogReadShowsOnlyFailuresAsNotices(t *testing.T) {
	m := softwareModel{}
	_, result := m.loadCatalog(domain.SoftwareCatalogReport{
		State: "ready", Message: "Supported software was resolved from the pinned package set.",
	})
	if !result.accepted || result.message != "" {
		t.Fatalf("successful read produced a notice: %+v", result)
	}
	failure := domain.SoftwareCatalogReport{State: "failed", Message: "Cannot load the software configuration.",
		Issues: []domain.ValidationIssue{{Field: "source", Message: "Unavailable"}},
	}
	_, result = m.loadCatalog(failure)
	if !result.accepted || result.message != failure.Message {
		t.Fatalf("read failure was hidden: %+v", result)
	}
}

func TestConfiguredPackageEnterOpensDetailsWithoutRequestingAChange(t *testing.T) {
	m := softwareModel{mode: softwareConfigured, catalog: domain.SoftwareCatalogReport{
		Packages: []domain.SoftwareDeclaration{{Package: "ghostty", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}}},
	}}
	m, intent := m.update(keyPress("enter"))
	if m.stage != softwareDetails || m.selected != "ghostty" || intent.kind != softwareNoIntent {
		t.Fatalf("opening details requested a change: stage=%v intent=%+v", m.stage, intent)
	}
	body := strings.Join(m.detailsView(softwareViewContext{}), "\n")
	if strings.Contains(body, "Managed package from") || !strings.Contains(body, "Review removing") || !strings.Contains(body, "all current or future clients") {
		t.Fatalf("details do not explain destinations/actions: %s", body)
	}
}
