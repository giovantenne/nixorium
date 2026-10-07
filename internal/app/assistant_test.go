package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

const runnerTurnID = "12345678-1234-4234-8234-123456789abc"

func runnerGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid"}, args...)...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(data))
	}
	return string(data)
}
func runnerFixture(t *testing.T, handler http.HandlerFunc) (app.AssistantTurnRunner, domain.AssistantConversation, adapters.AssistantWorkspaceStore, string, string) {
	t.Helper()
	repo := t.TempDir()
	runnerGit(t, repo, "init", "-q")
	os.WriteFile(filepath.Join(repo, "flake.nix"), []byte("{outputs = _: {}; }\n"), 0644)
	runnerGit(t, repo, "add", "flake.nix")
	runnerGit(t, repo, "commit", "-qm", "synthetic base")
	state := t.TempDir()
	os.Chmod(state, 0700)
	store := adapters.AssistantWorkspaceStore{Directory: state}
	token := strings.Repeat("t", 43)
	local := adapters.AssistantStateStore{Directory: state}
	if err := local.WriteToken(token); err != nil {
		t.Fatal(err)
	}
	if err := local.WriteConsent(); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	runner := app.AssistantTurnRunner{Client: adapters.AssistantClient{Endpoint: server.URL + "/v1", Version: "3.2.0"}, Tools: adapters.AssistantWorkspaceTools{Store: store}, Store: store, Version: "3.2.0"}
	return runner, conversation, store, repo, token
}
func runnerAdmit(w http.ResponseWriter) {
	w.WriteHeader(201)
	fmt.Fprintf(w, `{"turnId":%q,"limits":{"steps":24,"seconds":900,"maxRequestBytes":2000000}}`, runnerTurnID)
}
func runnerTool(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, "event: tool_calls\ndata: {\"calls\":[{\"id\":\"c1\",\"name\":\"write_file\",\"arguments\":{\"path\":\"modules/course.nix\",\"content\":\"{ programs.course.enable = true; }\\n\"}}]}\n\n")
}
func TestAssistantRunnerCreatesCandidateAndRetainsHistory(t *testing.T) {
	var steps atomic.Int32
	requests := []domain.AssistantStepRequest{}
	runner, conversation, store, repo, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/turns":
			runnerAdmit(w)
		case strings.HasSuffix(r.URL.Path, "/steps"):
			var request domain.AssistantStepRequest
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("invalid request")
			}
			requests = append(requests, request)
			if steps.Add(1) == 1 {
				runnerTool(w)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Prepared\"}\n\nevent: final\ndata: {\"summary\":\"Course prepared\"}\n\n")
		default:
			t.Error("unexpected cancel or route")
		}
	})
	before := runnerGit(t, repo, "rev-parse", "HEAD")
	activities := []string{}
	result, err := runner.Run(t.Context(), conversation, "Prepare course", func(event domain.AssistantEvent) {
		if event.Type == "activity" {
			activities = append(activities, event.Text)
		}
	})
	if err != nil || result.Summary != "Course prepared" {
		t.Fatal(result, err)
	}
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := workspace.Read("modules/course.nix")
	workspace.Close()
	if err != nil || !strings.Contains(string(content), "programs.course") {
		t.Fatal("tool did not create overlay")
	}
	if _, err := os.Stat(filepath.Join(repo, "modules/course.nix")); !os.IsNotExist(err) {
		t.Fatal("tool wrote deployment")
	}
	if runnerGit(t, repo, "rev-parse", "HEAD") != before || runnerGit(t, repo, "status", "--porcelain") != "" {
		t.Fatal("deployment changed")
	}
	saved, err := store.ReadTranscript(conversation)
	if err != nil || len(saved) != 4 || saved[2].Role != "tool" || saved[2].ToolCallID != "c1" {
		t.Fatal(saved, err)
	}
	if requests[0].Context == nil || len(requests[0].Context.Files) != 1 || requests[1].Context != nil || requests[1].Messages[2].Content != `{"ok":true}` {
		t.Fatal("context or tool result missing")
	}
	if !strings.Contains(strings.Join(activities, ";"), "Writing modules/course.nix") {
		t.Fatal("missing tool activity")
	}
	if _, err = runner.Run(t.Context(), conversation, "Another request", func(domain.AssistantEvent) {}); err != nil {
		t.Fatal(err)
	}
	if requests[2].Context != nil || len(requests[2].Messages) != 5 {
		t.Fatal("next turn lost history or resent context")
	}
}
func TestAssistantRunnerCancellationRetainsOverlayAndCallsCancel(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var steps atomic.Int32
	runner, conversation, store, repo, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/turns":
			runnerAdmit(w)
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			w.WriteHeader(204)
			close(cancelled)
		case strings.HasSuffix(r.URL.Path, "/steps"):
			if steps.Add(1) == 1 {
				runnerTool(w)
				return
			}
			io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := runner.Run(ctx, conversation, "Prepare course", func(domain.AssistantEvent) {})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("second step not started")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel returned success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not cancel")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("remote Cancel missing")
	}
	workspace, err := store.Open(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()
	if _, err = workspace.Read("modules/course.nix"); err != nil {
		t.Fatal("cancel discarded overlay")
	}
	if runnerGit(t, repo, "status", "--porcelain") != "" {
		t.Fatal("cancel changed deployment")
	}
	transcript, err := store.ReadTranscript(conversation)
	if err != nil || len(transcript) < 3 || transcript[2].Role != "tool" {
		t.Fatal("cancel lost tool result")
	}
}
func TestAssistantRunnerNoConsentMakesNoRequests(t *testing.T) {
	var requests atomic.Int32
	runner, conversation, store, _, _ := runnerFixture(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })
	os.Remove(filepath.Join(store.Directory, "consent.json"))
	if _, err := runner.Run(t.Context(), conversation, "Private draft", func(domain.AssistantEvent) {}); err == nil || requests.Load() != 0 {
		t.Fatal("runner bypassed consent")
	}
}
func TestAssistantRunnerRedactsTokenAcrossChunksAndPersistsNoCredential(t *testing.T) {
	token := strings.Repeat("t", 43)
	runner, conversation, store, _, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/turns" {
			runnerAdmit(w)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: text\ndata: {\"delta\":%q}\n\nevent: text\ndata: {\"delta\":%q}\n\nevent: final\ndata: {\"summary\":\"Ready\"}\n\n", token[:20], token[20:])
	})
	display := ""
	result, err := runner.Run(t.Context(), conversation, "Do not echo "+token, func(event domain.AssistantEvent) { display += event.Delta })
	if err != nil || strings.Contains(display, token) {
		t.Fatal("stream leaked token", err)
	}
	serialized, _ := json.Marshal(result.Messages)
	if strings.Contains(string(serialized), token) {
		t.Fatal("messages leaked token")
	}
	data, err := os.ReadFile(filepath.Join(store.Directory, "conversations", conversation.ID, "transcript.json"))
	if err != nil || strings.Contains(string(data), token) {
		t.Fatal("transcript leaked token")
	}
}

func TestAssistantRunnerStepLimitAndOversizedChat(t *testing.T) {
	for _, code := range []string{"step_limit", "request_too_large", "protocol_error"} {
		t.Run(code, func(t *testing.T) {
			var steps, cancelled atomic.Int32
			runner, conversation, _, _, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/turns":
					runnerAdmit(w)
				case strings.HasSuffix(r.URL.Path, "/cancel"):
					cancelled.Add(1)
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/steps"):
					step := steps.Add(1)
					if code == "request_too_large" {
						w.WriteHeader(413)
						fmt.Fprint(w, `{"error":{"code":"request_too_large","message":"Request is too large."}}`)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					if code == "protocol_error" {
						fmt.Fprintf(w, "event: text\ndata: {\"delta\":%q}\n\n", strings.Repeat("x", 70000))
						if step == 2 {
							fmt.Fprint(w, "event: final\ndata: {\"summary\":\"Ready\"}\n\n")
							return
						}
					}
					fmt.Fprintf(w, "event: tool_calls\ndata: {\"calls\":[{\"id\":\"c%d\",\"name\":\"read_file\",\"arguments\":{\"path\":\"missing.nix\"}}]}\n\n", step)
				}
			})
			_, err := runner.Run(t.Context(), conversation, "Synthetic request", func(domain.AssistantEvent) {})
			var failure *domain.AssistantError
			if !errors.As(err, &failure) || failure.Code != code || cancelled.Load() != 1 {
				t.Fatal("limit or cancel missing", err)
			}
			if code == "step_limit" && steps.Load() != 24 {
				t.Fatal("client step bound ignored")
			}
		})
	}
}

func TestAssistantRunnerInvalidArgumentsReturnToModel(t *testing.T) {
	var steps atomic.Int32
	runner, conversation, _, _, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/turns" {
			runnerAdmit(w)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if steps.Add(1) == 1 {
			fmt.Fprint(w, "event: tool_calls\ndata: {\"calls\":[{\"id\":\"c1\",\"name\":\"write_file\",\"arguments\":{\"path\":\"x\",\"unknown\":true}}]}\n\n")
			return
		}
		var body domain.AssistantStepRequest
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) != 3 || !strings.Contains(body.Messages[2].Content, "invalid_arguments") {
			t.Error("invalid arguments did not become tool result")
		}
		fmt.Fprint(w, "event: final\ndata: {\"summary\":\"Ready\"}\n\n")
	})
	if _, err := runner.Run(t.Context(), conversation, "Synthetic request", func(domain.AssistantEvent) {}); err != nil {
		t.Fatal(err)
	}
}
