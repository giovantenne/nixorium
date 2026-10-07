package presentation

import "github.com/giovantenne/nixorium/internal/domain"

// RenderAssistantMenuDemo uses the real opt-in navigation with no service calls.
func RenderAssistantMenuDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	actions.Assistant = domain.AssistantConfig{Enabled: true,
		Endpoint: "https://assistant.nixorium.org/v1", SubscribeURL: "https://nixorium.org/assistant/"}
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Assistant appears only after controller opt-in", 1200)
	r.pressAndCapture(demoText(assistantTask.shortcut), "Open Assistant", 1200)
	r.model.assistant.input.SetValue("Prepare Python with Jupyter on pc01–pc10")
	r.capture("Describe a customization before sending", 2500)
	r.model.assistant.state = "cta-none"
	r.capture("A subscription is required before sending", 2500)
	r.model.assistant.state = "consent"
	r.capture("Accept separate Assistant consent before sending", 2500)
	r.model.assistant.state = "chat"
	r.model.assistant.summary = "Prepared the course for pc01."
	r.model.assistant.changedFiles = 1
	r.model.message = "Proposal ready: 1 file changed. Press Ctrl+R to review."
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
