package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantTestTurnID = "12345678-1234-4234-8234-123456789abc"

func TestAssistantCapabilitiesHeaderPreservesLegacyStepAndSharingBodies(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]json.RawMessage
		if json.Unmarshal(data, &body) != nil {
			t.Fatal(string(data))
		}
		var context map[string]json.RawMessage
		if json.Unmarshal(body["context"], &context) != nil {
			t.Fatal(string(data))
		}
		// Model an old service's strict context validator. The optional transport
		// header is ignored, so one ordinary step succeeds without any replay.
		for key := range context {
			if key != "nixorium" && key != "core" && key != "hosts" && key != "files" {
				t.Errorf("legacy service rejected context field %q", key)
				w.WriteHeader(400)
				return
			}
		}
		if strings.Contains(string(data), "capabilities") {
			t.Error("capabilities leaked into shared/request JSON")
		}
		if strings.HasSuffix(r.URL.Path, "/steps") {
			if r.Header.Get("X-Nixorium-Capabilities") != "prepare_npm_v1,host_diagnostics_v1,git_history_v1,prefetch_v1,settings_v1,home_files_v1" {
				t.Error("missing fixed header")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: final\ndata: {\"summary\":\"Ready\"}\n\n")
		} else {
			if r.Header.Get("X-Nixorium-Capabilities") != "" {
				t.Error("capability header on sharing route")
			}
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	client := AssistantClient{Endpoint: server.URL + "/v1", Version: "3.3.0"}
	payload := domain.AssistantStepRequest{Context: &domain.AssistantContext{Nixorium: "3.3.0", Files: []string{"flake.nix"}, Capabilities: []string{"prepare_npm_v1", "host_diagnostics_v1", "git_history_v1", "prefetch_v1", "settings_v1", "home_files_v1", "home_files_v1", "prefetch_v1", "PREFETCH_V1", "SETTINGS_V1", "git_history_v1", "unknown_v1", "HOST_DIAGNOSTICS_V1", "host_diagnostics_v1,unknown_v1"}}, Messages: []domain.AssistantMessage{{Role: "user", Content: "Synthetic"}}}
	token := strings.Repeat("t", 43)
	if err := client.Step(t.Context(), token, assistantTestTurnID, payload, func(domain.AssistantEvent) {}); err != nil {
		t.Fatal(err)
	}
	if err := client.ShareConversation(t.Context(), token, assistantTestTurnID, payload); err != nil {
		t.Fatal(err)
	}
	if err := client.ReportConversation(t.Context(), token, assistantTestTurnID, "helpful", "Synthetic", payload); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatal("unexpected request replay", requests)
	}
}

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
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
		{"annual short", "annual", "/u/" + strings.Repeat("Ab0_-", 8), true},
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

func TestAssistantClientActiveTrialAccountWithShortUpgradeLink(t *testing.T) {
	token := strings.Repeat("t", 43)
	shortPath := "/u/" + strings.Repeat("Ab0_-", 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/account" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		labName := "Lab 1"
		if r.Method == http.MethodPatch {
			var body struct {
				LabName string `json:"labName"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid laboratory name request")
			}
			labName = body.LabName
		} else if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"account":{"name":"Synthetic School","labName":%q,"plan":"trial","status":"active","periodEnd":"2099-01-01","turnsRemaining":10,"upgradeUrl":%q}}`, labName, "http://"+r.Host+shortPath)
	}))
	defer server.Close()
	client := AssistantClient{Endpoint: server.URL + "/v1"}
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			var account domain.AssistantAccount
			var err error
			if method == http.MethodGet {
				account, err = client.Account(t.Context(), token)
			} else {
				account, err = client.SetLabName(t.Context(), token, "Lab 2")
			}
			wantName := "Lab 1"
			if method == http.MethodPatch {
				wantName = "Lab 2"
			}
			if err != nil || account.Name != "Synthetic School" || account.LabName != wantName || account.Plan != "trial" ||
				account.Status != "active" || account.TurnsRemaining != 10 || account.PeriodEnd != "2099-01-01" || account.UpgradeURL != server.URL+shortPath {
				t.Fatalf("lost active trial account metadata: %+v, %v", account, err)
			}
		})
	}
}

func TestAssistantShortUpgradeURLValidation(t *testing.T) {
	client := AssistantClient{Endpoint: "https://assistant.example/v1"}
	shortPath := "/u/" + strings.Repeat("Ab0_-", 8)
	valid := "https://assistant.example" + shortPath
	for _, test := range []struct {
		name, link string
		valid      bool
	}{
		{"current", valid, true},
		{"foreign host", "https://other.example" + shortPath, false},
		{"foreign port", "https://assistant.example:444" + shortPath, false},
		{"insecure scheme", "http://assistant.example" + shortPath, false},
		{"relative", shortPath, false},
		{"userinfo", "https://user@assistant.example" + shortPath, false},
		{"fragment", valid + "#fragment", false},
		{"query", valid + "?other=1", false},
		{"empty query", valid + "?", false},
		{"escaped path", strings.Replace(valid, "/u/", "/%75/", 1), false},
		{"short token", valid[:len(valid)-1], false},
		{"long token", valid + "A", false},
		{"padded token", valid[:len(valid)-1] + "=", false},
		{"invalid token character", valid[:len(valid)-1] + "+", false},
		{"extra segment", valid + "/extra", false},
		{"trailing slash", valid + "/", false},
		{"wrong route", strings.Replace(valid, "/u/", "/v1/u/", 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := client.validUpgradeURL(test.link); got != test.valid {
				t.Fatalf("validUpgradeURL(%q) = %v, want %v", test.link, got, test.valid)
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

func TestAssistantClientKeepsServiceErrorCodesDistinctFromTransportFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"code":"model_unavailable","message":"The AI provider is unavailable."}}`))
	}))
	_, err := AssistantClient{Endpoint: server.URL}.Activate(context.Background(), "code")
	var serviceError *domain.AssistantError
	if !errors.As(err, &serviceError) || serviceError.Code != "model_unavailable" || serviceError.Status != 503 {
		t.Fatal("service error code lost", err)
	}
	server.Close()
	_, err = AssistantClient{Endpoint: server.URL}.Activate(context.Background(), "code")
	if err == nil || errors.As(err, &serviceError) {
		t.Fatal("connection failure must not carry a service code", err)
	}
	var terminal domain.AssistantEvent
	stream := "event: error\ndata: {\"code\":\"model_unavailable\",\"message\":\"The AI provider is unavailable.\"}\n\n"
	if err := parseAssistantSSE(strings.NewReader(stream), func(event domain.AssistantEvent) { terminal = event }); err != nil || terminal.Code != "model_unavailable" {
		t.Fatal("error event", err, terminal)
	}
}

func TestAssistantClientSetLabName(t *testing.T) {
	token := strings.Repeat("t", 43)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if r.Method != "PATCH" || r.URL.Path != "/v1/account" || r.Header.Get("Authorization") != "Bearer "+token ||
			json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if body["labName"] == "bad" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"code":"invalid_request","message":"The laboratory name must be plain text."}}`)
			return
		}
		fmt.Fprintf(w, `{"account":{"name":"Synthetic","labName":%q,"plan":"test","status":"active","periodEnd":"2099-01-01","turnsRemaining":42}}`, body["labName"])
	}))
	defer server.Close()
	client := AssistantClient{Endpoint: server.URL + "/v1", Version: "3.2.0"}
	account, err := client.SetLabName(context.Background(), token, "Lab 2")
	if err != nil || account.LabName != "Lab 2" || account.Name != "Synthetic" {
		t.Fatalf("%+v %v", account, err)
	}
	var serviceError *domain.AssistantError
	if _, err = client.SetLabName(context.Background(), token, "bad"); !errors.As(err, &serviceError) || serviceError.Code != "invalid_request" || serviceError.Message == "" {
		t.Fatalf("validation error lost: %v", err)
	}
	if _, err = client.SetLabName(context.Background(), "short", "x"); err == nil {
		t.Fatal("invalid token accepted")
	}
}

func TestAssistantAccountTrialAllowanceValidation(t *testing.T) {
	token := strings.Repeat("a", 43)
	for _, test := range []struct {
		plan    string
		trial   int
		valid   bool
		comment string
	}{
		{"trial", 0, true, "older services omit the allowance"},
		{"trial", 30, true, "announced allowance"},
		{"trial", 1000, true, "largest allowance"},
		{"trial", 1001, false, "allowance above the accepted range"},
		{"trial", -1, false, "negative allowance"},
		{"annual", 30, false, "allowance on a non-trial account"},
	} {
		t.Run(test.comment, func(t *testing.T) {
			account := fmt.Sprintf(`{"name":"Synthetic School","plan":%q,"status":"active","periodEnd":"2099-01-01","turnsRemaining":5,"trialRequests":%d}`, test.plan, test.trial)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/activate" {
					fmt.Fprintf(w, `{"token":%q,"account":%s}`, token, account)
					return
				}
				fmt.Fprintf(w, `{"account":%s}`, account)
			}))
			defer server.Close()
			client := AssistantClient{Endpoint: server.URL + "/v1", Version: "3.6.0"}
			fetched, err := client.Account(t.Context(), token)
			activated, activationErr := client.Activate(t.Context(), "synthetic-code")
			if test.valid {
				if err != nil || activationErr != nil || fetched.TrialRequests != test.trial || activated.Account.TrialRequests != test.trial {
					t.Fatalf("valid allowance rejected: %+v %v %+v %v", fetched, err, activated, activationErr)
				}
			} else if err == nil || activationErr == nil {
				t.Fatalf("invalid allowance accepted: %v %v", err, activationErr)
			}
		})
	}
}
