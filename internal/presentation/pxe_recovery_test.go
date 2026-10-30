package presentation

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

const pxeAddressMissing = "normal controller address 10.0.0.254 is still missing on enp1s0; apply the current controller configuration (Maintenance > Rebuild controller), then run Diagnostics"

// pxeRecoveryFixture is a controller left on its temporary network setup.
type pxeRecoveryFixture struct {
	recovered, stopped int
	fail               bool
}

func (fixture *pxeRecoveryFixture) report(mode string) domain.StatusReport {
	report := testDashboardReport(mode)
	report.PXE.Listener = domain.ServiceState{Name: "nixorium-pxe.service", Loaded: true, State: "inactive"}
	report.PXE.Network = domain.ServiceState{Name: "nixorium-pxe-network.service", Loaded: true, State: "active", Active: mode == "recovery-required"}
	return report
}

func (fixture *pxeRecoveryFixture) model() dashboardModel {
	report := fixture.report("recovery-required")
	actions := DashboardActions{
		Refresh: func(context.Context) (domain.StatusReport, error) {
			if fixture.recovered+fixture.stopped > 0 && !fixture.fail {
				return fixture.report("stopped"), nil
			}
			return report, nil
		},
		RecoverPXE: func() domain.PXELifecycleReport {
			fixture.recovered++
			if fixture.fail {
				return domain.PXELifecycleReport{Operation: "pxe-recover", State: "failed", Mode: "stopped", Message: pxeAddressMissing}
			}
			return domain.PXELifecycleReport{Operation: "pxe-recover", State: "completed", Mode: "stopped", Message: "PXE recovery completed; installation mode is stopped"}
		},
		StopPXE: func() domain.PXELifecycleReport {
			fixture.stopped++
			return domain.PXELifecycleReport{Operation: "pxe-stop", State: "completed", Mode: "stopped", Message: "PXE installation mode stopped and normal addressing restored"}
		},
	}
	return dashboardModel{report: report, actions: actions, screen: dashboardPXE, width: 160, height: 40}
}

func TestPXERecoveryReviewSaysWhatWasFoundAndConfirmsWithEnter(t *testing.T) {
	fixture := &pxeRecoveryFixture{}
	model := fixture.model()
	view := plainView(model)
	for _, want := range []string{
		"Recover the controller network",
		"What Nixorium found",
		"The network installation was interrupted: the temporary network setup is still in place, but nothing serves installations.",
		"Network boot service stopped", "Temporary network setup still in place",
		"What you can do",
		"Enter Recover the network: restore the normal address, turn network installation off.",
		"x Stop the network installation: stop both services, check the normal address.",
		"Press Enter to recover the network. It changes only the controller; nothing changes on the student computers.",
		"Enter Recover the network", "x Stop the network installation", "Esc Installation",
	} {
		if !contains(view, want) {
			t.Fatalf("recovery review omits %q:\n%s", want, view)
		}
	}
	if contains(view, "Type ") || contains(view, "nixorium-pxe") {
		t.Fatalf("recovery asks for a word or shows unit names:\n%s", view)
	}
	model = runRead(model.Update(keyPress("enter")))
	view = plainView(model)
	if fixture.recovered != 1 || fixture.stopped != 0 || !contains(view, "✓ The controller network is back to normal; network installation is off.") ||
		!contains(view, "Nothing changed on the student computers.") || contains(view, "PXE recovery completed") || contains(view, "What you can do") {
		t.Fatalf("result (recovered %d):\n%s", fixture.recovered, view)
	}
}

func TestPXERecoveryCanStopTheSessionInstead(t *testing.T) {
	fixture := &pxeRecoveryFixture{}
	model := runRead(fixture.model().Update(keyPress("x")))
	if fixture.stopped != 1 || fixture.recovered != 0 || !contains(plainView(model), "Network installation is off and the controller network is back to normal.") {
		t.Fatalf("stop (stopped %d):\n%s", fixture.stopped, plainView(model))
	}
}

func TestPXERecoveryFailureKeepsTheTechnicalTextBehindD(t *testing.T) {
	fixture := &pxeRecoveryFixture{fail: true}
	model := runRead(fixture.model().Update(keyPress("enter")))
	view := plainView(model)
	for _, want := range []string{"× The controller network was not recovered.",
		"The normal controller address did not come back. Apply the saved configuration to the controller (Maintenance → Apply to controller), then open Maintenance → Diagnostics.",
		"d Details", "Enter Recover the network"} {
		if !contains(view, want) {
			t.Fatalf("failed recovery omits %q:\n%s", want, view)
		}
	}
	if contains(view, "still missing on enp1s0") {
		t.Fatalf("raw text shown by default:\n%s", view)
	}
	model = cleanupPress(t, model, keyPress("d"))
	if !contains(plainView(model), "Details: "+pxeAddressMissing) || !contains(plainView(model), "d Hide details") {
		t.Fatalf("d did not reveal the raw text:\n%s", plainView(model))
	}
}

func TestControllerRefusalOffersTheNetworkRecovery(t *testing.T) {
	fixture := &pxeRecoveryFixture{}
	model := fixture.model()
	model.screen = dashboardController
	model.message = "controller network or installation state requires recovery before activation"
	view := plainView(model)
	if !contains(view, "network installation is still active.") || !contains(view, "Enter Finish the network installation, or recover the controller network") ||
		contains(view, "requires recovery before activation") {
		t.Fatalf("controller refusal:\n%s", view)
	}
	opened := runRead(model.Update(keyPress("enter")))
	if opened.screen != dashboardPXE || !contains(plainView(opened), "Recover the controller network") || fixture.recovered != 0 {
		t.Fatalf("Enter opened screen %d:\n%s", opened.screen, plainView(opened))
	}
}

func TestPXELifecycleTextStartsWithThePlainOutcome(t *testing.T) {
	var output bytes.Buffer
	PXELifecycleText(&output, domain.PXELifecycleReport{Operation: "pxe-recover", State: "failed", Message: pxeAddressMissing})
	if first, _, _ := strings.Cut(output.String(), "\n"); first != "The controller network was not recovered. The normal controller address did not come back. Run nixorium controller plan and apply it, then nixorium doctor." {
		t.Fatalf("first line %q", first)
	}
	output.Reset()
	PXELifecycleText(&output, domain.PXELifecycleReport{Operation: "pxe-start", State: "completed"})
	if strings.Contains(output.String(), "back to normal") {
		t.Fatalf("start reported a recovery:\n%s", output.String())
	}
}

func TestPXELifecycleOutcomeSaysWhenTheRestoredAddressWasNotChecked(t *testing.T) {
	report := domain.PXELifecycleReport{Operation: "pxe-stop", State: "failed", Mode: "stopped",
		Message: "the restored controller address could not be verified: evaluate lab metadata: nix evaluation failed"}
	headline, next := pxeLifecycleOutcome(report, false)
	if !strings.Contains(headline, "could not be checked") || strings.Contains(headline, "was not stopped") ||
		!strings.Contains(next, "Check the laboratory again") || !strings.Contains(next, "Apply to controller") {
		t.Fatalf("outcome = %q / %q", headline, next)
	}
	var output bytes.Buffer
	PXELifecycleText(&output, report)
	if first, _, _ := strings.Cut(output.String(), "\n"); !strings.Contains(first, "could not be checked") || !strings.Contains(first, "nixorium doctor") {
		t.Fatalf("first line %q", first)
	}
}
