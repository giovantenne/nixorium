package presentation

import (
	"charm.land/lipgloss/v2"
	"context"
	"errors"
	"github.com/giovantenne/nixorium/internal/domain"
	"net/http"
	"strings"
	"testing"
)

func TestAssistantSharingCheckboxAndSettingsAreExplicitAndPersistent(t *testing.T) {
	m, store := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("settings made HTTP request") })
	store.WriteToken(flowToken)
	store.WriteConsent()
	actions := m.actions.AssistantService
	actions.ReadSharing = store.ReadSharing
	actions.WriteSharing = store.WriteSharing
	actions.WriteConsentSharing = store.WriteConsentSharing
	m.assistant.state = "consent"
	if !m.assistant.sharing || !strings.Contains(m.View().Content, "[x] Save conversations on the service: On") {
		t.Fatal("saving not on by default")
	}
	for _, disclosure := range []string{"configuration history", "recent logs", "best effort", "Student home files are not read", "30 days", "90 days"} {
		if !strings.Contains(assistantConsentText, disclosure) {
			t.Fatalf("consent lacks %q", disclosure)
		}
	}
	updated, _ := m.Update(keyPress("space"))
	m = updated.(dashboardModel)
	if m.assistant.sharing || !strings.Contains(m.View().Content, "[ ] Save conversations on the service: Off") {
		t.Fatal("checkbox did not toggle")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if saved, _ := store.ReadSharing(); saved {
		t.Fatal("cancel persisted checkbox")
	}
	updated, _ = m.openAssistantSharing()
	m = updated.(dashboardModel)
	if m.assistant.state != "sharing" || m.assistant.sharing || !strings.Contains(m.View().Content, "Save conversations on the service: Off") {
		t.Fatal("settings did not load saved state")
	}
	updated, _ = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	if saved, _ := store.ReadSharing(); !saved || !m.assistant.sharing || m.assistant.state != "chat" {
		t.Fatal("setting not persisted")
	}
	updated, _ = m.Update(keyPress("ctrl+n"))
	m = updated.(dashboardModel)
	if !m.assistant.sharing {
		t.Fatal("new chat lost sharing setting")
	}
	updated, _ = m.openAssistantSharing()
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if saved, _ := store.ReadSharing(); !saved {
		t.Fatal("Esc changed setting")
	}
	m.navigation.helpOpen = true
	view := m.View().Content
	if !strings.Contains(view, "/share") || !strings.Contains(view, "/report") {
		t.Fatal("missing help")
	}
}
func TestAssistantReportConsentRatingCommentCancellationAndResult(t *testing.T) {
	m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network") })
	m.assistant.conversation = domain.AssistantConversation{ID: flowTurnID}
	updated, _ := m.openAssistantReport()
	m = updated.(dashboardModel)
	if m.assistant.state != "chat" {
		t.Fatal("report offered before final answer")
	}
	m.assistant.canReport = true
	called := 0
	m.actions.AssistantService.Report = func(_ context.Context, c domain.AssistantConversation, rating, comment string) error {
		called++
		if c.ID != flowTurnID || rating != "wrong" || comment != "Synthetic comment" {
			t.Fatal("wrong report arguments")
		}
		return nil
	}
	updated, _ = m.openAssistantReport()
	m = updated.(dashboardModel)
	if !strings.Contains(m.View().Content, "Report this answer") {
		t.Fatal("report absent")
	}
	for _, disclosure := range []string{"whole conversation", "history", "diagnostics", "may leave personal or sensitive text", "90 days"} {
		if !strings.Contains(assistantReportText, disclosure) {
			t.Fatal("report disclosure missing", disclosure)
		}
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	if called != 0 {
		t.Fatal("Esc submitted report")
	}
	updated, _ = m.openAssistantReport()
	m = updated.(dashboardModel)
	updated, _ = m.Update(keyPress("down"))
	m = updated.(dashboardModel)
	m.assistant.reportComment.SetValue("Synthetic comment")
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if called != 1 || m.message != "Report sent. Thank you." || m.assistant.state != "chat" {
		t.Fatal("report result not shown")
	}
	if m.assistant.input.Value() != "Synthetic private request" {
		t.Fatal("report lost draft")
	}
	m.actions.AssistantService.Report = func(context.Context, domain.AssistantConversation, string, string) error {
		return errors.New("Synthetic reporting failure")
	}
	updated, _ = m.openAssistantReport()
	m = updated.(dashboardModel)
	m.assistant.reportComment.SetValue(strings.Repeat("😀", 501))
	if len([]rune(m.assistant.reportComment.Value())) != 500 {
		t.Fatal("comment limit")
	}
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	message := cmd()
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(message)
	m = updated.(dashboardModel)
	if m.message != "" || m.assistant.state != "chat" {
		t.Fatal("late report result replaced cancellation")
	}
	updated, _ = m.openAssistantReport()
	m = updated.(dashboardModel)
	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(dashboardModel)
	updated, _ = m.Update(cmd())
	m = updated.(dashboardModel)
	if m.message != "Synthetic reporting failure" {
		t.Fatal("report error missing")
	}
}
func TestAssistantSharingAndReportLayouts(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
			m, _ := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("render sent HTTP") })
			m.isDark = dark
			m.width, m.height = size[0], size[1]
			for _, state := range []string{"consent", "sharing", "report", "report-sending"} {
				m.assistant.state = state
				view := m.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%s exceeds %v", state, size)
				}
				if !strings.Contains(view, "Esc") {
					t.Fatal("cancel hidden")
				}
				if state == "report" && !strings.Contains(view, "90 days.") {
					t.Fatal("disclosure clipped")
				}
				if state == "consent" && !strings.Contains(view, "Save conversations on the service") {
					t.Fatal("checkbox clipped")
				}
			}
		}
	}
}

func TestAssistantAcceptConsentSavesSelectedSharingAndReplacementResetsCheckbox(t *testing.T) {
	m, store := assistantFlowFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("consent checks sent a draft") })
	if err := store.WriteToken(flowToken); err != nil {
		t.Fatal(err)
	}
	m.actions.AssistantService.WriteConsentSharing = store.WriteConsentSharing
	m.actions.AssistantService.ReadSharing = store.ReadSharing
	for _, share := range []bool{true, false} {
		m.assistant.sharing = share
		result := m.assistantAccess(t.Context(), m.assistant.id, true).(assistantAccessMsg)
		if result.err != nil || !result.consent || result.sharing != share {
			t.Fatal("consent checkbox not saved", result.err)
		}
		if actual, err := store.ReadSharing(); err != nil || actual != share {
			t.Fatal("consent persistence", err)
		}
	}
	m.assistant.sharing = false
	updated, _ := m.finishAssistantActivation(assistantActivationMsg{id: m.assistant.id, sharing: true, account: domain.AssistantAccount{Name: "Synthetic account"}})
	if !updated.(dashboardModel).assistant.sharing {
		t.Fatal("new consent did not preselect saving")
	}
}
