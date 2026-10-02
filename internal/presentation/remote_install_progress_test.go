package presentation

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestUSBInstallRefreshesItselfWithStatusOnlyWhileRunning(t *testing.T) {
	var requests []domain.RemoteInstallRequest
	states := []string{"running", "running", "ready-to-reboot"}
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{remote: remoteInstallationModel{
			stage: remoteInstallApplying, host: "pc01", operationID: remoteTUITestOperationID,
		}},
		actions: DashboardActions{RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			requests = append(requests, request)
			state := states[min(len(requests), len(states)-1)]
			return domain.RemoteInstallResponse{State: state, OperationID: remoteTUITestOperationID, Session: &domain.RemoteInstallSession{
				OperationID: remoteTUITestOperationID, State: state,
				Receipt: &domain.RemoteInstallReceipt{Phase: domain.RemoteInstallPhaseInstall, DiskMayBeModified: true},
			}}, nil
		}},
	}
	next, cmd := model.Update(dashboardRemoteInstallMsg{action: "apply", response: domain.RemoteInstallResponse{State: "running", OperationID: remoteTUITestOperationID, Execution: &domain.RemoteInstallExecutionReport{Phase: domain.RemoteInstallPhasePartition}}})
	model = next.(dashboardModel)
	view := model.View().Content
	if !strings.Contains(view, "Installing pc01: erasing and partitioning the disk") || !strings.Contains(view, "refreshes by itself") || strings.Contains(view, "Disk may be changed") {
		t.Fatalf("running result is not plain:\n%s", view)
	}
	if cmd == nil || model.installation.remote.watchStarted.IsZero() {
		t.Fatal("no automatic refresh was scheduled while the job runs")
	}
	for round := 0; round < 3; round++ {
		tick := remoteInstallTickMsg{operationID: remoteTUITestOperationID, watch: model.installation.remote.watch}
		next, cmd = model.Update(tick)
		model = next.(dashboardModel)
		if cmd == nil {
			break
		}
		next, cmd = model.Update(cmd())
		model = next.(dashboardModel)
	}
	for _, request := range requests {
		if request.Operation != domain.RemoteInstallStatusOperation {
			t.Fatalf("automatic refresh issued %s", request.Operation)
		}
	}
	if model.installation.remote.response.State != "ready-to-reboot" || !strings.Contains(model.View().Content, "press b to restart") {
		t.Fatalf("finished job not reported plainly:\n%s", model.View().Content)
	}
	// A finished job schedules nothing further, and stale ticks are ignored.
	if cmd != nil {
		t.Fatal("refresh continued after the job finished")
	}
	before := len(requests)
	next, cmd = model.Update(remoteInstallTickMsg{operationID: remoteTUITestOperationID, watch: model.installation.remote.watch})
	if cmd != nil || len(requests) != before {
		t.Fatal("a tick after completion issued a request")
	}
	model = next.(dashboardModel)
	next, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if view := next.(dashboardModel).View().Content; !strings.Contains(view, "Technical details") || !strings.Contains(view, "Operation ID:") {
		t.Fatalf("details toggle did not show raw fields:\n%s", view)
	}
}

func TestUSBInstallVerifiesAutomaticallyAfterReboot(t *testing.T) {
	var requests []domain.RemoteInstallRequest
	attempts := 0
	rebooted := domain.RemoteInstallResponse{
		State: "reboot-requested", OperationID: remoteTUITestOperationID,
		Session: &domain.RemoteInstallSession{
			OperationID: remoteTUITestOperationID, State: "reboot-requested", RebootRequested: true,
			Preparation: &domain.RemoteInstallPreparation{OperationID: remoteTUITestOperationID},
		},
	}
	model := dashboardModel{
		screen: dashboardUSBInstall,
		installation: installationModel{remote: remoteInstallationModel{
			stage: remoteInstallConfirmReboot, host: "pc01", operationID: remoteTUITestOperationID,
		}},
		actions: DashboardActions{RemoteInstallRequest: func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
			requests = append(requests, request)
			if request.Operation == domain.RemoteInstallRebootOperation {
				return rebooted, nil
			}
			attempts++
			if attempts == 1 {
				waiting := rebooted
				waiting.Message = "installed SSH is not reachable yet"
				return waiting, nil
			}
			return domain.RemoteInstallResponse{State: "verified", OperationID: remoteTUITestOperationID}, nil
		}},
	}
	for _, character := range "REBOOT" {
		next, _ := model.Update(tea.KeyPressMsg{Text: string(character)})
		model = next.(dashboardModel)
	}
	next, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = next.(dashboardModel)
	if command == nil {
		t.Fatal("confirmed reboot did not dispatch")
	}
	next, command = model.Update(command())
	model = next.(dashboardModel)
	if command == nil || !strings.Contains(model.View().Content, "checks the installed identity automatically") {
		t.Fatalf("reboot result did not start automatic verification:\n%s", model.View().Content)
	}
	for attempts < 2 {
		tick := remoteInstallTickMsg{operationID: remoteTUITestOperationID, watch: model.installation.remote.watch}
		next, command = model.Update(tick)
		model = next.(dashboardModel)
		if command == nil {
			t.Fatal("automatic verification tick did not issue a request")
		}
		next, command = model.Update(command())
		model = next.(dashboardModel)
	}
	for index, request := range requests {
		if index == 0 && request.Operation != domain.RemoteInstallRebootOperation {
			t.Fatalf("first request was %s, want reboot", request.Operation)
		}
		if index > 0 && request.Operation != domain.RemoteInstallVerifyOperation {
			t.Fatalf("automatic post-reboot request was %s", request.Operation)
		}
	}
	if model.installation.remote.response.State != "verified" || command != nil {
		t.Fatalf("automatic verification did not finish cleanly: state=%s command=%v", model.installation.remote.response.State, command != nil)
	}
}

func TestUSBInstallFailureStatesDiskEffectPlainly(t *testing.T) {
	model := dashboardModel{screen: dashboardUSBInstall, installation: installationModel{remote: remoteInstallationModel{
		stage: remoteInstallResult, host: "pc03", operationID: remoteTUITestOperationID,
		response: domain.RemoteInstallResponse{State: "failed", OperationID: remoteTUITestOperationID, Message: "live medium lost"},
	}}}
	if view := model.View().Content; !strings.Contains(view, "did not complete. No disk was changed.") {
		t.Fatalf("failure without disk change:\n%s", view)
	}
	model.installation.remote.response.Execution = &domain.RemoteInstallExecutionReport{DiskMayBeModified: true}
	if view := model.View().Content; !strings.Contains(view, "The disk may have been changed.") {
		t.Fatalf("failure after disk change:\n%s", view)
	}
	model.installation.remote.response = domain.RemoteInstallResponse{State: "reconciliation-required", OperationID: remoteTUITestOperationID}
	if view := model.View().Content; !strings.Contains(view, "Do not start another installation") {
		t.Fatalf("uncertain result:\n%s", view)
	}
}

func TestUSBInstallChecklistWaitsAndSizesAndIdentitiesAreReadable(t *testing.T) {
	model := dashboardModel{screen: dashboardUSBInstall, height: 24, width: 100, busy: "Working", installation: installationModel{remote: remoteInstallationModel{stage: remoteInstallFingerprint}}}
	view := model.View().Content
	if !strings.Contains(view, "Prepare the system for this computer") || !strings.Contains(view, "Read the PC's fingerprint · in progress") || !strings.Contains(view, "Start the installation · waiting") || strings.Contains(view, "! Start") {
		t.Fatalf("checklist shows pending steps as warnings:\n%s", view)
	}
	for size, want := range map[uint64]string{512: "512 B", 512110190592: "512.1 GB", 2000398934016: "2.0 TB"} {
		if got := humanBytes(size); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", size, got, want)
		}
	}
	hosts := []domain.HostMeta{}
	for index := 1; index <= 30; index++ {
		hosts = append(hosts, domain.HostMeta{Name: fmt.Sprintf("pc%02d", index), IP: fmt.Sprintf("10.0.0.%d", index)})
	}
	model = dashboardModel{screen: dashboardUSBInstall, height: 24, width: 100, installation: installationModel{remote: remoteInstallationModel{stage: remoteInstallSelectHost, hostCursor: 27}}}
	model.report.Meta.Clients.Hosts = hosts
	view = model.View().Content
	if !strings.Contains(view, "pc28") || strings.Contains(view, "pc01 ") || !strings.Contains(view, "of 30 computers") {
		t.Fatalf("identity list is not windowed around the selection:\n%s", view)
	}
}
