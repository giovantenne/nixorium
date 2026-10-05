package presentation

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDisclaimerRequiresAcceptanceBeforeInitialLoad(t *testing.T) {
	m := experienceFixture(2)
	m.screen = dashboardDisclaimer
	m.initializing = true
	accepted, loaded := 0, 0
	m.actions.AcceptDisclaimer = func() error { accepted++; return nil }
	m.actions.LoadInitial = func(context.Context) (domain.StatusReport, domain.SetupReport, error) {
		loaded++
		return m.report, m.setup, nil
	}
	for _, key := range []string{"esc", "c", "n", "e", "f1"} {
		next, cmd := workspaceKey(m, keyPress(key))
		if cmd != nil || next.screen != dashboardDisclaimer || accepted != 0 || loaded != 0 {
			t.Fatal("bypassed acknowledgement", key)
		}
	}
	_, quit := workspaceKey(m, keyPress("q"))
	if quit == nil {
		t.Fatal("exit unavailable")
	}
	next, cmd := workspaceKey(m, keyPress("enter"))
	if cmd == nil {
		t.Fatal("accept missing")
	}
	if accepted != 0 || loaded != 0 {
		t.Fatal("side effect outside callback")
	}
	nextModel, start := next.Update(cmd())
	next = nextModel.(dashboardModel)
	if accepted != 1 || loaded != 0 || next.screen != dashboardHome || start == nil {
		t.Fatal("bad accepted transition")
	}
	if _, ok := start().(dashboardBeginInitialMsg); !ok {
		t.Fatal("initial loading not scheduled")
	}
}
func TestInitialInvitationPrecedesSetupAndLateReplyCannotReopen(t *testing.T) {
	m := experienceFixture(2)
	m.actions.LoadDisclaimer = func(context.Context) (bool, error) { return true, nil }
	calls := 0
	m.actions.Telemetry = func(_ context.Context, action string) (domain.TelemetryReport, error) {
		calls++
		if action != "status" {
			t.Fatal("unexpected consent")
		}
		return domain.TelemetryReport{Consent: "undecided"}, nil
	}
	next, _ := m.Update(dashboardInitialMsg{report: m.report, setup: domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageNetwork}})
	m = next.(dashboardModel)
	if m.screen != dashboardTelemetry || !m.telemetry.firstOffer {
		t.Fatal("setup interrupted invitation")
	}
	m, cmd := workspaceKey(m, keyPress("esc"))
	if cmd != nil || m.screen != dashboardHome || calls != 0 {
		t.Fatal("skip saved consent")
	}
	next, _ = m.Update(telemetryMsg{offer: true, startup: true, report: domain.TelemetryReport{Consent: "undecided"}})
	if next.(dashboardModel).screen != dashboardHome {
		t.Fatal("late reply reopened invitation")
	}
}
func TestComputersExplainMissingConfigurationAndOfferInstallation(t *testing.T) {
	m := experienceFixture(0)
	m.actions.LoadClientSetup = func(context.Context) (bool, error) { return false, nil }
	m, cmd := workspaceKey(m, keyPress("c"))
	m = workspaceComplete(t, m, cmd)
	if m.screen != dashboardComputersArea || m.computerSetup != "missing" {
		t.Fatal("missing configuration not observed")
	}
	if !strings.Contains(m.View().Content, "have not been configured") || !strings.Contains(m.View().Content, "Configure clients") {
		t.Fatal("no explicit guidance")
	}
	m, cmd = workspaceKey(m, keyPress("n"))
	if m.screen != dashboardInstallationArea || cmd != nil {
		t.Fatal("configuration route unavailable")
	}
}
func TestUpdateNotificationDoesNotInterruptOrApply(t *testing.T) {
	m := experienceFixture(2)
	m.screen = dashboardSoftware
	notice := domain.UpdateNotification{Key: "test", Target: "v3.1.0", Channel: domain.UpdateChannelStable}
	next, _ := m.Update(updateNotificationMsg{notice: notice})
	m = next.(dashboardModel)
	if m.screen != dashboardSoftware || m.updateNotification.Key != "test" {
		t.Fatal("notification interrupted work")
	}
	m.screen = dashboardHome
	dismissed := false
	m.actions.DismissUpdateNotification = func(n domain.UpdateNotification) error { dismissed = n.Key == "test"; return nil }
	m, cmd := workspaceKey(m, keyPress("x"))
	if cmd == nil || dismissed {
		t.Fatal("invalid dismissal callback")
	}
	next, _ = m.Update(cmd())
	m = next.(dashboardModel)
	if !dismissed || m.updateNotification.Key != "" {
		t.Fatal("dismissal failed")
	}
}
func TestOnboardingAndNotificationRenderGallery(t *testing.T) {
	var gallery strings.Builder
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		for _, dark := range []bool{false, true} {
			base := experienceFixture(2)
			base.width, base.height, base.isDark = size[0], size[1], dark
			states := []struct {
				name     string
				model    dashboardModel
				required []string
			}{}
			m := base
			m.screen = dashboardDisclaimer
			states = append(states, struct {
				name     string
				model    dashboardModel
				required []string
			}{"disclaimer", m, []string{"Accept & continue", "Exit", "without warranty"}})
			m = base
			m.screen = dashboardTelemetry
			m.telemetry.firstOffer = true
			m.telemetry.report = domain.TelemetryReport{Consent: "undecided"}
			states = append(states, struct {
				name     string
				model    dashboardModel
				required []string
			}{"telemetry", m, []string{"Share statistics", "No thanks", "Menu", "Exit", "off by default"}})
			m = base
			m.updateNotification = domain.UpdateNotification{Key: "version", Target: "v3.1.0", Channel: domain.UpdateChannelStable}
			states = append(states, struct {
				name     string
				model    dashboardModel
				required []string
			}{"update", m, []string{"v3.1.0", "Dismiss update", "Maintenance"}})
			m = base
			m.screen = dashboardComputersArea
			m.computerSetup = "missing"
			states = append(states, struct {
				name     string
				model    dashboardModel
				required []string
			}{"computers", m, []string{"not been configured", "Configure clients", "Esc"}})
			for _, state := range states {
				view := state.model.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%s overflows %v", state.name, size)
				}
				for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
					var b bytes.Buffer
					w := colorprofile.Writer{Forward: &b, Profile: profile}
					_, _ = w.Write([]byte(view))
					plain := demoANSI.ReplaceAllString(b.String(), "")
					for _, text := range state.required {
						if !strings.Contains(plain, text) {
							t.Fatalf("%s %v lacks %q:\n%s", state.name, size, text, plain)
						}
					}
				}
				if size[0] == 80 && dark {
					gallery.WriteString(state.name + "\n" + demoANSI.ReplaceAllString(view, "") + "\n")
				}
			}
		}
	}
	if path := os.Getenv("NIXORIUM_ONBOARDING_GALLERY"); path != "" {
		if err := os.WriteFile(path, []byte(gallery.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSkippedTelemetryReturnsOnNextLaunch(t *testing.T) {
	for _, consent := range []string{"undecided", "enabled", "disabled"} {
		m := experienceFixture(2)
		m.actions.Telemetry = func(_ context.Context, action string) (domain.TelemetryReport, error) {
			if action != "status" {
				t.Fatal("startup saved consent")
			}
			return domain.TelemetryReport{Consent: consent}, nil
		}
		// Each fresh model represents a new TUI invocation with the same saved consent.
		for launch := 0; launch < 2; launch++ {
			next, cmd := m.beginTelemetryInvitation()
			updated, _ := next.Update(cmd())
			current := updated.(dashboardModel)
			if (current.screen == dashboardTelemetry) != (consent == "undecided") {
				t.Fatal("wrong repeat policy", consent, launch)
			}
			if consent == "undecided" {
				current, cmd = workspaceKey(current, keyPress("esc"))
				if cmd != nil || current.screen != dashboardHome {
					t.Fatal("skip saved a decision")
				}
			}
		}
	}
}

func TestTelemetryLateStartupStatusCannotOverwriteChoice(t *testing.T) {
	m := experienceFixture(2)
	m.actions.Telemetry = func(_ context.Context, action string) (domain.TelemetryReport, error) {
		consent := "undecided"
		if action == "disable" {
			consent = "disabled"
		}
		return domain.TelemetryReport{Consent: consent}, nil
	}
	next, status := m.beginTelemetryInvitation()
	m = next.(dashboardModel)
	// A visible invitation can be refreshed while a previous status is still pending.
	m.telemetry.loadingOffer = false
	m, choice := workspaceKey(m, keyPress("n"))
	updated, _ := m.Update(status())
	m = updated.(dashboardModel)
	if !m.telemetry.applying {
		t.Fatal("late status interrupted consent save")
	}
	updated, _ = m.Update(choice())
	m = updated.(dashboardModel)
	if m.screen != dashboardHome || m.telemetry.report.Consent != "disabled" {
		t.Fatal("refusal not preserved")
	}
}
func TestUSBConsoleInstructionsScrollWithInputVisible(t *testing.T) {
	m := experienceFixture(2)
	m.width, m.height = 80, 24
	m.screen = dashboardUSBInstall
	m.installation.remote = remoteInstallationModel{stage: remoteInstallConsole, host: "pc01", address: "192.0.2.20"}
	var seen strings.Builder
	for offset := 0; offset < 24; offset++ {
		m.pageScroll = offset
		view := demoANSI.ReplaceAllString(m.View().Content, "")
		if !strings.Contains(view, "IP address of the PC") || !strings.Contains(view, "192.0.2.20") || !strings.Contains(view, "Check PC identity") {
			t.Fatal("input or action scrolled away", view)
		}
		if lipgloss.Height(view) > 24 || lipgloss.Width(view) > 80 {
			t.Fatal("USB instructions overflow")
		}
		seen.WriteString(view)
	}
	for _, command := range []string{"passwd", "systemctl is-active sshd", "ip -4 -br address show scope global"} {
		if !strings.Contains(seen.String(), command) {
			t.Fatal("unreachable console command", command)
		}
	}
}

func TestReturningAdministratorDoesNotFlashDisclaimer(t *testing.T) {
	m := experienceFixture(2)
	m.initializing = true
	m.disclaimerChecking = true
	if strings.Contains(m.View().Content, "Accept & continue") {
		t.Fatal("unread acknowledgement flashed disclaimer")
	}
	next, cmd := m.Update(disclaimerMsg{accepted: true})
	m = next.(dashboardModel)
	if m.disclaimerChecking || m.screen != dashboardHome || cmd == nil {
		t.Fatal("accepted acknowledgement did not proceed")
	}
}

func TestSavedRefusalNeverDisplaysInvitationDuringLoad(t *testing.T) {
	m := experienceFixture(2)
	m.actions.Telemetry = func(context.Context, string) (domain.TelemetryReport, error) {
		return domain.TelemetryReport{Consent: "disabled"}, nil
	}
	next, cmd := m.beginTelemetryInvitation()
	m = next.(dashboardModel)
	if strings.Contains(m.View().Content, "Share statistics") {
		t.Fatal("saved choice loading flashed invitation")
	}
	updated, _ := m.Update(cmd())
	m = updated.(dashboardModel)
	if m.screen != dashboardHome {
		t.Fatal("saved refusal was offered again")
	}
}
