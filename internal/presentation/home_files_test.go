package presentation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func homeFilesFixture() dashboardModel {
	m := experienceFixture(2)
	m.screen = dashboardHomeFiles
	loaded := demoHomeFilesPlan()
	loaded.Candidate = &domain.HomeFilesDraft{Profile: domain.HomeFilesProfile{SchemaVersion: 1}, BaseFingerprint: "base"}
	loaded.Entries = nil
	m.homeFiles = homeFilesModel{stage: "overview", loaded: loaded, draft: cloneHomeFilesDraft(*loaded.Candidate), sizes: map[string]int64{}}
	m.actions.LoadGitReview = func(context.Context) domain.GitReviewReport { return domain.GitReviewReport{State: "changed"} }
	m.actions.LoadHomeFiles = func(context.Context) domain.HomeFilesPlanReport { return loaded }
	m.actions.CaptureHomeFiles = func(context.Context, string, string) domain.HomeFilesCaptureReport { return demoHomeFilesCapture() }
	m.actions.CompleteHomeFilesPath = func(context.Context, string) ([]string, error) { return []string{"/local/lessons/"}, nil }
	m.actions.PlanHomeFiles = func(_ context.Context, draft domain.HomeFilesDraft) domain.HomeFilesPlanReport {
		p := demoHomeFilesPlan()
		p.Candidate = &draft
		return p
	}
	m.actions.ApplyController = func(domain.ControllerRebuildPlanReport) domain.ControllerRebuildExecutionReport {
		return domain.ControllerRebuildExecutionReport{}
	}
	m.actions.SaveHomeFiles = func(domain.HomeFilesPlanReport) domain.HomeFilesApplyReport {
		return domain.HomeFilesApplyReport{State: "saved", Recorded: true, Revision: strings.Repeat("a", 40)}
	}
	return m
}

func TestHomeFilesNavigationRemainsAvailableAfterPreferenceFailure(t *testing.T) {
	m := homeFilesFixture()
	m.screen = dashboardWorkspace
	m.workspace.loaded = domain.WorkspacePlanReport{State: "invalid"}
	if !strings.Contains(m.View().Content, "Initial files") {
		t.Fatal("preference failure hides initial files")
	}
	m, cmd := workspaceKey(m, demoCode(tea.KeyEnter))
	m = workspaceComplete(t, m, cmd)
	if m.screen != dashboardHomeFiles || m.homeFiles.loaded.Candidate == nil {
		t.Fatal("initial files not reachable")
	}
	m.screen = dashboardWorkspace
	m.workspace.loaded = demoWorkspacePlan()
	m.workspace.group = len(workspaceGroups) - 1
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	m = workspaceComplete(t, m, cmd)
	if m.screen != dashboardHomeFiles {
		t.Fatal("section does not open feature")
	}
	m.screen = dashboardWorkspace
	m.actions.LoadHomeFiles = nil
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd != nil || !strings.Contains(m.message, "Update Nixorium") {
		t.Fatal("older feature lacks actionable update guidance")
	}
	m = homeFilesFixture()
	m.homeFiles.loaded = domain.HomeFilesPlanReport{State: "invalid"}
	if !strings.Contains(m.View().Content, "Maintenance → Update Nixorium") {
		t.Fatal("unsupported pin view names the package-base update instead of the core update")
	}
}

func TestHomeFilesCapturedDraftExecutableSaveAndFollowup(t *testing.T) {
	m := homeFilesFixture()
	captures, saves, controller := 0, 0, 0
	m.actions.CaptureHomeFiles = func(_ context.Context, source, dest string) domain.HomeFilesCaptureReport {
		captures++
		if source != "/local/lessons" || dest != "Documents/Lessons" {
			t.Fatalf("capture arguments %q %q", source, dest)
		}
		return demoHomeFilesCapture()
	}
	m.actions.SaveHomeFiles = func(p domain.HomeFilesPlanReport) domain.HomeFilesApplyReport {
		saves++
		if len(p.Candidate.CaptureIDs) != 1 || !p.Candidate.Profile.Files[1].Executable || p.Candidate.BaseFingerprint != "base" {
			t.Fatalf("save lost exact reviewed draft: %+v", p.Candidate)
		}
		return domain.HomeFilesApplyReport{State: "saved", Recorded: true, Revision: strings.Repeat("a", 40)}
	}
	m.actions.PlanController = func(context.Context) domain.ControllerRebuildPlanReport {
		controller++
		return domain.ControllerRebuildPlanReport{State: "ready"}
	}
	m, _ = workspaceKey(m, demoText("a"))
	m, _ = workspaceKey(m, demoText("/local/lessons"))
	m, _ = workspaceKey(m, demoCode(tea.KeyEnter))
	if m.homeFiles.stage != "destination" {
		t.Fatal("source form did not advance")
	}
	m, cmd := workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd != nil || m.homeFiles.destination != "" {
		t.Fatal("example saved as destination")
	}
	m, _ = workspaceKey(m, demoText("Documents/Lessons"))
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	m = workspaceComplete(t, m, cmd)
	if m.homeFiles.stage != "capture" || len(m.homeFiles.draft.Profile.Files) != 0 {
		t.Fatal("capture mutated draft before acceptance")
	}
	for _, f := range m.homeFiles.pending.Files {
		if f.Executable {
			t.Fatal("executability inherited")
		}
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyDown))
	m, _ = workspaceKey(m, demoCode(tea.KeySpace))
	m, _ = workspaceKey(m, demoCode(tea.KeyEnter))
	if m.homeFiles.stage != "overview" || m.homeFiles.source != "" {
		t.Fatal("capture not retained or source path retained")
	}
	m, cmd = workspaceKey(m, demoText("v"))
	m = workspaceComplete(t, m, cmd)
	if saves != 0 {
		t.Fatal("review saved before it was confirmed")
	}
	// Enter on the review is the confirmation; no word is typed.
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	// A local mutation stays protected from exit, and F1 cannot save again.
	m, _ = workspaceKey(m, demoText("q"))
	if !m.homeFiles.saving {
		t.Fatal("quit abandoned protected save")
	}
	m = workspaceComplete(t, m, cmd)
	if captures != 1 || saves != 1 || controller != 0 || m.homeFiles.stage != "result" {
		t.Fatal("save reread source or applied controller automatically")
	}
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("separate fresh controller review unavailable")
	}
	m = workspaceComplete(t, m, cmd)
	if controller != 1 {
		t.Fatal("followup reused stale controller review")
	}
}

func TestHomeFilesCancelLateReadsAndSourceCompletionPreserveDraft(t *testing.T) {
	m := homeFilesFixture()
	m, _ = workspaceKey(m, demoText("a"))
	m, _ = workspaceKey(m, demoText("/loc"))
	m, cmd := workspaceKey(m, demoCode(tea.KeyTab))
	m = workspaceComplete(t, m, cmd)
	if m.homeFiles.source != "/local/lessons/" || m.homeFiles.stage != "source" {
		t.Fatal("completion altered editor context")
	}
	m.homeFiles.destination = "Documents/Lessons"
	m.homeFiles.stage = "destination"
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	id := m.homeFiles.requestID
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	m = workspaceComplete(t, m, cmd)
	if m.busy != "" || m.homeFiles.stage != "destination" || len(m.homeFiles.draft.Profile.Files) != 0 {
		t.Fatal("cancelled capture resumed draft mutation")
	}
	next, _ := m.Update(homeFilesReadMsg{id: id, kind: "capture", capture: demoHomeFilesCapture()})
	m = next.(dashboardModel)
	if m.homeFiles.stage != "destination" {
		t.Fatal("late direct message accepted")
	}
	m.homeFiles.stage = "review"
	m.helpOpen = true
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd != nil || m.homeFiles.saving {
		t.Fatal("help triggered save")
	}
}

func TestHomeFilesReplacementRemovalAndDriftDoNotMutateReviewedObjects(t *testing.T) {
	m := homeFilesFixture()
	profile := demoHomeFilesPlan().Candidate.Profile
	m.homeFiles.draft.Profile = profile
	reviewed := cloneHomeFilesDraft(m.homeFiles.draft)
	m.homeFiles.pending = demoHomeFilesCapture()
	m.homeFiles.replace = "Documents/Lessons"
	m.homeFiles.stage = "capture"
	if message := m.homeFiles.acceptCapture(); !strings.Contains(message, "draft") {
		t.Fatal(message)
	}
	if len(m.homeFiles.draft.Profile.Files) != 2 || len(reviewed.Profile.Files) != 2 {
		t.Fatal("replacement retained duplicates or mutated reviewed candidate")
	}
	m.homeFiles.pending = demoHomeFilesCapture()
	m.homeFiles.replace = ""
	if message := m.homeFiles.acceptCapture(); !strings.Contains(message, "already uses") {
		t.Fatal("duplicate import silently replaces declaration")
	}
	m.homeFiles.stage = "overview"
	m.homeFiles.cursor = 0
	m, _ = workspaceKey(m, demoText("d"))
	if len(reviewed.Profile.Files) != 2 {
		t.Fatal("removal changed an earlier plan")
	}
	m.actions.PlanHomeFiles = func(context.Context, domain.HomeFilesDraft) domain.HomeFilesPlanReport {
		p := demoHomeFilesPlan()
		p.Snapshot.BaseFingerprint = "changed"
		return p
	}
	m, cmd := workspaceKey(m, demoText("v"))
	m = workspaceComplete(t, m, cmd)
	if m.homeFiles.stage == "review" || !strings.Contains(m.message, "reopen Initial files") {
		t.Fatal("saved manifest drift was reviewed against wrong base")
	}
}

func TestHomeFilesPartialSaveRequiresGitInspection(t *testing.T) {
	m := homeFilesFixture()
	m.homeFiles.stage = "result"
	m.homeFiles.result = domain.HomeFilesApplyReport{State: "partial", RecoveryRequired: true, Message: "Copies saved; local record incomplete."}
	if !strings.Contains(m.View().Content, "Inspect Git state") || strings.Contains(m.View().Content, "Enter Apply to this controller") {
		t.Fatalf("partial save offers automatic continuation: %s", m.View().Content)
	}
	m.actions.LoadGitReview = func(context.Context) domain.GitReviewReport { return domain.GitReviewReport{State: "changed"} }
	m, cmd := workspaceKey(m, demoText("g"))
	if cmd == nil || m.screen != dashboardGitReview {
		t.Fatal("inspection unavailable")
	}
	m = workspaceComplete(t, m, cmd)
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	if m.screen != dashboardHomeFiles || m.homeFiles.stage != "result" {
		t.Fatal("Git inspection lost save outcome")
	}
}

func TestHomeFilesRenderGallery(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, dark := range []bool{false, true} {
			for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
				scenario := RenderHomeFilesDemo(strings.Repeat("a", 40), size[0], size[1])
				if directory := os.Getenv("NIXORIUM_HOME_FILES_GALLERY"); directory != "" && dark && profile == colorprofile.ANSI256 {
					if err := os.MkdirAll(directory, 0700); err != nil {
						t.Fatal(err)
					}
					data, err := json.MarshalIndent(scenario, "", "  ")
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("home-files-%dx%d.json", size[0], size[1])), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				for _, frame := range scenario.Frames {
					if lipgloss.Width(frame.ANSI) > size[0] || lipgloss.Height(frame.ANSI) > size[1] {
						t.Fatalf("%s overflows %dx%d", frame.Label, size[0], size[1])
					}
				}
				m := homeFilesFixture()
				m.width, m.height, m.isDark = size[0], size[1], dark
				m.homeFiles.draft = cloneHomeFilesDraft(*demoHomeFilesPlan().Candidate)
				for i := 0; i < 30; i++ {
					text := "lesson"
					m.homeFiles.draft.Profile.Files = append(m.homeFiles.draft.Profile.Files, domain.HomeFile{Destination: fmt.Sprintf("Documents/z-%02d.txt", i), Text: &text})
				}
				m.homeFiles.cursor = len(m.homeFiles.destinations()) - 1
				var rendered bytes.Buffer
				writer := colorprofile.Writer{Forward: &rendered, Profile: profile}
				_, _ = writer.Write([]byte(m.View().Content))
				output := rendered.String()
				if lipgloss.Width(output) > size[0] || lipgloss.Height(output) > size[1] || !strings.Contains(output, "z-29.txt") || !strings.Contains(output, "Review") || !strings.Contains(output, "Workspace") {
					t.Fatalf("focused item/actions lost in %dx%d:\n%s", size[0], size[1], output)
				}
			}
		}
	}
}

func TestHomeFilesReviewNamesRemovalsAndPermissionChangesFirst(t *testing.T) {
	p := demoHomeFilesPlan()
	p.Changes = []domain.HomeFilesChange{
		{Destination: "home-files.json", Kind: "declaration"},
		{Destination: "Documents/removed.txt", Kind: "removed", BeforeSize: 32},
		{Destination: "bin/lesson", Kind: "permissions", BeforeExecutable: false, AfterExecutable: true},
		{Destination: "Documents/lesson.txt", Kind: "replaced", BeforeSize: 3, AfterSize: 8},
	}
	for i := 0; i < 60; i++ {
		p.Targets = append(p.Targets, domain.WorkspaceTarget{Name: fmt.Sprintf("pc%02d", i+10), Role: "client"})
	}
	var text bytes.Buffer
	HomeFilesPlanText(&text, p)
	review := text.String()
	for _, want := range []string{"remove Documents/removed.txt", "permissions bin/lesson · 0600 · not executable → 0700 · executable", "replace Documents/lesson.txt · 3 B → 8 B"} {
		if !strings.Contains(review, want) || strings.Index(review, want) > strings.Index(review, "Complete draft:") {
			t.Fatalf("change hidden behind candidate: %s", review)
		}
	}
	if !strings.Contains(review, "Declaration metadata changed; file bytes and permissions stay the same.") || strings.Contains(review, "replace home-files.json") {
		t.Fatal("provenance change presented as a byte replacement")
	}
	if strings.Index(review, "Declaration changes:") > strings.Index(review, "Target computers:") {
		t.Fatal("target detail hides change review")
	}
	m := homeFilesFixture()
	m.homeFiles.stage = "result"
	m.homeFiles.result = domain.HomeFilesApplyReport{State: "unchanged"}
	if strings.Contains(demoANSI.ReplaceAllString(m.View().Content, ""), "✓ UNCHANGED") {
		t.Fatal("unchanged declarations imply verified success")
	}
	if got := homeFilesInput(strings.Repeat("folder/", 80)+"lesson.pdf", 50); !strings.HasSuffix(got, "lesson.pdf_") || lipgloss.Width(got) > 50 {
		t.Fatalf("long source input loses cursor: %s", got)
	}
}

func TestHomeFilesDiscardOnlyCancelledCopiesAndRefuseClassroomAccess(t *testing.T) {
	m := homeFilesFixture()
	discarded := []string{}
	m.actions.DiscardHomeFilesCapture = func(_ context.Context, id string) error { discarded = append(discarded, id); return nil }
	m.homeFiles.stage = "capture"
	m.homeFiles.pending = demoHomeFilesCapture()
	m, cmd := workspaceKey(m, demoCode(tea.KeyEscape))
	m = workspaceComplete(t, m, cmd)
	if len(discarded) != 1 || discarded[0] != "synthetic-capture" || len(m.homeFiles.draft.Profile.Files) != 0 || m.homeFiles.stage != "source" {
		t.Fatal("cancelled copy not discarded safely")
	}
	m.homeFiles.stage = "overview"
	m.homeFiles.draft.CaptureIDs = []string{"unused-draft-copy"}
	m, cmd = workspaceKey(m, demoCode(tea.KeyEscape))
	m = workspaceComplete(t, m, cmd)
	if m.screen != dashboardWorkspace || len(discarded) != 2 || discarded[1] != "unused-draft-copy" {
		t.Fatal("abandoned draft copy retained")
	}
	m.actions.ClassroomMode = true
	next, cmd := m.openHomeFiles()
	m = next.(dashboardModel)
	if cmd != nil || m.screen == dashboardHomeFiles || !strings.Contains(m.message, "administrator") {
		t.Fatal("classroom reached administrative import")
	}
}
