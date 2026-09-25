package presentation

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

const remoteTUITestOperationID = "0123456789abcdef0123456789abcdef"

func remoteTUITestPreparation() domain.RemoteInstallPreparation {
	return domain.RemoteInstallPreparation{
		OperationID: remoteTUITestOperationID, DeploymentRevision: strings.Repeat("a", 40),
		SystemPath: "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-pc01-test",
		BundlePath: "/nix/store/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb-remote-installer",
		Host:       domain.RemoteInstallHost{Name: "pc01", Interface: "enp1s0", LiveIP: "192.0.2.20", StaticIP: "10.42.0.11"},
		Cache:      domain.RemoteInstallCache{URL: "http://10.42.0.99:5000"}, HostFingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Facts: domain.RemoteMachineFacts{Disks: []domain.RemoteDisk{
			{Path: "/dev/sda", SizeBytes: 16 << 30, Model: "Live USB", Serial: "USB", ExclusionReasons: []string{"boot-media"}},
			{Path: "/dev/nvme0n1", SizeBytes: 64 << 30, Model: "Target", Serial: "NVME-1", WWN: "wwn-1", Eligible: true},
		}},
	}
}

func remoteTUITestPreparedResponse(state string) domain.RemoteInstallResponse {
	preparation := remoteTUITestPreparation()
	return domain.RemoteInstallResponse{
		State: state, OperationID: remoteTUITestOperationID,
		Session: &domain.RemoteInstallSession{OperationID: remoteTUITestOperationID, State: state, Preparation: &preparation},
	}
}

func remoteTUITestPlan() domain.RemoteInstallPlanReport {
	preparation := remoteTUITestPreparation()
	return domain.RemoteInstallPlanReport{
		State: "ready", OperationID: remoteTUITestOperationID, Host: preparation.Host, Disk: preparation.Facts.Disks[1],
		Revision: preparation.DeploymentRevision, SystemPath: preparation.SystemPath, BundlePath: preparation.BundlePath,
		CacheURL: preparation.Cache.URL, ReviewToken: "sha256:" + strings.Repeat("b", 64),
		Confirmation: "ERASE /dev/nvme0n1 FOR pc01",
	}
}

func TestUSBInstallUsesCommonReadinessWithoutPreparingPXE(t *testing.T) {
	pxePreparations := 0
	model := dashboardModel{
		report:       testDashboardReport("stopped"),
		installation: installationModel{flow: true, method: domain.RemoteInstallUSBSSH},
		actions: DashboardActions{PreparePXE: func() domain.ActionReport {
			pxePreparations++
			return domain.ActionReport{}
		}},
	}
	updated, command := model.continueComputerInstallation(domain.SetupReport{State: "action-required", CurrentStage: domain.SetupStageArtifacts})
	model = updated.(dashboardModel)
	if command != nil || model.screen != dashboardUSBInstall || model.installation.remote.stage != remoteInstallSelectHost || pxePreparations != 0 {
		t.Fatalf("USB readiness entered PXE preparation: screen=%d stage=%d pxe=%d", model.screen, model.installation.remote.stage, pxePreparations)
	}
}

func TestInstallMethodDetectsAndReattachesKnownUSBOperation(t *testing.T) {
	prepared := remoteTUITestPreparedResponse("prepared")
	statusRequests := 0
	model := dashboardModel{actions: DashboardActions{
		LoadRemoteInstall: func() (domain.RemoteInstallResponse, error) { return prepared, nil },
		RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			if request.Operation == domain.RemoteInstallStatusOperation && request.OperationID == remoteTUITestOperationID {
				statusRequests++
			}
			return prepared, nil
		},
	}}
	updated, command := model.startComputerInstallation()
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardInstallMethod {
		t.Fatal("installation method did not probe the worker")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.installation.remote.operationID != remoteTUITestOperationID || !strings.Contains(model.View().Content, "Reattach USB operation") {
		t.Fatal("known USB operation was not offered for reattachment")
	}
	updated, command = model.Update(tea.KeyPressMsg{Text: "r"})
	model = updated.(dashboardModel)
	if command == nil || model.screen != dashboardUSBInstall {
		t.Fatal("reattach action did not request status")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if statusRequests != 1 || model.installation.remote.stage != remoteInstallResult {
		t.Fatalf("reattach status requests=%d stage=%d", statusRequests, model.installation.remote.stage)
	}
}

func TestUSBInstallPasswordIsMaskedAndClearedBeforeCallbackResult(t *testing.T) {
	received := ""
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{method: domain.RemoteInstallUSBSSH, remote: remoteInstallationModel{
			stage: remoteInstallPassword, host: "pc01", operationID: remoteTUITestOperationID,
			address: "192.0.2.20", fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password: "temporary-secret", formField: 2,
		}},
		actions: DashboardActions{BootstrapRemoteInstall: func(_, _, _ string, password []byte) (domain.RemoteInstallResponse, error) {
			received = string(password)
			return remoteTUITestPreparedResponse("bootstrapped-artifacts"), nil
		}, RemoteInstallRequest: func(domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			return remoteTUITestPreparedResponse("prepared"), nil
		}},
	}
	if view := model.View().Content; strings.Contains(view, "temporary-secret") || !strings.Contains(view, "••••") {
		t.Fatalf("password was not masked in the form:\n%s", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.installation.remote.password != "" || strings.Contains(model.View().Content, "temporary-secret") {
		t.Fatal("password remained in the model after bootstrap dispatch")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if received != "temporary-secret" || model.installation.remote.password != "" {
		t.Fatalf("bootstrap secret transfer=%q model password=%q", received, model.installation.remote.password)
	}
}

func TestUSBInstallPasswordConsumesGlobalShortcutCharacters(t *testing.T) {
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{method: domain.RemoteInstallUSBSSH, remote: remoteInstallationModel{
			stage: remoteInstallPassword, host: "pc01", operationID: remoteTUITestOperationID,
			address: "192.0.2.20", fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", formField: 2,
		}},
	}
	for _, character := range "q?" {
		updated, command := model.Update(tea.KeyPressMsg{Text: string(character)})
		model = updated.(dashboardModel)
		if command != nil {
			t.Fatalf("password character %q produced a command", character)
		}
	}
	if model.screen != dashboardUSBInstall || model.installation.remote.stage != remoteInstallPassword {
		t.Fatal("password shortcut characters left the password form")
	}
	if model.helpOpen || model.installation.remote.password != "q?" {
		t.Fatalf("help open=%t password=%q", model.helpOpen, model.installation.remote.password)
	}
}

func TestUSBInstallObservesFingerprintBeforePasswordEntry(t *testing.T) {
	observedAddress := ""
	bootstrapCalls := 0
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{method: domain.RemoteInstallUSBSSH, remote: remoteInstallationModel{
			stage: remoteInstallConsole, host: "pc01", operationID: remoteTUITestOperationID, address: "192.0.2.20",
		}},
		actions: DashboardActions{
			ObserveRemoteInstall: func(address string) (string, error) {
				observedAddress = address
				return "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", nil
			},
			BootstrapRemoteInstall: func(_, _, _ string, _ []byte) (domain.RemoteInstallResponse, error) {
				bootstrapCalls++
				return remoteTUITestPreparedResponse("bootstrapped-artifacts"), nil
			},
		},
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil || model.installation.remote.stage != remoteInstallFingerprint || model.installation.remote.password != "" {
		t.Fatal("address submission did not start credential-free host-key observation")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if observedAddress != "192.0.2.20" || model.installation.remote.fingerprint == "" || !strings.Contains(model.View().Content, "Type MATCH") || bootstrapCalls != 0 {
		t.Fatalf("observed=%q fingerprint=%q bootstrap=%d", observedAddress, model.installation.remote.fingerprint, bootstrapCalls)
	}
	for _, character := range "WRONG" {
		updated, _ = model.Update(tea.KeyPressMsg{Text: string(character)})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || model.installation.remote.stage != remoteInstallFingerprint || bootstrapCalls != 0 {
		t.Fatal("wrong physical-match confirmation reached password entry or bootstrap")
	}
	for _, character := range "MATCH" {
		updated, _ = model.Update(tea.KeyPressMsg{Text: string(character)})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || model.installation.remote.stage != remoteInstallPassword || bootstrapCalls != 0 {
		t.Fatal("fingerprint confirmation did not open password entry cleanly")
	}
}

func TestUSBInstallRequiresEligibleDiskAndExactReviewConfirmation(t *testing.T) {
	requests := []domain.RemoteInstallRequest{}
	prepared := remoteTUITestPreparedResponse("prepared")
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{method: domain.RemoteInstallUSBSSH, remote: remoteInstallationModel{
			stage: remoteInstallSelectDisk, host: "pc01", operationID: remoteTUITestOperationID, response: prepared,
		}},
		actions: DashboardActions{RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			requests = append(requests, request)
			plan := remoteTUITestPlan()
			if request.Operation == domain.RemoteInstallApplyOperation {
				return domain.RemoteInstallResponse{State: "ready-to-reboot", OperationID: remoteTUITestOperationID}, nil
			}
			return domain.RemoteInstallResponse{State: "review-ready", OperationID: remoteTUITestOperationID, Session: prepared.Session, Plan: &plan}, nil
		}},
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || len(requests) != 0 || !strings.Contains(model.message, "eligible") {
		t.Fatal("excluded boot media was accepted")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(dashboardModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("eligible disk did not create a plan request")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if len(requests) != 1 || requests[0].Disk != "/dev/nvme0n1" || model.installation.remote.stage != remoteInstallReview {
		t.Fatalf("disk review request=%+v stage=%d", requests, model.installation.remote.stage)
	}
	for _, character := range "WRONG" {
		updated, _ = model.Update(tea.KeyPressMsg{Text: string(character)})
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command != nil || len(requests) != 1 {
		t.Fatal("wrong destructive confirmation sent apply")
	}
	for _, character := range model.installation.remote.plan.Confirmation {
		key := tea.KeyPressMsg{Text: string(character)}
		if character == ' ' {
			key = tea.KeyPressMsg{Code: tea.KeySpace}
		}
		updated, _ = model.Update(key)
		model = updated.(dashboardModel)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("exact destructive confirmation did not dispatch apply")
	}
	_ = command()
	if len(requests) != 2 || requests[1].Operation != domain.RemoteInstallApplyOperation || requests[1].Confirmation != "ERASE /dev/nvme0n1 FOR pc01" {
		t.Fatalf("apply request=%+v", requests)
	}
}

func TestUSBInstallEscapeCancelsBeforeApplyAndClearsSecret(t *testing.T) {
	cancelled := false
	model := dashboardModel{
		screen:       dashboardUSBInstall,
		installation: installationModel{remote: remoteInstallationModel{stage: remoteInstallConsole, operationID: remoteTUITestOperationID, password: "secret"}},
		actions: DashboardActions{RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			cancelled = request.Operation == domain.RemoteInstallCancelOperation
			return domain.RemoteInstallResponse{State: "cancelled", OperationID: remoteTUITestOperationID}, nil
		}},
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(dashboardModel)
	if command == nil || model.installation.remote.password != "" {
		t.Fatal("escape did not clear the secret and request cancellation")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if !cancelled || model.installation.remote.response.State != "cancelled" {
		t.Fatal("pre-apply cancellation was not confirmed")
	}
}

func TestUSBInstallConfirmedFailureBeforeMutationOffersCancellation(t *testing.T) {
	response := remoteTUITestPreparedResponse("failed")
	response.Session.TokenConsumed = true
	response.Session.Receipt = &domain.RemoteInstallReceipt{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: remoteTUITestOperationID,
		State: "failed", Phase: domain.RemoteInstallPhasePreflight,
	}
	cancelled := false
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{remote: remoteInstallationModel{
			stage: remoteInstallResult, operationID: remoteTUITestOperationID, response: response,
		}},
		actions: DashboardActions{RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			cancelled = request.Operation == domain.RemoteInstallCancelOperation
			return domain.RemoteInstallResponse{State: "cancelled", OperationID: remoteTUITestOperationID}, nil
		}},
	}
	if !strings.Contains(model.View().Content, "Cancel safely") {
		t.Fatal("confirmed pre-mutation failure did not offer safe cancellation")
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model = updated.(dashboardModel)
	if command == nil {
		t.Fatal("safe cancellation did not dispatch a request")
	}
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if !cancelled || model.installation.remote.response.State != "cancelled" {
		t.Fatal("confirmed pre-mutation failure was not cancelled")
	}

	response.Session.Receipt.MutationStarted = true
	response.Session.Receipt.DiskMayBeModified = true
	if remoteInstallSafelyCancellable(response) {
		t.Fatal("post-mutation failure was exposed as safely cancellable")
	}
}

func TestUSBInstallRecoveryAccessReturnsToSameOperation(t *testing.T) {
	response := remoteTUITestPreparedResponse("reconciliation-required")
	response.Session.Bootstrap = &domain.RemoteInstallBootstrapRecord{Host: "pc01", Address: "192.0.2.20"}
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{remote: remoteInstallationModel{
			stage: remoteInstallResult, operationID: remoteTUITestOperationID, response: response,
		}},
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	model = updated.(dashboardModel)
	if command != nil || model.installation.remote.stage != remoteInstallConsole || !model.installation.remote.recovery || model.installation.remote.host != "pc01" {
		t.Fatalf("recovery console model=%+v command=%v", model.installation.remote, command)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(dashboardModel)
	if command != nil || model.installation.remote.stage != remoteInstallResult || model.installation.remote.recovery {
		t.Fatalf("recovery detach model=%+v command=%v", model.installation.remote, command)
	}
}

func TestUSBInstallReviewFitsSupportedLayoutsAndSanitizesControlText(t *testing.T) {
	plan := remoteTUITestPlan()
	response := remoteTUITestPreparedResponse("review-ready")
	response.Message = "remote\x1b[31m failure\x00 detail"
	response.Plan = &plan
	for _, size := range [][2]int{{80, 24}, {120, 30}, {180, 45}} {
		model := dashboardModel{
			width: size[0], height: size[1], isDark: size[0] != 120, screen: dashboardUSBInstall,
			installation: installationModel{remote: remoteInstallationModel{stage: remoteInstallReview, operationID: remoteTUITestOperationID, response: response, plan: plan}},
		}
		view := model.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("USB review overflows %dx%d", size[0], size[1])
		}
		if strings.Contains(view, "\x1b[31m") || strings.ContainsRune(view, '\x00') || !strings.Contains(view, "ERASE /dev/nvme0n1 FOR pc01") || !strings.Contains(view, "Cancel safely") {
			t.Fatalf("USB review lost safety meaning at %dx%d:\n%s", size[0], size[1], view)
		}
		for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
			var output bytes.Buffer
			writer := colorprofile.Writer{Forward: &output, Profile: profile}
			if _, err := writer.Write([]byte(view)); err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{"Logical identity", "/dev/nvme0n1", "Type exactly", "Cancel safely"} {
				if !strings.Contains(output.String(), expected) {
					t.Fatalf("USB review lost %q with profile %v", expected, profile)
				}
			}
		}
	}
	if sanitized := sanitizeRemoteInstallText("failure\x1b[31m\x00detail"); strings.ContainsRune(sanitized, '\x1b') || strings.ContainsRune(sanitized, '\x00') {
		t.Fatalf("remote control text was not sanitized: %q", sanitized)
	}
}
