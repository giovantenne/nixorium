package presentation

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestDashboardBrowsesBoundedOperationLogTail(t *testing.T) {
	id := "deploy-20260914T113000.000000000Z-11.log"
	loaded := ""
	actions := DashboardActions{
		LoadLogs: func(ctx context.Context) domain.OperationLogsReport {
			return domain.OperationLogsReport{Operation: "logs-list", State: "available", Records: []domain.OperationRecord{
				{RecordedAt: time.Date(2026, 9, 14, 11, 31, 0, 0, time.UTC), Operation: "pxe-start", State: "completed", Subject: "active", Summary: "PXE lifecycle transition finished"},
			}, Logs: []domain.OperationLogEntry{
				{ID: id, Kind: "deployment", StartedAt: time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC), SizeBytes: 70000, State: "partial", Available: true},
			}}
		},
		LoadLog: func(ctx context.Context, selected string) domain.OperationLogReport {
			loaded = selected
			lines := make([]string, 20)
			for index := range lines {
				lines[index] = fmt.Sprintf("line-%02d", index+1)
			}
			entry := domain.OperationLogEntry{ID: id, Kind: "deployment", StartedAt: time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC), SizeBytes: 70000, State: "partial", Available: true}
			return domain.OperationLogReport{Operation: "logs-show", State: "available", Log: &entry, Content: strings.Join(lines, "\n") + "\n", Truncated: true}
		},
	}
	model := dashboardModel{report: testDashboardReport("ready"), actions: actions}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 16})
	model = updated.(dashboardModel)
	updated, _ = model.Update(tea.KeyPressMsg{Text: "m"})
	model = updated.(dashboardModel)
	updated, command := model.Update(tea.KeyPressMsg{Text: "l"})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if model.screen != dashboardLogs || !strings.Contains(model.View().Content, "Recent actions") || !strings.Contains(model.View().Content, "Start network installation") || !strings.Contains(model.View().Content, id) || !strings.Contains(model.View().Content, "partial") {
		t.Fatalf("operation log list missing:\n%s", model.View().Content)
	}
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(dashboardModel)
	updated, _ = model.Update(command())
	model = updated.(dashboardModel)
	if loaded != id || model.screen != dashboardLogDetail || !strings.Contains(model.View().Content, "earlier bytes omitted") || !strings.Contains(model.View().Content, "line-20") || strings.Contains(model.View().Content, "line-01") {
		t.Fatalf("bounded tail detail missing: loaded=%q\n%s", loaded, model.View().Content)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	model = updated.(dashboardModel)
	if !strings.Contains(model.View().Content, "line-01") {
		t.Fatalf("log detail did not scroll to the beginning:\n%s", model.View().Content)
	}
}

func TestOperationHistoryShowsWhyAnOperationFailed(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), screen: dashboardLogs, width: 120, height: 40}
	model.maintenance.logs = domain.OperationLogsReport{Records: []domain.OperationRecord{
		{RecordedAt: time.Date(2026, 10, 20, 11, 58, 0, 0, time.UTC), Operation: "controller-apply", State: "failed", Subject: "pc99", Summary: "the controller cannot reach the Internet: it could not find cache.nixos.org"},
		{RecordedAt: time.Date(2026, 10, 20, 10, 3, 0, 0, time.UTC), Operation: "pxe-stop", State: "completed", Subject: "stopped", Summary: "network installation stopped"},
	}}
	view := model.View().Content
	for _, want := range []string{"Apply to controller", "could not find cache.nixos.org", "Finish network installation"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "network installation stopped") {
		t.Fatalf("a completed operation's summary is shown:\n%s", view)
	}
}
