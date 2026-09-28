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
		for _, want := range []string{"learner", "controller (controller)", "pc01 (client)", "absent (legacy mode)", "Effective preferences", "vscode @ 1.0", "example.plugin @ 2.0", "nodejs", "not changed by saving", "No commit, deployment or reset"} {
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
