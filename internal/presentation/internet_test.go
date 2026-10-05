package presentation

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func TestInternetRenderGallery(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			for _, state := range []string{"selection", "empty", "review", "blocked", "busy", "partial"} {
				model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), DashboardActions{}, false)
				opened, _ := model.openComputerTask("i")
				model = opened.(dashboardModel)
				model.width, model.height, model.isDark = size[0], size[1], dark
				required := []string{"Internet access", "Help"}
				switch state {
				case "selection":
					required = append(required, "pc01", "Select", "Review", "Back")
				case "empty":
					model.report.Meta.Clients.Hosts = nil
					required = append(required, "No client", "Back")
				case "review", "blocked":
					model.internet.stage = 1
					model.internet.plan = domain.InternetPlan{State: "ready", Message: "Ready for review"}
					required = append(required, "Cancel")
					if state == "blocked" {
						model.internet.plan.State = "blocked"
						model.internet.plan.Message = "No selected client supports authenticated Internet control."
						required = append(required, "No selected client")
					} else {
						required = append(required, "Apply")
					}
				case "busy":
					model.busy = "Applying and verifying Internet access"
					required = append(required, "Applying")
				case "partial":
					model.internet.stage = 2
					model.internet.result = domain.InternetReport{State: "partial", Message: "Verified on 1 of 2 selected clients.", Targets: []domain.InternetOutcome{{Name: "pc02", State: "unconfirmed", Detail: "Refresh and review before retrying."}}}
					required = append(required, "sent, not confirmed", "New review", "Computers")
				}
				view := model.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("%s overflow %dx%d:\n%s", state, size[0], size[1], view)
				}
				for _, profile := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.ANSI, colorprofile.ANSI256} {
					var output bytes.Buffer
					writer := colorprofile.Writer{Forward: &output, Profile: profile}
					if _, err := writer.Write([]byte(view)); err != nil {
						t.Fatal(err)
					}
					for _, text := range required {
						if !strings.Contains(output.String(), text) {
							t.Fatalf("%s missing %q:\n%s", state, text, output.String())
						}
					}
				}
			}
		}
	}
}

func TestInternetReviewCancelAndApply(t *testing.T) {
	applied := 0
	actions := DashboardActions{
		PlanInternet: func(ctx context.Context, requested string, a domain.InternetAction) domain.InternetPlan {
			if requested != "pc01" || a != domain.InternetUnblock {
				t.Fatalf("review %s %s", requested, a)
			}
			return domain.InternetPlan{State: "ready", Action: a, ReviewToken: "test", Targets: []domain.InternetTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true, Observed: domain.InternetObservation{State: "blocked"}}}}
		},
		ApplyInternet: func(p domain.InternetPlan) domain.InternetReport {
			applied++
			return domain.InternetReport{State: "completed", Action: p.Action, Message: "Verified"}
		},
	}
	model := newDashboardModel(testDashboardReport("ready"), testSetupReport(true, true, true, true), actions, false)
	opened, _ := model.openComputerTask("i")
	model = opened.(dashboardModel)
	key := func(code rune) {
		updated, cmd := model.updateInternet(tea.KeyPressMsg{Code: code})
		model = updated.(dashboardModel)
		if cmd != nil {
			updated, _ = model.Update(cmd())
			model = updated.(dashboardModel)
		}
	}
	key(' ')
	key(tea.KeyTab)
	key(tea.KeyEnter)
	if model.internet.stage != 1 || applied != 0 {
		t.Fatal("did not stop for review")
	}
	key(tea.KeyEscape)
	if applied != 0 || model.internet.stage != 0 {
		t.Fatal("cancel applied change")
	}
	key(tea.KeyEnter)
	key(tea.KeyEnter)
	if applied != 1 || model.internet.stage != 2 {
		t.Fatal("review not applied")
	}
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		model.width = size[0]
		model.height = size[1]
		view := model.internetView()
		if !strings.Contains(view, "Internet") || !strings.Contains(view, "Verified") {
			t.Fatal(view)
		}
	}
}

func TestClassroomViewEntryAppearsOnlyWhenAvailable(t *testing.T) {
	model := dashboardModel{report: testDashboardReport("ready"), width: 120, height: 30, screen: dashboardComputersArea}
	for _, task := range model.availableComputerTasks() {
		if task.id == "view" {
			t.Fatal("classroom view listed without its service")
		}
	}
	opened := 0
	model.actions.ClassroomMode = true
	model.actions.OpenClassroomView = func(context.Context) (string, error) {
		opened++
		return "The classroom view opened in a browser window.", nil
	}
	tasks := model.availableComputerTasks()
	if tasks[0].id != "view" || !strings.Contains(model.View().Content, "Classroom view") {
		t.Fatalf("tasks = %+v\n%s", tasks, model.View().Content)
	}
	next, command := model.updatePrimaryScreenKey(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if command == nil {
		t.Fatal("classroom view did not start")
	}
	next, _ = next.(dashboardModel).Update(command())
	if opened != 1 || !strings.Contains(next.(dashboardModel).View().Content, "opened in a browser window") {
		t.Fatalf("opened=%d\n%s", opened, next.(dashboardModel).View().Content)
	}
}
