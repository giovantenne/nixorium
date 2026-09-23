package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func softwarePresetFixture(t *testing.T) (*fakeSoftwareSource, SoftwareManager) {
	t.Helper()
	source, manager := softwareManagerFixture(t)
	source.presets = &domain.SoftwarePresetCatalog{
		SchemaVersion: domain.SoftwarePresetSchemaVersion,
		DefaultPreset: "essential",
		Presets: []domain.SoftwarePreset{
			{ID: "essential", Label: "Essential", Description: "Common software", Packages: []string{"vlc", "gimp"}},
			{ID: "programming", Label: "Programming", Description: "Programming tools", Packages: []string{"gimp", "hello", "vlc"}},
		},
	}
	return source, manager
}

func TestSoftwarePresetsAreOptionalAndInvalidCatalogsFail(t *testing.T) {
	source, manager := softwareManagerFixture(t)
	report := manager.Presets(context.Background(), "/deployment")
	if report.HasErrors() || report.State != "unavailable" || report.Catalog != nil {
		t.Fatalf("absent optional catalog = %+v", report)
	}

	source.presets = &domain.SoftwarePresetCatalog{
		SchemaVersion: 99,
		DefaultPreset: "essential",
		Presets:       []domain.SoftwarePreset{{ID: "essential", Label: "Essential", Description: "Base", Packages: []string{"vlc"}}},
	}
	report = manager.Presets(context.Background(), "/deployment")
	if !report.HasErrors() || report.State != "failed" {
		t.Fatalf("invalid preset catalog accepted: %+v", report)
	}
}

func TestSoftwarePresetPlanAddsOneAtomicCandidateAndPreservesExistingScopes(t *testing.T) {
	source, manager := softwarePresetFixture(t)
	source.definition.Controller = "pc99"
	existing := domain.SoftwareDeclaration{Package: "vlc", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeClients, Clients: []string{"pc01"}}}
	source.definition.Packages = []domain.SoftwareDeclaration{{Package: existing.Package, Scope: existing.Scope, Origin: "managed"}}
	data, err := domain.MarshalLabSoftware(domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion, Packages: []domain.SoftwareDeclaration{existing}})
	if err != nil {
		t.Fatal(err)
	}
	source.data = data

	plan := manager.PlanPreset(context.Background(), "/deployment", domain.SoftwarePresetRequest{
		Preset: "programming",
		Scope:  domain.SoftwareScope{Kind: domain.SoftwareScopeShared},
	})
	if plan.HasErrors() || plan.State != "ready" || plan.ReviewToken == "" || plan.Confirmation != "ADD PROFILE" {
		t.Fatalf("preset plan = %+v", plan)
	}
	if source.validations != 1 || len(plan.SelectedPackages) != 3 || len(plan.Existing) != 1 || len(plan.Additions) != 2 || len(plan.Candidate.Packages) != 3 {
		t.Fatalf("preset aggregation = selected %d existing %d additions %d candidate %d validations %d", len(plan.SelectedPackages), len(plan.Existing), len(plan.Additions), len(plan.Candidate.Packages), source.validations)
	}
	if plan.Existing[0].Scope.Kind != domain.SoftwareScopeClients || plan.Existing[0].Scope.Clients[0] != "pc01" {
		t.Fatalf("existing scope changed: %+v", plan.Existing)
	}
	if plan.AffectedController != "pc99" || strings.Join(plan.AffectedClients, ",") != "pc01,pc02" {
		t.Fatalf("preset impact = controller %q clients %v", plan.AffectedController, plan.AffectedClients)
	}

	result := manager.ApplyPresetPlan(context.Background(), plan, plan.ReviewToken)
	if result.HasErrors() || result.State != "applied" || source.writes != 1 {
		t.Fatalf("preset apply = %+v writes=%d", result, source.writes)
	}
	stored, err := domain.DecodeLabSoftware(source.data)
	if err != nil || len(stored.Packages) != 3 {
		t.Fatalf("stored preset candidate = %+v error=%v", stored, err)
	}
	storedVLC, found := softwareDeclaration(stored.Packages, "vlc")
	if !found || storedVLC.Scope.Kind != domain.SoftwareScopeClients {
		t.Fatalf("stored existing declaration changed: %+v", storedVLC)
	}

	repeated := manager.ApplyPresetPlan(context.Background(), plan, plan.ReviewToken)
	if repeated.HasErrors() || repeated.State != "unchanged" || source.writes != 1 {
		t.Fatalf("repeated preset apply = %+v writes=%d", repeated, source.writes)
	}
}

func TestSoftwarePresetPlanSupportsExclusionsIncludingAllPackages(t *testing.T) {
	source, manager := softwarePresetFixture(t)
	plan := manager.PlanPreset(context.Background(), "/deployment", domain.SoftwarePresetRequest{
		Preset:  "essential",
		Scope:   domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
		Exclude: []string{"vlc", "gimp", "vlc"},
	})
	if plan.HasErrors() || plan.State != "unchanged" || len(plan.SelectedPackages) != 0 || len(plan.Request.Exclude) != 2 || source.validations != 0 || source.writes != 0 {
		t.Fatalf("fully excluded preset = %+v validations=%d writes=%d", plan, source.validations, source.writes)
	}

	unknown := manager.PlanPreset(context.Background(), "/deployment", domain.SoftwarePresetRequest{
		Preset: "essential", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients}, Exclude: []string{"hello"},
	})
	if !unknown.HasErrors() || source.validations != 0 || source.writes != 0 {
		t.Fatalf("unknown exclusion accepted: %+v", unknown)
	}
}

func TestSoftwarePresetPlanRejectsAnyUnavailablePackageWithoutWriting(t *testing.T) {
	source, manager := softwarePresetFixture(t)
	source.resolved["gimp"] = domain.SoftwareCatalogItem{ID: "gimp", Label: "GIMP", Summary: "Edit images", Availability: "blocked-insecure"}
	plan := manager.PlanPreset(context.Background(), "/deployment", domain.SoftwarePresetRequest{
		Preset: "essential", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
	})
	if !plan.HasErrors() || !strings.Contains(plan.Message, "blocked-insecure") || source.validations != 0 || source.writes != 0 {
		t.Fatalf("blocked preset package = %+v validations=%d writes=%d", plan, source.validations, source.writes)
	}
}

func TestSoftwarePresetApplyRejectsCatalogFileAndTokenDrift(t *testing.T) {
	for _, mutate := range []func(*fakeSoftwareSource, *domain.SoftwarePresetPlanReport){
		func(source *fakeSoftwareSource, _ *domain.SoftwarePresetPlanReport) {
			source.presets.Presets[0].Description = "Changed after review"
		},
		func(source *fakeSoftwareSource, _ *domain.SoftwarePresetPlanReport) {
			source.data = []byte("{\n  \"schemaVersion\": 1,\n  \"packages\": [{\"package\":\"hello\",\"scope\":{\"kind\":\"all-clients\"}}]\n}\n")
		},
		func(_ *fakeSoftwareSource, plan *domain.SoftwarePresetPlanReport) {
			plan.ReviewToken = "sha256:wrong"
		},
		func(source *fakeSoftwareSource, _ *domain.SoftwarePresetPlanReport) {
			source.resolved["gimp"] = domain.SoftwareCatalogItem{ID: "gimp", Label: "GIMP", Summary: "Edit images", Version: "changed-lock-version", Availability: "available"}
		},
	} {
		source, manager := softwarePresetFixture(t)
		plan := manager.PlanPreset(context.Background(), "/deployment", domain.SoftwarePresetRequest{
			Preset: "essential", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
		})
		mutate(source, &plan)
		result := manager.ApplyPresetPlan(context.Background(), plan, plan.ReviewToken)
		if result.State != "conflict" || source.writes != 0 {
			t.Fatalf("stale preset plan applied: %+v writes=%d", result, source.writes)
		}
	}
}

func TestSoftwarePresetPlanUsesCandidateValidationAndControllerCapability(t *testing.T) {
	source, manager := softwarePresetFixture(t)
	shared := domain.SoftwarePresetRequest{Preset: "essential", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}}
	if plan := manager.PlanPreset(context.Background(), "/deployment", shared); !plan.HasErrors() || source.validations != 0 {
		t.Fatalf("legacy contract accepted shared preset: %+v", plan)
	}

	source.definition.Controller = "pc99"
	source.validationErr = errors.New("private validator rejected candidate")
	plan := manager.PlanPreset(context.Background(), "/deployment", shared)
	if !plan.HasErrors() || !strings.Contains(plan.Message, "private validator") || source.validations != 1 || source.writes != 0 {
		t.Fatalf("candidate validation bypassed: %+v validations=%d writes=%d", plan, source.validations, source.writes)
	}
}

func TestSoftwarePresetApplyReportsAtomicWriterFailures(t *testing.T) {
	for _, testCase := range []struct {
		err   error
		state string
	}{
		{err: errors.New("read-only deployment"), state: "failed"},
		{err: domain.ErrSoftwareDurability, state: "partial"},
	} {
		source, manager := softwarePresetFixture(t)
		plan := manager.PlanPreset(context.Background(), "/deployment", domain.SoftwarePresetRequest{
			Preset: "essential", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeAllClients},
		})
		source.writeErr = testCase.err
		result := manager.ApplyPresetPlan(context.Background(), plan, plan.ReviewToken)
		if !result.HasErrors() || result.State != testCase.state || source.writes != 1 {
			t.Fatalf("writer failure %v = %+v writes=%d", testCase.err, result, source.writes)
		}
	}
}
