package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

// yesNoCases are the answers of every review that is confirmed with a yes/no
// question: only y or yes approves, and the former typed words do not.
func yesNoCases(oldWords ...string) []struct {
	input string
	want  bool
} {
	cases := []struct {
		input string
		want  bool
	}{{"y\n", true}, {"yes\n", true}, {"Y\n", true}, {"\n", false}, {"n\n", false}, {"no\n", false}}
	for _, word := range oldWords {
		cases = append(cases, struct {
			input string
			want  bool
		}{word + "\n", false})
	}
	return cases
}

func TestConfirmControllerApplyAsksYesNo(t *testing.T) {
	for _, test := range yesNoCases("APPLY") {
		output := &bytes.Buffer{}
		got, err := ConfirmControllerApply(strings.NewReader(test.input), output, "pc99")
		if err != nil || got != test.want || !strings.Contains(output.String(), "services and networking may restart") || !strings.Contains(output.String(), "Continue? [y/N]") || strings.Contains(output.String(), "Type ") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}

func TestConfirmControllerRebuildAsksYesNo(t *testing.T) {
	report := domain.ControllerRebuildPlanReport{Controller: "pc99", Revision: "0123456789abcdef0123456789abcdef01234567"}
	for _, test := range yesNoCases("REBUILD") {
		output := &bytes.Buffer{}
		got, err := ConfirmControllerRebuild(strings.NewReader(test.input), output, report)
		if err != nil || got != test.want || !strings.Contains(output.String(), report.Revision) || strings.Contains(output.String(), "Type ") {
			t.Fatalf("input %q: got %t, err %v, output %q", test.input, got, err, output.String())
		}
	}
}

func TestConfirmServiceRestartAsksYesNo(t *testing.T) {
	service := domain.ManagedService{Name: "Binary cache"}
	for _, test := range yesNoCases("RESTART") {
		output := &bytes.Buffer{}
		got, err := ConfirmServiceRestart(strings.NewReader(test.input), output, service)
		if err != nil || got != test.want || !strings.Contains(output.String(), "PXE networking") || strings.Contains(output.String(), "Type ") {
			t.Fatalf("input %q: got %t, err %v, output %q", test.input, got, err, output.String())
		}
	}
}

func TestConfirmPXEStartAsksYesNo(t *testing.T) {
	report := domain.PXELifecycleReport{Interface: "enp1s0", DHCPAddress: "192.0.2.10", StaticCIDR: "10.0.0.99/8"}
	for _, test := range yesNoCases("START") {
		output := &bytes.Buffer{}
		got, err := ConfirmPXEStart(strings.NewReader(test.input), output, report)
		if err != nil || got != test.want || !strings.Contains(output.String(), "10.0.0.99/8") || !strings.Contains(output.String(), "reboot recovery is enabled") || strings.Contains(output.String(), "Type ") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}

func TestConfirmDeploymentApplyAsksYesNo(t *testing.T) {
	report := domain.DeploymentPlanReport{
		Revision:        "0123456789abcdef",
		ColmenaSelector: "pc01,pc03",
		Targets:         []domain.DeploymentTarget{{Name: "pc01"}, {Name: "pc03"}},
	}
	for _, test := range yesNoCases("DEPLOY") {
		output := &bytes.Buffer{}
		got, err := ConfirmDeploymentApply(strings.NewReader(test.input), output, report)
		if err != nil || got != test.want || !strings.Contains(output.String(), report.Revision) || !strings.Contains(output.String(), "rebuilds before every apply") || !strings.HasSuffix(output.String(), "Continue? [y/N] ") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}

func TestConfirmGitCommitAsksYesNo(t *testing.T) {
	report := domain.GitCommitPlanReport{Revision: "abc", Paths: []string{"lab-settings.json"}, CommitMessage: "chore: update laboratory settings"}
	for _, test := range yesNoCases("COMMIT") {
		var output bytes.Buffer
		approved, err := ConfirmGitCommit(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "no hook, signing action, remote, or push") || strings.Contains(output.String(), "Type ") {
			t.Fatalf("confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}

func TestConfirmUpdateAsksYesNo(t *testing.T) {
	report := domain.UpdatePlanReport{CurrentRef: "v2.0.0", CurrentRev: "abc", Target: "v2.1.0", TargetChannel: domain.UpdateChannelStable}
	for _, test := range yesNoCases("UPDATE") {
		var output bytes.Buffer
		approved, err := ConfirmUpdate(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "no branch, commit, push, activation") || strings.Contains(output.String(), "Type ") {
			t.Fatalf("confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}

func TestConfirmSoftwareChangeAsksYesNo(t *testing.T) {
	report := domain.SoftwareChangePlanReport{
		Request:         domain.SoftwareChangeRequest{Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: "graphics"}},
		AffectedClients: []string{"pc01", "pc02"},
	}
	for _, test := range yesNoCases("SAVE", "SAVE SOFTWARE") {
		var output bytes.Buffer
		approved, err := ConfirmSoftwareChange(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "only lab-software.json") || !strings.Contains(output.String(), "no commit, build, controller activation") || strings.Contains(output.String(), "Type ") {
			t.Fatalf("confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}

func TestConfirmSoftwareRemovalAsksYesNo(t *testing.T) {
	report := domain.SoftwareChangePlanReport{Request: domain.SoftwareChangeRequest{Package: "vlc", Present: false}}
	for _, test := range yesNoCases("REMOVE", "SAVE") {
		var output bytes.Buffer
		approved, err := ConfirmSoftwareChange(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "Continue? [y/N]") {
			t.Fatalf("removal confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}

func TestConfirmSoftwarePresetAsksYesNo(t *testing.T) {
	report := domain.SoftwarePresetPlanReport{
		Request: domain.SoftwarePresetRequest{Preset: "essential", Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}},
		Preset:  domain.SoftwarePreset{ID: "essential", Label: "Essential"}, Additions: []domain.SoftwareDeclaration{{Package: "vlc"}},
		Existing: []domain.SoftwareDeclaration{{Package: "firefox"}},
	}
	for _, test := range yesNoCases("ADD PROFILE") {
		var output bytes.Buffer
		approved, err := ConfirmSoftwarePreset(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "atomically update only lab-software.json") || !strings.Contains(output.String(), "complete current configuration") || strings.Contains(output.String(), "Type ") {
			t.Fatalf("profile confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}

func TestConfirmShutdownTypesTheWordOnlyWhenASessionMayBeInterrupted(t *testing.T) {
	target := func(name string, session domain.ShutdownSessionState) domain.ShutdownTargetPlan {
		return domain.ShutdownTargetPlan{Name: name, Eligible: true, Session: session}
	}
	// Student work may be lost: the word stays.
	for _, session := range []domain.ShutdownSessionState{domain.ShutdownSessionActive, domain.ShutdownSessionUnknown} {
		report := domain.ShutdownPlanReport{Eligible: 2, Targets: []domain.ShutdownTargetPlan{target("pc01", session), target("pc02", domain.ShutdownSessionIdle)}, Policy: domain.ShutdownAcknowledgeUnknown, Confirmation: "SHUTDOWN"}
		for _, test := range []struct {
			input string
			want  bool
		}{{"SHUTDOWN\n", true}, {"shutdown\n", false}, {"SHUTDOWN pc01,pc02\n", false}, {"y\n", false}, {"yes\n", false}, {"\n", false}} {
			var output bytes.Buffer
			approved, err := ConfirmShutdown(strings.NewReader(test.input), &output, report)
			if err != nil || approved != test.want || !strings.Contains(output.String(), "Controller: always excluded") || !strings.Contains(output.String(), "SHUTDOWN authorizes shutdown of 1 computer") || !strings.Contains(output.String(), "Type SHUTDOWN to continue") || !strings.Contains(output.String(), "not physical power state") {
				t.Fatalf("%s session: confirmation %q = %t, %v:\n%s", session, test.input, approved, err, output.String())
			}
		}
	}
	// Every computer is idle, logged in without use, or off: a yes/no answer.
	for _, sessions := range [][]domain.ShutdownTargetPlan{
		{target("pc01", domain.ShutdownSessionIdle), target("pc02", domain.ShutdownSessionUnused)},
		{target("pc01", domain.ShutdownSessionIdle), {Name: "pc03", Eligible: false, Session: domain.ShutdownSessionActive}},
	} {
		report := domain.ShutdownPlanReport{Eligible: 1, Targets: sessions, Policy: domain.ShutdownProtectUnknown}
		for _, test := range yesNoCases("SHUTDOWN") {
			var output bytes.Buffer
			approved, err := ConfirmShutdown(strings.NewReader(test.input), &output, report)
			if err != nil || approved != test.want || strings.Contains(output.String(), "Type SHUTDOWN") || !strings.Contains(output.String(), "Continue? [y/N]") {
				t.Fatalf("idle review: confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
			}
		}
	}
	// A restart follows the same rule with its own word.
	restart := domain.ShutdownPlanReport{Action: domain.ClientRestart, Eligible: 1, Targets: []domain.ShutdownTargetPlan{target("pc01", domain.ShutdownSessionActive)}, Policy: domain.ShutdownProtectUnknown, Confirmation: "RESTART"}
	var output bytes.Buffer
	approved, err := ConfirmShutdown(strings.NewReader("RESTART\n"), &output, restart)
	if err != nil || !approved || !strings.Contains(output.String(), "Type RESTART to continue") {
		t.Fatalf("restart with an active session: %t, %v:\n%s", approved, err, output.String())
	}
}
