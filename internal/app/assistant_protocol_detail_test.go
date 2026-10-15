package app_test

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantRunnerKeepsFailureDetailPrivate(t *testing.T) {
	var stepBodies []string
	runner, conversation, store, _, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/turns":
			runnerAdmit(w)
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/steps"):
			buffer := make([]byte, 1<<20)
			n, _ := r.Body.Read(buffer)
			stepBodies = append(stepBodies, string(buffer[:n]))
			w.Header().Set("Content-Type", "text/event-stream")
			// The stream ends without a terminal event, as a cancelled turn does.
			fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Working\"}\n\n")
		}
	})
	_, err := runner.Run(t.Context(), conversation, "Synthetic request", func(domain.AssistantEvent) {})
	var failure *domain.AssistantError
	if !errors.As(err, &failure) || failure.Code != "service_interrupted" || failure.Detail != "stream ended before a terminal event" {
		t.Fatal(err)
	}
	directory := filepath.Join(store.Directory, "conversations", conversation.ID)
	record, readErr := os.ReadFile(filepath.Join(directory, "turn-errors-v1.json"))
	if readErr != nil || !strings.Contains(string(record), "stream ended before a terminal event") || !strings.Contains(string(record), "service_interrupted") {
		t.Fatalf("private record missing: %s %v", record, readErr)
	}
	transcript, _ := os.ReadFile(filepath.Join(directory, "transcript.json"))
	if strings.Contains(string(transcript), "terminal event") {
		t.Fatal("detail entered the transcript", string(transcript))
	}

	// The next step request carries the transcript; it must not carry the detail.
	if _, err = runner.Run(t.Context(), conversation, "Try again", func(domain.AssistantEvent) {}); err == nil {
		t.Fatal("second interrupted stream accepted")
	}
	for _, body := range stepBodies {
		if strings.Contains(body, "terminal event") || strings.Contains(body, "detail") {
			t.Fatal("detail sent to the service", body)
		}
	}
}
