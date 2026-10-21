package presentation

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestSavedInstallationSummaryContinuesEditsAndCancels(t *testing.T) {
	m := newTestModel(t)
	m.installation = installationModel{flow: true, startingLabSetup: true, method: "pxe"}
	m.actions.LoadSetup = func(context.Context) domain.SetupReport {
		return domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageValidate}
	}
	updated, cmd := m.Update(dashboardSettingsMsg{settings: wizardSettings()})
	m = updated.(dashboardModel)
	if cmd != nil || !m.installation.savedSummary || m.screen != dashboardSettings {
		t.Fatal("complete settings replayed form")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		m.width, m.height = size[0], size[1]
		view := m.View().Content
		if !strings.Contains(view, "saved installation settings") || strings.Contains(view, "$6$") || lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatal("invalid summary layout or leaked credentials")
		}
	}
	edit, _ := workspaceKey(m, tea.KeyPressMsg{Text: "e"})
	if edit.screen != dashboardSettingsEdit || edit.installation.savedSummary {
		t.Fatal("edit unavailable")
	}
	cancel, cmd := workspaceKey(m, demoCode(tea.KeyEscape))
	if cmd != nil || cancel.installation.flow || cancel.screen != dashboardPXE {
		t.Fatal("cancel started work")
	}
	continued, cmd := workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd == nil || continued.installation.savedSummary {
		t.Fatal("preflight skipped")
	}
	continued = workspaceComplete(t, continued, cmd)
	if !continued.installation.failed {
		t.Fatal("incomplete fresh preflight accepted")
	}
}

func TestSavedInstallationSummaryAlsoServesUSBInstallation(t *testing.T) {
	m := newTestModel(t)
	m.installation = installationModel{flow: true, startingLabSetup: true, method: domain.RemoteInstallUSBSSH}
	m.actions.LoadSetup = func(context.Context) domain.SetupReport {
		return domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageValidate}
	}
	updated, cmd := m.Update(dashboardSettingsMsg{settings: wizardSettings()})
	m = updated.(dashboardModel)
	if cmd != nil || !m.installation.savedSummary || m.screen != dashboardSettings {
		t.Fatal("complete settings replayed the form for a USB installation")
	}
	m.width, m.height = 120, 30
	if view := m.View().Content; !strings.Contains(view, "USB over SSH") || strings.Contains(view, "Network boot") {
		t.Fatal("summary names the wrong installation method")
	}
	cancel, cmd := workspaceKey(m, demoCode(tea.KeyEscape))
	if cmd != nil || cancel.installation.flow || cancel.screen != dashboardInstallationArea {
		t.Fatal("cancel did not return to the installation area")
	}
	continued, cmd := workspaceKey(m, demoCode(tea.KeyEnter))
	if cmd == nil || continued.installation.savedSummary {
		t.Fatal("preflight skipped")
	}
}

func TestSummaryShowsTheDHCPAddressTheControllerHasNow(t *testing.T) {
	addresses := domain.ControllerAddresses{Detected: true, Choices: []domain.DHCPAddressChoice{{Address: "10.0.2.15", Device: "enp0s8"}, {Address: "192.168.57.99", Device: "enp0s3"}}}
	if got := summaryDHCPAddress("192.168.57.99", "enp0s3", addresses); got != "192.168.57.99" {
		t.Fatalf("present address = %q", got)
	}
	if got := summaryDHCPAddress("192.168.56.99", "enp0s3", addresses); got != "192.168.57.99 (detected now; the saved 192.168.56.99 is no longer on this controller)" {
		t.Fatalf("stale address = %q", got)
	}
	if got := summaryDHCPAddress("192.168.56.99", "enp0s3", domain.ControllerAddresses{}); got != "192.168.56.99" {
		t.Fatalf("undetected = %q", got)
	}
}
