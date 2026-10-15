package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestJSONAddsNextStepsWithoutChangingTheReport(t *testing.T) {
	report := domain.ShutdownPlanReport{State: "blocked", Issues: []domain.ValidationIssue{{Field: "operation", Message: "another Nixorium controller or client operation is already running"}}}
	var output bytes.Buffer
	if err := JSON(&output, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"code": "OP-BUSY"`) {
		t.Fatalf("json = %s", output.String())
	}
	if report.Issues[0].Next != nil {
		t.Fatal("JSON output modified the caller's report")
	}
}

func TestNoticesShowTheNextStep(t *testing.T) {
	line := noticeNextStep(tuiNotice{kind: tuiStatusAttention, title: "deployment worktree has 3 changed path(s)"})
	if !strings.Contains(line, "Next:") || !strings.Contains(line, "Review Git changes") {
		t.Fatalf("line = %q", line)
	}
	if noticeNextStep(tuiNotice{kind: tuiStatusSuccess, title: "deployment worktree has 3 changed path(s)"}) != "" {
		t.Fatal("success notice got a next step")
	}
}

func TestSituationNoticeKeepsTheTechnicalTextWhereDCannotShowIt(t *testing.T) {
	shell := tuiShell{notices: []tuiNotice{failureNotice("The settings were not saved", "managed settings are invalid; repair them before applying a candidate")}}
	notices := buildTUIShellRegions(shell, 200, false).notices
	for _, want := range []string{
		"The settings were not saved: the laboratory settings are not valid.",
		"Nixorium refuses to apply settings it cannot check.",
		"Next: Repair the settings (Maintenance → Change settings)",
		"Details: managed settings are invalid",
	} {
		if !strings.Contains(notices, want) {
			t.Fatalf("notice omits %q:\n%s", want, notices)
		}
	}
	shell.detailsKey = true
	if notices := buildTUIShellRegions(shell, 200, false).notices; strings.Contains(notices, "Details:") {
		t.Fatalf("hidden details shown:\n%s", notices)
	}
	plain := tuiShell{notices: []tuiNotice{{kind: tuiStatusNeutral, title: "retry from a fresh plan", plain: true}}}
	if notices := buildTUIShellRegions(plain, 200, false).notices; strings.Contains(notices, "Next:") {
		t.Fatalf("plain notice was reworded:\n%s", notices)
	}
}
