package presentation

import (
	"context"
	"net/http"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func tickFixture(t *testing.T, account *domain.AssistantAccount) dashboardModel {
	t.Helper()
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	m.actions.AssistantService.ReadToken = func() (string, error) { return flowToken, nil }
	m.actions.AssistantService.Account = func(context.Context, string) (domain.AssistantAccount, error) { return *account, nil }
	m.screen = dashboardAssistant
	m.assistant.state = "chat"
	m.assistant.token = flowToken
	return m
}

func exhaustedTrial() domain.AssistantAccount {
	return domain.AssistantAccount{Plan: "trial", Status: "active", TurnsRemaining: 0, UpgradeURL: "https://assistant.nixorium.org/u/x"}
}

func TestAssistantAccountTickNoticesSubscription(t *testing.T) {
	account := exhaustedTrial()
	m := tickFixture(t, &account)
	m.assistant.account = account
	m.message = m.assistantNotice(&domain.AssistantError{Code: "quota_exhausted"})
	updated, command := m.finishAssistantAccount(assistantAccountMsg{id: m.assistant.id, token: flowToken, account: &account})
	m = updated.(dashboardModel)
	if command == nil {
		t.Fatal("exhausted trial scheduled no tick")
	}
	id := m.assistant.accountTickID
	for i := 0; i < 2; i++ {
		updated, command = m.Update(assistantAccountTickMsg{id: m.assistant.accountTickID})
		m = updated.(dashboardModel)
		if command == nil {
			t.Fatal("valid tick did not refresh")
		}
		updated, command = m.Update(command())
		m = updated.(dashboardModel)
		if command == nil || m.assistant.accountTickID == id {
			t.Fatal("exhausted account did not schedule another tick")
		}
		id = m.assistant.accountTickID
	}
	account = domain.AssistantAccount{Plan: "annual", Status: "active", TurnsRemaining: 1000}
	updated, command = m.Update(assistantAccountTickMsg{id: m.assistant.accountTickID})
	updated, command = updated.Update(command())
	m = updated.(dashboardModel)
	if m.message != "Subscription active · 1000 requests per year." {
		t.Fatalf("message = %q", m.message)
	}
	if command != nil {
		t.Fatal("active annual account scheduled a tick")
	}
}

func TestAssistantAccountTickIgnoredWhenStaleOrElsewhere(t *testing.T) {
	account := exhaustedTrial()
	m := tickFixture(t, &account)
	m.assistant.account = account
	updated, _ := m.finishAssistantAccount(assistantAccountMsg{id: m.assistant.id, token: flowToken, account: &account})
	m = updated.(dashboardModel)
	if _, command := m.Update(assistantAccountTickMsg{id: m.assistant.accountTickID + 1}); command != nil {
		t.Fatal("stale tick refreshed")
	}
	m.screen = dashboardHome
	if _, command := m.Update(assistantAccountTickMsg{id: m.assistant.accountTickID}); command != nil {
		t.Fatal("tick outside Assistant refreshed")
	}
	m.screen = dashboardAssistant
	m.assistant.state = "running"
	if _, command := m.Update(assistantAccountTickMsg{id: m.assistant.accountTickID}); command != nil {
		t.Fatal("tick while running refreshed")
	}
}

func TestAssistantActiveAnnualAccountNeverTicks(t *testing.T) {
	account := domain.AssistantAccount{Plan: "annual", Status: "active", TurnsRemaining: 10}
	m := tickFixture(t, &account)
	m.message = "unrelated"
	updated, command := m.finishAssistantAccount(assistantAccountMsg{id: m.assistant.id, token: flowToken, account: &account})
	if command != nil {
		t.Fatal("active annual account scheduled a tick")
	}
	if updated.(dashboardModel).message != "unrelated" {
		t.Fatal("unrelated message changed")
	}
}
