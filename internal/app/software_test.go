package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSoftwareSource struct {
	definition    domain.SoftwareDefinition
	data          []byte
	definitionErr error
	validationErr error
	writeErr      error
	validations   int
	writes        int
}

func (f *fakeSoftwareSource) SearchSoftwarePackages(_ context.Context, _ string, query string, _ int) ([]domain.SoftwareCatalogItem, error) {
	result := []domain.SoftwareCatalogItem{}
	for _, item := range f.definition.Catalog {
		if strings.Contains(strings.ToLower(item.ID), strings.ToLower(query)) {
			result = append(result, item)
		}
	}
	if strings.Contains("hello", strings.ToLower(query)) {
		result = append(result, domain.SoftwareCatalogItem{ID: "hello", Label: "hello", Summary: "A friendly greeting program", Version: "2.12", Availability: "available"})
	}
	return result, nil
}

func (f *fakeSoftwareSource) ResolveSoftwarePackage(_ context.Context, _ string, packageID string) (domain.SoftwareCatalogItem, error) {
	for _, item := range f.definition.Catalog {
		if item.ID == packageID {
			return item, nil
		}
	}
	if packageID == "hello" {
		return domain.SoftwareCatalogItem{ID: "hello", Label: "hello", Summary: "A friendly greeting program", Version: "2.12", Availability: "available"}, nil
	}
	return domain.SoftwareCatalogItem{}, errors.New("package not found")
}

func (f *fakeSoftwareSource) SoftwareDefinition(context.Context, string) (domain.SoftwareDefinition, error) {
	return f.definition, f.definitionErr
}
func (f *fakeSoftwareSource) ReadSoftware(string) ([]byte, error) {
	return append([]byte(nil), f.data...), nil
}
func (f *fakeSoftwareSource) ValidateSoftwareCandidate(context.Context, string, domain.LabSoftwareFile) error {
	f.validations++
	return f.validationErr
}
func (f *fakeSoftwareSource) WriteSoftwareIfUnchanged(_ string, expected []byte, software domain.LabSoftwareFile) error {
	f.writes++
	if f.writeErr != nil {
		return f.writeErr
	}
	if string(expected) != string(f.data) {
		return domain.ErrSoftwareConflict
	}
	data, err := domain.MarshalLabSoftware(software)
	if err != nil {
		return err
	}
	f.data = data
	return nil
}

func softwareManagerFixture(t *testing.T) (*fakeSoftwareSource, SoftwareManager) {
	t.Helper()
	data, err := domain.MarshalLabSoftware(domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{}})
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeSoftwareSource{data: data, definition: domain.SoftwareDefinition{
		SchemaVersion: domain.SoftwareSchemaVersion,
		ManagedFile:   "lab-software.json",
		Clients:       []string{"pc01", "pc02"},
		Groups:        map[string][]string{"graphics": {"pc02"}},
		Catalog: []domain.SoftwareCatalogItem{
			{ID: "vlc", Label: "VLC", Summary: "Play media", Availability: "available"},
			{ID: "gimp", Label: "GIMP", Summary: "Edit images", Availability: "available"},
		},
		Packages: []domain.SoftwareDeclaration{},
	}}
	return source, NewSoftwareManager(source)
}

func TestSoftwarePlanUsesCatalogAndSeparatesConfigurationFromDistribution(t *testing.T) {
	source, manager := softwareManagerFixture(t)
	report := manager.Plan(context.Background(), "/deployment", domain.SoftwareChangeRequest{
		Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	if report.HasErrors() || report.State != "ready" || source.validations != 1 || len(report.AffectedClients) != 2 || report.ReviewToken == "" || !strings.HasPrefix(report.Confirmation, "SAVE SOFTWARE ") {
		t.Fatalf("software plan = %+v validations=%d", report, source.validations)
	}
	if !strings.Contains(report.Message, "no system has been built or changed") {
		t.Fatalf("plan confused declaration with effects: %s", report.Message)
	}
}

func TestSoftwareSearchAndPlanAcceptPackageOutsideSuggestions(t *testing.T) {
	source, manager := softwareManagerFixture(t)
	search := manager.Search(context.Background(), "/deployment", "hell")
	if search.HasErrors() || search.State != "ready" || len(search.Results) != 1 || search.Results[0].ID != "hello" || search.Results[0].Version != "2.12" {
		t.Fatalf("software search = %+v", search)
	}
	plan := manager.Plan(context.Background(), "/deployment", domain.SoftwareChangeRequest{
		Package: "hello", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	if plan.HasErrors() || plan.State != "ready" || source.validations != 1 {
		t.Fatalf("outside-suggestion plan = %+v validations=%d", plan, source.validations)
	}
}

func TestSoftwareRemovalDoesNotRequirePackageToRemainResolvable(t *testing.T) {
	source, manager := softwareManagerFixture(t)
	configured, err := domain.MarshalLabSoftware(domain.LabSoftwareFile{
		SchemaVersion: domain.SoftwareSchemaVersion,
		Packages:      []domain.SoftwareDeclaration{{Package: "retired-package", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source.data = configured
	source.definition.Packages = []domain.SoftwareDeclaration{{Package: "retired-package", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Origin: "managed"}}
	plan := manager.Plan(context.Background(), "/deployment", domain.SoftwareChangeRequest{Package: "retired-package", Present: false})
	if plan.HasErrors() || plan.State != "ready" || len(plan.Candidate.Packages) != 0 {
		t.Fatalf("retired package removal = %+v", plan)
	}
}

func TestSoftwareSearchRejectsAmbiguousOrShortQueries(t *testing.T) {
	_, manager := softwareManagerFixture(t)
	for _, query := range []string{"", "x", "hello world", "../hello"} {
		report := manager.Search(context.Background(), "/deployment", query)
		if !report.HasErrors() || report.State != "invalid" {
			t.Fatalf("query %q accepted: %+v", query, report)
		}
	}
}

func TestSoftwarePlanRejectsUncataloguedPackageAndUnknownScope(t *testing.T) {
	for _, request := range []domain.SoftwareChangeRequest{
		{Package: "curl;rm", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}},
		{Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeClients, Clients: []string{"pc99"}}},
		{Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: "missing"}},
	} {
		source, manager := softwareManagerFixture(t)
		report := manager.Plan(context.Background(), "/deployment", request)
		if !report.HasErrors() || source.validations != 0 || source.writes != 0 {
			t.Fatalf("unsafe request was evaluated or written: %+v", report)
		}
	}
}

func TestSoftwareCatalogRejectsMalformedEvaluatedContract(t *testing.T) {
	for _, mutate := range []func(*domain.SoftwareDefinition){
		func(definition *domain.SoftwareDefinition) { definition.Groups["../graphics"] = []string{"pc01"} },
		func(definition *domain.SoftwareDefinition) { definition.Groups["graphics"] = []string{"pc02", "pc02"} },
		func(definition *domain.SoftwareDefinition) {
			entry := domain.SoftwareDeclaration{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Origin: "managed"}
			definition.Packages = []domain.SoftwareDeclaration{entry, entry}
		},
	} {
		source, manager := softwareManagerFixture(t)
		mutate(&source.definition)
		report := manager.Catalog(context.Background(), "/deployment")
		if !report.HasErrors() || report.State != "failed" {
			t.Fatalf("malformed software contract accepted: %+v", report)
		}
	}
}

func TestSoftwareApplyReplansAndWritesOnlyReviewedCandidate(t *testing.T) {
	source, manager := softwareManagerFixture(t)
	plan := manager.Plan(context.Background(), "/deployment", domain.SoftwareChangeRequest{
		Package: "gimp", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: "graphics"},
	})
	report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if report.HasErrors() || report.State != "applied" || source.writes != 1 || len(report.AffectedClients) != 1 || report.AffectedClients[0] != "pc02" {
		t.Fatalf("software apply = %+v writes=%d", report, source.writes)
	}
	stored, err := domain.DecodeLabSoftware(source.data)
	if err != nil || len(stored.Packages) != 1 || stored.Packages[0].Package != "gimp" {
		t.Fatalf("stored software = %+v error=%v", stored, err)
	}
}

func TestSoftwareApplyRejectsReviewAndFileDrift(t *testing.T) {
	for _, mutate := range []func(*fakeSoftwareSource, *domain.SoftwareChangePlanReport){
		func(_ *fakeSoftwareSource, plan *domain.SoftwareChangePlanReport) { plan.ReviewToken = "sha256:wrong" },
		func(source *fakeSoftwareSource, _ *domain.SoftwareChangePlanReport) {
			source.data = []byte("{\n  \"schemaVersion\": 1,\n  \"packages\": [\n    {\"package\":\"vlc\",\"scope\":{\"kind\":\"all-clients\"}}\n  ]\n}\n")
		},
	} {
		source, manager := softwareManagerFixture(t)
		plan := manager.Plan(context.Background(), "/deployment", domain.SoftwareChangeRequest{Package: "gimp", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}})
		mutate(source, &plan)
		report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
		if report.State != "conflict" || source.writes != 0 {
			t.Fatalf("stale plan was applied: %+v writes=%d", report, source.writes)
		}
	}
}

func TestSoftwareApplySurfacesAtomicWriterConflict(t *testing.T) {
	source, manager := softwareManagerFixture(t)
	plan := manager.Plan(context.Background(), "/deployment", domain.SoftwareChangeRequest{Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}})
	source.writeErr = errors.New("read-only deployment")
	report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if !report.HasErrors() || !strings.Contains(report.Message, "not saved") {
		t.Fatalf("writer error = %+v", report)
	}
}

func TestSoftwareApplyReportsUncertainDurabilityAsPartial(t *testing.T) {
	source, manager := softwareManagerFixture(t)
	plan := manager.Plan(context.Background(), "/deployment", domain.SoftwareChangeRequest{Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}})
	source.writeErr = domain.ErrSoftwareDurability
	report := manager.ApplyPlan(context.Background(), plan, plan.ReviewToken)
	if !report.HasErrors() || report.State != "partial" || !strings.Contains(report.Message, "Inspect the file and Git state") {
		t.Fatalf("durability error = %+v", report)
	}
}
