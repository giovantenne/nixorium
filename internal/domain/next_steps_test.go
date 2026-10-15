package domain

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestNextStepForSharedBlockers(t *testing.T) {
	for message, code := range map[string]string{
		"another Nixorium controller or client operation is already running: Update computers, started by admin at 10:02": "OP-BUSY",
		MessageDeploymentPending + "; inspect deployment-pending.json":                                                    "DEPLOY-PENDING",
		"inspect active client operations: " + MessageDeploymentPending:                                                   "DEPLOY-PENDING",
		MessageUSBReserved + "; reconcile or close that operation first":                                                  "USB-RESERVED",
		"unfinished deployment template reset: preserve .git/nixorium-template-reset.json":                                "RESET-PENDING",
		"deployment worktree has 2 changed path(s)":                                                                       "GIT-DIRTY",
		"shutdown review expired before dispatch":                                                                         "REVIEW-EXPIRED",
		"cleanup review token does not match the frozen plan":                                                             "REVIEW-CHANGED",
		"network installation is active; stop it before freeing disk space":                                               "PXE-ACTIVE",
		"managed settings are invalid; repair them before applying a candidate":                                           "SETTINGS-INVALID",
		"request result could not be confirmed; inspect the computer before retrying":                                     "CLIENT-UNCONFIRMED",
	} {
		step, found := NextStepFor(message)
		if !found || step.Code != code || step.Action == "" {
			t.Fatalf("NextStepFor(%q) = %+v, %v; want %s", message, step, found, code)
		}
	}
	for _, message := range []string{"Old versions removed on 2 computer(s).", "The administrator is working on the computers. Code: OP-BUSY."} {
		if step, found := NextStepFor(message); found {
			t.Fatalf("NextStepFor(%q) = %+v", message, step)
		}
	}
	issues := WithNextSteps([]ValidationIssue{{Field: "git", Message: "deployment worktree has 1 changed path(s)"}, {Field: "x", Message: "unrelated"}})
	if issues[0].Next == nil || issues[0].Next.Code != "GIT-DIRTY" || issues[1].Next != nil {
		t.Fatalf("issues = %+v", issues)
	}
	if line := issues[0].Next.Line(); !strings.Contains(line, "Next (GIT-DIRTY)") || !strings.Contains(line, "nixorium git review") {
		t.Fatalf("line = %q", line)
	}
}

func TestEveryNextStepCodeIsDocumented(t *testing.T) {
	guide, err := os.ReadFile("../../docs/troubleshooting.md")
	if os.IsNotExist(err) {
		// Package builds omit documentation; tests/ux-shell-regressions.sh
		// repeats this check against the source tree.
		t.Skip("documentation is not part of this source tree")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range NextStepCodes() {
		if !strings.Contains(string(guide), "`"+code+"`") {
			t.Fatalf("code %s is not explained in docs/troubleshooting.md", code)
		}
	}
}

func TestSituationCatalogueIsComplete(t *testing.T) {
	for _, code := range NextStepCodes() {
		step, _ := NextStepByCode(code)
		if step.Code != code || step.Title == "" || step.Why == "" || step.Action == "" {
			t.Fatalf("situation %s is incomplete: %+v", code, step)
		}
		for _, word := range []string{".json", "PID", "Colmena", "token", "lock"} {
			if strings.Contains(step.Title, word) {
				t.Fatalf("situation %s title %q uses the implementation word %q", code, step.Title, word)
			}
		}
		if strings.HasSuffix(step.Title, ".") || strings.HasSuffix(step.Action, ".") {
			t.Fatalf("situation %s title and action are phrases, not sentences: %+v", code, step)
		}
	}
	step, _ := NextStepByCode("DEPLOY-PENDING")
	if step.Screen != RecoveryDeploymentPending {
		t.Fatalf("DEPLOY-PENDING screen = %q", step.Screen)
	}
	if got := step.Headline("The controller was not updated"); got != "The controller was not updated: a previous update of the student computers was interrupted." {
		t.Fatalf("headline = %q", got)
	}
	if got := step.Headline(""); got != "A previous update of the student computers was interrupted." {
		t.Fatalf("headline = %q", got)
	}
}

func TestBlockedErrorRoundTrip(t *testing.T) {
	raw := errors.New("something the operator should not have to read")
	err := fmt.Errorf("controller build failed: %w", Blocked("RESET-PENDING", raw))
	step, found := NextStepForError(err)
	if !found || step.Code != "RESET-PENDING" {
		t.Fatalf("NextStepForError = %+v, %v", step, found)
	}
	if err.Error() != "controller build failed: "+raw.Error() || !errors.Is(err, raw) {
		t.Fatalf("blocked error text or chain changed: %v", err)
	}
	var blocked *BlockedError
	if !errors.As(err, &blocked) || blocked.Code != "RESET-PENDING" {
		t.Fatalf("errors.As = %+v", blocked)
	}
	// Untyped errors still fall back to the message.
	if step, found := NextStepForError(errors.New(MessageDeploymentPending)); !found || step.Code != "DEPLOY-PENDING" {
		t.Fatalf("fallback = %+v, %v", step, found)
	}
	if _, found := NextStepForError(nil); found {
		t.Fatal("nil error recognized")
	}
	issue := BlockedIssue("git", "GIT-DIRTY", "2 files")
	if issue.Next == nil || issue.Next.Code != "GIT-DIRTY" {
		t.Fatalf("issue = %+v", issue)
	}
}
