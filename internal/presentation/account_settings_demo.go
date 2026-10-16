package presentation

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// RenderAccountSettingsDemo follows Maintenance → Change settings → Accounts
// with the real key handling. Settings load from a synthetic file; no
// password is collected and nothing is saved.
func RenderAccountSettingsDemo(revision string, width, height int) DemoScenario {
	actions := demoActions()
	actions.LoadSettings = func(context.Context) (domain.LabSettingsFile, error) { return demoSettings(), nil }
	r := newDemoRecorder(actions, revision, width, height)
	r.capture("Overview", 1000)
	r.pressAndCapture(demoText("m"), "Open Maintenance", 1600)
	load := r.key(demoText("e"))
	r.command(load)
	r.capture("Settings categories", 1600)
	r.pressAndCapture(demoText("a"), "Accounts: names or passwords", 2200)
	r.pressAndCapture(demoText("n"), "Change account names", 2200)
	r.pressAndCapture(demoCode(tea.KeyEscape), "Back to Accounts", 1000)
	r.pressAndCapture(demoText("p"), "Change account passwords", 2400)
	r.pressAndCapture(demoCode(tea.KeyEscape), "Esc returns to Accounts", 1000)
	return DemoScenario{ID: "account-settings", Title: "Change account names or passwords", Description: "Maintenance → Change settings → Accounts offers account names and account passwords as two separate reviewed forms.", Frames: r.frames}
}
