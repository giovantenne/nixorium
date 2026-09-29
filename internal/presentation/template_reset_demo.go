package presentation

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func demoTemplateResetPlan() domain.TemplateResetPlan {
	return domain.TemplateResetPlan{
		State: "ready", UpstreamRevision: strings.Repeat("a", 40), ReviewToken: "sha256:synthetic-reset", Confirmation: "RESET DEPLOYMENT",
		Preset:    domain.SoftwarePreset{ID: "essential", Label: "Essential", Packages: []string{"chromium", "ghostty", "libreoffice"}},
		Preserved: []string{"lab-settings.json", "flake.lock", "keys/admin-ssh.pub", ".gitignore"},
		Changes:   []domain.TemplateResetChange{{Path: "flake.nix", Action: "replace"}, {Path: "modules/local.nix", Action: "remove"}, {Path: "workspace-profile.json", Action: "add"}},
	}
}

func renderTemplateResetDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	plan := demoTemplateResetPlan()
	actions.LoadTemplateReset = func(context.Context) domain.TemplateResetCatalog {
		return domain.TemplateResetCatalog{UpstreamRevision: plan.UpstreamRevision, Catalog: domain.SoftwarePresetCatalog{Presets: []domain.SoftwarePreset{plan.Preset}}}
	}
	actions.PlanTemplateReset = func(context.Context, string, func(string)) domain.TemplateResetPlan { return plan }
	actions.ApplyTemplateReset = func(domain.TemplateResetPlan) domain.TemplateResetResult {
		return domain.TemplateResetResult{State: "saved", Revision: strings.Repeat("b", 40), BackupRef: "refs/nixorium/template-backups/demo", Message: "Saved and committed locally. Apply systems and reboot separately; nothing pushed."}
	}
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1000)
	r.key(demoText("a"))
	load := r.key(demoText("t"))
	r.capture("Load the locked upstream template", 1000)
	r.command(load)
	r.capture("Choose the replacement preset", 1500)
	r.command(r.key(demoCode(tea.KeyEnter)))
	r.capture("Review replacement and preserved files", 2000)
	r.model.templateReset.scroll = len(r.model.templateResetLines())
	r.capture("Review all removed and replaced paths", 2000)
	r.key(demoText("RESET DEPLOYMENT"))
	save := r.key(demoCode(tea.KeyEnter))
	r.capture("Create the backup before local replacement", 1200)
	r.command(save)
	r.capture("Local reset saved without activation", 2000)
	return DemoScenario{ID: "template-reset", Title: "Reset a private deployment template", Description: "Choose a pinned preset, review losses and preserve a recoverable local backup without activation.", Frames: r.frames}
}
