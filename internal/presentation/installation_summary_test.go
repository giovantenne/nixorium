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
	m := newDashboardModel(domain.StatusReport{}, domain.SetupReport{}, DashboardActions{}, false)
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
