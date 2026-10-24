package presentation

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// usbRecoveryResponse is an unfinished installation of pc01 in one state.
func usbRecoveryResponse(t *testing.T, kind string) domain.RemoteInstallResponse {
	t.Helper()
	switch kind {
	case "prepared":
		response := remoteTUITestPreparedResponse("prepared")
		response.Message = domain.RemoteStatusLoaded
		response.Session.Bootstrap = &domain.RemoteInstallBootstrapRecord{Host: "pc01", Address: "192.0.2.20"}
		return response
	case "lost":
		response := usbRecoveryResponse(t, "prepared")
		response.State, response.Session.State = "reconciliation-required", "reconciliation-required"
		return response
	case "answering":
		response := remoteTUITestPreparedResponse("failed")
		response.Session.TokenConsumed = true
		response.Session.Receipt = &domain.RemoteInstallReceipt{SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: remoteTUITestOperationID,
			State: "failed", Phase: domain.RemoteInstallPhasePartition, MutationStarted: true, DiskMayBeModified: true}
		response.Message = domain.RemoteStatusLoaded + "; " + domain.RemoteStatusAnswered
		response.Session.Bootstrap = &domain.RemoteInstallBootstrapRecord{Host: "pc01", Address: "192.0.2.20"}
		return response
	case "silent":
		response := usbRecoveryResponse(t, "answering")
		response.State, response.Session.State = "reconciliation-required", "reconciliation-required"
		response.Session.DispatchUncertain = true
		response.Message = domain.RemoteStatusLoaded + "; " + domain.RemoteStatusRefreshFailed + ": dial tcp 192.0.2.20:22: connect: no route to host"
		return response
	case "installed":
		response := completedDeploymentUSB(t)
		response.Session.DispatchUncertain, response.Session.RebootRequested = false, false
		response.State, response.Session.State = "ready-to-reboot", "ready-to-reboot"
		response.Message = domain.RemoteStatusLoaded + "; " + domain.RemoteStatusAnswered
		return response
	}
	t.Fatalf("unknown fixture %s", kind)
	return domain.RemoteInstallResponse{}
}

func usbRecoveryModel(response domain.RemoteInstallResponse, requests *[]domain.RemoteInstallRequest) dashboardModel {
	return dashboardModel{width: 160, height: 45, screen: dashboardUSBInstall, report: testDashboardReport("ready"),
		installation: installationModel{remote: remoteInstallationModel{stage: remoteInstallResult, reopened: true, host: "pc01",
			operationID: response.OperationID, response: response}},
		actions: DashboardActions{RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			*requests = append(*requests, request)
			return domain.RemoteInstallResponse{State: "closed-before-apply", OperationID: request.OperationID}, nil
		}}}
}

func TestUSBRecoveryStatesInPlainWords(t *testing.T) {
	for _, test := range []struct {
		kind, computer, disk string
		offers, refuses      []string
	}{
		{"prepared", "The computer was last seen booted from the USB stick.", "The disk was not touched.",
			[]string{"x Cancel safely", "c Close this record"}, []string{"a Resume", "v Verify"}},
		{"lost", "Nixorium is no longer connected to the computer started from the USB stick.", "The disk was not touched.",
			[]string{"a Resume the installation", "c Close this record"}, []string{"x Cancel", "n Check", "v Verify"}},
		// A possibly written disk is never cancelled, but the record can
		// always be closed (CLOSE), so a broken computer does not hold the
		// controller.
		{"answering", "The computer is still booted from the USB stick and answers.", "The disk may have been partly written.",
			[]string{"c Close this record"}, []string{"x Cancel"}},
		{"silent", "The computer does not answer.", "The disk may have been partly written.",
			[]string{"n Check how far it got", "a Resume the installation", "v Verify installed system", "c Close this record"}, []string{"x Cancel"}},
		{"installed", "The computer is still booted from the USB stick and answers.", "The system is installed on the disk.",
			[]string{"b Restart the computer", "c Close this record"}, []string{"x Cancel"}},
	} {
		t.Run(test.kind, func(t *testing.T) {
			response := usbRecoveryResponse(t, test.kind)
			view := plainView(usbRecoveryModel(response, &[]domain.RemoteInstallRequest{}))
			for _, want := range append([]string{"Resume or close the USB installation", "What Nixorium found", test.computer, test.disk, "What you can do"}, test.offers...) {
				if !contains(view, want) {
					t.Fatalf("review omits %q:\n%s", want, view)
				}
			}
			for _, refused := range test.refuses {
				if contains(view, refused) {
					t.Fatalf("review offers %q:\n%s", refused, view)
				}
			}
			// The worker's own text is a detail, never the headline.
			if response.Message != "" && contains(view, response.Message) {
				t.Fatalf("raw text shown by default:\n%s", view)
			}
		})
	}
}

func TestUSBRecoveryClosesAnUntouchedRecordWithEnter(t *testing.T) {
	requests := []domain.RemoteInstallRequest{}
	model := usbRecoveryModel(usbRecoveryResponse(t, "lost"), &requests)
	model = cleanupPress(t, model, keyPress("c"))
	view := plainView(model)
	if model.installation.remote.stage != remoteInstallConfirmClose || !contains(view, "Close this record?") || !contains(view, "The disk was not touched") ||
		contains(view, "Type exactly") || !contains(view, "Enter Close this record") {
		t.Fatalf("close confirmation:\n%s", view)
	}
	if back := cleanupPress(t, model, tea.KeyPressMsg{Code: tea.KeyEsc}); back.installation.remote.stage != remoteInstallResult || len(requests) != 0 {
		t.Fatal("Esc did not go back without a request")
	}
	model = runRead(model.Update(keyPress("enter")))
	if len(requests) != 1 || requests[0].Operation != domain.RemoteInstallCloseOperation || requests[0].OperationID != remoteTUITestOperationID {
		t.Fatalf("requests %+v", requests)
	}
	if view := plainView(model); !contains(view, "The record was closed; the disk was not touched.") {
		t.Fatalf("close result:\n%s", view)
	}
}

func TestUSBRecoveryClosingAfterTheDiskWasWrittenNeedsCLOSE(t *testing.T) {
	requests := []domain.RemoteInstallRequest{}
	model := usbRecoveryModel(usbRecoveryResponse(t, "installed"), &requests)
	model = cleanupPress(t, model, keyPress("c"))
	view := plainView(model)
	if !contains(view, "CLOSE stops following this installation: the system stays on the disk, but Nixorium does not check that the computer starts correctly.") ||
		!contains(view, "Type exactly CLOSE") || !model.textEntry() {
		t.Fatalf("close after the disk was written:\n%s", view)
	}
	model = cleanupPress(t, model, keyPress("enter"))
	for _, letter := range "CLOS" {
		model = cleanupPress(t, model, keyPress(string(letter)))
	}
	model = cleanupPress(t, model, keyPress("enter"))
	if len(requests) != 0 || !strings.Contains(model.message, "did not match") {
		t.Fatalf("closed without CLOSE: %+v", requests)
	}
	for _, letter := range "CLOSE" {
		model = cleanupPress(t, model, keyPress(string(letter)))
	}
	_ = runRead(model.Update(keyPress("enter")))
	if len(requests) != 1 || requests[0].Operation != domain.RemoteInstallCloseOperation {
		t.Fatalf("CLOSE did not close: %+v", requests)
	}
}

func TestUSBRecoveryDetailsShowTheWorkerText(t *testing.T) {
	response := usbRecoveryResponse(t, "silent")
	model := cleanupPress(t, usbRecoveryModel(response, &[]domain.RemoteInstallRequest{}), keyPress("d"))
	if view := plainView(model); !contains(view, "Message: "+response.Message) || !contains(view, "d Hide details") {
		t.Fatalf("details:\n%s", view)
	}
}

func TestUSBResultOffersTheNetworkRecovery(t *testing.T) {
	response := domain.RemoteInstallResponse{State: "blocked", OperationID: remoteTUITestOperationID,
		Message: "controller network or installation state requires recovery before a USB installation"}
	model := usbRecoveryModel(response, &[]domain.RemoteInstallRequest{})
	model.installation.remote.reopened = false
	refreshed := 0
	model.actions.Refresh = func(context.Context) (domain.StatusReport, error) {
		refreshed++
		return testDashboardReport("recovery-required"), nil
	}
	view := plainView(model)
	for _, want := range []string{"The USB installation cannot continue: network installation is still active.", "Next: Finish the network installation, or recover the controller network (Enter)",
		"Enter Finish the network installation, or recover the controller network"} {
		if !contains(view, want) {
			t.Fatalf("USB result omits %q:\n%s", want, view)
		}
	}
	opened := runRead(model.Update(keyPress("enter")))
	if opened.screen != dashboardPXE || refreshed != 1 {
		t.Fatalf("Enter opened screen %d (refreshed %d)", opened.screen, refreshed)
	}
}

func TestDeploymentUSBRecoverySaysWhatWasFound(t *testing.T) {
	response := usbRecoveryResponse(t, "silent")
	response.Session.Preparation.Host.Name = "pc01"
	model := deploymentUSBModel(t, response)
	model.width, model.height = 120, 40
	model = settleDeploymentUSB(t, model, keyPress("enter"))
	view := plainView(model)
	for _, want := range []string{"! The computers were not updated: a USB installation was not finished.",
		"The computer may be half installed; Nixorium keeps it reserved until you resume or close that installation.",
		"What Nixorium found", "pc01 has an unfinished USB installation.", "The computer does not answer.", "The disk may have been partly written.",
		"Enter Resume or close the USB installation"} {
		if !contains(view, want) {
			t.Fatalf("deployment check omits %q:\n%s", want, view)
		}
	}
	if contains(view, "no route to host") {
		t.Fatalf("raw text shown by default:\n%s", view)
	}
}

func TestUSBStatusTextStartsWithTheFindings(t *testing.T) {
	var output bytes.Buffer
	RemoteInstallResponseText(&output, usbRecoveryResponse(t, "lost"))
	text := output.String()
	for _, want := range []string{
		"What Nixorium found: Nixorium is no longer connected to the computer started from the USB stick. The disk was not touched.",
		"  Resume the installation: nixorium install usb start --host pc01",
		"  Close this record: nixorium install usb close --id " + remoteTUITestOperationID,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text omits %q:\n%s", want, text)
		}
	}
	if !strings.HasPrefix(text, "What Nixorium found") {
		t.Fatalf("status text does not start with the findings:\n%s", text)
	}
	if RemoteInstallCloseNeedsWord(usbRecoveryResponse(t, "lost")) || !RemoteInstallCloseNeedsWord(usbRecoveryResponse(t, "installed")) {
		t.Fatal("CLOSE is typed only after a possible disk change")
	}
}

func TestUSBRecordAfterARestartWithoutAnswerCanBeClosed(t *testing.T) {
	response := usbRecoveryResponse(t, "installed")
	response.State, response.Session.State = "reboot-requested", "reboot-requested"
	response.Session.RebootRequested = true
	view := plainView(usbRecoveryModel(response, &[]domain.RemoteInstallRequest{}))
	for _, want := range []string{"v Verify now", "c Close this record"} {
		if !contains(view, want) {
			t.Fatalf("a computer that did not come back cannot be closed; missing %q:\n%s", want, view)
		}
	}
	if !remoteInstallCloseNeedsWord(response) {
		t.Fatal("closing after the disk was written must ask for CLOSE")
	}
}
