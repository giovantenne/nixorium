package presentation

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/giovantenne/nixorium/internal/domain"
)

func completedDeploymentUSB(t *testing.T) domain.RemoteInstallResponse {
	t.Helper()
	data, err := os.ReadFile("../../tests/usb-completed-session.json")
	if err != nil {
		t.Fatal(err)
	}
	session, err := domain.DecodeRemoteInstallSession(data)
	if err != nil {
		t.Fatal(err)
	}
	return domain.RemoteInstallResponse{OperationID: session.OperationID, State: session.State, Session: &session}
}

func deploymentUSBModel(t *testing.T, response domain.RemoteInstallResponse) dashboardModel {
	t.Helper()
	report := testDashboardReport("ready")
	report.Meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}}
	return dashboardModel{screen: dashboardDeploy, report: report, width: 80, height: 24,
		deployment: deploymentModel{chosen: map[string]bool{"pc01": true, "pc02": true}},
		actions: DashboardActions{
			LoadRemoteInstall: func() (domain.RemoteInstallResponse, error) { return response, nil },
			PlanDeployment: func(string) domain.DeploymentPlanReport {
				t.Fatal("unexpected deployment plan")
				return domain.DeploymentPlanReport{}
			},
			ApplyDeployment: func(domain.DeploymentPlanReport, func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
				t.Error("unexpected deployment apply")
				return domain.DeploymentExecutionReport{}
			},
			RemoteInstallRequest: func(domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
				t.Fatal("unexpected installation action")
				return domain.RemoteInstallResponse{}, nil
			},
		},
	}
}

func settleDeploymentUSB(t *testing.T, model dashboardModel, msg tea.Msg) dashboardModel {
	t.Helper()
	updated, command := model.Update(msg)
	model = updated.(dashboardModel)
	for i := 0; command != nil; i++ {
		if i > 8 {
			t.Fatal("recovery command loop")
		}
		updated, command = model.Update(command())
		model = updated.(dashboardModel)
	}
	return model
}

func TestDeploymentUSBRecoveryVerifiesThenRequiresFreshReview(t *testing.T) {
	response := completedDeploymentUSB(t)
	model := deploymentUSBModel(t, response)
	probes, verifies, plans := 0, 0, 0
	model.actions.LoadRemoteInstall = func() (domain.RemoteInstallResponse, error) {
		probes++
		if verifies == 0 {
			return response, nil
		}
		return domain.RemoteInstallResponse{State: "ready"}, nil
	}
	model.actions.RemoteInstallRequest = func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
		if request.Operation != domain.RemoteInstallVerifyOperation || request.OperationID != response.OperationID {
			t.Fatalf("unsafe recovery action: %+v", request)
		}
		verifies++
		verified := *response.Session
		verified.BootVerified = true
		return domain.RemoteInstallResponse{State: "verified", OperationID: response.OperationID, Session: &verified}, nil
	}
	model.actions.PlanDeployment = func(requested string) domain.DeploymentPlanReport {
		plans++
		if requested != "pc01,pc02" {
			t.Fatalf("selection changed to %q", requested)
		}
		return domain.DeploymentPlanReport{State: "ready", Requested: requested, Revision: strings.Repeat("b", 40), ColmenaSelector: requested}
	}
	model.deployment.confirmation = "DEPLOY"
	model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if probes != 1 || plans != 0 || verifies != 0 || !strings.Contains(model.View().Content, "Verify pc01 and resume") {
		t.Fatalf("missing guided recovery:\n%s", model.View().Content)
	}
	// New inventory entries must not silently expand the saved selection.
	model.report.Meta.Clients.Hosts = append(model.report.Meta.Clients.Hosts, domain.HostMeta{Name: "pc03"})
	model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if verifies != 1 || probes != 2 || plans != 1 || model.screen != dashboardDeployReview || model.deployment.confirmation != "" || model.deployment.plan.Revision != strings.Repeat("b", 40) {
		t.Fatalf("did not return to fresh review: %+v", model.deployment)
	}
	model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.deployment.applying {
		t.Fatal("recovery reused deployment authorization")
	}
}

func TestDeploymentUSBRecoveryFailureKeepsIdentityAndOffersRetry(t *testing.T) {
	for _, failure := range []string{"offline", "key-mismatch", "cleanup", "wrong-operation", "unconfirmed"} {
		t.Run(failure, func(t *testing.T) {
			response := completedDeploymentUSB(t)
			model := deploymentUSBModel(t, response)
			verifies := 0
			model.actions.RemoteInstallRequest = func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
				verifies++
				if request.Operation != domain.RemoteInstallVerifyOperation || request.OperationID != response.OperationID {
					t.Fatalf("unsafe retry: %+v", request)
				}
				switch failure {
				case "offline":
					return domain.RemoteInstallResponse{}, errors.New("No route to host")
				case "key-mismatch":
					return domain.RemoteInstallResponse{State: "reconciliation-required", Message: "host key mismatch\x1b[31m"}, nil
				case "cleanup":
					return domain.RemoteInstallResponse{State: "reconciliation-required", Message: "could not release reservation"}, nil
				case "wrong-operation":
					return domain.RemoteInstallResponse{State: "verified", OperationID: strings.Repeat("f", 32), Session: &domain.RemoteInstallSession{OperationID: strings.Repeat("f", 32), BootVerified: true}}, nil
				default:
					return domain.RemoteInstallResponse{State: "verified", OperationID: response.OperationID}, nil
				}
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
			if model.deployment.usbRecovery == nil || model.deployment.usbRecovery.response.OperationID != response.OperationID || !strings.Contains(model.View().Content, "Retry verification") || !strings.Contains(model.View().Content, "network cable") {
				t.Fatalf("missing retry guidance:\n%s", model.View().Content)
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Text: "d"})
			if strings.Contains(model.deployment.usbRecovery.detail, "\x1b") {
				t.Fatal("unsafe remote detail")
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyF1})
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
			if verifies != 1 {
				t.Fatal("help triggered verification")
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
			if verifies != 2 {
				t.Fatal("retry did not use the same verification")
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
			if model.screen != dashboardDeploy || model.deployment.usbRecovery != nil || !model.deployment.chosen["pc01"] || !model.deployment.chosen["pc02"] {
				t.Fatal("cancel lost selection")
			}
		})
	}
}

func TestDeploymentUSBRecoveryDoesNotBypassActiveOrIncompleteInstallation(t *testing.T) {
	for _, state := range []string{"active", "no-receipt", "no-reboot", "wrong-binding"} {
		t.Run(state, func(t *testing.T) {
			response := completedDeploymentUSB(t)
			switch state {
			case "active":
				response.Session.State = "running"
				response.State = "running"
			case "no-receipt":
				response.Session.Receipt = nil
			case "no-reboot":
				response.Session.RebootRequested = false
			case "wrong-binding":
				response.Session.Preparation.OperationID = strings.Repeat("f", 32)
			}
			model := deploymentUSBModel(t, response)
			statuses := 0
			model.installation.remote = remoteInstallationModel{operationID: response.OperationID, bootstrapError: "original connection error"}
			model.actions.RemoteInstallRequest = func(request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
				if request.Operation != domain.RemoteInstallStatusOperation || request.OperationID != response.OperationID {
					t.Fatalf("sent mutation/verification: %+v", request)
				}
				statuses++
				return response, nil
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
			if !strings.Contains(model.View().Content, "Open installation") || strings.Contains(model.View().Content, "and resume") {
				t.Fatalf("unsafe recovery option:\n%s", model.View().Content)
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
			if model.screen != dashboardUSBInstall || statuses != 1 || !model.installation.remote.returnToDeployment {
				t.Fatal("did not open existing installation")
			}
			if model.installation.remote.bootstrapError != "original connection error" {
				t.Fatal("opening recovery erased the upstream-preserved bootstrap error")
			}
			model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})
			if model.screen != dashboardDeploy || model.deployment.usbRecovery == nil {
				t.Fatal("return lost recovery context")
			}
		})
	}
}

func TestDeploymentUSBRecoveryHandlesLateReservationAndUnrelatedFailure(t *testing.T) {
	for _, reserved := range []bool{true, false} {
		response := domain.RemoteInstallResponse{State: "ready"}
		if reserved {
			response = completedDeploymentUSB(t)
		}
		model := deploymentUSBModel(t, response)
		report := domain.DeploymentExecutionReport{Operation: "deploy-apply", State: "failed", Phase: domain.DeploymentPhasePreflight, Targets: []domain.DeploymentTarget{{Name: "pc01"}}, Message: "operation could not start"}
		model = settleDeploymentUSB(t, model, dashboardDeploymentResultMsg{report: report})
		if reserved {
			if model.deployment.usbRecovery == nil || model.deployment.usbRecovery.requested != "pc01" {
				t.Fatal("late reservation missed")
			}
		} else if model.deployment.usbRecovery != nil || model.deployment.result.Message != report.Message || model.message != report.Message {
			t.Fatal("unrelated failure was hidden")
		}
	}
}

func TestDeploymentUSBRecoveryRechecksForNewReservation(t *testing.T) {
	original := completedDeploymentUSB(t)
	model := deploymentUSBModel(t, original)
	model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	next := completedDeploymentUSB(t)
	next.Session.OperationID = strings.Repeat("f", 32)
	next.OperationID = next.Session.OperationID
	model.actions.LoadRemoteInstall = func() (domain.RemoteInstallResponse, error) { return next, nil }
	model.actions.RemoteInstallRequest = func(domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
		verified := *original.Session
		verified.BootVerified = true
		return domain.RemoteInstallResponse{State: "verified", OperationID: original.OperationID, Session: &verified}, nil
	}
	model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.deployment.usbRecovery == nil || model.deployment.usbRecovery.response.OperationID != next.OperationID {
		t.Fatal("new reservation was bypassed")
	}
}

func TestDeploymentUSBRecoveryUnknownStatusNeverPlans(t *testing.T) {
	for _, state := range []string{"transport-error", "blocked", "future-state"} {
		model := deploymentUSBModel(t, domain.RemoteInstallResponse{State: state})
		if state == "transport-error" {
			model.actions.LoadRemoteInstall = func() (domain.RemoteInstallResponse, error) {
				return domain.RemoteInstallResponse{}, errors.New("worker unavailable")
			}
		}
		model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
		if !strings.Contains(model.View().Content, "Retry check") {
			t.Fatal("missing status recovery")
		}
		model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
		if model.deployment.usbRecovery == nil {
			t.Fatal("unknown status permitted deployment")
		}
	}
}

func TestDeploymentUSBRecoveryLayouts(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 30}, {160, 40}} {
		for _, dark := range []bool{false, true} {
			for _, failed := range []bool{false, true} {
				model := deploymentUSBModel(t, completedDeploymentUSB(t))
				model.width, model.height, model.isDark = size[0], size[1], dark
				model = settleDeploymentUSB(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
				model.deployment.usbRecovery.failed = failed
				view := model.View().Content
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("overflow %v:\n%s", size, view)
				}
				primary := "Verify pc01 and resume"
				if failed {
					primary = "Retry verification"
				}
				for _, profile := range []colorprofile.Profile{colorprofile.Ascii, colorprofile.ANSI, colorprofile.ANSI256} {
					var output bytes.Buffer
					writer := colorprofile.NewWriter(&output, []string{})
					writer.Profile = profile
					if _, err := writer.Write([]byte(view)); err != nil {
						t.Fatal(err)
					}
					rendered := output.String()
					for _, want := range []string{primary, "Selection", "pc01,pc02", "network cable"} {
						if !strings.Contains(rendered, want) {
							t.Fatalf("missing %q at %v:\n%s", want, size, rendered)
						}
					}
				}
				if size[0] == 80 && !dark && failed {
					t.Log(view)
				}
			}
		}
	}
}
