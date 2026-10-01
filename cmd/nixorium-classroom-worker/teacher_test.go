package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestTeacherMessagesNameTheNextStep(t *testing.T) {
	for message, want := range map[string]string{
		adapters.OperationBusyMessage + ": Update computers, started by admin at 10:02":                       "The administrator is working on the computers (Update computers, started by admin at 10:02). Try again in a few minutes. Code: OP-BUSY.",
		"an unfinished client deployment blocks new operations; inspect deployment-pending.json":              "Code: DEPLOY-PENDING.",
		"a USB installation remains reserved; reconcile or close that operation first":                        "Code: USB-RESERVED.",
		"network installation is active; stop it before shutting down clients":                                "Code: PXE-ACTIVE.",
		"No selected computer is currently eligible for this power action. Nothing will be queued for later.": "No selected computer is currently eligible",
	} {
		if got := teacherMessage(message); !strings.Contains(got, want) {
			t.Fatalf("teacherMessage(%q) = %q", message, got)
		}
	}
	if got := teacherMessage("deployment.json: open /home/admin: permission denied"); strings.Contains(got, "Code") {
		t.Fatalf("unknown message rewritten: %q", got)
	}
	failure := classroomFailure(errors.New("evaluate labMeta: nix: error: attribute missing"))
	if failure.State != "failed" || strings.Contains(failure.Message, "labMeta") || !strings.Contains(failure.Message, "CLASSROOM-LOAD") {
		t.Fatalf("failure = %+v", failure)
	}
	if issues := teacherIssues(nil); issues != nil {
		t.Fatal("empty issue list changed")
	}
	issues := teacherIssues([]domain.ValidationIssue{{Field: "operation", Message: adapters.OperationBusyMessage}})
	if !strings.Contains(issues[0].Message, "OP-BUSY") {
		t.Fatalf("issues = %+v", issues)
	}
}
