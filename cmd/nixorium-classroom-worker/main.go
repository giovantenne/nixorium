package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

const deploymentPathFile = "/etc/nixorium/deployment-path"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nixorium classroom worker:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 1 {
		return errors.New("usage: nixorium-classroom-worker")
	}
	repository, err := readDeploymentPath(deploymentPathFile)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	local := newCachedLocal()
	// Evaluate the laboratory identities while no teacher is waiting, so the
	// first classroom dashboard opens from the cache.
	go func() { _, _ = local.LabMeta(ctx, repository) }()
	hosts := func(ctx context.Context) ([]domain.HostMeta, error) {
		meta, err := local.LabMeta(ctx, repository)
		return meta.Clients.Hosts, err
	}
	worker := classroomWorker{
		repository: repository,
		view:       newViewServer(app.NewClassroomViewHub(hosts, adapters.ClassroomAgentConnector{})),
		viewOn:     func() bool { return classroomViewEnabled(adapters.Local{}, repository) },
		inspector:  app.NewInspector(local),
		power:      app.NewShutdownManager(local),
		internet:   app.NewInternetManager(local),
		records:    local,
	}
	return adapters.NewClassroomIPCServer(adapters.ClassroomSocketPath, worker.handle).Serve(ctx)
}

type classroomWorker struct {
	repository string
	view       *viewServer
	viewOn     func() bool
	inspector  *app.Inspector
	power      *app.ShutdownManager
	internet   *app.InternetManager
	records    app.OperationRecordSink
}

func (worker classroomWorker) handle(ctx context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	response := domain.ClassroomResponse{State: "completed"}
	switch request.Operation {
	case domain.ClassroomOverviewOperation, domain.ClassroomStatusOperation:
		// Classroom controls show only identities and the PXE state, so
		// deployment readiness and installation artifacts are not evaluated.
		report, err := worker.inspector.Inventory(ctx, worker.repository)
		if err != nil {
			return classroomFailure(err)
		}
		report = classroomStatus(report)
		response.Status = &report
	case domain.ClassroomHostsOperation:
		report, err := worker.inspector.Hosts(ctx, worker.repository)
		if err != nil {
			return classroomFailure(err)
		}
		report.Repository = ""
		response.Hosts = &report
	case domain.ClassroomPowerPlanOperation:
		plan := worker.power.PlanAction(ctx, worker.repository, request.Requested, request.SessionPolicy, request.PowerAction)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		response.PowerPlan = &plan
	case domain.ClassroomPowerApplyOperation:
		if request.PowerPlan == nil || request.PowerPlan.Repository != worker.repository {
			return classroomFailure(errors.New("power review does not belong to the fixed deployment"))
		}
		report := worker.power.ApplyPlan(ctx, *request.PowerPlan, request.PowerPlan.ReviewToken)
		if err := app.RecordOperationOutcome(worker.records, report); err != nil {
			report.Message += " The operation finished, but its history record could not be saved."
		}
		report.Message, report.Issues = teacherMessage(report.Message), teacherIssues(report.Issues)
		response.PowerReport = &report
	case domain.ClassroomInternetPlanOperation:
		plan := worker.internet.Plan(ctx, worker.repository, request.Requested, request.InternetAction)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		response.InternetPlan = &plan
	case domain.ClassroomInternetApplyOperation:
		if request.InternetPlan == nil || request.InternetPlan.Repository != worker.repository {
			return classroomFailure(errors.New("Internet review does not belong to the fixed deployment"))
		}
		report := worker.internet.Apply(ctx, *request.InternetPlan, request.InternetPlan.ReviewToken)
		if err := app.RecordOperationOutcome(worker.records, report); err != nil {
			report.Message += " The operation finished, but its history record could not be saved."
		}
		report.Message = teacherMessage(report.Message)
		response.InternetReport = &report
	case domain.ClassroomViewOpenOperation:
		if worker.view == nil || worker.viewOn == nil || !worker.viewOn() {
			return domain.ClassroomResponse{State: "failed", Message: errViewDisabled.Error()}
		}
		address, err := worker.view.Open()
		if err != nil {
			return classroomFailure(err)
		}
		response.ViewURL = address
	default:
		return classroomFailure(errors.New("unsupported classroom operation"))
	}
	return response
}

func classroomStatus(report domain.StatusReport) domain.StatusReport {
	return domain.StatusReport{
		SchemaVersion: report.SchemaVersion,
		Operation:     report.Operation,
		GeneratedAt:   report.GeneratedAt,
		State:         report.State,
		Meta:          report.Meta,
		PXE:           report.PXE,
	}
}

func readDeploymentPath(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errors.New("deployment path configuration filename is invalid")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect deployment path configuration: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || !ok || stat.Uid != 0 || info.Size() < 2 || info.Size() > 4096 {
		return "", errors.New("deployment path configuration must be a small root-owned non-writable regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	repository := strings.TrimSpace(string(content))
	if strings.ContainsAny(repository, "\r\n\x00") || !filepath.IsAbs(repository) || filepath.Clean(repository) != repository {
		return "", errors.New("configured deployment path is not canonical")
	}
	return repository, nil
}

// classroomViewEnabled reads only the classroomView switch; a missing or
// unreadable settings file means the view stays off.
func classroomViewEnabled(reader interface{ ReadSettings(string) ([]byte, error) }, repository string) bool {
	data, err := reader.ReadSettings(repository)
	if err != nil {
		return false
	}
	settings, _ := domain.DecodeLabSettings(data)
	return settings.Lab.ClassroomView
}
