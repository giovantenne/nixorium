package presentation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantTrialAccountAndPurchaseGate(t *testing.T) {
	for _, test := range []struct {
		name, plan, status string
		remaining          int
	}{
		{"active", "trial", "active", 5}, {"exhausted", "trial", "active", 0},
		{"expired", "trial", "inactive", 3}, {"annual", "annual", "active", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests, runs := 0, 0
			var link string
			m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/account" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+flowToken {
					t.Error("preflight leaked content or lost authentication")
				}
				link = "http://" + r.Host + "/upgrade?account=" + flowTurnID + "&expires=2000000000&sig=" + strings.Repeat("A", 43)
				account := domain.AssistantAccount{Name: "Synthetic", Plan: test.plan, Status: test.status, PeriodEnd: "2099-01-01", TurnsRemaining: test.remaining}
				if test.plan == "trial" {
					account.UpgradeURL = link
				}
				json.NewEncoder(w).Encode(map[string]any{"account": account})
			})
			client := adapters.AssistantClient{Endpoint: m.actions.Assistant.Endpoint}
			m.actions.AssistantService.Account = client.Account
			m.actions.AssistantService.Run = func(context.Context, domain.AssistantConversation, string, func(domain.AssistantEvent)) (domain.AssistantTurnResult, error) {
				runs++
				return domain.AssistantTurnResult{Summary: "Synthetic proposal"}, nil
			}
			if err := store.WriteToken(flowToken); err != nil {
				t.Fatal(err)
			}
			if err := store.WriteConsent(); err != nil {
				t.Fatal(err)
			}
			updated, _ := m.Update(m.refreshAssistantAccount()())
			m = updated.(dashboardModel)
			if test.plan == "trial" && !strings.Contains(m.View().Content, fmt.Sprintf("Free trial: %d of 5 requests left", test.remaining)) {
				t.Fatal("trial quota not visible")
			}
			updated, command := m.Update(keyPress("ctrl+s"))
			m = assistantWork(t, updated.(dashboardModel), command)
			if test.name == "active" {
				if runs != 1 {
					t.Fatal("active trial cannot send")
				}
				return
			}
			if runs != 0 || m.assistant.input.Value() != "Synthetic private request" {
				t.Fatal("unavailable quota ran or lost draft")
			}
			if test.plan == "trial" {
				view := m.View().Content
				if m.assistant.state != "cta-inactive" || !strings.Contains(view, "Your free trial has ended") || strings.Contains(view, m.actions.Assistant.SubscribeURL) {
					t.Fatal("trial did not use purchase gate")
				}
				// The shell wraps long links, so compare without rendered whitespace.
				if !strings.Contains(strings.Join(strings.Fields(view), ""), strings.Join(strings.Fields(link), "")) {
					t.Fatal("signed purchase link missing")
				}
			} else if !strings.Contains(m.message, "Contact hello@nixorium.org") || strings.Contains(m.message, "/upgrade") {
				t.Fatal("annual quota offers an upgrade")
			}
			if requests != 2 {
				t.Fatalf("unexpected account requests: %d", requests)
			}
			if token, _ := store.ReadToken(); token != flowToken {
				t.Fatal("trial end discarded controller activation")
			}
		})
	}
}

func TestAssistantTrialLayoutsAndStaleAccount(t *testing.T) {
	link := "https://assistant.nixorium.org/upgrade?account=" + flowTurnID + "&expires=2000000000&sig=" + strings.Repeat("A", 43)
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("render made HTTP request") })
			m.width, m.height, m.isDark = size[0], size[1], dark
			m.assistant.account = domain.AssistantAccount{Plan: "trial", Status: "inactive", TurnsRemaining: 0, UpgradeURL: link}
			for _, state := range []string{"chat", "running", "cta-inactive"} {
				m.assistant.state = state
				view := m.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%s exceeds %dx%d", state, size[0], size[1])
				}
				if state == "cta-inactive" && (!strings.Contains(view, "Subscribe at") || !strings.Contains(view, "Back")) {
					t.Fatal("purchase action or cancellation hidden")
				}
			}
			old := m.assistant.account
			updated, _ := m.Update(assistantAccountMsg{id: m.assistant.id + 1, account: &domain.AssistantAccount{Plan: "annual"}})
			if updated.(dashboardModel).assistant.account != old {
				t.Fatal("stale account changed current screen")
			}
		}
	}
}

func TestAssistantAccountRefreshAfterTurnAndConversion(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) {})
	account := domain.AssistantAccount{Plan: "trial", Status: "active", TurnsRemaining: 4}
	m.actions.AssistantService.ReadToken = func() (string, error) { return flowToken, nil }
	m.actions.AssistantService.Account = func(context.Context, string) (domain.AssistantAccount, error) { return account, nil }
	m.assistant.state = "running"
	updated, command := m.finishAssistantStream(assistantStreamMsg{id: m.assistant.id, done: true, result: &domain.AssistantTurnResult{Summary: "Ready"}})
	m = updated.(dashboardModel)
	if command == nil {
		t.Fatal("completed turn did not refresh account")
	}
	updated, _ = m.Update(command())
	m = updated.(dashboardModel)
	if !strings.Contains(m.View().Content, "Free trial: 4 of 5 requests left") {
		t.Fatal("post-turn count not refreshed")
	}
	account = domain.AssistantAccount{Plan: "annual", Status: "active", TurnsRemaining: 1000}
	updated, _ = m.Update(m.refreshAssistantAccount()())
	m = updated.(dashboardModel)
	if strings.Contains(m.View().Content, "Free trial:") {
		t.Fatal("converted account still shows trial")
	}
}
