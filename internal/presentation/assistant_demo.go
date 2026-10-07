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
	return DemoScenario{ID: "assistant-menu", Title: "Optional administrator Assistant", Description: "The installed controller opt-in adds the fifth administrator area; the classroom dashboard never offers it.", Frames: r.frames}
}
