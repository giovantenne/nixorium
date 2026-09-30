package presentation

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func workspaceFixture() dashboardModel {
	m := experienceFixture(2)
	m.screen = dashboardWorkspace
	plan := demoWorkspacePlan()
	m.workspace = workspaceModel{loaded: plan, candidate: *plan.Candidate}
	m.actions.LoadWorkspace = func(context.Context) domain.WorkspacePlanReport { return demoWorkspacePlan() }
	m.actions.PlanWorkspace = func(_ context.Context, p domain.WorkspaceProfile) domain.WorkspacePlanReport {
		plan := demoWorkspacePlan()
		plan.Candidate = &p
		plan.Inspection.Resolution.Declared = p
		return plan
	}
	m.actions.SaveWorkspace = func(domain.WorkspacePlanReport) domain.WorkspaceApplyReport {
		return domain.WorkspaceApplyReport{State: "saved", Message: "No computer or student home changed."}
	}
	return m
}

func workspaceKey(m dashboardModel, key tea.KeyPressMsg) (dashboardModel, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(dashboardModel), cmd
}

func workspaceComplete(t *testing.T, m dashboardModel, cmd tea.Cmd) dashboardModel {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a typed callback command")
	}
	next, _ := m.Update(cmd())
	return next.(dashboardModel)
}

func TestWorkspaceFieldsPreserveInheritanceAndReviewedObjects(t *testing.T) {
	p, issues := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"desktop":{"favorites":["firefox.desktop","code.desktop"],"enableAnimations":false},"vscode":{"settings":{"editor.fontSize":16}}}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	before, _ := domain.MarshalWorkspaceProfile(p)
	field := workspaceFields[0]
	for _, test := range []struct {
		value any
		want  string
	}{
		{nil, "Inherit"}, {[]string{}, "None (explicit empty list)"},
		{[]string{"code.desktop", "firefox.desktop"}, "code.desktop, firefox.desktop"},
	} {
		updated, err := workspaceSetValue(p, field, test.value)
		if err != nil || workspaceValueText(workspaceValue(updated, field)) != test.want {
			t.Fatalf("edit %v: %+v %v", test.value, updated, err)
		}
		if *updated.Desktop.EnableAnimations || *updated.VSCode.Settings.FontSize != 16 {
			t.Fatal("unrelated preferences changed")
		}
	}
	after, _ := domain.MarshalWorkspaceProfile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("editing mutated the reviewed candidate")
	}
	ext := workspaceGroupFields(2)[0]
	updated, err := workspaceSetValue(p, ext, []string{"z.last", "a.first"})
	if err != nil || !reflect.DeepEqual(*updated.VSCode.Extensions, []string{"a.first", "z.last"}) {
		t.Fatal("extension normalization was bypassed")
	}
	if _, err := workspaceSetValue(p, workspaceGroupFields(1)[1], 129); err == nil {
		t.Fatal("invalid dock size accepted")
	}
}

func TestWorkspaceFieldSelectionOrderInheritanceAndNumericInput(t *testing.T) {
	m := workspaceFixture()
	m.workspace.startField()
	if !m.workspace.inherit || len(m.workspace.selected) != 2 {
		t.Fatal("baseline preview missing")
	}
	m, _ = workspaceKey(m, demoText("c"))
	m, _ = workspaceKey(m, demoCode(tea.KeySpace))
	m, _ = workspaceKey(m, demoCode(tea.KeyDown))
	m, _ = workspaceKey(m, demoCode(tea.KeySpace))
	m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	if !reflect.DeepEqual(m.workspace.selected, []string{"code.desktop", "firefox.desktop"}) || m.pageScroll != 0 {
		t.Fatal("ordered favorites escaped their field")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEnter))
	if got := *m.workspace.candidate.Desktop.Favorites; !reflect.DeepEqual(got, []string{"code.desktop", "firefox.desktop"}) {
		t.Fatal(got)
	}
	m.workspace.startField()
	m, _ = workspaceKey(m, demoText("i"))
	if !reflect.DeepEqual(m.workspace.selected, []string{"firefox.desktop", "code.desktop"}) {
		t.Fatal("inherit preview retained the draft order")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEnter))
	if m.workspace.candidate.Desktop != nil {
		t.Fatal("inherit did not remove the explicit field")
	}
	m.workspace.group, m.workspace.cursor = 1, 1
	m.workspace.startField()
	m, cmd := workspaceKey(m, demoText("qv99"))
	if cmd != nil || m.screen != dashboardWorkspace || m.workspace.number != "99" {
		t.Fatal("numeric input triggered a global action")
	}
	m.workspace.number = "999"
	m, _ = workspaceKey(m, demoCode(tea.KeyEnter))
	if m.workspace.stage != workspaceFieldEdit || m.message == "" {
		t.Fatal("invalid input silently accepted")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEsc))
	if m.workspace.candidate.Desktop != nil {
		t.Fatal("cancel changed the draft")
	}
	m.workspace.group, m.workspace.cursor = 3, 0
	m.workspace.startField()
	if !reflect.DeepEqual(m.workspace.choices, []string{"Inherit", "firefox.desktop"}) {
		t.Fatal("non-browser offered as default")
	}
	m.workspace.group, m.workspace.cursor = 2, 0
	m.workspace.startField()
	if !reflect.DeepEqual(m.workspace.choices, []string{"ritwickdey.liveserver"}) {
		t.Fatal("extension picker does not use the pinned catalog")
	}
}

func TestWorkspaceSettingsRouteAndCancelledLateLoad(t *testing.T) {
	m := workspaceFixture()
	m.screen = dashboardSettings
	m.settings.menu = newRoutineSettingsMenu(true, 80, 24)
	var request context.Context
	m.actions.LoadWorkspace = func(ctx context.Context) domain.WorkspacePlanReport { request = ctx; return demoWorkspacePlan() }
	m, cmd := workspaceKey(m, demoText("w"))
	if cmd == nil || m.screen != dashboardWorkspace || m.busy == "" {
		t.Fatal("workspace shortcut did not load metadata")
	}
	late := cmd()
	m, _ = workspaceKey(m, demoCode(tea.KeyEsc))
	if request.Err() != context.Canceled || m.screen != dashboardSettings {
		t.Fatal("read-only cancellation failed")
	}
	next, _ := m.Update(late)
	if next.(dashboardModel).screen != dashboardSettings {
		t.Fatal("late load reopened the editor")
	}
	for _, classroom := range []bool{false, true} {
		m := workspaceFixture()
		m.screen = dashboardSettings
		m.actions.ClassroomMode = classroom
		if !classroom {
			m.actions.SaveWorkspace = nil
		}
		next, cmd := m.openWorkspace()
		if cmd != nil || next.(dashboardModel).screen != dashboardSettings {
			t.Fatal("unavailable editor opened")
		}
	}
}

func TestWorkspaceReviewRejectsConcurrentBaseAndPreservesDraft(t *testing.T) {
	m := workspaceFixture()
	m.workspace.candidate, _ = workspaceSetValue(m.workspace.candidate, workspaceFields[1], "dark")
	m.actions.PlanWorkspace = func(context.Context, domain.WorkspaceProfile) domain.WorkspacePlanReport {
		p := demoWorkspacePlan()
		p.Inspection.Snapshot.BaseFingerprint = "changed"
		return p
	}
	m, cmd := workspaceKey(m, demoText("v"))
	m = workspaceComplete(t, m, cmd)
	if m.workspace.stage != workspaceOverview || !strings.Contains(m.message, "changed while editing") || *m.workspace.candidate.Desktop.ColorScheme != "dark" {
		t.Fatal("concurrent base was accepted or draft lost")
	}
}

func TestWorkspaceSaveRequiresReviewAndNeverStartsOtherOperations(t *testing.T) {
	m := workspaceFixture()
	saves := 0
	m.actions.SaveWorkspace = func(plan domain.WorkspacePlanReport) domain.WorkspaceApplyReport {
		saves++
		if plan.ReviewToken != "sha256:synthetic-workspace" {
			t.Fatal("review identity was lost")
		}
		return domain.WorkspaceApplyReport{State: "saved"}
	}
	// Leave unrelated callbacks nil: any implicit deployment/commit would panic.
	m.actions.ApplyController = nil
	m.actions.PlanGitCommit = nil
	m.actions.ApplyGitCommit = nil
	m, cmd := workspaceKey(m, demoText("v"))
	m = workspaceComplete(t, m, cmd)
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd != nil || saves != 0 {
		t.Fatal("save without typed confirmation")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyF1))
	m, _ = workspaceKey(m, demoText("SAVE"))
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	if !m.helpOpen || cmd != nil || saves != 0 {
		t.Fatal("help triggered a save")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEsc))
	m, _ = workspaceKey(m, demoText("SAVE"))
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd == nil || !m.workspace.saving {
		t.Fatal("reviewed save did not start")
	}
	for _, key := range []tea.KeyPressMsg{demoCode(tea.KeyEsc), demoText("q"), {Code: 'c', Mod: tea.ModCtrl}} {
		var exit tea.Cmd
		m, exit = workspaceKey(m, key)
		if exit != nil || m.screen != dashboardWorkspace || !m.workspace.saving {
			t.Fatal("closed during atomic save")
		}
	}
	result := cmd()
	next, chained := m.Update(result)
	m = next.(dashboardModel)
	if chained != nil || saves != 1 || m.workspace.result.State != "saved" || m.workspace.stage != workspaceResult {
		t.Fatal("save did not remain declaration-only")
	}
	next, chained = m.Update(dashboardWorkspaceSaveMsg{id: m.workspace.requestID, report: domain.WorkspaceApplyReport{State: "failed"}})
	if chained != nil || next.(dashboardModel).workspace.result.State != "saved" {
		t.Fatal("duplicate result replaced success")
	}
	for _, state := range []string{"unchanged", "invalid"} {
		m.workspace.stage, m.workspace.plan.State, m.workspace.confirmation = workspaceReview, state, "SAVE"
		m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
		if cmd != nil || saves != 1 {
			t.Fatalf("%s plan requested save", state)
		}
	}
}

func TestWorkspaceResultRecoveryAndContextualGitReview(t *testing.T) {
	m := workspaceFixture()
	m.workspace.stage = workspaceResult
	m.workspace.result = domain.WorkspaceApplyReport{State: "partial", RecoveryRequired: true}
	m, cmd := workspaceKey(m, demoText("r"))
	if cmd != nil || m.workspace.stage != workspaceResult {
		t.Fatal("hidden retry after partial replacement")
	}
	m.workspace.result = domain.WorkspaceApplyReport{State: "saved"}
	m.actions.LoadGitReview = func(ctx context.Context) domain.GitReviewReport { return domain.GitReviewReport{State: "clean"} }
	m.maintenance.gitCommitResult.Operation = "old-result"
	m, cmd = workspaceKey(m, demoText("g"))
	m = workspaceComplete(t, m, cmd)
	if m.screen != dashboardGitReview || m.maintenance.gitCommitResult.Operation != "" {
		t.Fatal("Git follow-up retained a stale result")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEsc))
	if m.screen != dashboardWorkspace || m.workspace.result.State != "saved" {
		t.Fatal("Git review lost its workspace parent")
	}
}

func TestWorkspaceRenderingFitsAndKeepsActionsVisible(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, dark := range []bool{false, true} {
			for _, state := range []string{"overview", "fields", "favorites", "extensions", "empty", "numeric", "review", "loading", "failure", "saving", "saved", "partial"} {
				t.Run(fmt.Sprintf("%dx%d/dark%t/%s", size[0], size[1], dark, state), func(t *testing.T) {
					m := workspaceFixture()
					m.width, m.height, m.isDark = size[0], size[1], dark
					w := &m.workspace
					want := []string{"Student workspace", "F1", "Help", "Esc"}
					switch state {
					case "fields":
						w.group, w.cursor, w.stage = 2, 7, workspaceFieldList
						want = append(want, "Auto save", "Review")
					case "favorites", "extensions", "empty":
						if state == "extensions" {
							w.group = 2
						}
						if state == "empty" {
							w.loaded.Inspection.Resolution.Catalog.Applications = nil
						}
						w.startField()
						if state != "empty" {
							for i := 0; i < 30; i++ {
								w.choices = append(w.choices, fmt.Sprintf("choice-%02d.desktop", i))
							}
							w.choice = len(w.choices) - 1
							want = append(want, "choice-29.desktop")
						} else {
							want = append(want, "no entries")
						}
						want = append(want, "Keep draft", "Cancel field")
					case "numeric":
						w.group, w.cursor = 1, 1
						w.startField()
						want = append(want, "Value:", "Keep draft")
					case "review":
						w.plan = demoWorkspacePlan()
						w.stage = workspaceReview
						want = append(want, "Type SAVE:", "Save JSON", "no system apply or reset")
					case "loading":
						m.busy = "Loading workspace metadata"
						m.beginRead(dashboardReadTimeout)
						t.Cleanup(m.read.cancel)
					case "failure":
						w.loaded.State = "invalid"
						m.message = "The deployment candidate hook is unavailable."
						want = append(want, "Retry", "could not be loaded")
					case "saving":
						w.saving = true
						m.busy = "Saving reviewed preferences"
						want = []string{"Student workspace", "Saving reviewed preferences", "Help"}
					case "saved":
						w.stage = workspaceResult
						w.result = domain.WorkspaceApplyReport{State: "saved", Message: "Only the declaration was saved."}
						want = append(want, "SAVED", "Preferences take effect")
					case "partial":
						w.stage = workspaceResult
						w.result = domain.WorkspaceApplyReport{State: "partial", RecoveryRequired: true, Message: "Replacement completed but durability is uncertain. Inspect the file before another change."}
						want = append(want, "PARTIAL", "durability")
					}
					view := m.View().Content
					if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
						t.Fatalf("overflow:\n%s", view)
					}
					if w.stage == workspaceResult {
						for range 20 {
							m, _ = workspaceKey(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
							view += m.View().Content
						}
					}
					for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
						var output bytes.Buffer
						writer := colorprofile.Writer{Forward: &output, Profile: profile}
						if _, err := writer.Write([]byte(view)); err != nil {
							t.Fatal(err)
						}
						plain := demoANSI.ReplaceAllString(output.String(), "")
						for _, text := range want {
							if !strings.Contains(plain, text) {
								t.Fatalf("missing %q:\n%s", text, plain)
							}
						}
					}
				})
			}
		}
	}
}
