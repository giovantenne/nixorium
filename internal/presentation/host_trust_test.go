package presentation

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func trustPresentationPlan() domain.HostTrustPlan {
	return domain.HostTrustPlan{State: "ready", Host: domain.HostMeta{Name: "pc01", IP: "10.0.0.1"}, Inspection: domain.HostTrustInspection{Recorded: []string{"SHA256:old"}, Offered: "SHA256:new"}, Confirmation: "ROTATE HOST KEY"}
}
func TestHostTrustConfirmationIsExact(t *testing.T) {
	for _, input := range []string{"ROTATE HOST KEY\n", "rotate host key\n", "yes\n", "ROTATE HOST\n"} {
		var output bytes.Buffer
		approved, err := ConfirmHostTrust(strings.NewReader(input), &output, trustPresentationPlan())
		if err != nil || approved != (input == "ROTATE HOST KEY\n") || !strings.Contains(output.String(), "SHA256:old") || !strings.Contains(output.String(), "SHA256:new") {
			t.Fatalf("%q approved=%t error=%v output=%s", input, approved, err, output.String())
		}
	}
}
func TestHostTrustReadCancellationIgnoresLateResult(t *testing.T) {
	var readContext context.Context
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{PlanHostTrust: func(ctx context.Context, _ string) domain.HostTrustPlan {
		readContext = ctx
		return trustPresentationPlan()
	}, ApplyHostTrust: func(domain.HostTrustPlan) domain.HostTrustResult {
		t.Fatal("cancel triggered write")
		return domain.HostTrustResult{}
	}}, false)
	model.screen = dashboardHosts
	opened, read := model.openHostTrust("pc01")
	model = opened.(dashboardModel)
	late := read()
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if readContext.Err() == nil || model.screen != dashboardHosts {
		t.Fatal("read not cancelled")
	}
	updated, _ = model.Update(late)
	model = updated.(dashboardModel)
	if model.screen != dashboardHosts || model.busy != "" || !strings.Contains(model.message, "Nothing was changed") {
		t.Fatal("late reply resumed cancelled action")
	}
}
func TestHostTrustTUIRequiresConfirmationAndProtectsMutation(t *testing.T) {
	writes := 0
	model := newDashboardModel(testDashboardReport("stopped"), domain.SetupReport{}, DashboardActions{PlanHostTrust: func(context.Context, string) domain.HostTrustPlan { return trustPresentationPlan() }, ApplyHostTrust: func(domain.HostTrustPlan) domain.HostTrustResult {
		writes++
		return domain.HostTrustResult{Operation: "host-key-apply", State: "saved", Message: "Reviewed trust saved"}
	}}, false)
	opened, read := model.openHostTrust("pc01")
	model = opened.(dashboardModel)
	updated, _ := model.Update(read())
	model = updated.(dashboardModel)
	if !model.textEntry() {
		t.Fatal("confirmation does not consume text input")
	}
	updated, help := model.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	model = updated.(dashboardModel)
	if help != nil || !model.helpOpen {
		t.Fatal("help invoked work")
	}
	updated, help = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if help != nil || writes != 0 {
		t.Fatal("help confirmed trust")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	model.hostTrust.confirmation = "yes"
	updated, write := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if write != nil || writes != 0 {
		t.Fatal("wrong confirmation started write")
	}
	model.hostTrust.confirmation = "ROTATE HOST KEY"
	updated, write = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEscape}, {Code: 'c', Mod: tea.ModCtrl}, demoText("q")} {
		updated, quit := model.Update(key)
		model = updated.(dashboardModel)
		if quit != nil || !model.hostTrust.applying || model.screen != dashboardHostTrust {
			t.Fatal("mutation was interruptible")
		}
	}
	updated, _ = model.Update(write())
	model = updated.(dashboardModel)
	if writes != 1 || model.hostTrust.applying || model.hostTrust.result.State != "saved" {
		t.Fatal("write result missing")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dashboardModel)
	if strings.Contains(model.message, "Nothing was changed") {
		t.Fatal("successful write described as cancelled")
	}
}
func TestClassroomCannotOpenHostTrust(t *testing.T) {
	model := dashboardModel{actions: DashboardActions{ClassroomMode: true, PlanHostTrust: func(context.Context, string) domain.HostTrustPlan {
		t.Fatal("classroom observed key")
		return domain.HostTrustPlan{}
	}, ApplyHostTrust: func(domain.HostTrustPlan) domain.HostTrustResult {
		t.Fatal("classroom changed key")
		return domain.HostTrustResult{}
	}}}
	opened, cmd := model.openHostTrust("pc01")
	if cmd != nil || opened.(dashboardModel).screen == dashboardHostTrust {
		t.Fatal("classroom gained administrative action")
	}
}
