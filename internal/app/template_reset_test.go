package app

import (
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeTemplateReset struct {
	applied int
	plan    domain.TemplateResetPlan
}

func (f *fakeTemplateReset) TemplateResetCatalog(context.Context, string) domain.TemplateResetCatalog {
	return domain.TemplateResetCatalog{}
}
func (f *fakeTemplateReset) PrepareTemplateReset(context.Context, string, string, func(string)) domain.TemplateResetPlan {
	return f.plan
}
func (f *fakeTemplateReset) ApplyTemplateReset(context.Context, domain.TemplateResetPlan) domain.TemplateResetResult {
	f.applied++
	return domain.TemplateResetResult{State: "saved"}
}

func TestTemplateResetManagerBindsReviewBeforeDelegatingWrite(t *testing.T) {
	p := domain.TemplateResetPlan{State: "ready", Repository: "/deployment", Revision: strings.Repeat("a", 40), UpstreamRevision: strings.Repeat("b", 40), Preset: domain.SoftwarePreset{ID: "essential"}}
	files := map[string]domain.TemplateFile{"lab-settings.json": {Mode: "100644", Data: []byte("settings")}, "flake.lock": {Mode: "100644", Data: []byte("lock")}}
	p.Proposal = domain.TemplateResetProposal{Revision: p.Revision, UpstreamRevision: p.UpstreamRevision, Branch: "refs/heads/master", SourceIdentity: "identity", Original: files, Candidate: files}
	source := &fakeTemplateReset{plan: p}
	manager := NewTemplateResetManager(source)
	plan := manager.Plan(t.Context(), p.Repository, "essential", nil)
	if plan.HasErrors() || plan.ReviewToken == "" || plan.Confirmation != "RESET DEPLOYMENT" {
		t.Fatalf("plan: %+v", plan)
	}
	for _, mutate := range []func(*domain.TemplateResetPlan){
		func(p *domain.TemplateResetPlan) { p.ReviewToken = "" },
		func(p *domain.TemplateResetPlan) { p.Confirmation = "RESET" },
		func(p *domain.TemplateResetPlan) { p.Preset.ID = "programming" },
		func(p *domain.TemplateResetPlan) {
			p.Changes = []domain.TemplateResetChange{{Path: "lost.nix", Action: "remove"}}
		},
	} {
		changed := plan
		mutate(&changed)
		if result := manager.Apply(t.Context(), changed); result.State != "blocked" || source.applied != 0 {
			t.Fatal("invalid review reached writer")
		}
	}
	if result := manager.Apply(t.Context(), plan); result.State != "saved" || source.applied != 1 {
		t.Fatal("valid proposal not delegated")
	}
}
