package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantFinalHistoryMatchesVisibleAnswerAndErrorsKeepLastAnswer(t *testing.T) {
	for _, test := range []struct{ text, summary, want string }{
		{"Prepared", "Course prepared", "Prepared\n\nCourse prepared"},
		{"", "Summary only", "Summary only"},
		{"Already final", "Already final", "Already final"},
	} {
		t.Run(test.summary, func(t *testing.T) {
			failNext := false
			runner, c, store, repo, _ := runnerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/turns":
					runnerAdmit(w)
				case strings.HasSuffix(r.URL.Path, "/cancel"):
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/steps"):
					w.Header().Set("Content-Type", "text/event-stream")
					if failNext {
						fmt.Fprint(w, "event: text\ndata: {\"delta\":\"Failed partial\"}\n\nevent: error\ndata: {\"code\":\"model_error\"}\n\n")
						return
					}
					fmt.Fprintf(w, "event: text\ndata: {\"delta\":%q}\n\nevent: final\ndata: {\"summary\":%q}\n\n", test.text, test.summary)
				default:
					t.Error("unexpected request")
				}
			})
			var eventText string
			result, err := runner.Run(t.Context(), c, "First title\nDetails", func(e domain.AssistantEvent) {
				if e.Type == "final" {
					eventText = domain.AssistantFinalAnswer(e.Text, e.Summary)
				}
			})
			if err != nil || eventText != test.want || result.Messages[len(result.Messages)-1].Content != test.want {
				t.Fatal(result, eventText, err)
			}
			h, err := store.ConversationHistory(t.Context(), repo, c.ID)
			if err != nil || *h.Conversation.LastAnswer != test.want || h.Conversation.Title != "First title" {
				t.Fatal(h, err)
			}
			failNext = true
			if _, err = runner.Run(t.Context(), c, "Second title", func(domain.AssistantEvent) {}); err == nil {
				t.Fatal("expected failure")
			}
			h, err = store.ConversationHistory(t.Context(), repo, c.ID)
			if err != nil || *h.Conversation.LastAnswer != test.want || h.Conversation.Title != "First title" || h.Messages[len(h.Messages)-1].Content != "Failed partial" {
				t.Fatal(h, err)
			}
		})
	}
}
