package presentation

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

type nameServer struct {
	mu       sync.Mutex
	requests []string
	reply    func(w http.ResponseWriter, name string)
}

func nameFixture(t *testing.T, reply func(http.ResponseWriter, string)) (dashboardModel, *nameServer) {
	t.Helper()
	server := &nameServer{reply: reply}
	m, _ := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			LabName string `json:"labName"`
		}
		if r.Method != "PATCH" || r.URL.Path != "/v1/account" || r.Header.Get("Authorization") != "Bearer "+flowToken ||
			json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		server.mu.Lock()
		server.requests = append(server.requests, body.LabName)
		server.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		server.reply(w, body.LabName)
	})
	client := adapters.AssistantClient{Endpoint: m.actions.Assistant.Endpoint, Version: "3.2.0"}
	m.actions.AssistantService.SetLabName = client.SetLabName
	m.assistant.token = flowToken
	m.assistant.account = domain.AssistantAccount{Name: "Synthetic School", Plan: "annual", Status: "active", TurnsRemaining: 100}
	m.assistant.input.Reset()
	return m, server
}

func acceptName(w http.ResponseWriter, name string) {
	json.NewEncoder(w).Encode(map[string]any{"account": map[string]any{"name": "Synthetic School", "labName": name, "plan": "annual", "status": "active", "periodEnd": "2099-01-01", "turnsRemaining": 100}})
}

func runName(m dashboardModel, command string) dashboardModel {
	m.assistant.input.SetValue(command)
	updated, _ := m.Update(keyPress("enter"))
	return updated.(dashboardModel)
}

func TestAssistantNameCommandSavesAndShowsInHeader(t *testing.T) {
	m, server := nameFixture(t, acceptName)
	m.assistant.account.LabName = "Old lab"
	m = runName(m, "/name")
	if m.assistant.state != "name" || m.assistant.labName.Value() != "Old lab" {
		t.Fatalf("state=%q value=%q", m.assistant.state, m.assistant.labName.Value())
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Laboratory name") {
		t.Fatal("missing name editor")
	}
	m.assistant.labName.SetValue("  Lab 2 – Informatica ")
	updated, command := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if m.assistant.state != "name-saving" || command == nil {
		t.Fatal("Enter did not start saving")
	}
	updated, _ = m.Update(command())
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || m.message != "Laboratory name saved." || m.assistant.account.LabName != "Lab 2 – Informatica" {
		t.Fatalf("state=%q message=%q name=%q", m.assistant.state, m.message, m.assistant.account.LabName)
	}
	if len(server.requests) != 1 || server.requests[0] != "Lab 2 – Informatica" {
		t.Fatalf("requests: %v", server.requests)
	}
	if header := ansi.Strip(m.assistantHeader(100)); !strings.Contains(header, "Assistant · Synthetic School · Lab 2 – Informatica") {
		t.Fatalf("header: %q", header)
	}
}

func TestAssistantNameEscCancelsWithoutRequest(t *testing.T) {
	m, server := nameFixture(t, acceptName)
	m = runName(m, "/name")
	m.assistant.labName.SetValue("Draft")
	updated, _ := m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || m.assistant.account.LabName != "" || len(server.requests) != 0 {
		t.Fatalf("state=%q requests=%v", m.assistant.state, server.requests)
	}
}

func TestAssistantNameValidationErrorsAreShownAndEditable(t *testing.T) {
	m, _ := nameFixture(t, func(w http.ResponseWriter, _ string) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":{"code":"invalid_request","message":"The laboratory name can have at most 60 characters."}}`))
	})
	m = runName(m, "/name")
	m.assistant.labName.SetValue("x")
	updated, command := m.Update(keyPress("enter"))
	updated, _ = updated.Update(command())
	m = updated.(dashboardModel)
	if m.assistant.state != "name" || m.message != "The laboratory name can have at most 60 characters." || m.assistant.labName.Value() != "x" {
		t.Fatalf("state=%q message=%q", m.assistant.state, m.message)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "at most 60 characters") {
		t.Fatal("notice not visible in the editor")
	}
}

func TestAssistantNameNeedsActivation(t *testing.T) {
	m, server := nameFixture(t, acceptName)
	m.assistant.token = ""
	m = runName(m, "/name")
	if m.assistant.state != "chat" || !strings.Contains(m.message, "/code") || len(server.requests) != 0 {
		t.Fatalf("state=%q message=%q", m.assistant.state, m.message)
	}
}

func TestAssistantNameHintOnlyUntilNamed(t *testing.T) {
	m, _ := nameFixture(t, acceptName)
	m.width, m.height = 120, 30
	if !strings.Contains(ansi.Strip(m.View().Content), assistantNameHint) {
		t.Fatal("missing hint")
	}
	m.assistant.account.LabName = "Lab 2"
	if strings.Contains(ansi.Strip(m.View().Content), assistantNameHint) {
		t.Fatal("hint still shown after naming")
	}
	m.assistant.account.LabName = ""
	m.assistant.token = ""
	if strings.Contains(ansi.Strip(m.View().Content), assistantNameHint) {
		t.Fatal("hint shown before activation")
	}
}

func TestAssistantHeaderTruncatesLabNameGracefully(t *testing.T) {
	m, _ := nameFixture(t, acceptName)
	m.assistant.account.LabName = "Laboratorio di informatica del secondo piano"
	for _, width := range []int{80, 60} {
		header := ansi.Strip(m.assistantHeader(width))
		if len([]rune(header)) > width || !strings.HasPrefix(header, "Assistant · Synthetic School") || !strings.Contains(header, "requests left") {
			t.Fatalf("width %d: %q", width, header)
		}
	}
}

func TestAssistantNameHelpAndMenu(t *testing.T) {
	if !strings.Contains(assistantHelpText, "/name     Set this laboratory's name, shown on invoices emails and in support") ||
		assistantCommandDescriptions["/name"] != "Set this laboratory's name, shown on invoices emails and in support (for example 'Lab 2 – Informatica')." {
		t.Fatal("help lacks /name")
	}
	if assistantCommands[len(assistantCommands)-2] != "/name" || assistantCommands[len(assistantCommands)-1] != "/code" {
		t.Fatal("/name must precede /code")
	}
}
