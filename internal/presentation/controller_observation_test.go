package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func overviewHasControllerTask(model dashboardModel) bool {
	for _, task := range model.pendingTasks() {
		if task.id == "pending-controller" {
			return true
		}
	}
	return false
}

func TestControllerOverviewSupersedesOlderSaveAndObservations(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	current := domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "current", Current: true, Revision: b}
	verified := domain.ControllerRebuildExecutionReport{Operation: "controller-apply", State: "completed", Revision: b, Applied: true, Verified: true}
	for name, message := range map[string]tea.Msg{
		"controller review": dashboardControllerPlanMsg{report: current},
		"controller apply":  dashboardControllerResultMsg{report: verified},
		"software follow-up": dashboardSoftwareControllerMsg{plan: domain.ControllerRebuildPlanReport{
			Operation: "controller-plan", State: "ready", Revision: b,
		}, report: verified},
		"update follow-up": dashboardUpdateControllerMsg{plan: domain.ControllerRebuildPlanReport{
			Operation: "controller-plan", State: "ready", Revision: b,
		}, report: verified},
		"setup review": dashboardSetupMsg{report: domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageArtifacts,
			Stages: []domain.SetupStage{{ID: domain.SetupStageApply, State: domain.SetupStageComplete}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			model := savedFollowupFixture(dashboardHome)
			model.actions.ApplyController = func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
				t.Fatal("observation authorized an apply")
				return domain.ControllerRebuildExecutionReport{}
			}
			// These independent cached sources used to retain the same warning.
			model.setup = domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageApply}
			model.recovery.Conditions = []domain.BlockingCondition{{Kind: domain.RecoveryControllerChanged}, {Kind: domain.RecoveryBackupDue}}
			model.noteControllerSave(a)
			updated, _ := model.Update(dashboardControllerPlanMsg{report: domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "ready", Revision: a}})
			model = updated.(dashboardModel)
			updated, _ = model.Update(message)
			model = updated.(dashboardModel)
			if overviewHasControllerTask(model) {
				t.Fatal("new verified evidence retained an older controller warning")
			}
			if tasks := model.pendingTasks(); len(tasks) != 1 || tasks[0].id != "pending-backup" {
				t.Fatalf("reconciliation changed unrelated pending work: %#v", tasks)
			}
			// Overview clearing must not claim that an old save result is an
			// exact match for a different activation or authorize client work.
			if controllerVerifiedForSave(a, model.controller.result) {
				t.Fatal("save-result revision binding was weakened")
			}
		})
	}
}

func TestControllerOverviewNewSavesInvalidateEarlierVerification(t *testing.T) {
	a, b := strings.Repeat("a", 40), strings.Repeat("b", 40)
	save := domain.ConfigurationSaveReport{State: "saved", Revision: b}
	software := domain.SoftwareChangeApplyReport{State: "saved", Revision: b, AffectedController: "pc99"}
	for name, message := range map[string]tea.Msg{
		"settings":         dashboardSettingsApplyMsg{report: save},
		"workspace":        dashboardWorkspaceSaveMsg{report: domain.WorkspaceApplyReport{State: "saved", Recorded: true, Revision: b}},
		"template reset":   templateResetResultMsg{result: domain.TemplateResetResult{State: "saved", Revision: b}},
		"software":         dashboardSoftwareApplyMsg{report: software},
		"software profile": dashboardSoftwarePresetApplyMsg{report: domain.SoftwarePresetApplyReport{State: "saved", Revision: b, AffectedController: "pc99"}},
		"update":           dashboardUpdateResultMsg{report: domain.UpdateApplyReport{State: "saved", Updated: true, Revision: b}},
		"setup":            dashboardSetupSaveMsg{report: save},
		"generated keys":   dashboardSetupKeysMsg{save: save},
		"Git commit":       dashboardGitCommitResultMsg{report: domain.GitCommitReport{State: "committed", Committed: true, Revision: b}},
	} {
		t.Run(name, func(t *testing.T) {
			model := savedFollowupFixture(dashboardHome)
			updated, _ := model.Update(dashboardControllerResultMsg{report: domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Revision: a, Applied: true, Verified: true}})
			model = updated.(dashboardModel)
			if overviewHasControllerTask(model) {
				t.Fatal("verified controller starts with a warning")
			}
			model.screen, model.workspace.saving = dashboardWorkspace, true
			model.templateReset.saving = true
			if name == "template reset" {
				model.screen = dashboardTemplateReset
			}
			updated, _ = model.Update(message)
			model = updated.(dashboardModel)
			if !overviewHasControllerTask(model) {
				t.Fatal("an earlier verification concealed a newer save")
			}
		})
	}
}

func TestControllerOverviewRetainsFailedAndUnverifiedWork(t *testing.T) {
	revision := strings.Repeat("a", 40)
	for name, message := range map[string]tea.Msg{
		"failed":           dashboardControllerResultMsg{report: domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Revision: revision, State: "failed", Applied: true}},
		"unverified":       dashboardControllerResultMsg{report: domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Revision: revision, State: "completed", Applied: true}},
		"no operation":     dashboardControllerResultMsg{report: domain.ControllerRebuildExecutionReport{Revision: revision, Applied: true, Verified: true}},
		"missing revision": dashboardControllerResultMsg{report: domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Applied: true, Verified: true}},
		"blocked review":   dashboardControllerPlanMsg{report: domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "blocked", Current: true, Revision: revision, Issues: []domain.ValidationIssue{{Field: "controller", Message: "unavailable"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			model := savedFollowupFixture(dashboardHome)
			model.noteControllerSave(revision)
			updated, _ := model.Update(message)
			model = updated.(dashboardModel)
			if !overviewHasControllerTask(model) {
				t.Fatal("incomplete verification cleared pending work")
			}
			model.screen = dashboardController
			model, _ = workspaceKey(model, demoCode(tea.KeyEscape))
			if strings.Contains(model.message, "activated and verified") {
				t.Fatal("incomplete verification produced a success notice")
			}
		})
	}
}

func TestControllerOverviewFreshDriftSupersedesPreviousSuccess(t *testing.T) {
	revision := strings.Repeat("a", 40)
	drift := domain.RecoveryReport{Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryControllerChanged}}}
	for name, message := range map[string]tea.Msg{
		"controller review": dashboardControllerPlanMsg{report: domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "ready", Revision: revision}},
		"recovery review":   recoveryMsg{report: drift},
		"local refresh":     overviewRefreshMsg{recovery: drift},
		"refresh with earlier setup check": overviewRefreshMsg{recovery: drift, setup: domain.SetupReport{
			Stages: []domain.SetupStage{{ID: domain.SetupStageApply, State: domain.SetupStageComplete}},
		}},
		"setup review": dashboardSetupMsg{report: domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageApply,
			Stages: []domain.SetupStage{{ID: domain.SetupStageApply, State: domain.SetupStageCurrent}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			model := savedFollowupFixture(dashboardHome)
			updated, _ := model.Update(dashboardControllerResultMsg{report: domain.ControllerRebuildExecutionReport{Operation: "controller-apply", Revision: revision, Applied: true, Verified: true}})
			model = updated.(dashboardModel)
			updated, _ = model.Update(message)
			if !overviewHasControllerTask(updated.(dashboardModel)) {
				t.Fatal("old successful apply concealed freshly observed drift")
			}
		})
	}
}

func TestControllerOverviewCancelledReviewCannotClearNewSave(t *testing.T) {
	model := savedFollowupFixture(dashboardHome)
	model.noteControllerSave(strings.Repeat("a", 40))
	model.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		return domain.ControllerRebuildPlanReport{Operation: "controller-plan", State: "current", Current: true, Revision: strings.Repeat("b", 40)}
	}
	updated, command := model.openControllerReview()
	model = updated.(dashboardModel)
	reply := command()
	model, _ = workspaceKey(model, demoCode(tea.KeyEscape))
	model.noteControllerSave(strings.Repeat("c", 40))
	updated, _ = model.Update(reply)
	if !overviewHasControllerTask(updated.(dashboardModel)) {
		t.Fatal("cancelled review cleared a newer save")
	}
}

func TestControllerOverviewRecoveryRefreshDoesNotRetainClearedDrift(t *testing.T) {
	for _, saved := range []bool{false, true} {
		model := savedFollowupFixture(dashboardHome)
		if saved {
			model.noteControllerSave(strings.Repeat("a", 40))
		}
		updated, _ := model.Update(overviewRefreshMsg{recovery: domain.RecoveryReport{State: "attention", Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryControllerChanged}}}})
		model = updated.(dashboardModel)
		if !overviewHasControllerTask(model) {
			t.Fatal("fresh drift was hidden")
		}
		updated, _ = model.Update(overviewRefreshMsg{recovery: domain.RecoveryReport{State: "clear"}})
		if overviewHasControllerTask(updated.(dashboardModel)) != saved {
			t.Fatal("recovery refresh retained cleared drift or concealed an unapplied save")
		}
	}
}

func TestControllerOverviewRecoveryCannotReviveSupersededSetup(t *testing.T) {
	setup := domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageApply,
		Stages: []domain.SetupStage{{ID: domain.SetupStageApply, State: domain.SetupStageCurrent}},
	}
	model := newDashboardModel(domain.StatusReport{}, setup, DashboardActions{}, false)
	if !overviewHasControllerTask(model) {
		t.Fatal("initial setup application was not reported")
	}
	updated, _ := model.Update(dashboardControllerPlanMsg{report: domain.ControllerRebuildPlanReport{
		Operation: "controller-plan", State: "current", Current: true, Revision: strings.Repeat("a", 40),
	}})
	model = updated.(dashboardModel)
	if overviewHasControllerTask(model) {
		t.Fatal("fresh current review did not clear the setup reminder")
	}
	updated, _ = model.Update(recoveryMsg{report: domain.RecoveryReport{State: "attention",
		Conditions: []domain.BlockingCondition{{Kind: domain.RecoveryControllerChanged}},
	}})
	model = updated.(dashboardModel)
	if !overviewHasControllerTask(model) {
		t.Fatal("fresh recovery drift was hidden")
	}
	updated, _ = model.Update(recoveryMsg{report: domain.RecoveryReport{State: "clear"}})
	model = updated.(dashboardModel)
	if overviewHasControllerTask(model) {
		t.Fatal("resolved recovery revived the superseded setup reminder")
	}
}

func TestControllerOverviewCancelledReviewDoesNotAdvertiseOlderSuccess(t *testing.T) {
	model := savedFollowupFixture(dashboardHome)
	updated, _ := model.Update(dashboardControllerResultMsg{report: domain.ControllerRebuildExecutionReport{
		Operation: "controller-apply", Revision: strings.Repeat("a", 40), Applied: true, Verified: true,
	}})
	model = updated.(dashboardModel)
	updated, _ = model.Update(dashboardSettingsApplyMsg{report: domain.ConfigurationSaveReport{State: "saved", Revision: strings.Repeat("b", 40)}})
	model = updated.(dashboardModel)
	model.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		t.Fatal("cancelled review should not run")
		return domain.ControllerRebuildPlanReport{}
	}
	updated, _ = model.openControllerReview()
	model = updated.(dashboardModel)
	model, _ = workspaceKey(model, demoCode(tea.KeyEscape))
	model, _ = workspaceKey(model, demoCode(tea.KeyEscape))
	if model.screen != dashboardHome || !overviewHasControllerTask(model) {
		t.Fatal("cancelled review lost the pending save")
	}
	if strings.Contains(model.message, "activated and verified") {
		t.Fatal("an older activation was advertised after a newer save")
	}
}

// A saved change can add or remove computers, so client selection must load
// the inventory of the new revision instead of reusing the old one.
func TestSavedChangeReloadsTheInventoryBeforeClientSelection(t *testing.T) {
	loads := 0
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{
		LoadInventory: func(context.Context) (domain.StatusReport, error) {
			loads++
			report := domain.StatusReport{}
			report.Meta.Controller.Name = "pc99"
			report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}, {Name: "pc03"}}
			return report, nil
		},
	}, false)
	m.report.Meta.Controller.Name = "pc99"
	m.report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}}
	m.noteControllerSave("0123456789abcdef0123456789abcdef01234567")
	next, cmd := m.openComputerTask("d")
	m = next.(dashboardModel)
	if cmd == nil {
		t.Fatal("stale inventory reused after a saved change")
	}
	next, _ = m.Update(cmd())
	m = next.(dashboardModel)
	if loads != 1 || len(m.report.Meta.Clients.Hosts) != 3 || m.screen != dashboardDeploy {
		t.Fatalf("loads=%d hosts=%d screen=%v", loads, len(m.report.Meta.Clients.Hosts), m.screen)
	}
}
