package presentation

import (
	"fmt"
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
	setDemoAssistantWebProposal(&r.model, "pc01–pc02")
	r.model.message = ""
	r.capture("A candidate is ready for native review", 2500)
	r.model.message = ""
	r.model.screen = dashboardAssistantReview
	r.model.assistant.review.plan = demoAssistantWebPlan()
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
	pc03 := r.model.computers.hosts.Hosts[2]
	r.model.assistant.entries = []assistantEntry{
		{role: "user", text: assistantExamples[1]},
		{role: "activity", text: "● Reading pc03 update evidence"},
		{role: "assistant", text: fmt.Sprintf("**pc03 is out of date.** Its observed revision is `%s`; desired `%s`. The cause is not established.\n\nNo failure log was supplied. Next: run the read-only updates diagnostic on pc03; check `nix-daemon.service` and its current-boot journal before reviewing a retry. Nothing was changed.", shortRevision(pc03.CurrentRevision), shortRevision(pc03.DesiredRevision))},
	}
	r.capture("Diagnose from observed evidence and name the next check", 3500)
	r.model.assistant.entries = []assistantEntry{
		{role: "user", text: assistantExamples[2]},
		{role: "assistant", text: "**Lesson proposal:** a pinned Python environment with Jupyter + pandas, plus a notebook and CSV in `home-files.json`.\n\nReview and save first. Prepare all dependencies and lesson files on the controller before going offline; deploy separately. Lessons then need no downloads.\n\nAt boot, the student home returns to the lesson baseline. Student edits are lost; save work elsewhere before restarting. No running session was reset."},
	}
	r.model.assistant.changedFiles = 3
	r.capture("Offline lesson proposal with boot restoration", 3500)
	return DemoScenario{ID: "assistant-menu", Title: "Optional administrator Assistant", Description: "The controller offers the fifth administrator area by default; the classroom dashboard never offers it.", Frames: r.frames}
}

// These are illustrative candidate files, not modules installed by the demo.
// Both the scope and the LAN source restriction remain visible in review.
func demoAssistantWebPlan() domain.AssistantSavePlan {
	return domain.AssistantSavePlan{
		State: "ready", Added: []string{"modules/web-lab.nix"}, Changed: []string{"flake.nix"},
		ChangedHosts: []string{"pc01", "pc02"}, Evaluation: domain.AssistantEvaluation{OK: true},
		Diff: `--- a/flake.nix
+++ b/flake.nix
 hostModules = {
+  pc01 = [ ./modules/web-lab.nix ];
+  pc02 = [ ./modules/web-lab.nix ];
 };
--- /dev/null
+++ b/modules/web-lab.nix
+{ pkgs, labSettings, ... }:
+{
+  assertions = [{ assertion = labSettings.ifaceExplicit;
+    message = "Declare the lab interface before opening the web port."; }];
+  environment.systemPackages = [ pkgs.nodejs ];
+  networking.firewall.extraInputRules = ''
+    iifname "${labSettings.ifaceName}" ip saddr ${labSettings.networkBase}/${toString labSettings.networkPrefixLength} tcp dport 3000 accept
+  '';
+}`,
	}
}

func demoAssistantWebActivities(hosts string) []assistantEntry {
	return []assistantEntry{
		{role: "activity", text: "● Reading flake.nix and client scopes"},
		{role: "activity", text: "● Reading the lab network and interface"},
		{role: "activity", text: "● Resolving Node.js from the pinned package set"},
		{role: "activity", text: "● Drafting modules/web-lab.nix"},
		{role: "activity", text: "● Scoping the proposal to " + hosts},
		{role: "activity", text: "● Preparing the proposal for native review"},
	}
}

func setDemoAssistantWebProposal(model *dashboardModel, hosts string) {
	model.assistant = newAssistantModel()
	model.assistant.account = domain.AssistantAccount{Name: "Your school", Plan: "annual", Status: "active", TurnsRemaining: 982}
	model.assistant.offer = demoAssistantOffer()
	model.assistant.entries = []assistantEntry{{role: "user", text: fmt.Sprintf("Prepare Node.js on %s; open port 3000 only to the lab LAN", hosts)}}
	model.assistant.entries = append(model.assistant.entries, demoAssistantWebActivities(hosts)...)
	model.assistant.entries = append(model.assistant.entries, assistantEntry{role: "assistant", text: demoAssistantWebAnswer(hosts)})
	model.assistant.summary = fmt.Sprintf("Node.js and LAN-only TCP 3000 on %s. Controller and other PCs unchanged.", hosts)
	model.assistant.changedFiles = 2
}

func demoAssistantWebAnswer(hosts string) string {
	return fmt.Sprintf("**Proposal ready:** Node.js on %s.\n\n- TCP 3000 accepts only the lab LAN on its declared interface.\n- The controller and other PCs are unchanged.\n\nCtrl+R reviews and saves the proposal. Deployment needs a separate review; nothing is installed yet.", hosts)
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
// a sending frame, six activities and the proposal. No service or build runs.
func RenderAssistantPricingDemos() []AssistantPricingDemo {
	type localized struct{ code, school, prompt, answer string }
	copies := []localized{
		{"en", "Your school", assistantExamples[0], demoAssistantWebAnswer("pc01–pc06")},
		{"it", "La tua scuola", "Prepara Node.js su pc01–pc06; apri la porta 3000 solo alla LAN del laboratorio", "**Proposta pronta:** Node.js su pc01–pc06.\n\n- TCP 3000 solo dalla LAN, sull’interfaccia dichiarata.\n- Controller e altri PC invariati.\n\nCtrl+R per rivedere e salvare. La distribuzione richiede una revisione separata; nulla è ancora installato."},
		{"es", "Tu escuela", "Prepara Node.js en pc01–pc06; abre el puerto 3000 solo a la LAN del laboratorio", "**Propuesta lista:** Node.js en pc01–pc06.\n\n- TCP 3000 solo desde la LAN, en la interfaz declarada.\n- Controlador y otros equipos sin cambios.\n\nCtrl+R para revisar y guardar. El despliegue requiere otra revisión; aún no se ha instalado nada."},
		{"fr", "Votre école", "Prépare Node.js sur pc01–pc06 ; ouvre le port 3000 seulement au LAN du laboratoire", "**Proposition prête :** Node.js sur pc01–pc06.\n\n- TCP 3000 depuis le LAN, sur son interface déclarée.\n- Contrôleur et autres postes inchangés.\n\nCtrl+R pour vérifier et enregistrer. Le déploiement exige une autre validation ; rien n’est encore installé."},
		{"de", "Ihre Schule", "Bereite Node.js auf pc01–pc06 vor; öffne Port 3000 nur für das Labor-LAN", "**Vorschlag bereit:** Node.js auf pc01–pc06.\n\n- TCP 3000 nur aus dem LAN, über die festgelegte Schnittstelle.\n- Controller und andere PCs bleiben unverändert.\n\nStrg+R zum Prüfen und Speichern. Bereitstellung separat prüfen; noch nichts installiert."},
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
		for _, activity := range demoAssistantWebActivities("pc01–pc06") {
			r.model.assistant.entries = append(r.model.assistant.entries, activity)
			r.capture("Draft the scoped proposal", 1400)
		}
		r.model.assistant.state = "chat"
		r.model.assistant.changedFiles = 2
		r.model.assistant.entries = append(r.model.assistant.entries, assistantEntry{role: "assistant", text: copy.answer})
		r.capture("Proposal awaiting review and deployment", 4500)
		demos = append(demos, AssistantPricingDemo{Locale: copy.code, Prompt: copy.prompt, Frames: r.frames})
	}
	return demos
}
