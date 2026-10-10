package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantTestTurnID = "12345678-1234-4234-8234-123456789abc"

func TestAssistantClientProtocolAndContentFreeAdmission(t *testing.T) {
	token := strings.Repeat("t", 43)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/activate" && r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("missing token")
		}
		switch r.URL.Path {
		case "/v1/activate":
			fmt.Fprintf(w, `{"token":%q,"account":{"name":"Synthetic","plan":"test","periodEnd":"2099-01-01"}}`, token)
		case "/v1/account":
			fmt.Fprint(w, `{"account":{"name":"Synthetic","plan":"test","status":"active","periodEnd":"2099-01-01","turnsRemaining":42}}`)
		case "/v1/turns":
			var body map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["client"] == nil {
				t.Error("turn admission contains conversation content")
			}
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"turnId":%q,"limits":{"steps":24,"seconds":900,"maxRequestBytes":2000000}}`, assistantTestTurnID)
		case "/v1/turns/" + assistantTestTurnID + "/steps":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Preparing\"}\n\nevent: usage\ndata: {\"inputTokens\":12,\"outputTokens\":3}\n\nevent: final\ndata: {\"summary\":\"Ready\"}\n\n")
		case "/v1/turns/" + assistantTestTurnID + "/cancel":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := AssistantClient{Endpoint: server.URL + "/v1", Version: "3.2.0"}
	activation, err := client.Activate(context.Background(), "synthetic-code")
	if err != nil || activation.Token != token {
		t.Fatalf("activate: %+v, %v", activation, err)
	}
	account, err := client.Account(context.Background(), token)
	if err != nil || account.TurnsRemaining != 42 {
		t.Fatalf("account: %+v, %v", account, err)
	}
	turn, err := client.OpenTurn(context.Background(), token)
	if err != nil || turn.TurnID != assistantTestTurnID {
		t.Fatalf("open: %+v, %v", turn, err)
	}
	events := []domain.AssistantEvent{}
	err = client.Step(context.Background(), token, turn.TurnID, domain.AssistantStepRequest{Messages: []domain.AssistantMessage{{Role: "user", Content: "Synthetic request"}}}, func(event domain.AssistantEvent) { events = append(events, event) })
	if err != nil || len(events) != 3 || events[0].Delta != "Preparing" || events[2].Summary != "Ready" {
		t.Fatalf("events: %+v, %v", events, err)
	}
	if err := client.Cancel(context.Background(), token, turn.TurnID); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantClientRefusesRedirectsAndUnknownJSON(t *testing.T) {
	requests := 0
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer server.Close()
	client := AssistantClient{Endpoint: server.URL}
	if _, err := client.Activate(context.Background(), "code"); err == nil || requests != 0 {
		t.Fatal("redirect followed")
	}
	if assistantDecodeStrict([]byte(`{"summary":"ok","unknown":true}`), &struct {
		Summary string `json:"summary"`
	}{}) == nil {
		t.Fatal("unknown JSON accepted")
	}
}

func TestAssistantAccountUpgradeLink(t *testing.T) {
	for _, test := range []struct {
		name, plan, link string
		invalid          bool
	}{
		{"trial", "trial", "/upgrade?account=" + assistantTestTurnID + "&expires=2000000000&sig=" + strings.Repeat("A", 43), false},
		{"annual", "annual", "/upgrade?account=" + assistantTestTurnID + "&expires=2000000000&sig=" + strings.Repeat("A", 43), true},
		{"foreign", "trial", "https://other.example/upgrade", true},
		{"script", "trial", "javascript:alert(1)", true},
		{"extra", "trial", "/upgrade?account=" + assistantTestTurnID + "&expires=2000000000&sig=" + strings.Repeat("A", 43) + "&other=1", true},
		{"duplicate", "trial", "/upgrade?account=" + assistantTestTurnID + "&expires=2000000000&sig=" + strings.Repeat("A", 43) + "&sig=other", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				link := test.link
				if strings.HasPrefix(link, "/") {
					link = "http://" + r.Host + link
				}
				fmt.Fprintf(w, `{"account":{"name":"Synthetic","plan":%q,"status":"inactive","periodEnd":"2000-01-01","turnsRemaining":0,"upgradeUrl":%q}}`, test.plan, link)
			}))
			defer server.Close()
			client := AssistantClient{Endpoint: server.URL + "/v1"}
			account, err := client.Account(context.Background(), strings.Repeat("t", 43))
			if (err != nil) != test.invalid {
				t.Fatalf("invalid=%v err=%v", test.invalid, err)
			}
			if !test.invalid && (account.UpgradeURL == "" || account.Plan != "trial" || account.TurnsRemaining != 0) {
				t.Fatal("lost trial account metadata")
			}
		})
	}
}

func TestAssistantSSEStrictSchemaAndLimits(t *testing.T) {
	for name, stream := range map[string]string{
		"unknown event":      "event: secret\ndata: {}\n\n",
		"unknown field":      "event: text\ndata: {\"delta\":\"x\",\"extra\":1}\n\n",
		"missing field":      "event: final\ndata: {}\n\n",
		"missing terminal":   "event: text\ndata: {\"delta\":\"x\"}\n\n",
		"truncated terminal": "event: final\ndata: {\"summary\":\"x\"}\n",
		"duplicate terminal": "event: final\ndata: {\"summary\":\"x\"}\n\nevent: final\ndata: {\"summary\":\"y\"}\n\n",
		"unknown tool":       "event: tool_calls\ndata: {\"calls\":[{\"id\":\"c1\",\"name\":\"bash\",\"arguments\":{}}]}\n\n",
		"text memory limit":  "event: text\ndata: {\"delta\":\"" + strings.Repeat("x", domain.AssistantTextLimit+1) + "\"}\n\n",
		"event limit":        "event: activity\ndata: {\"text\":\"" + strings.Repeat("x", 256*1024) + "\"}\n\n",
		"step limit":         strings.Repeat(": "+strings.Repeat("x", 1020)+"\n\n", 4200) + "event: final\ndata: {\"summary\":\"x\"}\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := parseAssistantSSE(strings.NewReader(stream), func(domain.AssistantEvent) {}); err == nil {
				t.Fatal("invalid stream accepted")
			}
		})
	}
	valid := "event: tool_calls\ndata: {\"calls\":[{\"id\":\"c1\",\"name\":\"read_file\",\"arguments\":{\"path\":\"flake.nix\"}}]}\n\n"
	if err := parseAssistantSSE(strings.NewReader(valid), func(domain.AssistantEvent) {}); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantSSEServiceMessageOrdering(t *testing.T) {
	service := "event: service_messages\ndata: {\"messages\":[]}\n\n"
	final := "event: final\ndata: {\"summary\":\"Ready\"}\n\n"
	for _, stream := range []string{service + service + final, final + service, service + "event: text\ndata: {\"delta\":\"late\"}\n\n" + final} {
		if err := parseAssistantSSE(strings.NewReader(stream), func(domain.AssistantEvent) {}); err == nil {
			t.Fatal("invalid service event order accepted")
		}
	}
	events := []domain.AssistantEvent{}
	if err := parseAssistantSSE(strings.NewReader(service+final), func(event domain.AssistantEvent) { events = append(events, event) }); err != nil || len(events) != 2 || events[0].Type != "service_messages" || events[0].Messages == nil {
		t.Fatal(events, err)
	}
}
