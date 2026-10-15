package presentation

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantCompleteHelpAndScrolling(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("help sent HTTP") })
	m.width, m.height = 80, 24
	m.assistant.input.SetValue("/help")
	updated, _ := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if !m.helpOpen {
		t.Fatal("help did not open")
	}
	text := m.helpView()
	for _, want := range []string{
		"How Assistant works\n  Describe what your lab should do. Assistant prepares the change in a private\n  copy and checks it. Nothing is saved or installed until you review and save it.\n  Each message you send uses one request from your trial or yearly allowance.",
		"/review   Review the current proposal and save it.", "/save     Same as /review: opens the review to save the proposal.", "/new      Start a new chat. Use it when you change topic or after saving.",
		"/report   Tell us one answer was helpful or wrong. Sends this conversation\n            to Nixorium once, so we can fix mistakes. Kept for 90 days.",
		"/share    Turn automatic sharing on or off (off by default). When on, every\n            conversation of this lab is sent to Nixorium to improve Assistant.\n            Kept for 90 days.",
		"/code     Enter an activation code (for example after subscribing).", "/help     Show this help.",
		"Keys\n  Enter        Send the message\n  Alt+Enter    New line (Ctrl+J also works)\n  Ctrl+R       Review the proposal\n  Ctrl+N       New chat\n  PgUp/PgDn    Scroll the conversation\n  Select text      Drag with the mouse (or Shift+drag in some terminals)\n  Esc          Stop Assistant while it works, or go back",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("help omits %q", want)
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "How Assistant works") {
		t.Fatal("help start invisible")
	}
	updated, _ = m.Update(keyPress("end"))
	m = updated.(dashboardModel)
	if !strings.Contains(ansi.Strip(m.View().Content), "Stop Assistant while it works, or go back") {
		t.Fatal("help end not scrollable")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.helpOpen || m.screen != dashboardAssistant {
		t.Fatal("help Esc changed screen")
	}
	updated, _ = m.Update(keyPress("f1"))
	m = updated.(dashboardModel)
	if !m.helpOpen || m.helpView() != text {
		t.Fatal("F1 differs from /help")
	}
}

func TestAssistantExplanations(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
	assistantEnableCommands(&m)
	m.assistant.input.SetValue("/")
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"/review  Review the current proposal and save it.", "/save  Same as /review: opens the review to save the proposal.", "/new  Start a new chat.", "/report  Tell us one answer was helpful or wrong.", "/share  Turn automatic sharing on or off (off by default).", "/code  Enter an activation code (for example after subscribing).", "/help  Show this help."} {
		if !strings.Contains(view, want) {
			t.Fatal("missing command explanation", want, view)
		}
	}
	for _, state := range []string{"consent"} {
		m.assistant.state = state
		if !strings.Contains(ansi.Strip(m.View().Content), "Each message you send uses one request.") {
			t.Fatal(state, "missing request explanation")
		}
	}
	m.assistant.state = "report"
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Sends only this conversation, once.") || strings.Index(view, "Sends only") > strings.Index(view, "This sends the whole") {
		t.Fatal("report distinction missing", view)
	}
	m.assistant.state = "sharing"
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Applies to all future conversations of this lab until you turn it off.") {
		t.Fatal("sharing distinction missing", view)
	}
}

func assistantCompletionGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid"}, args...)...)
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(data))
	}
	return strings.TrimSpace(string(data))
}

func TestAssistantOwnSaveStartsFreshChatAndExternalChangeStillWarns(t *testing.T) {
	for _, ownSave := range []bool{true, false} {
		t.Run(map[bool]string{true: "own-save", false: "external-change"}[ownSave], func(t *testing.T) {
			m, auth := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("test called network") })
			if err := auth.WriteToken(flowToken); err != nil {
				t.Fatal(err)
			}
			if err := auth.WriteConsent(); err != nil {
				t.Fatal(err)
			}
			repo := t.TempDir()
			assistantCompletionGit(t, repo, "init", "-q")
			if err := os.WriteFile(filepath.Join(repo, "flake.nix"), []byte("{outputs = _: {}; }\n"), 0644); err != nil {
				t.Fatal(err)
			}
			assistantCompletionGit(t, repo, "add", "flake.nix")
			assistantCompletionGit(t, repo, "commit", "-qm", "synthetic base")
			store := adapters.AssistantWorkspaceStore{Directory: auth.Directory}
			old, err := store.CreateConversation(repo)
			if err != nil {
				t.Fatal(err)
			}
			previous := []domain.AssistantMessage{{Role: "user", Content: "Previous question"}, {Role: "assistant", Content: "Previous answer"}}
			if err = store.WriteTranscript(old, previous); err != nil {
				t.Fatal(err)
			}
			oldTranscriptPath := filepath.Join(auth.Directory, "conversations", old.ID, "transcript.json")
			oldTranscriptBytes, err := os.ReadFile(oldTranscriptPath)
			if err != nil {
				t.Fatal(err)
			}
			m.assistant.conversation = old
			m.assistant.messages = previous
			m.assistant.entries = []assistantEntry{{role: "user", text: "Previous question"}, {role: "assistant", text: "Previous answer"}}
			if err = os.WriteFile(filepath.Join(repo, "course.nix"), []byte("{}\n"), 0644); err != nil {
				t.Fatal(err)
			}
			assistantCompletionGit(t, repo, "add", "course.nix")
			assistantCompletionGit(t, repo, "commit", "-qm", "synthetic change")
			revision := assistantCompletionGit(t, repo, "rev-parse", "HEAD")
			preparedID := "unprepared"
			runs := 0
			m.actions.AssistantService.PrepareConversation = func(id string) (domain.AssistantConversation, error) {
				preparedID = id
				return store.PrepareConversation(repo, id)
			}
			m.actions.AssistantService.Run = func(_ context.Context, c domain.AssistantConversation, prompt string, emit func(domain.AssistantEvent)) (domain.AssistantTurnResult, error) {
				runs++
				transcript, err := store.ReadTranscript(c)
				if err != nil || len(transcript) != 0 || c.ID == old.ID || c.Base != revision {
					t.Error("new conversation reused state", err)
				}
				emit(domain.AssistantEvent{Type: "turn_started"})
				emit(domain.AssistantEvent{Type: "final", Summary: "New answer"})
				return domain.AssistantTurnResult{Summary: "New answer", Messages: []domain.AssistantMessage{{Role: "user", Content: prompt}, {Role: "assistant", Content: "New answer"}}}, nil
			}
			if ownSave {
				m.screen = dashboardAssistantReview
				m.assistant.review.saving = true
				updated, _ := m.finishAssistantSave(assistantSaveResultMsg{domain.TemplateResetResult{State: "saved", Revision: revision}})
				m = updated.(dashboardModel)
				updated, _ = m.Update(keyPress("esc"))
				m = updated.(dashboardModel)
				if !strings.Contains(ansi.Strip(m.View().Content), assistantSavedChatText) {
					t.Fatal("saved chat box missing")
				}
			}
			m.assistant.input.SetValue("Next question")
			updated, cmd := m.Update(keyPress("enter"))
			m = assistantWork(t, updated.(dashboardModel), cmd)
			if ownSave {
				if preparedID != "" || runs != 1 || m.assistant.conversation.ID == old.ID || m.assistant.saved || len(m.assistant.messages) != 2 || strings.Contains(m.message, "deployment changed") {
					t.Fatal("own save reused the old chat", preparedID, runs, m.message)
				}
				view := ansi.Strip(m.View().Content)
				if !strings.Contains(view, "Previous answer") || !strings.Contains(view, "New chat") || !strings.Contains(view, "New answer") || len(m.assistant.archivedEntries) != 2 {
					t.Fatal("old conversation not preserved above", view)
				}
			} else if runs != 0 || preparedID != old.ID || m.message != "The deployment changed since this chat started. Press Ctrl+N to start a new chat." {
				t.Fatal("external change guard removed", runs, m.message)
			}
			stored, err := os.ReadFile(oldTranscriptPath)
			if err != nil || string(stored) != string(oldTranscriptBytes) {
				t.Fatal("previous transcript changed", err)
			}
		})
	}
}

func TestAssistantLongChatNoticeOncePerChat(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("notice sent HTTP") })
	m.assistant.state = "running"
	for _, size := range []int{10000, 10001, 15000, 20000} {
		updated, _ := m.Update(assistantStreamMsg{id: m.assistant.id, event: domain.AssistantEvent{Type: "transcript_size", TranscriptBytes: size, MaxRequestBytes: 20000}})
		m = updated.(dashboardModel)
		count := 0
		for _, entry := range m.assistant.entries {
			if entry.text == assistantLongChatText {
				count++
				if entry.role != "notice" {
					t.Fatal("notice is not muted")
				}
			}
		}
		want := 1
		if size == 10000 {
			want = 0
		}
		if count != want {
			t.Fatal("notice count", size, count)
		}
	}
	updated, _ := m.newAssistantChat()
	m = updated.(dashboardModel)
	if m.assistant.longChatShown {
		t.Fatal("new chat retained notice")
	}
	m.assistant.noteTranscriptSize(20000)
	if m.assistant.longChatShown {
		t.Fatal("unknown limit guessed")
	}
}
