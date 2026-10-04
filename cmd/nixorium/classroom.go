package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

func tryRunClassroomDashboard(ctx context.Context, stderr io.Writer) (bool, int) {
	if _, err := os.Stat(adapters.ClassroomSocketPath); err != nil {
		return false, 0
	}
	// The overview is requested from inside the dashboard, which shows its
	// loading screen at once instead of a blank terminal.
	actions := presentation.DashboardActions{
		ClassroomMode:     true,
		OpenClassroomView: openClassroomView,
		RunningVersion:    nixoriumVersion,
		LoadInitial: func(ctx context.Context) (domain.StatusReport, domain.SetupReport, error) {
			response, err := classroomRequest(ctx, domain.ClassroomOverviewOperation, nil)
			if err != nil || response.Status == nil {
				if err == nil {
					err = errors.New("classroom worker returned no laboratory overview")
				}
				return domain.StatusReport{}, domain.SetupReport{}, err
			}
			return *response.Status, classroomSetupReport(), nil
		},
		Refresh: func(ctx context.Context) (domain.StatusReport, error) {
			response, err := classroomRequest(ctx, domain.ClassroomStatusOperation, nil)
			if err != nil || response.Status == nil {
				if err == nil {
					err = errors.New("classroom worker returned no laboratory status")
				}
				return domain.StatusReport{}, err
			}
			return *response.Status, nil
		},
		LoadHosts: func(ctx context.Context) (domain.HostsReport, error) {
			response, err := classroomRequest(ctx, domain.ClassroomHostsOperation, nil)
			if err != nil || response.Hosts == nil {
				if err == nil {
					err = errors.New("classroom worker returned no computer inventory")
				}
				return domain.HostsReport{}, err
			}
			return *response.Hosts, nil
		},
		PlanPower: func(ctx context.Context, requested string, policy domain.ShutdownSessionPolicy, action domain.ClientPowerAction) domain.ShutdownPlanReport {
			response, err := classroomRequest(ctx, domain.ClassroomPowerPlanOperation, func(request *domain.ClassroomRequest) {
				request.Requested = requested
				request.SessionPolicy = policy
				request.PowerAction = action
			})
			if err != nil || response.PowerPlan == nil {
				return classroomPowerPlanFailure(action, err)
			}
			return *response.PowerPlan
		},
		ApplyShutdown: func(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
			response, err := classroomRequest(ctx, domain.ClassroomPowerApplyOperation, func(request *domain.ClassroomRequest) {
				request.PowerPlan = &plan
			})
			if err != nil || response.PowerReport == nil {
				return classroomPowerApplyFailure(plan.Action, err)
			}
			return *response.PowerReport
		},
		PlanInternet: func(ctx context.Context, requested string, action domain.InternetAction) domain.InternetPlan {
			response, err := classroomRequest(ctx, domain.ClassroomInternetPlanOperation, func(request *domain.ClassroomRequest) {
				request.Requested = requested
				request.InternetAction = action
			})
			if err != nil || response.InternetPlan == nil {
				return classroomInternetPlanFailure(action, err)
			}
			return *response.InternetPlan
		},
		ApplyInternet: func(plan domain.InternetPlan) domain.InternetReport {
			response, err := classroomRequest(ctx, domain.ClassroomInternetApplyOperation, func(request *domain.ClassroomRequest) {
				request.InternetPlan = &plan
			})
			if err != nil || response.InternetReport == nil {
				return classroomInternetApplyFailure(plan.Action, err)
			}
			return *response.InternetReport
		},
		PlanLock: func(ctx context.Context, requested string, action domain.LockAction) domain.LockPlan {
			response, err := classroomRequest(ctx, domain.ClassroomLockPlanOperation, func(request *domain.ClassroomRequest) {
				request.Requested = requested
				request.LockAction = action
			})
			if err != nil || response.LockPlan == nil {
				message := "Locking screens is unavailable."
				if err != nil {
					message = err.Error()
				} else if response.Message != "" {
					message = response.Message
				}
				return domain.LockPlan{SchemaVersion: domain.SchemaVersion, Operation: "lock-plan", State: "blocked", Action: action, Message: message, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
			}
			return *response.LockPlan
		},
		PlanShare: planDesktopShare,
		ApplyShare: func(plan domain.SharePlan) domain.ShareReport {
			return applyDesktopShare(ctx, plan)
		},
		ApplyLock: func(plan domain.LockPlan) domain.LockReport {
			response, err := classroomRequest(ctx, domain.ClassroomLockApplyOperation, func(request *domain.ClassroomRequest) {
				request.LockPlan = &plan
			})
			if err != nil || response.LockReport == nil {
				message := "The reviewed lock change could not be applied."
				if err != nil {
					message = err.Error()
				} else if response.Message != "" {
					message = response.Message
				}
				return domain.LockReport{SchemaVersion: domain.SchemaVersion, Operation: "lock-apply", State: "blocked", Action: plan.Action, Message: message, Targets: []domain.LockOutcome{}}
			}
			return *response.LockReport
		},
	}
	if err := presentation.RunLoadingDashboard(actions, false); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return true, 1
	}
	return true, 0
}

func classroomRequest(ctx context.Context, operation domain.ClassroomOperation, configure func(*domain.ClassroomRequest)) (domain.ClassroomResponse, error) {
	request, err := domain.NewClassroomRequest(operation)
	if err != nil {
		return domain.ClassroomResponse{}, err
	}
	if configure != nil {
		configure(&request)
	}
	response, err := adapters.ClassroomIPCRequest(ctx, adapters.ClassroomSocketPath, request)
	if err != nil && response.State != "failed" {
		// The worker already words its own failures for the teacher.
		return response, errClassroomService
	}
	return response, err
}

var errClassroomService = errors.New("the classroom service is not answering. Ask the administrator to check nixorium-classroom.service. Code: CLASSROOM-SERVICE")

// classroomUnavailable explains a missing classroom service to a teacher
// instead of an administrator's repository error.
func classroomUnavailable() (string, bool) {
	current, err := user.Current()
	if err != nil {
		return "", false
	}
	groups, err := current.GroupIds()
	if err != nil {
		return "", false
	}
	classroom, operations := false, false
	for _, id := range groups {
		group, lookupErr := user.LookupGroupId(id)
		if lookupErr != nil {
			continue
		}
		classroom = classroom || group.Name == "nixorium-classroom"
		operations = operations || group.Name == "nixorium-operations"
	}
	if !classroom || operations {
		return "", false
	}
	return "Classroom controls are not available right now: the classroom service is not running. Ask the administrator to check nixorium-classroom.service. Code: CLASSROOM-SERVICE", true
}

func classroomSetupReport() domain.SetupReport {
	return domain.SetupReport{SchemaVersion: domain.SchemaVersion, Operation: "setup-status", State: "unchecked", Stages: []domain.SetupStage{}}
}

func classroomPowerPlanFailure(action domain.ClientPowerAction, err error) domain.ShutdownPlanReport {
	message := "Classroom power control is unavailable."
	if err != nil {
		message += " " + err.Error()
	}
	return domain.ShutdownPlanReport{SchemaVersion: domain.SchemaVersion, Operation: "power-plan", State: "blocked", Action: action, Message: message, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
}

func classroomPowerApplyFailure(action domain.ClientPowerAction, err error) domain.ShutdownApplyReport {
	message := "The reviewed power request could not be sent."
	if err != nil {
		message += " " + err.Error()
	}
	return domain.ShutdownApplyReport{SchemaVersion: domain.SchemaVersion, Operation: "power-apply", State: "blocked", Action: action, Message: message, RetrySafe: true, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
}

func classroomInternetPlanFailure(action domain.InternetAction, err error) domain.InternetPlan {
	message := "Classroom Internet control is unavailable."
	if err != nil {
		message += " " + err.Error()
	}
	return domain.InternetPlan{SchemaVersion: domain.SchemaVersion, Operation: "internet-plan", State: "blocked", Action: action, Message: message, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
}

func classroomInternetApplyFailure(action domain.InternetAction, err error) domain.InternetReport {
	message := "The reviewed Internet change could not be applied."
	if err != nil {
		message += " " + err.Error()
	}
	return domain.InternetReport{SchemaVersion: domain.SchemaVersion, Operation: "internet-apply", State: "blocked", Action: action, Message: message, Targets: []domain.InternetOutcome{}}
}
