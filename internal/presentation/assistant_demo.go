package presentation

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// RenderAssistantMenuDemo uses the installed controller menu with no service calls.
func RenderAssistantMenuDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Assistant is available by default on the controller", 1200)
	r.pressAndCapture(demoText(assistantTask.shortcut), "Open Assistant", 1200)
	r.model.assistant.offer = demoAssistantOffer()
	r.capture("The chat opens first, even before activation", 2500)
	r.model.assistant.input.SetValue(assistantExamples[0])
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
	r.model.assistant.reportComment.SetValue("The scope review was helpful.")
	r.capture("Report an answer with whole-conversation consent", 2500)
	r.model.assistant.reportComment.Reset()
	r.model.assistant.state = "chat"
	r.model.assistant.input.Reset()
	r.model.assistant.account = domain.AssistantAccount{Name: "Example laboratory", Plan: "trial", Status: "active", TurnsRemaining: 3, TrialRequests: 10}
	r.capture("Check remaining free trial requests", 2500)
	r.model.assistant.account.TurnsRemaining = 0
	r.model.assistant.account.UpgradeURL = "https://assistant.nixorium.org/u/EjRWeJASNEjRWeJASNEjRWeJASNEjRWeJASNEjRW"
	r.model.assistant.state = "chat"
	r.model.message = r.model.assistantNotice(&domain.AssistantError{Code: "quota_exhausted"})
	r.capture("Convert a free trial without reactivating the controller", 2500)
	r.model.assistant.account = domain.AssistantAccount{}
	r.model.assistant.state = "chat"
	setDemoAssistantProposal(&r.model, "")
	r.model.message = ""
	r.capture("A candidate is ready for native review", 2500)
	r.model.message = ""
	r.model.screen = dashboardAssistantReview
	r.model.assistant.review.plan = demoAssistantPlan()
	r.model.assistant.review.cursor = 1
	r.capture("Review evaluated changes before saving", 2500)
	r.model.assistant.review.plan.Evaluation = domain.AssistantEvaluation{Error: "The candidate module does not evaluate."}
	r.capture("A failed evaluation prevents saving", 2500)
	r.model.assistant.review.plan.Evaluation = domain.AssistantEvaluation{OK: true}
	r.model.assistant.review.saveOnly = true
	r.model.assistant.review.result = domain.TemplateResetResult{State: "saved", Revision: revision, BackupRef: "refs/nixorium/assistant-backups/example"}
	r.capture("A local commit and backup precede separate application", 2500)
	r.model.screen = dashboardAssistant
	r.model.assistant = newAssistantModel()
	r.model.assistant.account = domain.AssistantAccount{Name: "Example laboratory", Plan: "annual", Status: "active", TurnsRemaining: 982}
	r.model.computers.hosts = demoExploreHosts(revision)
	r.model.assistant.entries = []assistantEntry{
		{role: "user", text: assistantExamples[2]},
		{role: "activity", text: "● Reading the devices of pc99"},
		{role: "assistant", text: "**No display is connected to the HDMI port of pc99.** The computer sees the port, so check the cable and the projector's input source. Nothing was changed."},
	}
	r.capture("Diagnose a device from what the computer sees", 3500)
	r.model.assistant.entries = []assistantEntry{
		{role: "user", text: assistantExamples[1]},
		{role: "assistant", text: "**Proposal ready:** 2 files changed.\n\nReview and save first. Deployment is a separate review; nothing is installed yet."},
	}
	r.model.assistant.changedFiles = 2
	r.capture("A second proposal awaits review", 3500)
	return DemoScenario{ID: "assistant-menu", Title: "Optional administrator Assistant", Description: "The controller offers the fifth administrator area by default; the classroom dashboard never offers it.", Frames: r.frames}
}

// This is an illustrative candidate, not a file installed by the demo.
func demoAssistantPlan() domain.AssistantSavePlan {
	return domain.AssistantSavePlan{
		State: "ready", Changed: []string{"lab-software.json"},
		ChangedHosts: []string{"pc01", "pc02"}, Evaluation: domain.AssistantEvaluation{OK: true},
		Diff: `--- a/lab-software.json
+++ b/lab-software.json
+    { "package": "python3Packages.jupyter", "scope": { "kind": "group", "group": "4B" } },
+    { "package": "python3Packages.pandas", "scope": { "kind": "group", "group": "4B" } },`,
	}
}

func demoAssistantActivities(hosts string) []assistantEntry {
	return []assistantEntry{{role: "activity", text: "● Preparing the proposal for native review"}}
}

func setDemoAssistantProposal(model *dashboardModel, hosts string) {
	model.assistant = newAssistantModel()
	model.assistant.account = domain.AssistantAccount{Name: "Your school", Plan: "annual", Status: "active", TurnsRemaining: 982}
	model.assistant.offer = demoAssistantOffer()
	model.assistant.entries = []assistantEntry{{role: "user", text: assistantExamples[0]}}
	model.assistant.entries = append(model.assistant.entries, demoAssistantActivities(hosts)...)
	model.assistant.entries = append(model.assistant.entries, assistantEntry{role: "assistant", text: demoAssistantAnswer(hosts)})
	model.assistant.summary = "Python with pandas and Jupyter for class 4B in lab-software.json."
	model.assistant.changedFiles = 1
}

func demoAssistantAnswer(string) string {
	return "**Proposal ready:** 1 file changed.\n\nCtrl+R reviews and saves the proposal. Deployment needs a separate review; nothing is installed yet."
}

// AssistantPricingDemo is the reproducible input to the website ANSI converter.
// The interface stays English; only the illustrative conversation is localized.
type AssistantPricingDemo struct {
	Locale string      `json:"locale"`
	Prompt string      `json:"prompt"`
	Frames []DemoFrame `json:"frames"`
}

// RenderAssistantPricingDemos calls the real renderer, with local synthetic
// state only. Each sequence has an empty chat, one frame per Unicode character,
// a sending frame, one activity and the proposal. No service or build runs.
func RenderAssistantPricingDemos() []AssistantPricingDemo {
	type localized struct{ code, school, prompt, answer string }
	copies := []localized{
		{"en", "Your school", "Block YouTube and TikTok on the student computers", "**Proposal ready:** 1 file changed.\n\nCtrl+R reviews and saves the proposal. Deployment needs a separate review; nothing is installed yet."},
		{"it", "La tua scuola", "Blocca YouTube e TikTok sui computer degli studenti", "**Proposta pronta:** 1 file modificato.\n\nCtrl+R per rivedere e salvare la proposta. La distribuzione richiede una revisione separata; nulla è ancora installato."},
		{"es", "Tu escuela", "Bloquea YouTube y TikTok en los equipos de los estudiantes", "**Propuesta lista:** 1 archivo modificado.\n\nCtrl+R para revisar y guardar la propuesta. El despliegue requiere otra revisión; aún no se ha instalado nada."},
		{"fr", "Votre école", "Bloque YouTube et TikTok sur les postes des élèves", "**Proposition prête :** 1 fichier modifié.\n\nCtrl+R pour vérifier et enregistrer la proposition. Le déploiement exige une autre validation ; rien n’est encore installé."},
		{"de", "Ihre Schule", "Sperre YouTube und TikTok auf den Schüler-PCs", "**Vorschlag bereit:** 1 Datei geändert.\n\nStrg+R zum Prüfen und Speichern des Vorschlags. Bereitstellung separat prüfen; noch nichts installiert."},
	}
	demos := make([]AssistantPricingDemo, 0, len(copies))
	for _, copy := range copies {
		r := newDemoRecorder(DashboardActions{Assistant: demoAssistantConfig()}, "demo", 84, 28)
		r.model.screen = dashboardAssistant
		r.model.assistant = newAssistantModel()
		r.model.assistant.account = domain.AssistantAccount{Name: copy.school, Plan: "annual", Status: "active", TurnsRemaining: 982}
		r.capture("Empty chat", 600)
		var draft strings.Builder
		for _, character := range copy.prompt {
			draft.WriteRune(character)
			r.model.assistant.input.SetValue(draft.String())
			r.model.assistant.input.CursorEnd()
			r.capture("Type the request", 80)
		}
		r.model.assistant.input.Reset()
		r.model.assistant.entries = []assistantEntry{{role: "user", text: copy.prompt}}
		r.model.assistant.state = "running"
		r.capture("Send the illustrative request", 1200)
		for _, activity := range demoAssistantActivities("") {
			r.model.assistant.entries = append(r.model.assistant.entries, activity)
			r.capture("Prepare the proposal", 1400)
		}
		r.model.assistant.state = "chat"
		r.model.assistant.changedFiles = 1
		r.model.assistant.entries = append(r.model.assistant.entries, assistantEntry{role: "assistant", text: copy.answer})
		r.capture("Proposal awaiting review and deployment", 4500)
		demos = append(demos, AssistantPricingDemo{Locale: copy.code, Prompt: copy.prompt, Frames: r.frames})
	}
	return demos
}
