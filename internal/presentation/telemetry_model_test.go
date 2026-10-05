package presentation

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/giovantenne/nixorium/internal/domain"
	"strings"
	"testing"
)

func TestTelemetryUIRequiresExplicitChoice(t *testing.T) {
	m := experienceFixture(2)
	m.screen = dashboardAdministration
	enabled := 0
	m.actions.Telemetry = func(_ context.Context, action string) (domain.TelemetryReport, error) {
		if action == "enable" {
			enabled++
		}
		return domain.TelemetryReport{Consent: "undecided", Payload: domain.TelemetryPayload{Version: "3.0.0"}}, nil
	}
	next, cmd := m.openMaintenanceTask("a")
	m = workspaceComplete(t, next.(dashboardModel), cmd)
	if enabled != 0 || m.screen != dashboardTelemetry {
		t.Fatal("implicit consent")
	}
	m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd != nil || enabled != 0 {
		t.Fatal("default enabled")
	}
	m, _ = workspaceKey(m, demoText("?"))
	m, cmd = workspaceKey(m, demoText("e"))
	if cmd != nil || enabled != 0 {
		t.Fatal("help enabled sharing")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	m, cmd = workspaceKey(m, demoText("e"))
	m = workspaceComplete(t, m, cmd)
	if enabled != 1 {
		t.Fatal("explicit enable unavailable")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		m.width = size[0]
		m.height = size[1]
		v := m.View().Content
		if !strings.Contains(v, "Enable sharing") || !strings.Contains(v, "Esc") {
			t.Fatal("missing actions", size, v)
		}
	}
	m.actions.ClassroomMode = true
	if cmd := m.checkTelemetryOffer(); cmd != nil {
		t.Fatal("teacher prompt")
	}
}
