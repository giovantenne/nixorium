package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func assistantTestFailure(t *testing.T, err error) *domain.AssistantError {
	t.Helper()
	var failure *domain.AssistantError
	if !errors.As(err, &failure) {
		t.Fatalf("expected an Assistant error, got %v", err)
	}
	return failure
}

func TestAssistantSSEReportsReasonForEachRefusal(t *testing.T) {
	service := "event: service_messages\ndata: {\"messages\":[]}\n\n"
	final := "event: final\ndata: {\"summary\":\"x\"}\n\n"
	call := func(body string) string {
		return "event: tool_calls\ndata: {\"calls\":[" + body + "]}\n\n"
	}
	for _, test := range []struct {
		name, stream, code, detail string
	}{
		{"unknown event", "event: secret\ndata: {}\n\n", "protocol_error", `unknown event name "secret"`},
		{"unknown field", "event: text\ndata: {\"delta\":\"x\",\"extra\":1}\n\n", "protocol_error", `text event: unexpected field "extra"`},
		{"missing field", "event: final\ndata: {}\n\n", "protocol_error", "final event is missing field summary"},
		{"null field", "event: text\ndata: {\"delta\":null}\n\n", "protocol_error", "text event is missing field delta"},
		{"wrong type", "event: text\ndata: {\"delta\":5}\n\n", "protocol_error", "text event: field delta has the wrong type"},
		{"malformed json", "event: text\ndata: {\n\n", "protocol_error", "text event: malformed JSON"},
		{"extra json", "event: text\ndata: {\"delta\":\"x\"} {}\n\n", "protocol_error", "text event: extra data after the JSON value"},
		{"duplicate terminal", final + final, "protocol_error", "event after terminal"},
		{"event after tool calls", call(`{"id":"c1","name":"read_file","arguments":{}}`) + "event: text\ndata: {\"delta\":\"x\"}\n\n", "protocol_error", "event after terminal"},
		{"data after terminal", final + "event: usage\n", "protocol_error", "event after terminal"},
		{"event without name", "data: {}\n\n", "protocol_error", "event without a name"},
		{"event without data", "event: final\n\n", "protocol_error", "event final has no data"},
		{"duplicate service", service + service + final, "protocol_error", "event service_messages after service_messages"},
		{"service after final", final + service, "protocol_error", "event after terminal"},
		{"text after service", service + "event: text\ndata: {\"delta\":\"late\"}\n\n" + final, "protocol_error", "event text after service_messages"},
		{"line without field", "garbage\n\n", "protocol_error", "line without a field"},
		{"unknown stream field", "id: 1\ndata: {}\n\n", "protocol_error", `unknown stream field "id"`},
		{"duplicate event name", "event: text\nevent: text\n\n", "protocol_error", "duplicate event name"},
		{"negative usage", "event: usage\ndata: {\"inputTokens\":-1,\"outputTokens\":0}\n\n", "protocol_error", "usage event has negative token counts"},
		{"empty error code", "event: error\ndata: {\"code\":\"\",\"message\":\"m\"}\n\n", "protocol_error", "error event has an empty code"},
		{"no calls", call(""), "protocol_error", "tool_calls event has no calls"},
		{"call without id", call(`{"id":"","name":"read_file","arguments":{}}`), "protocol_error", "tool call without an id"},
		{"duplicate call id", call(`{"id":"c1","name":"read_file","arguments":{}},{"id":"c1","name":"read_file","arguments":{}}`), "protocol_error", "duplicate tool call id"},
		{"unknown tool", call(`{"id":"c1","name":"bash","arguments":{}}`), "protocol_error", `unknown tool "bash"`},
		{"call without arguments", call(`{"id":"c1","name":"read_file"}`), "protocol_error", "tool call read_file has no arguments"},
		{"text limit", "event: text\ndata: {\"delta\":\"" + strings.Repeat("x", domain.AssistantTextLimit+1) + "\"}\n\n", "protocol_error", "text exceeds limit"},
		{"event size", "event: activity\ndata: {\"text\":\"" + strings.Repeat("x", 256*1024) + "\"}\n\n", "protocol_error", "event exceeds 256 KiB"},
		{"stream size", strings.Repeat(": "+strings.Repeat("x", 1020)+"\n\n", 4200) + final, "protocol_error", "stream exceeds 4 MiB"},
		{"empty stream", "", "service_interrupted", "stream ended before a terminal event"},
		{"no terminal", "event: text\ndata: {\"delta\":\"x\"}\n\n", "service_interrupted", "stream ended before a terminal event"},
		{"truncated event", "event: final\ndata: {\"summary\":\"x\"}\n", "service_interrupted", "stream ended inside an event"},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := assistantTestFailure(t, parseAssistantSSE(strings.NewReader(test.stream), func(domain.AssistantEvent) {}))
			if failure.Code != test.code || failure.Detail != test.detail {
				t.Fatalf("got %s %q, want %s %q", failure.Code, failure.Detail, test.code, test.detail)
			}
			wantMessage := "The Assistant service returned an invalid response."
			if test.code == "service_interrupted" {
				wantMessage = "The Assistant service interrupted the answer. Send the message again."
			}
			if failure.Message != wantMessage {
				t.Fatalf("message %q", failure.Message)
			}
		})
	}
}

type assistantFailingReader struct {
	data string
	err  error
}

func (reader *assistantFailingReader) Read(buffer []byte) (int, error) {
	if reader.data == "" {
		return 0, reader.err
	}
	n := copy(buffer, reader.data)
	reader.data = reader.data[n:]
	return n, nil
}

func TestAssistantSSENetworkErrorIsServiceInterruption(t *testing.T) {
	reader := &assistantFailingReader{data: "event: text\ndata: {\"delta\":\"x\"}\n\n", err: errors.New("connection reset by peer")}
	failure := assistantTestFailure(t, parseAssistantSSE(reader, func(domain.AssistantEvent) {}))
	if failure.Code != "service_interrupted" || !strings.Contains(failure.Detail, "connection reset by peer") {
		t.Fatalf("%+v", failure)
	}
}

func assistantStepServer(t *testing.T, handler http.HandlerFunc) AssistantClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return AssistantClient{Endpoint: server.URL}
}

func TestAssistantStepServiceInterruptionsAndCancellation(t *testing.T) {
	token := strings.Repeat("t", 43)
	step := func(client AssistantClient, ctx context.Context) error {
		return client.Step(ctx, token, assistantTestTurnID, domain.AssistantStepRequest{}, func(domain.AssistantEvent) {})
	}
	t.Run("clean end without a terminal event", func(t *testing.T) {
		client := assistantStepServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: text\ndata: {\"delta\":\"partial\"}\n\n"))
		})
		failure := assistantTestFailure(t, step(client, context.Background()))
		if failure.Code != "service_interrupted" || failure.Detail != "stream ended before a terminal event" {
			t.Fatalf("%+v", failure)
		}
	})
	t.Run("connection dropped mid body", func(t *testing.T) {
		client := assistantStepServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write([]byte("event: text\ndata: {\"delta\":\"partial\"}\n\n"))
		})
		failure := assistantTestFailure(t, step(client, context.Background()))
		if failure.Code != "service_interrupted" || !strings.HasPrefix(failure.Detail, "stream read failed") {
			t.Fatalf("%+v", failure)
		}
	})
	t.Run("wrong content type", func(t *testing.T) {
		client := assistantStepServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		})
		failure := assistantTestFailure(t, step(client, context.Background()))
		if failure.Code != "protocol_error" || failure.Detail != "content-type is not text/event-stream" {
			t.Fatalf("%+v", failure)
		}
	})
	t.Run("user cancellation keeps the previous classification", func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		client := assistantStepServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			select {
			case <-release:
			case <-r.Context().Done():
			}
		})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()
		failure := assistantTestFailure(t, step(client, ctx))
		if failure.Code != "protocol_error" || failure.Detail != "request cancelled" {
			t.Fatalf("%+v", failure)
		}
	})
	t.Run("invalid turn id", func(t *testing.T) {
		failure := assistantTestFailure(t, AssistantClient{Endpoint: "http://127.0.0.1:1"}.Step(context.Background(), token, "nope", domain.AssistantStepRequest{}, func(domain.AssistantEvent) {}))
		if failure.Code != "protocol_error" || failure.Detail != "turn id is not valid" {
			t.Fatalf("%+v", failure)
		}
	})
}

func TestAssistantJSONResponsesReportReasons(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
		detail     string
		call       func(AssistantClient) error
	}{
		{"offer range", `{"trialRequests":0,"trialDays":1,"trialUrl":"https://a.example/t","subscribeUrl":"https://a.example/s"}`, 200, "offer out of range",
			func(c AssistantClient) error { _, err := c.Offer(context.Background()); return err }},
		{"offer link", `{"trialRequests":5,"trialDays":1,"trialUrl":"javascript:x","subscribeUrl":"https://a.example/s"}`, 200, "offer link is not valid",
			func(c AssistantClient) error { _, err := c.Offer(context.Background()); return err }},
		{"offer field", `{"unknown":1}`, 200, `unexpected field "unknown"`,
			func(c AssistantClient) error { _, err := c.Offer(context.Background()); return err }},
		{"error without code", `{"error":{"message":"m"}}`, 500, "error response has no code",
			func(c AssistantClient) error { _, err := c.Offer(context.Background()); return err }},
		{"error malformed", `not json`, 500, "error response: malformed JSON",
			func(c AssistantClient) error { _, err := c.Offer(context.Background()); return err }},
		{"oversize", strings.Repeat("x", 70000), 200, "response body exceeds 64 KiB",
			func(c AssistantClient) error { _, err := c.Offer(context.Background()); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := assistantStepServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})
			failure := assistantTestFailure(t, test.call(client))
			if failure.Code != "protocol_error" || failure.Detail != test.detail {
				t.Fatalf("%+v", failure)
			}
		})
	}
}

func TestAssistantErrorDetailIsShortAndPrintable(t *testing.T) {
	failure := assistantTestFailure(t, assistantProtocolError("bad\x1b[31m "+strings.Repeat("x", 500)))
	if strings.ContainsRune(failure.Detail, 0x1b) || len([]rune(failure.Detail)) != 160 {
		t.Fatalf("%q", failure.Detail)
	}
}

func TestAssistantWorkspaceKeepsTurnErrorsPrivateAndBounded(t *testing.T) {
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	store := AssistantWorkspaceStore{Directory: state}
	conversation, err := store.CreateConversation(workspaceRepository(t))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < assistantTurnErrorsKept+3; i++ {
		if err := store.RecordTurnError(conversation, at, "service_interrupted", "stream ended before a terminal event"); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(store.Directory, "conversations", conversation.ID, assistantTurnErrorsFile))
	if err != nil {
		t.Fatal(err)
	}
	var records []assistantTurnErrorRecord
	if json.Unmarshal(data, &records) != nil || len(records) != assistantTurnErrorsKept || records[0].Detail != "stream ended before a terminal event" || records[0].At != "2026-10-15T10:00:00Z" {
		t.Fatalf("%s", data)
	}
	transcript, err := os.ReadFile(filepath.Join(store.Directory, "conversations", conversation.ID, "transcript.json"))
	if err != nil || strings.Contains(string(transcript), "terminal") {
		t.Fatalf("detail leaked into the transcript sent to the service: %s %v", transcript, err)
	}
}
