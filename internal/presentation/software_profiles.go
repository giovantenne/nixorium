package presentation

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model softwareModel) loadProfiles(report domain.SoftwarePresetCatalogReport) (softwareModel, softwareMessageResult) {
	model.presets = report
	model.profileCursor = 0
	model.profilePackageCursor = 0
	model.profileReviewCursor = 0
	model.profileExcluded = map[string]bool{}
	model.profilePlan = domain.SoftwarePresetPlanReport{}
	model.profileResult = domain.SoftwarePresetApplyReport{}
	if report.HasErrors() || report.State != "ready" || report.Catalog == nil {
		model.stage = softwareCatalog
		return model, softwareMessageResult{accepted: true, message: report.Message}
	}
	model.stage = softwareProfiles
	return model, softwareMessageResult{accepted: true}
}

func (model softwareModel) finishProfilePlan(report domain.SoftwarePresetPlanReport) (softwareModel, softwareMessageResult) {
	model.profilePlan = report
	model.profileReviewCursor = 0
	if report.HasErrors() {
		model.stage = softwareProfileScope
	} else {
		model.stage = softwareProfileReview
	}
	return model, softwareMessageResult{accepted: true, message: report.Message}
}

func (model softwareModel) finishProfileApply(report domain.SoftwarePresetApplyReport) (softwareModel, softwareMessageResult) {
	model.applying = false
	model.profileResult = report
	model.result = domain.SoftwareChangeApplyReport{
		SchemaVersion:      domain.SoftwareSchemaVersion,
		Operation:          "software-profile-result",
		State:              report.State,
		Repository:         report.Repository,
		ManagedFile:        report.ManagedFile,
		AffectedController: report.AffectedController,
		AffectedClients:    append([]string{}, report.AffectedClients...),
		Revision:           report.Revision,
		RecoveryRequired:   report.RecoveryRequired,
		Issues:             append([]domain.ValidationIssue{}, report.Issues...),
		Message:            report.Message,
	}
	model.stage = softwareResult
	return model, softwareMessageResult{
		accepted:        true,
		message:         report.Message,
		startController: !report.HasErrors() && report.State == "saved" && report.AffectedController != "",
	}
}

func (model softwareModel) updateProfiles(key tea.KeyPressMsg) (softwareModel, softwareIntent) {
	presets := model.profileCatalog()
	switch key.String() {
	case "esc", "left":
		model.stage = softwareCatalog
		return model, softwareIntent{setMessage: true}
	case "up", "k":
		model.profileCursor = max(0, model.profileCursor-1)
	case "down", "j":
		model.profileCursor = min(max(0, len(presets)-1), model.profileCursor+1)
	case "enter":
		if len(presets) == 0 {
			return model, softwareIntent{}
		}
		model.profilePackageCursor = 0
		model.profileExcluded = map[string]bool{}
		model.profilePlan = domain.SoftwarePresetPlanReport{}
		model.profileResult = domain.SoftwarePresetApplyReport{}
		model.stage = softwareProfilePackages
		return model, softwareIntent{setMessage: true}
	}
	return model, softwareIntent{}
}

func (model softwareModel) updateProfilePackages(key tea.KeyPressMsg) (softwareModel, softwareIntent) {
	preset := model.selectedProfile()
	switch key.String() {
	case "esc", "left":
		model.stage = softwareProfiles
		return model, softwareIntent{setMessage: true}
	case "up", "k":
		model.profilePackageCursor = max(0, model.profilePackageCursor-1)
	case "down", "j":
		model.profilePackageCursor = min(max(0, len(preset.Packages)-1), model.profilePackageCursor+1)
	case "space":
		if len(preset.Packages) > 0 {
			packageID := preset.Packages[model.profilePackageCursor]
			model.profileExcluded[packageID] = !model.profileExcluded[packageID]
		}
	case "enter":
		model.scopeCursor = 0
		model.clientCursor = 0
		model.clients = map[string]bool{}
		model.stage = softwareProfileScope
		return model, softwareIntent{setMessage: true}
	}
	return model, softwareIntent{}
}

func (model softwareModel) updateProfileScope(key tea.KeyPressMsg) (softwareModel, softwareIntent) {
	switch key.String() {
	case "esc", "left":
		model.stage = softwareProfilePackages
		return model, softwareIntent{setMessage: true}
	case "enter":
		scope, notice := model.selectedScope()
		if notice != "" {
			return model, softwareIntent{message: notice, setMessage: true}
		}
		return model, softwareIntent{
			kind: softwarePresetPlanIntent,
			presetRequest: domain.SoftwarePresetRequest{
				Preset:  model.selectedProfile().ID,
				Scope:   scope,
				Exclude: model.profileExclusions(),
			},
		}
	default:
		model = model.updateScopeSelection(key)
	}
	return model, softwareIntent{}
}

func (model softwareModel) updateProfileReview(key tea.KeyPressMsg) (softwareModel, softwareIntent) {
	rows := model.profileReviewRows()
	switch key.String() {
	case "esc":
		model.stage = softwareProfileScope
		return model, softwareIntent{message: "Software profile cancelled; no file changed.", setMessage: true}
	case "up", "k":
		model.profileReviewCursor = max(0, model.profileReviewCursor-1)
	case "down", "j":
		model.profileReviewCursor = min(max(0, len(rows)-1), model.profileReviewCursor+1)
	case "enter":
		return model, softwareIntent{kind: softwarePresetSaveIntent}
	}
	return model, softwareIntent{}
}

func (model softwareModel) profileCatalog() []domain.SoftwarePreset {
	if model.presets.Catalog == nil {
		return nil
	}
	return model.presets.Catalog.Presets
}

func (model softwareModel) selectedProfile() domain.SoftwarePreset {
	presets := model.profileCatalog()
	if len(presets) == 0 {
		return domain.SoftwarePreset{}
	}
	return presets[min(model.profileCursor, len(presets)-1)]
}

func (model softwareModel) profileExclusions() []string {
	preset := model.selectedProfile()
	result := []string{}
	for _, packageID := range preset.Packages {
		if model.profileExcluded[packageID] {
			result = append(result, packageID)
		}
	}
	return result
}

func (model softwareModel) profileActions(_ softwareViewContext) []tuiAction {
	switch model.stage {
	case softwareProfiles:
		return []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Enter", label: "Choose packages"}, {key: "Esc", label: "Catalog"}, {key: "F1", label: "Help"}}
	case softwareProfilePackages:
		return []tuiAction{{key: "↑/↓", label: "Select"}, {key: "Space", label: "Include/exclude"}, {key: "Enter", label: "Choose scope"}, {key: "Esc", label: "Profiles"}, {key: "F1", label: "Help"}}
	case softwareProfileScope:
		actions := []tuiAction{{key: "↑/↓", label: "Select"}}
		options := model.scopeOptions()
		if len(options) > 0 && options[min(model.scopeCursor, len(options)-1)].scope.Kind == domain.SoftwareScopeClients {
			actions = append(actions, tuiAction{key: "Space", label: "Toggle"})
		}
		return append(actions, tuiAction{key: "Enter", label: "Review"}, tuiAction{key: "Esc", label: "Packages"}, tuiAction{key: "F1", label: "Help"})
	case softwareProfileReview:
		return []tuiAction{{key: "↑/↓", label: "Inspect"}, {key: "Enter", label: "Add profile"}, {key: "Esc", label: "Scope"}, {key: "F1", label: "Help"}}
	default:
		return nil
	}
}

func (model softwareModel) profilesView(context softwareViewContext) []string {
	presets := model.profileCatalog()
	lines := []string{
		tuiTitle("Add a software profile", context.dark),
		"",
		"Profiles add missing package declarations as one reviewed change.",
		tuiMuted("Existing packages keep their current scope. Profiles never remove software.", context.dark),
		"",
	}
	capacity := max(1, (context.height-len(lines)-9)/2)
	start, end := listWindow(len(presets), model.profileCursor, capacity)
	for index := start; index < end; index++ {
		preset := presets[index]
		defaultLabel := ""
		if model.presets.Catalog != nil && preset.ID == model.presets.Catalog.DefaultPreset {
			defaultLabel = "  " + tuiMuted("default for new sites", context.dark)
		}
		lines = append(lines,
			tuiSelection(preset.Label, index == model.profileCursor, context.dark)+defaultLabel,
			tuiMuted(fmt.Sprintf("    %s · %d packages", preset.Description, len(preset.Packages)), context.dark),
		)
	}
	if start > 0 || end < len(presets) {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d profiles", displayedLineStart(start, len(presets)), end, len(presets)), context.dark))
	}
	return lines
}

func (model softwareModel) profilePackagesView(context softwareViewContext) []string {
	preset := model.selectedProfile()
	included := len(preset.Packages) - len(model.profileExclusions())
	lines := []string{
		tuiTitle(preset.Label+" packages", context.dark),
		tuiMuted(preset.Description, context.dark),
		"",
		fmt.Sprintf("%d of %d packages included", included, len(preset.Packages)),
		tuiMuted("Space excludes or restores the highlighted package before review.", context.dark),
		"",
	}
	capacity := max(1, context.height-len(lines)-11)
	start, end := listWindow(len(preset.Packages), model.profilePackageCursor, capacity)
	for index := start; index < end; index++ {
		packageID := preset.Packages[index]
		checked := "[x]"
		if model.profileExcluded[packageID] {
			checked = "[ ]"
		}
		status := ""
		if existing, found := model.declaration(packageID); found {
			status = "  " + tuiStatus("already configured for "+softwareScopeLabel(existing.Scope), tuiStatusSuccess, context.dark)
		}
		lines = append(lines, tuiSelection(checked+" "+packageID, index == model.profilePackageCursor, context.dark)+status)
	}
	if start > 0 || end < len(preset.Packages) {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d packages", displayedLineStart(start, len(preset.Packages)), end, len(preset.Packages)), context.dark))
	}
	return lines
}

func (model softwareModel) profileScopeView(context softwareViewContext) []string {
	preset := model.selectedProfile()
	lines := []string{
		tuiTitle("Add "+preset.Label, context.dark),
		"Choose one destination for every missing package in this profile.",
		tuiMuted("Packages already configured keep their current scope.", context.dark),
		"",
	}
	options := model.scopeOptions()
	for index, option := range options {
		lines = append(lines, tuiSelection(option.label, index == model.scopeCursor, context.dark))
	}
	if len(options) > 0 && options[model.scopeCursor].scope.Kind == domain.SoftwareScopeClients {
		lines = append(lines, "")
		start, end := listWindow(len(model.catalog.Clients), model.clientCursor, max(2, context.height-len(lines)-14))
		for index := start; index < end; index++ {
			name := model.catalog.Clients[index]
			checked := "[ ]"
			if model.clients[name] {
				checked = "[x]"
			}
			lines = append(lines, tuiSelection(fmt.Sprintf("%s %s", checked, name), index == model.clientCursor, context.dark))
		}
	}
	lines = append(lines, "", fmt.Sprintf("Included packages: %d", len(preset.Packages)-len(model.profileExclusions())), "Managed file: "+model.catalog.ManagedFile)
	return lines
}

func (model softwareModel) profileReviewRows() []string {
	existing := map[string]domain.SoftwareScope{}
	for _, declaration := range model.profilePlan.Existing {
		existing[declaration.Package] = declaration.Scope
	}
	rows := make([]string, 0, len(model.profilePlan.SelectedPackages))
	for _, item := range model.profilePlan.SelectedPackages {
		if scope, found := existing[item.ID]; found {
			rows = append(rows, "= "+item.ID+" · already present; keep "+softwareScopeLabel(scope))
		} else {
			rows = append(rows, "+ "+item.ID+" · add for "+softwareScopeLabel(model.profilePlan.Request.Scope))
		}
	}
	return rows
}

func (model softwareModel) profileReviewView(context softwareViewContext) []string {
	plan := model.profilePlan
	rows := model.profileReviewRows()
	lines := []string{
		tuiTitle("Add "+plan.Preset.Label+"?", context.dark),
		"One local change adds every missing declaration shown below.",
		"",
		"Destination  " + softwareScopeLabel(plan.Request.Scope),
		fmt.Sprintf("Packages     %d add · %d keep scope · %d excluded", len(plan.Additions), len(plan.Existing), len(plan.Request.Exclude)),
		fmt.Sprintf("Clients      %d affected by new declarations", len(plan.AffectedClients)),
		"",
		tuiStatus("Validated together against the pinned package set", tuiStatusSuccess, context.dark),
	}
	capacity := max(1, context.height-len(lines)-12)
	start, end := listWindow(len(rows), model.profileReviewCursor, capacity)
	for index := start; index < end; index++ {
		lines = append(lines, tuiSelection(rows[index], index == model.profileReviewCursor, context.dark))
	}
	if len(rows) == 0 {
		lines = append(lines, "No package remains selected; saving will make no change.")
	} else if start > 0 || end < len(rows) {
		lines = append(lines, tuiMuted(fmt.Sprintf("%d–%d of %d reviewed packages", displayedLineStart(start, len(rows)), end, len(rows)), context.dark))
	}
	lines = append(lines, "", "Now          Save one update to "+plan.ManagedFile, "Later        Apply to the controller and update affected computers through their normal reviews")
	if plan.AffectedController == "" {
		lines[len(lines)-1] = "Later        Update affected computers through a fresh review"
	}
	if len(plan.AffectedClients) == 0 {
		lines[len(lines)-1] = "Later        No computer update required"
	}
	return lines
}
