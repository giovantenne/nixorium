package presentation

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantInactiveSubscriptionHeaderAndReactivation(t *testing.T) {
	account := domain.AssistantAccount{Plan: "annual", Status: "inactive", TurnsRemaining: 1000}
	requests := 0
	m, store := assistantFlowFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/account" || r.Method != http.MethodGet {
			t.Errorf("unexpected content request %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"account": account})
	})
	if err := store.WriteToken(flowToken); err != nil {
		t.Fatal(err)
	}
	client := adapters.AssistantClient{Endpoint: m.actions.Assistant.Endpoint}
	m.actions.AssistantService.Account = client.Account
	updated, _ := m.Update(m.refreshAssistantAccount()())
	m = updated.(dashboardModel)
	if requests != 1 || !strings.Contains(m.message, "Your subscription is not active. Renew at:") {
		t.Fatal("inactive account did not produce the renewal notice", m.message)
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			m.width, m.height, m.isDark = size[0], size[1], dark
			view := m.View().Content
			header := ansi.Strip(m.assistantHeader(size[0] - 4))
			if !strings.Contains(header, "Subscription inactive") || strings.Contains(header, "requests left") || strings.Contains(header, "1000") {
				t.Fatal("header advertises unavailable requests", header)
			}
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("inactive account exceeds %dx%d", size[0], size[1])
			}
		}
	}
	account.Status = "active"
	updated, _ = m.Update(m.refreshAssistantAccount()())
	m = updated.(dashboardModel)
	if requests != 2 || m.message != "" || !strings.Contains(ansi.Strip(m.assistantHeader(76)), "1000 requests left") {
		t.Fatal("reactivated subscription retained the inactive header or notice")
	}
}

func TestAssistantSubscriptionHeaderStatusPrecedence(t *testing.T) {
	for _, test := range []struct {
		plan, status, code string
		remaining          int
		want               string
		warn               bool
	}{
		{"annual", "inactive", "", 1000, "Subscription inactive", true},
		{"annual", "canceled", "", 1000, "Subscription inactive", true},
		{"annual", "past_due", "", 1000, "Subscription inactive", true},
		{"annual", "unpaid", "", 1000, "Subscription inactive", true},
		{"annual", "inactive", "quota_exhausted", 0, "Subscription inactive", true},
		{"annual", "active", "subscription_inactive", 1000, "Subscription inactive", true},
		{"annual", "active", "", 1000, "1000 requests left", false},
		{"annual", "", "", 1000, "1000 requests left", false},
		{"annual", "active", "", 0, "No requests left", true},
		{"annual", "active", "quota_exhausted", 1000, "No requests left", true},
		{"trial", "inactive", "", 3, "Free trial ended", true},
		{"trial", "active", "subscription_inactive", 3, "Free trial ended", true},
		{"trial", "active", "quota_exhausted", 3, "Free trial ended", true},
		{"trial", "active", "", 0, "Free trial ended", true},
		{"trial", "active", "", 3, "Free trial · 3 left", false},
	} {
		t.Run(test.plan+"/"+test.status+"/"+test.code+"/"+test.want, func(t *testing.T) {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
			m.assistant.account = domain.AssistantAccount{Plan: test.plan, Status: test.status, TurnsRemaining: test.remaining}
			m.assistant.accessError = test.code
			got, warn := m.assistantQuotaText()
			if got != test.want || warn != test.warn {
				t.Fatalf("quota=(%q, %t), want (%q, %t)", got, warn, test.want, test.warn)
			}
		})
	}
}

func TestAssistantTrialQuotaUsesTheAccountAllowance(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("render sent HTTP") })
	for _, test := range []struct {
		remaining, allowance int
		want                 string
	}{
		{3, 0, "Free trial · 3 left"},
		{3, 10, "Free trial · 3 of 10 left"},
		{29, 30, "Free trial · 29 of 30 left"},
		{1000, 1000, "Free trial · 1000 of 1000 left"},
	} {
		m.assistant.account = domain.AssistantAccount{Plan: "trial", Status: "active", TurnsRemaining: test.remaining, TrialRequests: test.allowance}
		if got, _ := m.assistantQuotaText(); got != test.want {
			t.Fatalf("quota=%q, want %q", got, test.want)
		}
	}
}
