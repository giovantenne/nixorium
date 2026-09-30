package presentation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func supportUIFixture() dashboardModel {
	m := experienceFixture(2)
	m.screen = dashboardDiagnostics
	m.actions.PreviewSupport = func(context.Context) (domain.SupportSnapshot, error) { return demoSupportSnapshot(), nil }
	m.actions.ExportSupport = func(domain.SupportSnapshot) domain.SupportExportResult {
		return domain.SupportExportResult{State: "saved", Message: "Saved locally. Nothing was uploaded."}
	}
	return m
}

func TestSupportUIExactPreviewAndSingleConfirmedSave(t *testing.T) {
	m := supportUIFixture()
	saves := 0
	m.actions.ExportSupport = func(snapshot domain.SupportSnapshot) domain.SupportExportResult {
		saves++
		if snapshot.JSON() != demoSupportSnapshot().JSON() {
			t.Fatal("export differs from preview")
		}
		return domain.SupportExportResult{State: "saved"}
	}
	m, command := workspaceKey(m, demoText("e"))
	if m.screen != dashboardSupport || command == nil || saves != 0 {
		t.Fatal("preview navigation failed")
	}
	m = workspaceComplete(t, m, command)
	m, command = workspaceKey(m, demoText("?"))
	if !m.helpOpen || command != nil {
		t.Fatal("help failed")
	}
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command != nil || saves != 0 {
		t.Fatal("help authorized export")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	m, _ = workspaceKey(m, demoCode(tea.KeyEnd))
	if m.support.scroll == 0 || !strings.Contains(m.View().Content, "excluded") {
		t.Fatal("cannot inspect entire payload")
	}
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command == nil || !m.support.saving || saves != 0 {
		t.Fatal("save was not dispatched explicitly")
	}
	for _, key := range []tea.KeyPressMsg{demoCode(tea.KeyEnter), demoText("r"), demoCode(tea.KeyEscape), demoText("q")} {
		next, duplicate := workspaceKey(m, key)
		if duplicate != nil || !next.support.saving || next.screen != dashboardSupport {
			t.Fatal("duplicate export or early exit")
		}
	}
	m = workspaceComplete(t, m, command)
	if saves != 1 || m.support.result.State != "saved" || m.support.saving {
		t.Fatal("incorrect result")
	}
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command != nil || m.screen != dashboardDiagnostics || saves != 1 {
		t.Fatal("result replayed export")
	}
}

func TestSupportUICancelAndStaleMessages(t *testing.T) {
	m := supportUIFixture()
	var collectedContext context.Context
	m.actions.PreviewSupport = func(ctx context.Context) (domain.SupportSnapshot, error) {
		collectedContext = ctx
		return demoSupportSnapshot(), nil
	}
	m, command := workspaceKey(m, demoText("e"))
	message := command()
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	if collectedContext.Err() == nil || m.busy != "" {
		t.Fatal("cancel did not stop collection")
	}
	next, _ := m.Update(message)
	m = next.(dashboardModel)
	if m.screen != dashboardDiagnostics || m.support.snapshot.Valid() {
		t.Fatal("late preview restored cancelled state")
	}
	m, command = workspaceKey(m, demoText("e"))
	old := command()
	m, unavailable := workspaceKey(m, demoText("r"))
	if unavailable != nil || !strings.Contains(m.message, "Esc") {
		t.Fatal("busy refresh did not explain how to cancel first")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	m, newCommand := workspaceKey(m, demoText("e"))
	next, _ = m.Update(old)
	m = next.(dashboardModel)
	if m.support.snapshot.Valid() {
		t.Fatal("refresh accepted old preview")
	}
	m = workspaceComplete(t, m, newCommand)
	m, command = workspaceKey(m, demoCode(tea.KeyEscape))
	if command != nil || m.support.snapshot.Valid() {
		t.Fatal("cancel retained export authority")
	}
}

func TestSupportUINoPrivateErrorOrClassroomCapability(t *testing.T) {
	m := supportUIFixture()
	m.actions.PreviewSupport = func(context.Context) (domain.SupportSnapshot, error) {
		return domain.SupportSnapshot{}, errors.New("SECRET evaluator error")
	}
	m, command := workspaceKey(m, demoText("e"))
	m = workspaceComplete(t, m, command)
	if strings.Contains(m.View().Content, "SECRET") {
		t.Fatal("collection error leaked")
	}
	_, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command != nil {
		t.Fatal("failed preview is exportable")
	}
	m = supportUIFixture()
	m.actions.ClassroomMode = true
	m, command = workspaceKey(m, demoText("e"))
	if command != nil || m.screen != dashboardDiagnostics || strings.Contains(m.View().Content, "Support report") {
		t.Fatal("classroom gained administrative support")
	}
}

func TestSupportUIRendering(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, dark := range []bool{false, true} {
			for _, state := range []string{"preview", "end", "loading", "empty", "failed", "saving", "saved", "partial"} {
				t.Run(fmt.Sprintf("%dx%d/dark%t/%s", size[0], size[1], dark, state), func(t *testing.T) {
					m := supportUIFixture()
					m.screen = dashboardSupport
					m.support.snapshot = demoSupportSnapshot()
					m.width, m.height, m.isDark = size[0], size[1], dark
					want := []string{"Local support report", "No upload", "F1", "Help", "Esc"}
					switch state {
					case "preview":
						want = append(want, "Save locally", "schemaVersion")
					case "end":
						m.support.scroll = 9999
						want = append(want, "Save locally", "operation-identifiers-and-times")
					case "loading":
						m.busy = "Collecting diagnostics"
						m.beginRead(dashboardReadTimeout)
						t.Cleanup(m.read.cancel)
					case "empty", "failed":
						m.support.snapshot = domain.SupportSnapshot{}
						want = append(want, "Retry")
						if state == "failed" {
							m.message = "Collection unavailable; nothing was saved."
						}
					case "saving":
						m.support.saving = true
						m.busy = "Saving the reviewed report locally"
						want = []string{"Local support report", "Saving", "Help"}
					case "saved", "partial":
						m.support.result = domain.SupportExportResult{State: state, Path: "/home/admin/.local/state/nixorium/support/support-0123456789abcdef.json", Message: "Nothing was uploaded.", SHA256: strings.Repeat("a", 64)}
						want = append(want, strings.ToUpper(state), "Diagnostics")
					}
					view := m.View().Content
					if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
						t.Fatalf("overflow:\n%s", view)
					}
					for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
						var out bytes.Buffer
						writer := colorprofile.Writer{Forward: &out, Profile: profile}
						if _, err := writer.Write([]byte(view)); err != nil {
							t.Fatal(err)
						}
						plain := demoANSI.ReplaceAllString(out.String(), "")
						for _, text := range want {
							if !strings.Contains(plain, text) {
								t.Fatalf("missing %q:\n%s", text, plain)
							}
						}
					}
				})
			}
		}
	}
}
