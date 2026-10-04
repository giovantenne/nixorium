package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestWorkspaceTextAndConfirmationKeepSaveSeparate(t *testing.T) {
	profile := domain.WorkspaceProfile{SchemaVersion: 1}
	plan := domain.WorkspacePlanReport{
		State: "ready", Repository: "/deployment", ManagedFile: domain.WorkspaceFileName, Confirmation: "SAVE", ReviewToken: "token",
		Inspection: &domain.WorkspaceInspection{Resolution: domain.WorkspaceResolution{
			StudentUser: "learner", Declared: profile, Effective: profile,
			Targets:    []domain.WorkspaceTarget{{Name: "controller", Role: "controller"}, {Name: "pc01", Role: "client"}},
			Packages:   []domain.WorkspacePackage{{Package: "vscode", Version: "1.0"}},
			Extensions: []domain.WorkspaceExtension{{ID: "example.plugin", Version: "2.0", RequiredPackages: []string{"nodejs"}}},
		}},
		Message: "No commit, deployment or reset is included.",
	}
	for _, input := range []string{"SAVE\n", "save\n", "cancel\n"} {
		var output bytes.Buffer
		approved, err := ConfirmWorkspace(strings.NewReader(input), &output, plan)
		if err != nil || approved != (input == "SAVE\n") {
			t.Fatalf("confirmation %q: %v %v", input, approved, err)
		}
		for _, want := range []string{"learner", "controller (controller)", "pc01 (client)", "no saved profile", "Effective preferences", "vscode @ 1.0", "example.plugin @ 2.0", "nodejs", "next computer start after system application", "No commit, deployment or reset"} {
			if !strings.Contains(output.String(), want) {
				t.Fatalf("missing %q in review:\n%s", want, &output)
			}
		}
	}
	plan.State = "invalid"
	var output bytes.Buffer
	if approved, _ := ConfirmWorkspace(strings.NewReader("SAVE\n"), &output, plan); approved || output.Len() != 0 {
		t.Fatal("invalid review prompted")
	}
}

func TestWorkspaceTextSanitizesTerminalControls(t *testing.T) {
	var output bytes.Buffer
	WorkspaceApplyText(&output, domain.WorkspaceApplyReport{State: "failed", Message: "error\x1b[2J\rhidden", Issues: []domain.ValidationIssue{{Field: "file", Message: "bad\u009bcontrol"}}})
	if strings.ContainsAny(output.String(), "\x1b\r\u009b") {
		t.Fatalf("terminal controls survived: %q", output.String())
	}
}

func TestWorkspaceReviewSummarizesChangesBeforeJSON(t *testing.T) {
	base, issues := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"desktop":{"favorites":["a.desktop","b.desktop"]},"browser":{"defaultApplication":"old.desktop"},"vscode":{"settings":{"editor.fontSize":14}}}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	proposed, issues := domain.DecodeWorkspaceProfile([]byte(`{"schemaVersion":1,"desktop":{"favorites":["b.desktop","a.desktop"]},"vscode":{"settings":{"editor.fontSize":18}}}`))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	var output bytes.Buffer
	WorkspacePlanText(&output, domain.WorkspacePlanReport{State: "ready", Inspection: &domain.WorkspaceInspection{
		Base: &base, Resolution: domain.WorkspaceResolution{Declared: proposed},
	}})
	view := output.String()
	jsonAt := strings.Index(view, `"schemaVersion"`)
	for _, label := range []string{"Preference changes:", "Favorite applications (ordered)", "Font size", "Default browser", "next computer start after system application", "Saving does not apply systems"} {
		if at := strings.Index(view, label); at < 0 || at >= jsonAt {
			t.Fatalf("missing summary before JSON for %q:\n%s", label, view)
		}
	}
	if strings.Contains(view, "opt-in") || strings.Contains(view, "No preference overrides changed") {
		t.Fatalf("misleading review:\n%s", view)
	}
	if !strings.Contains(view, "14 → 18") || !strings.Contains(view, "Inherit") {
		t.Fatalf("value changes or restored defaults missing:\n%s", view)
	}
}

func TestWorkspaceApplicationDescriptionDoesNotClaimLiveState(t *testing.T) {
	if text := workspaceApplicationText(); !strings.Contains(text, "after system application") || !strings.Contains(text, "next computer start") {
		t.Fatal(text)
	}
}
