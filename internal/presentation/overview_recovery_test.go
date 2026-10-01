package presentation

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestOverviewShowsUnfinishedUSBInstallation(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), usbReserved: true, width: 120, height: 30}
	found := false
	for _, task := range model.pendingTasks() {
		found = found || (task.id == "pending-usb" && strings.Contains(task.title, "USB installation is unfinished"))
	}
	if !found {
		t.Fatalf("pending tasks = %+v", model.pendingTasks())
	}
	model.actions.ClassroomMode = true
	model.actions.RemoteReservationPresent = func() bool { return true }
	if model.usbReservationPresent() {
		t.Fatal("classroom mode looked for administrator USB reservations")
	}
}

func TestLowDiskOpensFreeDiskSpace(t *testing.T) {
	free := uint64(1 << 30)
	model := dashboardModel{report: testDashboardReport("ready"), width: 120, height: 30}
	model.report.StoreSpaceLow, model.report.StoreFreeBytes = true, &free
	model.report.Meta.Controller.Name = "pc99"
	updated, _ := model.openPendingTask("pending-disk")
	if updated.(dashboardModel).screen != dashboardCleanup {
		t.Fatalf("low disk opened screen %d", updated.(dashboardModel).screen)
	}
}

func TestUnconfirmedResultsOfferAComputerCheck(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), screen: dashboardShutdownResult, shutdown: newShutdownModel(), width: 120, height: 30}
	model.report.Meta.Controller.Name = "pc99"
	model.shutdown.result = domain.ShutdownApplyReport{State: "partial", Unconfirmed: 1, Targets: []domain.ShutdownTargetOutcome{{Name: "pc01", State: "unconfirmed"}}}
	if !strings.Contains(model.View().Content, "Check computers") {
		t.Fatalf("result view:\n%s", model.View().Content)
	}
	model.internet = internetModel{stage: 2, chosen: map[string]bool{}, result: domain.InternetReport{Targets: []domain.InternetOutcome{{Name: "pc01", State: "unconfirmed"}}}}
	model.screen = dashboardInternet
	if !strings.Contains(model.View().Content, "Check computers") {
		t.Fatalf("internet result view:\n%s", model.View().Content)
	}
	model.cleanup = cleanupModel{stage: 2, result: domain.CleanupApplyReport{State: "partial", Unconfirmed: 1}}
	model.screen = dashboardCleanup
	if !strings.Contains(model.View().Content, "Check computers") {
		t.Fatalf("cleanup result view:\n%s", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if screen := updated.(dashboardModel).screen; screen != dashboardHosts {
		t.Fatalf("h opened screen %d", screen)
	}
}
