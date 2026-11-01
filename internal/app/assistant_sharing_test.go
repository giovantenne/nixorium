package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAssistantReportsSavedRedactedHistoryWithExactWireBody(t *testing.T) {
	var captured map[string]json.RawMessage
	runner, conversation, store, _, token := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/feedback" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 43) {
			t.Error("report wire authentication or route")
		}
		json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(204)
	})
	messages := []domain.AssistantMessage{{Role: "user", Content: "Synthetic " + token + " sk-synthetic"}, {Role: "assistant", Content: "", ToolCalls: []domain.AssistantToolCall{{ID: "c1", Name: "read_file", Arguments: json.RawMessage(`{"path":"modules/course.nix","offset":9007199254740993}`)}}}, {Role: "tool", ToolCallID: "c1", Content: `{"content":"$6$synthetic-salt$synthetic-hash"}`}, {Role: "assistant", Content: "Synthetic final answer"}}
	if err := store.WriteTranscript(conversation, messages); err != nil {
		t.Fatal(err)
	}
	reporter := app.AssistantConversationReporter{Store: store, Client: runner.Client.(adapters.AssistantClient), Version: runner.Version}
	if err := reporter.Report(t.Context(), conversation, "wrong", "Synthetic comment"); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 5 {
		t.Fatal("unexpected feedback fields")
	}
	for _, field := range []string{"conversation", "rating", "comment", "context", "messages"} {
		if _, ok := captured[field]; !ok {
			t.Fatal("missing field", field)
		}
	}
	raw, _ := json.Marshal(captured)
	for _, secret := range []string{token, "sk-synthetic", "$6$synthetic-salt$synthetic-hash"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("synthetic secret uploaded")
		}
	}
	var payload domain.AssistantStepRequest
	json.Unmarshal(captured["context"], &payload.Context)
	json.Unmarshal(captured["messages"], &payload.Messages)
	if !strings.Contains(string(payload.Messages[1].ToolCalls[0].Arguments), "9007199254740993") {
		t.Fatal("upload changed numeric arguments")
	}
	if len(payload.Messages) != 4 || payload.Messages[3].Content != "Synthetic final answer" || payload.Context.Nixorium != runner.Version || len(payload.Context.Files) == 0 {
		t.Fatal("saved conversation or context missing")
	}
	if string(captured["conversation"]) != fmt.Sprintf("%q", conversation.ID) || string(captured["rating"]) != `"wrong"` {
		t.Fatal("report identity or rating lost")
	}
	if err := reporter.Report(t.Context(), conversation, "wrong", strings.Repeat("😀", 501)); err == nil {
		t.Fatal("oversized comment uploaded")
	}
}
func TestAssistantAutomaticSharingOnFinalOrErrorWithConsentAndWithoutWaitingForUpload(t *testing.T) {
	for _, terminal := range []string{"final", "error"} {
		for _, share := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", terminal, share), func(t *testing.T) {
				received := make(chan domain.AssistantStepRequest, 1)
				release := make(chan struct{})
				defer close(release)
				runner, conversation, store, _, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.URL.Path == "/v1/turns":
						runnerAdmit(w)
					case strings.HasSuffix(r.URL.Path, "/steps"):
						w.Header().Set("Content-Type", "text/event-stream")
						fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Saved partial or final\"}\n\n")
						if terminal == "final" {
							fmt.Fprint(w, "event: final\ndata: {\"summary\":\"Ready\"}\n\n")
						} else {
							fmt.Fprint(w, "event: error\ndata: {\"code\":\"model_error\",\"message\":\"Synthetic failure\"}\n\n")
						}
					case strings.HasSuffix(r.URL.Path, "/cancel"):
						w.WriteHeader(204)
					case strings.HasPrefix(r.URL.Path, "/v1/conversations/"):
						if !share {
							t.Error("sharing without consent")
						}
						if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 43) || r.Method != "POST" {
							t.Error("sharing authentication")
						}
						var payload domain.AssistantStepRequest
						json.NewDecoder(r.Body).Decode(&payload)
						received <- payload
						select {
						case <-release:
						case <-r.Context().Done():
						}
						w.WriteHeader(503)
						io.WriteString(w, `{"error":{"code":"service_unavailable","message":"Synthetic upload failure"}}`)
					default:
						t.Error("unexpected request")
					}
				})
				local := adapters.AssistantStateStore{Directory: store.Directory}
				if err := local.WriteSharing(share); err != nil {
					t.Fatal(err)
				}
				reporter := app.AssistantConversationReporter{Store: store, Client: runner.Client.(adapters.AssistantClient), Version: runner.Version}
				runner.Sharing = &reporter
				done := make(chan struct{})
				var result domain.AssistantTurnResult
				var runErr error
				go func() {
					result, runErr = runner.Run(context.Background(), conversation, "Synthetic question", func(domain.AssistantEvent) {})
					close(done)
				}()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("sharing blocked completed turn")
				}
				if result.Outcome != terminal || (terminal == "final" && runErr != nil) || (terminal == "error" && runErr == nil) {
					t.Fatal("upload affected turn outcome", runErr)
				}
				if share {
					select {
					case payload := <-received:
						want := "Saved partial or final"
						if terminal == "final" {
							want += "\n\nReady"
						}
						if len(payload.Messages) != 2 || payload.Messages[1].Content != want || payload.Context == nil {
							t.Fatal("incomplete shared snapshot")
						}
					case <-time.After(5 * time.Second):
						t.Fatal("missing automatic upload")
					}
				} else {
					select {
					case <-received:
						t.Fatal("sharing without consent")
					default:
					}
				}
			})
		}
	}
}
func TestAssistantSharingAfterAFailedStepButNotAfterAStopOrARefusal(t *testing.T) {
	for _, mode := range []string{"protocol", "stopped", "refused"} {
		t.Run(mode, func(t *testing.T) {
			received := make(chan struct{}, 1)
			stepStarted := make(chan struct{})
			runner, conversation, store, _, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/turns" && mode == "refused":
					w.WriteHeader(429)
					io.WriteString(w, `{"error":{"code":"quota_exhausted","message":"Synthetic quota"}}`)
				case r.URL.Path == "/v1/turns":
					runnerAdmit(w)
				case strings.HasSuffix(r.URL.Path, "/steps") && mode == "protocol":
					// The platform's own error page, not a service response.
					w.WriteHeader(503)
					io.WriteString(w, "<html>Synthetic platform error</html>")
				case strings.HasSuffix(r.URL.Path, "/steps"):
					w.Header().Set("Content-Type", "text/event-stream")
					w.(http.Flusher).Flush()
					close(stepStarted)
					<-r.Context().Done()
				case strings.HasSuffix(r.URL.Path, "/cancel"):
					w.WriteHeader(204)
				case strings.HasPrefix(r.URL.Path, "/v1/conversations/"):
					received <- struct{}{}
					w.WriteHeader(204)
				default:
					t.Error("unexpected request")
				}
			})
			if err := (adapters.AssistantStateStore{Directory: store.Directory}).WriteSharing(true); err != nil {
				t.Fatal(err)
			}
			reporter := app.AssistantConversationReporter{Store: store, Client: runner.Client.(adapters.AssistantClient), Version: runner.Version}
			runner.Sharing = &reporter
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "stopped" {
				go func() {
					<-stepStarted
					cancel()
				}()
			}
			if _, err := runner.Run(ctx, conversation, "Synthetic question", func(domain.AssistantEvent) {}); err == nil {
				t.Fatal("failed turn reported success")
			}
			select {
			case <-received:
				if mode != "protocol" {
					t.Fatal("shared a turn that was stopped or never admitted")
				}
			case <-time.After(time.Second):
				if mode == "protocol" {
					t.Fatal("failed turn not shared")
				}
			}
		})
	}
}
