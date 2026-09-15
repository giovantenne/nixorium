package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestConfirmControllerApplyRequiresExactToken(t *testing.T) {
	for _, test := range []struct {
		input string
		want  bool
	}{{"APPLY\n", true}, {"yes\n", false}, {"apply\n", false}} {
		output := &bytes.Buffer{}
		got, err := ConfirmControllerApply(strings.NewReader(test.input), output, "pc99")
		if err != nil || got != test.want || !strings.Contains(output.String(), "services and networking may restart") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}

func TestConfirmControllerRebuildRequiresExactToken(t *testing.T) {
	report := domain.ControllerRebuildPlanReport{Controller: "pc99", Revision: "0123456789abcdef0123456789abcdef01234567", Confirmation: "REBUILD pc99"}
	for _, test := range []struct {
		input string
		want  bool
	}{{"REBUILD pc99\n", true}, {"REBUILD\n", false}, {"rebuild pc99\n", false}} {
		output := &bytes.Buffer{}
		got, err := ConfirmControllerRebuild(strings.NewReader(test.input), output, report)
		if err != nil || got != test.want || !strings.Contains(output.String(), report.Revision) {
			t.Fatalf("input %q: got %t, err %v, output %q", test.input, got, err, output.String())
		}
	}
}

func TestConfirmServiceRestartRequiresExactToken(t *testing.T) {
	service := domain.ManagedService{Name: "Binary cache"}
	for _, test := range []struct {
		input string
		want  bool
	}{{"RESTART CACHE\n", true}, {"restart cache\n", false}, {"RESTART\n", false}} {
		output := &bytes.Buffer{}
		got, err := ConfirmServiceRestart(strings.NewReader(test.input), output, service)
		if err != nil || got != test.want || !strings.Contains(output.String(), "PXE networking") {
			t.Fatalf("input %q: got %t, err %v, output %q", test.input, got, err, output.String())
		}
	}
}

func TestConfirmPXEStartRequiresExactToken(t *testing.T) {
	report := domain.PXELifecycleReport{Interface: "enp1s0", DHCPAddress: "192.0.2.10", StaticCIDR: "10.0.0.99/8"}
	for _, test := range []struct {
		input string
		want  bool
	}{{"START PXE\n", true}, {"start pxe\n", false}, {"yes\n", false}} {
		output := &bytes.Buffer{}
		got, err := ConfirmPXEStart(strings.NewReader(test.input), output, report)
		if err != nil || got != test.want || !strings.Contains(output.String(), "10.0.0.99/8") || !strings.Contains(output.String(), "reboot recovery is enabled") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}

func TestConfirmDeploymentApplyRequiresExactTargets(t *testing.T) {
	report := domain.DeploymentPlanReport{
		Revision:        "0123456789abcdef",
		ColmenaSelector: "pc01,pc03",
		Targets:         []domain.DeploymentTarget{{Name: "pc01"}, {Name: "pc03"}},
	}
	for _, test := range []struct {
		input string
		want  bool
	}{{"DEPLOY pc01,pc03\n", true}, {"DEPLOY pc01\n", false}, {"deploy pc01,pc03\n", false}} {
		output := &bytes.Buffer{}
		got, err := ConfirmDeploymentApply(strings.NewReader(test.input), output, report)
		if err != nil || got != test.want || !strings.Contains(output.String(), report.Revision) || !strings.Contains(output.String(), "rebuilds before every apply") {
			t.Fatalf("input %q: got %v, error %v, output %q", test.input, got, err, output)
		}
	}
}

func TestConfirmGitCommitRequiresExactReviewPhrase(t *testing.T) {
	report := domain.GitCommitPlanReport{Revision: "abc", Paths: []string{"lab-settings.json"}, CommitMessage: "chore: update laboratory settings", Confirmation: "COMMIT abcdef012345"}
	for _, test := range []struct {
		input string
		want  bool
	}{
		{input: "COMMIT abcdef012345\n", want: true},
		{input: "commit abcdef012345\n", want: false},
		{input: "COMMIT wrong\n", want: false},
	} {
		var output bytes.Buffer
		approved, err := ConfirmGitCommit(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "no hook, signing action, remote, or push") {
			t.Fatalf("confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}

func TestConfirmUpdateRequiresExactReleasePhrase(t *testing.T) {
	report := domain.UpdatePlanReport{CurrentRef: "v2.0.0", CurrentRev: "abc", Target: "v2.1.0", TargetChannel: domain.UpdateChannelStable, Confirmation: "UPDATE NIXORIUM TO v2.1.0"}
	for _, test := range []struct {
		input string
		want  bool
	}{{"UPDATE NIXORIUM TO v2.1.0\n", true}, {"update nixorium to v2.1.0\n", false}, {"UPDATE NIXORIUM\n", false}} {
		var output bytes.Buffer
		approved, err := ConfirmUpdate(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "no branch, commit, push, activation") {
			t.Fatalf("confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}

func TestConfirmSoftwareChangeRequiresExactReviewPhrase(t *testing.T) {
	report := domain.SoftwareChangePlanReport{
		Request:         domain.SoftwareChangeRequest{Package: "vlc", Present: true, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeGroup, Group: "graphics"}},
		AffectedClients: []string{"pc01", "pc02"},
		Confirmation:    "SAVE SOFTWARE abcdef012345",
	}
	for _, test := range []struct {
		input string
		want  bool
	}{{"SAVE SOFTWARE abcdef012345\n", true}, {"save software abcdef012345\n", false}, {"SAVE SOFTWARE wrong\n", false}} {
		var output bytes.Buffer
		approved, err := ConfirmSoftwareChange(strings.NewReader(test.input), &output, report)
		if err != nil || approved != test.want || !strings.Contains(output.String(), "only lab-software.json") || !strings.Contains(output.String(), "no commit, build, controller activation") {
			t.Fatalf("confirmation %q = %t, %v:\n%s", test.input, approved, err, output.String())
		}
	}
}
