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
