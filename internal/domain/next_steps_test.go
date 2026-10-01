package domain

import (
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
