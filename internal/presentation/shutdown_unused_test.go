package presentation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestPowerReviewSeparatesSessionsInUseFromUnusedLogins(t *testing.T) {
	target := func(name string, session domain.ShutdownSessionState) domain.ShutdownTargetPlan {
		return domain.ShutdownTargetPlan{Name: name, Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable, Session: session, Eligible: true}
	}
	plan := domain.ShutdownPlanReport{State: "ready", Action: domain.ClientPowerOff, Confirmation: "SHUTDOWN", Eligible: 2,
		Targets: []domain.ShutdownTargetPlan{target("pc01", domain.ShutdownSessionUnused), target("pc02", domain.ShutdownSessionIdle)}}
	m := shutdownModel{plan: plan}
	lines, prompt := m.reviewView(shutdownViewContext{height: 40})
	text := strings.Join(append(lines, prompt...), "\n")
	for _, want := range []string{"pc01 · Logged in, not in use · Ready", "Nobody is using the selected computers now", "Press Enter to shut down"} {
		if !strings.Contains(text, want) {
			t.Fatalf("unused review lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "unsaved work may be lost") || strings.Contains(text, "active sessions") {
		t.Fatalf("unused logins were described as sessions in use:\n%s", text)
	}
	plan.Targets = append(plan.Targets, target("pc03", domain.ShutdownSessionActive))
	m.plan = plan
	lines, prompt = m.reviewView(shutdownViewContext{height: 40})
	text = strings.Join(append(lines, prompt...), "\n")
	if !strings.Contains(text, "1 computer is in use") || !strings.Contains(text, "1 session in use will be interrupted") {
		t.Fatalf("session in use not highlighted:\n%s", text)
	}
	var output bytes.Buffer
	if _, err := ConfirmShutdown(strings.NewReader("no\n"), &output, plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Logged in but not in use: 1 computer") || !strings.Contains(output.String(), "of 1 computer with an active user session") {
		t.Fatalf("CLI review:\n%s", output.String())
	}
}

func TestPowerReviewNamesTheChosenActionForSessionsInUse(t *testing.T) {
	for action, want := range map[domain.ClientPowerAction]string{domain.ClientPowerOff: "pc02 · In use · will shut down", domain.ClientRestart: "pc02 · In use · will restart"} {
		model := shutdownModel{plan: domain.ShutdownPlanReport{State: "ready", Action: action, Eligible: 1, Targets: []domain.ShutdownTargetPlan{{Name: "pc02", Eligible: true, Session: domain.ShutdownSessionActive}}}}
		body, _ := model.reviewView(shutdownViewContext{height: 30})
		text := strings.Join(body, "\n")
		if !strings.Contains(text, want) {
			t.Fatalf("%s review misses %q:\n%s", action, want, text)
		}
		if action == domain.ClientRestart && strings.Contains(text, "will shut down") {
			t.Fatalf("restart review talks about shutting down:\n%s", text)
		}
	}
}
