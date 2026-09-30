package presentation

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func templateResetUIFixture() dashboardModel {
	m := experienceFixture(2)
	m.screen = dashboardAdministration
	p := demoTemplateResetPlan()
	m.actions.LoadTemplateReset = func(context.Context) domain.TemplateResetCatalog {
		return domain.TemplateResetCatalog{UpstreamRevision: p.UpstreamRevision, Catalog: domain.SoftwarePresetCatalog{Presets: []domain.SoftwarePreset{p.Preset}}}
	}
	m.actions.PlanTemplateReset = func(context.Context, string, func(string)) domain.TemplateResetPlan { return p }
	m.actions.ApplyTemplateReset = func(domain.TemplateResetPlan) domain.TemplateResetResult {
		return domain.TemplateResetResult{State: "saved"}
	}
	return m
}

func TestTemplateResetUIRequiresExactConfirmationAndSavesOnce(t *testing.T) {
	m := templateResetUIFixture()
	saves := 0
	m.actions.ApplyTemplateReset = func(p domain.TemplateResetPlan) domain.TemplateResetResult {
		saves++
		if p.ReviewToken != demoTemplateResetPlan().ReviewToken {
			t.Fatal("wrong reviewed proposal")
		}
		return domain.TemplateResetResult{State: "saved"}
	}
	m, command := workspaceKey(m, demoText("t"))
	m = workspaceComplete(t, m, command)
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	m = workspaceComplete(t, m, command)
	if m.templateReset.stage != "review" || saves != 0 {
		t.Fatal("did not enter a read-only review")
	}
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command != nil || saves != 0 {
		t.Fatal("empty confirmation authorized reset")
	}
	m, _ = workspaceKey(m, demoText("RESET DEPLOYMENT"))
	m, _ = workspaceKey(m, demoCode(tea.KeyF1))
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command != nil || saves != 0 {
		t.Fatal("help authorized reset")
	}
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command == nil || !m.templateReset.saving {
		t.Fatal("confirmed reset not dispatched")
	}
	for _, key := range []tea.KeyPressMsg{demoCode(tea.KeyEnter), demoCode(tea.KeyEscape), demoText("q"), {Code: 'c', Mod: tea.ModCtrl}} {
		next, duplicate := workspaceKey(m, key)
		if duplicate != nil || !next.templateReset.saving || next.screen != dashboardTemplateReset {
			t.Fatal("reset was repeated or left running without its view")
		}
	}
	m = workspaceComplete(t, m, command)
	if saves != 1 || m.templateReset.result.State != "saved" {
		t.Fatal("reset result missing")
	}
	m, command = workspaceKey(m, demoCode(tea.KeyEnter))
	if command != nil || saves != 1 || m.screen != dashboardAdministration {
		t.Fatal("result replayed mutation")
	}
}

func TestTemplateResetUICancellationAndLateMessages(t *testing.T) {
	m := templateResetUIFixture()
	var request context.Context
	m.actions.LoadTemplateReset = func(ctx context.Context) domain.TemplateResetCatalog {
		request = ctx
		return domain.TemplateResetCatalog{}
	}
	m, command := workspaceKey(m, demoText("t"))
	message := command()
	m, _ = workspaceKey(m, demoCode(tea.KeyEscape))
	if request.Err() == nil || m.screen != dashboardAdministration {
		t.Fatal("collection not cancelled")
	}
	next, _ := m.Update(message)
	m = next.(dashboardModel)
	if m.screen != dashboardAdministration || m.busy != "" {
		t.Fatal("late message restored cancelled screen")
	}
	m = templateResetUIFixture()
	m.actions.ClassroomMode = true
	_, command = m.openTemplateReset()
	if command != nil {
		t.Fatal("classroom gained reset capability")
	}
}

func TestTemplateResetUIRendering(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, dark := range []bool{true, false} {
			for _, state := range []string{"select", "review", "end", "loading", "saving", "blocked", "saved", "recovery-required"} {
				t.Run(fmt.Sprintf("%dx%d/%t/%s", size[0], size[1], dark, state), func(t *testing.T) {
					m := templateResetUIFixture()
					m.width, m.height, m.isDark, m.screen = size[0], size[1], dark, dashboardTemplateReset
					m.templateReset.stage, m.templateReset.plan = "review", demoTemplateResetPlan()
					want := []string{"Reset deployment template", "F1", "Help", "Esc"}
					switch state {
					case "select":
						m.templateReset.stage = "select"
						m.templateReset.catalog = m.actions.LoadTemplateReset(context.Background())
						want = append(want, "Essential", "Review reset")
					case "review", "end":
						want = append(want, "RESET DEPLOYMENT", "Back up and reset")
						if state == "end" {
							m.templateReset.scroll = 9999
							want = append(want, "modules/local.nix")
						}
					case "loading", "saving":
						m.busy = "Preparing the reset"
						if state == "saving" {
							m.templateReset.saving = true
							want = []string{"Reset deployment template", "Help"}
						} else {
							m.beginRead(dashboardReadTimeout)
							t.Cleanup(m.read.cancel)
						}
					default:
						m.templateReset.stage = "result"
						m.templateReset.result = domain.TemplateResetResult{State: state, BackupRef: "refs/nixorium/template-backups/20260929T120000.123456789Z-0123456789ab", Message: "No system was applied."}
						want = append(want, strings.ToUpper(state), "Backup:")
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
