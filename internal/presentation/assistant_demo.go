package presentation

import (
	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// RenderAssistantMenuDemo uses the real opt-in navigation with no service calls.
func RenderAssistantMenuDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Assistant appears only after controller opt-in", 1200)
	r.pressAndCapture(demoText(assistantTask.shortcut), "Open Assistant", 1200)
	r.model.assistant.offer = demoAssistantOffer()
	r.capture("The chat opens first, even before activation", 2500)
	r.model.assistant.input.SetValue("Install GIMP on every computer.")
	r.pressAndCapture(tea.KeyPressMsg{Code: tea.KeyEnter}, "Sending asks to activate; the request waits", 2500)
	r.capture("Choose a free trial, activation code or subscription", 2500)
	r.model.assistant.state = "activate"
	r.model.assistant.activationLink = r.model.assistant.offer.TrialURL
	r.model.assistant.activationKind = "free trial"
	r.capture("Open the free trial page to register with your email", 2500)
	r.model.assistant.codeStep = true
	r.capture("Paste the code you receive by email", 2500)
	r.model.assistant.state = "consent"
	r.capture("Continue after separate Assistant consent", 2500)
	r.model.assistant.state = "sharing"
	r.capture("Review optional conversation sharing", 2500)
	r.model.assistant.state = "report"
	r.model.assistant.reportComment.SetValue("The package check was helpful.")
	r.capture("Report an answer with whole-conversation consent", 2500)
	r.model.assistant.reportComment.Reset()
	r.model.assistant.state = "chat"
	r.model.assistant.input.Reset()
	r.model.assistant.account = domain.AssistantAccount{Name: "Example laboratory", Plan: "trial", Status: "active", TurnsRemaining: 3}
	r.capture("Check remaining free trial requests", 2500)
	r.model.assistant.account.TurnsRemaining = 0
	r.model.assistant.account.UpgradeURL = "https://assistant.nixorium.org/u/EjRWeJASNEjRWeJASNEjRWeJASNEjRWeJASNEjRW"
	r.model.assistant.state = "chat"
	r.model.message = r.model.assistantNotice(&domain.AssistantError{Code: "quota_exhausted"})
	r.capture("Convert a free trial without reactivating the controller", 2500)
	r.model.assistant.account = domain.AssistantAccount{}
	r.model.assistant.state = "chat"
	r.model.assistant.entries = []assistantEntry{{role: "user", text: "Prepare the course for pc01."}, {role: "activity", text: "● Reading modules/course.nix"}, {role: "activity", text: "● Writing modules/course.nix"}, {role: "activity", text: "✓ Evaluation passed · changes pc01"}, {role: "assistant", text: "**Prepared** the course for pc01."}}
	r.model.assistant.summary = "Prepared the course for pc01."
	r.model.assistant.changedFiles = 1
	r.model.message = ""
	r.capture("A candidate is ready for native review", 2500)
	r.model.message = ""
	r.model.screen = dashboardAssistantReview
	r.model.assistant.review.plan = domain.AssistantSavePlan{
		State: "ready", Changed: []string{"modules/course.nix"}, ChangedHosts: []string{"pc01"},
		Evaluation: domain.AssistantEvaluation{OK: true},
		Diff:       "--- a/modules/course.nix\n+++ b/modules/course.nix\n-{ course = 1; }\n+{ course = 2; }",
	}
	r.capture("Review evaluated changes before saving", 2500)
	r.model.assistant.review.plan.Evaluation = domain.AssistantEvaluation{Error: "The candidate module does not evaluate."}
	r.capture("A failed evaluation prevents saving", 2500)
	r.model.assistant.review.result = domain.TemplateResetResult{State: "saved", Revision: revision, BackupRef: "refs/nixorium/assistant-backups/example"}
	r.capture("A local commit and backup precede separate application", 2500)
	return DemoScenario{ID: "assistant-menu", Title: "Optional administrator Assistant", Description: "The installed controller opt-in adds the fifth administrator area; the classroom dashboard never offers it.", Frames: r.frames}
}
