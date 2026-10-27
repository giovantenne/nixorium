package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
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
	repository, err := adapters.ReadFixedDeploymentPath(deploymentPathFile)
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
	// The service has a private temporary folder for files being sent.
	shares := newShareStore(os.TempDir())
	hub := app.NewClassroomViewHub(hosts, adapters.ClassroomAgentConnector{})
	// A computer that answers the classroom view teaches the controller its
	// hardware address for Switch on.
	hub.Answered = func(host domain.HostMeta) { local.LearnWakeAddresses([]domain.HostMeta{host}) }
	worker := classroomWorker{
		repository: repository,
		plans:      newPlanStore(),
		view:       newViewServer(hub),
		viewOn:     adapters.ClassroomViewApplied,
		inspector:  app.NewInspector(local),
		power:      app.NewShutdownManager(local),
		internet:   app.NewInternetManager(local),
		lock:       app.NewLockManager(lockSource{local, adapters.ClassroomAgentConnector{}}),
		shares:     shares,
		share:      app.NewShareManager(lockSource{local, adapters.ClassroomAgentConnector{}}, shares),
		records:    local,
		wake:       app.NewWakeManager(local),
		schedule:   app.NewScheduleManager(local),
	}
	// Files collected from students' desktops wait in the private
	// temporary folder too, until the user who asked saves them.
	worker.collected = newCollectStore(os.TempDir())
	worker.collect = app.NewCollectManager(lockSource{local, adapters.ClassroomAgentConnector{}}, worker.collected)
	go worker.collected.expire(ctx, time.Minute)
	worker.message = app.NewMessageManager(lockSource{local, adapters.ClassroomAgentConnector{}})
	// The page's actions go through the same handling as the dashboard.
	worker.view.actions = worker.handle
	worker.view.desktops = newDesktopBroker()
	go worker.view.desktops.expireSaves(ctx, time.Minute)
	view := worker.view
	view.internet = &internetWatcher{
		observe:  local.ObserveInternet,
		hosts:    hosts,
		interval: 30 * time.Second,
		idle:     time.Minute,
		record: func(name, state string) {
			view.setLabel(name, func(labels *computerLabels) { labels.internet = state })
		},
	}
	// Showing the teacher's screen starts only from the page.
	worker.view.enabled = worker.viewOn
	worker.view.broadcasts = &broadcaster{manager: app.NewBroadcastManager(lockSource{local, adapters.ClassroomAgentConnector{}}), repository: repository, viewOn: worker.viewOn}
	return adapters.NewClassroomIPCServer(adapters.ClassroomSocketPath, worker.handle).Serve(ctx)
}

type classroomWorker struct {
	repository string
	plans      *planStore // the plans issued, by review token
	view       *viewServer
	viewOn     func() bool
	inspector  *app.Inspector
	power      *app.ShutdownManager
	internet   *app.InternetManager
	lock       *app.LockManager
	shares     *shareStore
	share      *app.ShareManager
	collect    *app.CollectManager // copies students' desktops
	collected  *collectStore       // until the user who asked saves them
	records    app.OperationRecordSink
	wake       *app.WakeManager
	message    *app.MessageManager  // shows a message on students' screens
	schedule   *app.ScheduleManager // shows and sets the schedule the timer runs
}

// lockSource reads the laboratory identities and reaches the classroom agents.
type lockSource struct {
	cachedLocal
	adapters.ClassroomAgentConnector
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
		rememberPlan(worker.plans, "power", plan.ReviewToken, plan.ExpiresAt, plan)
		response.PowerPlan = &plan
	case domain.ClassroomPowerApplyOperation:
		if request.PowerPlan == nil || request.PowerPlan.Repository != worker.repository {
			return classroomFailure(errors.New("power review does not belong to the fixed deployment"))
		}
		plan, err := takePlan[domain.ShutdownPlanReport](worker.plans, "power", request.PowerPlan.ReviewToken)
		if err != nil {
			return classroomFailure(err)
		}
		report := worker.power.ApplyPlan(ctx, plan, plan.ReviewToken)
		if err := app.RecordOperationOutcome(worker.records, report); err != nil {
			report.Message += " The operation finished, but its history record could not be saved."
		}
		report.Message, report.Issues = teacherMessage(report.Message), teacherIssues(report.Issues)
		response.PowerReport = &report
	case domain.ClassroomInternetPlanOperation:
		plan := worker.internet.Plan(ctx, worker.repository, request.Requested, request.InternetAction)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		rememberPlan(worker.plans, "internet", plan.ReviewToken, plan.ExpiresAt, plan)
		response.InternetPlan = &plan
	case domain.ClassroomInternetApplyOperation:
		if request.InternetPlan == nil || request.InternetPlan.Repository != worker.repository {
			return classroomFailure(errors.New("Internet review does not belong to the fixed deployment"))
		}
		plan, err := takePlan[domain.InternetPlan](worker.plans, "internet", request.InternetPlan.ReviewToken)
		if err != nil {
			return classroomFailure(err)
		}
		report := worker.internet.Apply(ctx, plan, plan.ReviewToken)
		if err := app.RecordOperationOutcome(worker.records, report); err != nil {
			report.Message += " The operation finished, but its history record could not be saved."
		}
		report.Message = teacherMessage(report.Message)
		response.InternetReport = &report
	case domain.ClassroomLockPlanOperation:
		if worker.viewOn == nil || !worker.viewOn() {
			return domain.ClassroomResponse{State: "failed", Message: errViewDisabled.Error()}
		}
		plan := worker.lock.Plan(ctx, worker.repository, request.Requested, request.LockAction)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		rememberPlan(worker.plans, "lock", plan.ReviewToken, plan.ExpiresAt, plan)
		response.LockPlan = &plan
	case domain.ClassroomLockApplyOperation:
		if request.LockPlan == nil || request.LockPlan.Repository != worker.repository {
			return classroomFailure(errors.New("lock review does not belong to the fixed deployment"))
		}
		plan, err := takePlan[domain.LockPlan](worker.plans, "lock", request.LockPlan.ReviewToken)
		if err != nil {
			return classroomFailure(err)
		}
		report := worker.lock.Apply(ctx, plan, plan.ReviewToken)
		report.Message = teacherMessage(report.Message)
		response.LockReport = &report
	case domain.ClassroomShareBeginOperation:
		if worker.viewOn == nil || !worker.viewOn() {
			return domain.ClassroomResponse{State: "failed", Message: errViewDisabled.Error()}
		}
		entries := make([]classroomview.FileEntry, len(request.ShareFiles))
		for index, file := range request.ShareFiles {
			entries[index] = classroomview.FileEntry{Path: file.Path, Size: file.Size, Dir: file.Dir}
		}
		transfer, err := worker.shares.Begin(entries)
		if err != nil {
			return domain.ClassroomResponse{State: "failed", Message: "The files cannot be sent: " + err.Error() + "."}
		}
		response.ShareTransfer = transfer
	case domain.ClassroomShareChunkOperation:
		if len(request.ShareData) > domain.ClassroomShareChunkBytes {
			return domain.ClassroomResponse{State: "failed", Message: "A piece of a file is too large."}
		}
		if err := worker.shares.Write(request.ShareTransfer, request.ShareIndex, request.ShareOffset, request.ShareData); err != nil {
			return domain.ClassroomResponse{State: "failed", Message: "The files could not be prepared: " + err.Error() + "."}
		}
	case domain.ClassroomSharePlanOperation:
		plan := worker.share.Plan(ctx, worker.repository, request.Requested, request.ShareTransfer)
		plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
		rememberPlan(worker.plans, "share", plan.ReviewToken, plan.ExpiresAt, plan)
		response.SharePlan = &plan
	case domain.ClassroomShareApplyOperation:
		if request.SharePlan == nil || request.SharePlan.Repository != worker.repository {
			return classroomFailure(errors.New("share review does not belong to the fixed deployment"))
		}
		plan, err := takePlan[domain.SharePlan](worker.plans, "share", request.SharePlan.ReviewToken)
		if err != nil {
			return classroomFailure(err)
		}
		report := worker.share.Apply(ctx, plan, plan.ReviewToken)
		if report.State == "completed" {
			worker.shares.Remove(plan.Transfer)
		}
		response.ShareReport = &report
	case domain.ClassroomSaveItemsOperation, domain.ClassroomSaveReadOperation, domain.ClassroomSaveDoneOperation:
		return worker.handleSave(ctx, request)
	case domain.ClassroomCollectPlanOperation, domain.ClassroomCollectApplyOperation:
		return worker.handleCollect(ctx, request)
	case domain.ClassroomDesktopWaitOperation, domain.ClassroomDesktopReadyOperation:
		uid, known := adapters.ClassroomPeerUID(ctx)
		if !known || worker.view == nil || worker.view.desktops == nil {
			return domain.ClassroomResponse{State: "failed", Message: "The desktop helper is not available."}
		}
		if request.Operation == domain.ClassroomDesktopWaitOperation {
			offer := worker.view.desktops.Wait(ctx, uid)
			response.DesktopJob, response.DesktopKind = offer.id, offer.kind
		} else if !worker.view.desktops.Ready(uid, request.DesktopJob, request.ShareTransfer, request.DesktopError, request.DesktopCancelled) {
			return domain.ClassroomResponse{State: "failed", Message: "This desktop job is unknown."}
		}
	case domain.ClassroomWakePlanOperation, domain.ClassroomWakeApplyOperation:
		return worker.handleWake(ctx, request)
	case domain.ClassroomMessagePlanOperation, domain.ClassroomMessageApplyOperation:
		return worker.handleMessage(ctx, request)
	case domain.ClassroomScheduleGetOperation, domain.ClassroomScheduleSetOperation:
		return worker.handleSchedule(ctx, request)
	case domain.ClassroomViewOpenOperation:
		if worker.view == nil || worker.viewOn == nil || !worker.viewOn() {
			return domain.ClassroomResponse{State: "failed", Message: errViewDisabled.Error()}
		}
		uid, known := adapters.ClassroomPeerUID(ctx)
		if !known {
			return classroomFailure(errors.New("the classroom socket did not identify its caller"))
		}
		address, err := worker.view.Open(uid)
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
