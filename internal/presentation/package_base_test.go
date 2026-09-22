package presentation

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestPackageBaseRenderGallery(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, dark := range []bool{false, true} {
			for _, state := range []string{"pin", "migration", "review", "error", "recovery"} {
				model := dashboardModel{screen: dashboardUpdate, baseUpdate: true, width: size[0], height: size[1], isDark: dark, baseStatus: domain.PackageBaseStatus{Channel: "nixos-26.05", Revision: strings.Repeat("a", 40)}}
				required := []string{"Update system and packages", "Esc", "Help"}
				switch state {
				case "migration":
					model.baseEditing = true
					model.baseTarget = "nixos-26.11"
					required = append(required, "unverified", "Space")
				case "review":
					model.screen = dashboardUpdateReview
					model.updatePlan = domain.UpdatePlanReport{State: "ready", Kind: "package-base", CurrentRef: "nixos-26.05", Target: "nixos-26.11", Diff: domain.GitDiff{Content: strings.Repeat("+ reviewed change\n", 40)}}
					required = append(required, "unverified", "Enter", "Cancel")
				case "error":
					model.baseStatus.Issues = []domain.ValidationIssue{{Field: "input", Message: "Review the legacy deployment migration"}}
					required = append(required, "legacy", "Refresh")
				case "recovery":
					model.updateResult = domain.UpdateApplyReport{Operation: "update-save", State: "partial", Updated: true, RecoveryRequired: true, Message: "Saving needs recovery"}
					required = []string{"Update system and packages", "recovery", "Complete save", "Enter", "Maintenance", "Help"}
				}
				view := model.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%s overflow %dx%d: %s", state, size[0], size[1], view)
				}
				for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
					var output bytes.Buffer
					writer := colorprofile.Writer{Forward: &output, Profile: profile}
					if _, err := writer.Write([]byte(view)); err != nil {
						t.Fatal(err)
					}
					for _, text := range required {
						if !strings.Contains(output.String(), text) {
							t.Fatalf("%s %dx%d missing %q:\n%s", state, size[0], size[1], text, output.String())
						}
					}
				}
			}
		}
	}
}

func TestSystemUpdateJourneyRequiresReviewAndUsesSeparateSave(t *testing.T) {
	planned, saved, activated := 0, 0, 0
	model := dashboardModel{screen: dashboardAdministration, width: 80, height: 24,
		actions: DashboardActions{
			LoadPackageBase: func() domain.PackageBaseStatus {
				return domain.PackageBaseStatus{Channel: "nixos-26.05", Revision: strings.Repeat("a", 40)}
			},
			PlanPackageBase: func(target string, allow bool, progress func(domain.UpdatePlanProgress)) domain.UpdatePlanReport {
				planned++
				if target != "nixos-26.11" || !allow {
					t.Fatalf("target=%s allow=%t", target, allow)
				}
				progress(domain.UpdatePlanProgress{Phase: domain.UpdatePlanPhaseBuild, Detail: "Building the controller", Current: 1, Total: 2})
				return domain.UpdatePlanReport{Kind: "package-base", State: "ready", Target: target, Confirmation: "MIGRATE", Diff: domain.GitDiff{Content: "+ reviewed base\n"}, PackageBase: &domain.PackageBaseChange{CurrentChannel: "nixos-26.05", TargetChannel: target}}
			},
			SavePackageBase: func(plan domain.UpdatePlanReport) domain.UpdateApplyReport {
				saved++
				return domain.UpdateApplyReport{State: "saved", Updated: true}
			},
			SaveUpdate: func(domain.UpdatePlanReport) domain.UpdateApplyReport {
				t.Fatal("used framework save")
				return domain.UpdateApplyReport{}
			},
			PlanController: func() domain.ControllerRebuildPlanReport { return domain.ControllerRebuildPlanReport{State: "ready"} },
			ApplyController: func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
				activated++
				return domain.ControllerRebuildExecutionReport{State: "completed", Applied: true, Verified: true}
			},
		},
	}
	next, command := model.Update(tea.KeyPressMsg{Text: "b"})
	model = next.(dashboardModel)
	if command == nil || !model.baseUpdate {
		t.Fatal("maintenance task unreachable")
	}
	next, _ = model.Update(command())
	model = next.(dashboardModel)
	if !strings.Contains(model.View().Content, "nixos-26.05") {
		t.Fatal(model.View().Content)
	}
	next, _ = model.Update(tea.KeyPressMsg{Text: "m"})
	model = next.(dashboardModel)
	model.baseTarget = "nixos-26.11"
	next, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(dashboardModel)
	if command != nil || planned != 0 || !strings.Contains(model.message, "Acknowledge") {
		t.Fatal("missing acknowledgement bypassed")
	}
	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = next.(dashboardModel)
	next, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(dashboardModel)
	for command != nil && model.screen != dashboardUpdateReview {
		next, command = model.Update(command())
		model = next.(dashboardModel)
	}
	if planned != 1 || saved != 0 || activated != 0 || model.screen != dashboardUpdateReview {
		t.Fatalf("plan=%d save=%d activation=%d screen=%d", planned, saved, activated, model.screen)
	}
	if !strings.Contains(model.View().Content, "unverified") {
		t.Fatal("review hides runtime limits")
	}
	next, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(dashboardModel)
	for command != nil {
		next, command = model.Update(command())
		model = next.(dashboardModel)
	}
	if saved != 1 || activated != 1 || !model.controllerResult.Verified {
		t.Fatalf("save=%d activation=%d", saved, activated)
	}
}

func TestSystemChannelFormCancelAndTextDoNotTriggerGlobalActions(t *testing.T) {
	model := dashboardModel{screen: dashboardUpdate, baseUpdate: true, baseEditing: true, baseStatus: domain.PackageBaseStatus{Channel: "nixos-26.05"}}
	next, command := model.Update(tea.KeyPressMsg{Text: "q"})
	model = next.(dashboardModel)
	if command != nil || model.baseTarget != "q" {
		t.Fatal("typing quit the form")
	}
	next, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = next.(dashboardModel)
	if command != nil || model.baseEditing || model.baseTarget != "nixos-26.05" || model.screen != dashboardUpdate {
		t.Fatal("cancel did not restore channel")
	}
}

func TestSystemPartialSaveDoesNotOfferOrStartControllerActivation(t *testing.T) {
	model := dashboardModel{screen: dashboardUpdate, baseUpdate: true, updateResult: domain.UpdateApplyReport{Operation: "update-save", State: "partial", Updated: true, RecoveryRequired: true}}
	if strings.Contains(model.View().Content, "Retry controller") || strings.Contains(model.View().Content, "saved safely") {
		t.Fatal("partial save presented as complete")
	}
	_, command := model.Update(tea.KeyPressMsg{Text: "a"})
	if command != nil {
		t.Fatal("partial save started activation")
	}
}
