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
	if cmd == nil || enabled != 0 {
		t.Fatal("Enter must request explicit acceptance")
	}
	m = workspaceComplete(t, m, cmd)
	if enabled != 1 {
		t.Fatal("Enter did not enable sharing")
	}
	m, _ = workspaceKey(m, demoText("?"))
	m, cmd = workspaceKey(m, demoText("e"))
	if cmd != nil || enabled != 1 {
		t.Fatal("help enabled sharing")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	m, cmd = workspaceKey(m, demoText("e"))
	m = workspaceComplete(t, m, cmd)
	if enabled != 2 {
		t.Fatal("explicit enable unavailable")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		m.width = size[0]
		m.height = size[1]
		v := m.View().Content
		if !strings.Contains(v, "Share statistics") || !strings.Contains(v, "Esc") {
			t.Fatal("missing actions", size, v)
		}
	}
	m.actions.ClassroomMode = true
	if cmd := m.checkTelemetryOffer(); cmd != nil {
		t.Fatal("teacher prompt")
	}
}

func TestTelemetryFirstInvitationChoices(t *testing.T) {
	for _, key := range []string{"e", "enter", "n", "d", "esc"} {
		t.Run(key, func(t *testing.T) {
			m := experienceFixture(2)
			m.screen = dashboardHome
			calls := []string{}
			m.actions.Telemetry = func(_ context.Context, action string) (domain.TelemetryReport, error) {
				calls = append(calls, action)
				consent := "disabled"
				if action == "enable" {
					consent = "enabled"
				}
				return domain.TelemetryReport{Consent: consent}, nil
			}
			next, _ := m.finishTelemetry(telemetryMsg{offer: true, report: domain.TelemetryReport{Consent: "undecided", Payload: domain.TelemetryPayload{Version: "3.0.0", DeploymentMode: "laboratory", ConfiguredClients: "16-30"}}})
			m = next.(dashboardModel)
			m, cmd := workspaceKey(m, demoText("p"))
			if cmd != nil || len(calls) != 0 || m.telemetry.detail != "p" {
				t.Fatal("preview changed consent")
			}
			if !strings.Contains(m.telemetryView(), "schemaVersion") {
				t.Fatal("exact report unavailable")
			}
			m, _ = workspaceKey(m, demoText("p"))
			if key == "esc" {
				m, cmd = workspaceKey(m, demoCode(tea.KeyEscape))
			} else if key == "enter" {
				m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
			} else {
				m, cmd = workspaceKey(m, demoText(key))
			}
			if cmd != nil {
				m = workspaceComplete(t, m, cmd)
			}
			if key == "esc" {
				if m.screen != dashboardHome || m.telemetry.firstOffer {
					t.Fatal("escape did not skip without consent")
				}
			} else if m.screen != dashboardHome {
				t.Fatal("first choice did not return home")
			}
			if key == "esc" && len(calls) != 0 {
				t.Fatal("escape saved consent")
			}
			if (key == "e" || key == "enter") && (len(calls) != 1 || calls[0] != "enable") {
				t.Fatal(calls)
			}
			if (key == "d" || key == "n") && (len(calls) != 1 || calls[0] != "disable") {
				t.Fatal(calls)
			}
		})
	}
}

func TestTelemetryInvitationRenderGallery(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			m := experienceFixture(2)
			m.screen = dashboardTelemetry
			m.width, m.height, m.isDark = size[0], size[1], dark
			m.telemetry.report = domain.TelemetryReport{Consent: "undecided", Payload: domain.TelemetryPayload{Version: "3.0.0", DeploymentMode: "laboratory", ConfiguredClients: "16-30"}}
			view := m.View().Content
			for _, text := range []string{"optional", "Share statistics", "No thanks", "Exact report", "Privacy & retention", "Status: Off", "Esc"} {
				if !strings.Contains(view, text) {
					t.Fatalf("missing %q at %v: %s", text, size, view)
				}
			}
			if len(strings.Split(view, "\n")) > size[1] {
				t.Fatal("invitation exceeds terminal")
			}
			if size[0] == 80 && !dark {
				t.Log("80x24 invitation:\n" + view)
			}
		}
	}
}

func TestTelemetryDetailsRemainLocalAndReturnToChoice(t *testing.T) {
	m := experienceFixture(2)
	m.screen = dashboardTelemetry
	m.telemetry.firstOffer = true
	m.telemetry.report = domain.TelemetryReport{Consent: "undecided", Payload: domain.TelemetryPayload{Version: "3.0.0"}}
	m.actions.Telemetry = func(context.Context, string) (domain.TelemetryReport, error) {
		t.Fatal("detail triggered action")
		return domain.TelemetryReport{}, nil
	}
	for _, key := range []string{"p", "i"} {
		m, cmd := workspaceKey(m, demoText(key))
		if cmd != nil || m.telemetry.detail != key {
			t.Fatal("detail unavailable")
		}
		m, cmd = workspaceKey(m, demoCode(tea.KeyEnter))
		if cmd != nil {
			t.Fatal("detail Enter accepted consent")
		}
		m, cmd = workspaceKey(m, demoCode(tea.KeyEscape))
		if cmd != nil || m.telemetry.detail != "" || m.screen != dashboardTelemetry || !m.telemetry.firstOffer {
			t.Fatal("detail escape bypassed choice")
		}
	}
}
func TestTelemetryOfferDoesNotRememberUndecidedAsRefused(t *testing.T) {
	for _, consent := range []string{"undecided", "enabled", "disabled"} {
		m := experienceFixture(2)
		m.screen = dashboardHome
		m.actions.Telemetry = func(_ context.Context, action string) (domain.TelemetryReport, error) {
			if action != "status" {
				t.Fatal("offer persisted without a choice", action)
			}
			// Legacy invitations could mark Prompted before saving a decision.
			return domain.TelemetryReport{Consent: consent, Prompted: true}, nil
		}
		msg := m.checkTelemetryOffer()().(telemetryMsg)
		if (msg.err == nil) != (consent == "undecided") {
			t.Fatal("refusal reoffered or undecided skipped")
		}
	}
	if administrationTasks[len(administrationTasks)-1].id != "telemetry" {
		t.Fatal("statistics must be last")
	}
}

func TestTelemetryDetailGalleryAndInvitationExitLegend(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			for _, detail := range []string{"", "p", "i"} {
				m := experienceFixture(2)
				m.screen = dashboardTelemetry
				m.width, m.height, m.isDark = size[0], size[1], dark
				m.telemetry.firstOffer = true
				m.telemetry.detail = detail
				m.telemetry.report = domain.TelemetryReport{Consent: "undecided", Payload: domain.TelemetryPayload{Version: "3.0.0"}}
				view := m.View().Content
				if len(strings.Split(view, "\n")) > size[1] {
					t.Fatal("detail exceeds terminal", size, detail)
				}
				for _, action := range []string{"Share statistics", "No thanks", "Exact report", "Privacy & retention"} {
					if !strings.Contains(view, action) {
						t.Fatal("missing choice or details", size, detail, action)
					}
				}
				if !strings.Contains(view, "Esc") || !strings.Contains(view, "Exit") {
					t.Fatal("incorrect first-invitation exit legend", detail)
				}
				if detail == "" && strings.Contains(view, "Version:") {
					t.Fatal("main invitation exposes exact values")
				}
			}
		}
	}
}
