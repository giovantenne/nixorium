package presentation

import (
	"charm.land/lipgloss/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestAssistantBillingCommandShowsLinkOrTrialNoticeWithoutRequest(t *testing.T) {
	for _, trial := range []bool{false, true} {
		t.Run(fmt.Sprint(trial), func(t *testing.T) {
			m, store := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("billing opened Assistant turn") })
			if err := store.WriteToken(flowToken); err != nil {
				t.Fatal(err)
			}
			calls := 0
			const link = "https://billing.stripe.com/p/session?secret=test_synthetic"
			m.actions.AssistantService.BillingPortal = func(_ context.Context, token string) (string, error) {
				calls++
				if token != flowToken {
					t.Fatal("billing lost authentication")
				}
				if trial {
					return "", &domain.AssistantError{Status: 409, Code: "no_subscription", Message: "Available after subscribing."}
				}
				return link, nil
			}
			m.assistant.account.TurnsRemaining = 5
			m.assistant.messages = []domain.AssistantMessage{{Role: "user", Content: "Previous question"}}
			m.assistant.entries = []assistantEntry{{role: "user", text: "Previous question"}}
			m.assistant.input.SetValue("/billing")
			updated, cmd := m.Update(keyPress("enter"))
			m = updated.(dashboardModel)
			if cmd == nil || m.assistant.state != "billing-loading" {
				t.Fatal("billing command unavailable")
			}
			updated, _ = m.Update(cmd())
			m = updated.(dashboardModel)
			if calls != 1 || m.assistant.state != "chat" || m.assistant.account.TurnsRemaining != 5 || m.assistant.input.Value() != "" || !reflect.DeepEqual(m.assistant.messages, []domain.AssistantMessage{{Role: "user", Content: "Previous question"}}) || !reflect.DeepEqual(m.assistant.entries, []assistantEntry{{role: "user", text: "Previous question"}}) {
				t.Fatal("billing changed chat or quota")
			}
			if !strings.Contains(assistantHelpText, "/billing   Manage your subscription and see payments (opens Stripe).") {
				t.Fatal("missing billing help")
			}
			view := m.View().Content
			if trial {
				if m.message != "Available after subscribing." || strings.Contains(view, link) {
					t.Fatal("trial notice", m.message)
				}
			} else {
				for _, text := range []string{link, "This link works for a few minutes.", "Official electronic invoices are delivered through SDI."} {
					if !strings.Contains(ansi.Strip(view), text) {
						t.Fatal("missing", text)
					}
				}
				if !strings.Contains(view, "\x1b]8;;"+link) {
					t.Fatal("billing link is not clickable")
				}
			}
			for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
				for _, dark := range []bool{true, false} {
					m.width, m.height, m.isDark = size[0], size[1], dark
					view = m.View().Content
					if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
						t.Fatal("billing overflow", size)
					}
				}
			}
		})
	}
}

func TestAssistantBillingEscDiscardsLateSession(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Fatal("model request") })
	m.actions.AssistantService.BillingPortal = func(ctx context.Context, _ string) (string, error) { return "", ctx.Err() }
	updated, cmd := m.openAssistantBilling()
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" || m.message != "" {
		t.Fatal("late portal session shown")
	}
}
