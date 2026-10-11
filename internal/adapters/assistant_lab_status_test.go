package adapters

import (
	"context"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantLabStatusUsesConversationRepository(t *testing.T) {
	workspace, store, repo := assistantWorkspaceFixture(t)
	calls := 0
	tools := AssistantWorkspaceTools{Store: store, LabStatus: func(_ context.Context, got string) string {
		calls++
		if got != repo {
			t.Fatalf("status inspected %q instead of deployment", got)
		}
		return `{"ok":true}`
	}}
	conversation := workspace.Conversation
	conversation.Repository = "/caller-cannot-change-repository"
	call := domain.AssistantToolCall{Name: "lab_status", Arguments: []byte(`{}`)}
	if got := tools.Execute(t.Context(), conversation, call); got != `{"ok":true}` || calls != 1 {
		t.Fatal(got, calls)
	}
	tools.LabStatus = nil
	if got := tools.Execute(t.Context(), conversation, call); got != `{"ok":false,"error":"status_unavailable"}` {
		t.Fatal(got)
	}
	conversation.ID = "missing"
	if got := tools.Execute(t.Context(), conversation, call); got != `{"ok":false,"error":"status_unavailable"}` {
		t.Fatal(got)
	}
}
