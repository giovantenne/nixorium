package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	workerSocketPath         = "/run/nixorium/remote-install/control.sock"
	workerStatePath          = "/var/lib/nixorium/remote-install"
	workerDeploymentPathFile = "/etc/nixorium/deployment-path"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nixorium remote installation worker:", err)
		os.Exit(1)
	}
}

func run() error {
	repository, err := readFixedDeploymentPath(workerDeploymentPathFile)
	if err != nil {
		return err
	}
	state, err := adapters.NewRemoteInstallStateStore(workerStatePath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	preparer, err := adapters.NewRemoteInstallPreparer(repository, workerStatePath)
	if err != nil {
		return err
	}
	worker := &remoteWorker{state: state, bootstrap: adapters.NewLiveBootstrap(), preparer: preparer, reservations: adapters.Local{}}
	if err := worker.recover(); err != nil {
		return err
	}
	server := adapters.NewRemoteInstallIPCServer(workerSocketPath, worker.handle)
	return server.Serve(ctx)
}

type remoteState interface {
	Load(string) (domain.RemoteInstallSession, error)
	Save(domain.RemoteInstallSession) error
}

type liveBootstrapper interface {
	Establish(context.Context, string, string, string, *adapters.LivePassword) (adapters.VerifiedLiveSession, error)
	RecoverSession(string, domain.RemoteInstallBootstrapRecord) (adapters.VerifiedLiveSession, error)
	CloseSession(context.Context, adapters.VerifiedLiveSession) error
	DiscardLocalSession(adapters.VerifiedLiveSession) error
}

type remoteReservationSource interface {
	ReserveRemoteSession(string) (domain.RemoteInstallReservation, error)
	RecoverRemoteSession() (string, domain.RemoteInstallReservation, bool, error)
}

type remotePreparer interface {
	PrepareArtifacts(context.Context, string, string) (domain.RemoteInstallArtifacts, error)
	Finalize(context.Context, domain.RemoteInstallArtifacts, adapters.VerifiedLiveSession) (domain.RemoteInstallPreparation, error)
	Discard(string) error
}

type remoteStatusConnection interface {
	Status(context.Context, string, string) (domain.RemoteInstallReceipt, error)
	OperationLog(context.Context, string, string) ([]byte, error)
}

type remoteStatusConnectionFactory func(adapters.VerifiedLiveSession) (remoteStatusConnection, error)
type remoteOperationLogPublisher func(string, []byte, string) (string, error)

type remoteWorker struct {
	mutex                  sync.Mutex
	state                  remoteState
	bootstrap              liveBootstrapper
	preparer               remotePreparer
	reservations           remoteReservationSource
	statusConnection       remoteStatusConnectionFactory
	publishRemoteLog       remoteOperationLogPublisher
	verifyInstalledRemote  func(context.Context, string, string, domain.RemoteInstallPreparation) (adapters.InstalledRemoteState, error)
	mergeVerifiedKnownHost func(string, string, string, string, string, bool) error
	operationID            string
	liveSession            *adapters.VerifiedLiveSession
	reservation            domain.RemoteInstallReservation
}

func (worker *remoteWorker) newStatusConnection(session adapters.VerifiedLiveSession) (remoteStatusConnection, error) {
	if worker.statusConnection != nil {
		return worker.statusConnection(session)
	}
	return adapters.NewStrictLiveSSH(session)
}

func (worker *remoteWorker) publishOperationLog(operationID string, content []byte, result string) (string, error) {
	if worker.publishRemoteLog != nil {
		return worker.publishRemoteLog(operationID, content, result)
	}
	return adapters.PublishRemoteOperationLog(operationID, content, result)
}

func (worker *remoteWorker) recover() error {
	operationID, reservation, present, err := worker.reservations.RecoverRemoteSession()
	if err != nil {
		return fmt.Errorf("recover persistent reservation: %w", err)
	}
	if !present {
		return nil
	}
	session, err := worker.state.Load(operationID)
	if err != nil {
		return fmt.Errorf("recover reserved operation state: %w", err)
	}
	worker.operationID = operationID
	worker.reservation = reservation
	if session.State == "closed-before-apply" && remoteSessionNeverDispatched(session) {
		return worker.releaseClosedBeforeApplyLocked(session)
	}
	if session.State == "failed-resolved" && remoteSessionConfirmedNoMutationFailure(session) {
		if err := worker.releaseResolvedFailureLocked(&session); err != nil {
			return fmt.Errorf("finish resolved USB failure cleanup: %w", err)
		}
		return nil
	}

	changed := false
	if session.State == "dispatching" || session.State == "reboot-dispatching" {
		session.State = "reconciliation-required"
		session.DispatchUncertain = true
		session.Events = append(session.Events, domain.RemoteInstallProgress{
			Phase:  domain.RemoteInstallPhaseReconciliationRequired,
			Detail: "worker restarted after a persisted dispatch boundary; automatic replay is forbidden",
		})
		changed = true
	}
	if session.Bootstrap != nil {
		liveSession, recoveryErr := worker.bootstrap.RecoverSession(operationID, *session.Bootstrap)
		if recoveryErr != nil {
			if session.State != "reconciliation-required" {
				session.State = "reconciliation-required"
				session.Events = append(session.Events, domain.RemoteInstallProgress{
					Phase:  domain.RemoteInstallPhaseReconciliationRequired,
					Detail: "worker restarted without a valid private live-session credential; destructive actions remain blocked",
				})
				changed = true
			}
		} else {
			worker.liveSession = &liveSession
		}
	}
	if changed {
		if err := worker.state.Save(session); err != nil {
			return fmt.Errorf("publish recovered operation state: %w", err)
		}
	}
	return nil
}

func (worker *remoteWorker) handle(ctx context.Context, request domain.RemoteInstallRequest, secret *adapters.LivePassword) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID}
	switch request.Operation {
	case domain.RemoteInstallWorkerProbeOperation:
		if secret != nil {
			secret.Destroy()
		}
		response.State = "ready"
		response.Message = "remote installation worker protocol is ready"
		worker.mutex.Lock()
		if worker.operationID != "" {
			if session, err := worker.state.Load(worker.operationID); err == nil {
				response.OperationID = worker.operationID
				response.State = session.State
				response.Session = &session
				response.Message = "remote installation worker owns an existing operation"
			}
		}
		worker.mutex.Unlock()
	case domain.RemoteInstallBootstrapOperation:
		return worker.handleBootstrap(ctx, request, secret)
	case domain.RemoteInstallPrepareOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handlePrepare(ctx, request)
	case domain.RemoteInstallPlanOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handlePlan(ctx, request)
	case domain.RemoteInstallApplyOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handleApply(ctx, request)
	case domain.RemoteInstallRebootOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handleReboot(ctx, request)
	case domain.RemoteInstallVerifyOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handleVerify(ctx, request)
	case domain.RemoteInstallCloseOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handleClose(ctx, request)
	case domain.RemoteInstallStatusOperation, domain.RemoteInstallReconcileOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handleStatus(ctx, request)
	case domain.RemoteInstallCancelOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handleCancel(ctx, request)
	default:
		if secret != nil {
			secret.Destroy()
		}
		response.State = "unavailable"
		response.Message = "operation is not enabled before verified live-session preparation"
	}
	return response
}

func (worker *remoteWorker) handleReboot(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "blocked"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.liveSession == nil || worker.reservation == nil {
		response.Message = "worker does not own the installed live session"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || session.Preparation == nil {
		response.Message = "installed remote session state is unavailable"
		return response
	}
	connection, err := adapters.NewStrictLiveSSH(*worker.liveSession)
	if err != nil {
		response.Message = err.Error()
		return response
	}
	receipt, err := connection.Status(ctx, session.Preparation.BundlePath, request.OperationID)
	if err != nil || receipt.State != "ready-to-reboot" || !receipt.Installed {
		response.Message = "remote installation is not confirmed ready for an explicit reboot"
		return response
	}
	session.Receipt = &receipt
	session.RebootRequested = true
	session.State = "reboot-dispatching"
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhaseReboot, Detail: "explicit reboot authorization persisted before dispatch"})
	if err := worker.state.Save(session); err != nil {
		response.Message = "could not persist reboot authorization before dispatch"
		return response
	}
	rebootReceipt, err := connection.Reboot(ctx, session.Preparation.BundlePath, request.OperationID)
	if err != nil {
		session.State = "reconciliation-required"
		session.DispatchUncertain = true
		_ = worker.state.Save(session)
		response.State = session.State
		response.Session = &session
		response.Message = "reboot dispatch may have reached the live client; verify the reviewed static identity before retrying"
		return response
	}
	session.Receipt = &rebootReceipt
	session.State = "reboot-requested"
	session.DispatchUncertain = false
	if err := worker.state.Save(session); err != nil {
		response.State = "reconciliation-required"
		response.Message = "reboot was requested but its state could not be persisted"
		return response
	}
	response.State = session.State
	response.Session = &session
	response.Message = rebootReceipt.Message
	return response
}

func (worker *remoteWorker) handleVerify(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "blocked"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.reservation == nil {
		response.Message = "worker does not own the rebooted installation session"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || session.Preparation == nil || !session.RebootRequested || session.Preparation.OperationID != request.OperationID {
		response.Message = "remote installation has no persisted reboot authorization"
		return response
	}
	verify := worker.verifyInstalledRemote
	if verify == nil {
		verify = adapters.VerifyInstalledRemote
	}
	installed, err := verify(ctx, "/run/nixorium/remote-install", "/home/admin/.ssh/id_ed25519", *session.Preparation)
	if err != nil {
		response.State = session.State
		response.Session = &session
		response.Message = err.Error()
		return response
	}
	merge := worker.mergeVerifiedKnownHost
	if merge == nil {
		merge = adapters.MergeVerifiedKnownHost
	}
	if err := merge(
		adapters.ManagedKnownHostsPath, "/var/lib/nixorium/remote-install/known-hosts-backups",
		session.Preparation.Host.StaticIP, session.Preparation.HostKeyPublic, session.OperationID, session.Plan.HostKeyRotation,
	); err != nil {
		response.State = "reconciliation-required"
		response.Session = &session
		response.Message = "installed identity verified but persistent known-host update requires reconciliation: " + err.Error()
		return response
	}
	session.BootVerified = true
	session.State = "verified"
	session.DispatchUncertain = false
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePostBootVerify, Detail: "verified hostname, exact system closure, revision, and preserved host key at the static address"})
	if err := worker.state.Save(session); err != nil {
		response.State = "reconciliation-required"
		response.Message = "post-boot identity was verified but persistent state could not be updated"
		return response
	}
	if err := worker.bootstrap.DiscardLocalSession(adapters.VerifiedLiveSession{OperationID: session.OperationID}); err != nil {
		response.State = "reconciliation-required"
		response.Message = "post-boot identity was verified but local ephemeral credentials could not be removed"
		return response
	}
	if worker.preparer == nil {
		response.State = "reconciliation-required"
		response.Message = "post-boot identity was verified but preparation cleanup is unavailable"
		return response
	}
	if err := worker.preparer.Discard(session.OperationID); err != nil {
		response.State = "reconciliation-required"
		response.Message = "post-boot identity was verified but preparation GC roots could not be removed"
		return response
	}
	if err := worker.reservation.ReleaseResolved(); err != nil {
		response.State = "reconciliation-required"
		response.Message = "post-boot identity was verified but the controller reservation could not be released"
		return response
	}
	worker.operationID = ""
	worker.liveSession = nil
	worker.reservation = nil
	response.State = session.State
	response.Session = &session
	response.Execution = &domain.RemoteInstallExecutionReport{
		SchemaVersion: domain.SchemaVersion, Operation: "usb-install-verify", State: "verified", OperationID: session.OperationID,
		Phase: domain.RemoteInstallPhasePostBootVerify, MutationStarted: true, DiskMayBeModified: true, Installed: true,
		RebootRequested: true, BootVerified: true, Message: fmt.Sprintf("verified %s at %s", installed.Hostname, session.Preparation.Host.StaticIP),
	}
	response.Message = response.Execution.Message
	return response
}

func (worker *remoteWorker) handleClose(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "reconciliation-required"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.reservation == nil {
		response.Message = "worker no longer owns the live credentials; reconcile the persistent reservation"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err == nil && remoteSessionNeverDispatched(session) {
		if session.State != "closed-before-apply" {
			session.State = "closed-before-apply"
			session.Events = append(session.Events, domain.RemoteInstallProgress{
				Phase:  domain.RemoteInstallPhasePreflight,
				Detail: "operator closed a never-dispatched session; discard local credentials without claiming remote key revocation",
			})
			if err := worker.state.Save(session); err != nil {
				response.Message = "could not persist session closure; reservation retained"
				return response
			}
		}
		if err := worker.releaseClosedBeforeApplyLocked(session); err != nil {
			response.Message = err.Error()
			return response
		}
		response.State = session.State
		response.Session = &session
		response.Message = "never-dispatched session closed; local credentials discarded, remote key revocation unconfirmed; a new installation requires a new review"
		return response
	}
	if err != nil || worker.liveSession == nil || session.Receipt == nil || !session.Receipt.Installed || session.RebootRequested || session.DispatchUncertain {
		response.Message = "only a confirmed installed session before reboot can be closed"
		return response
	}
	if err := worker.bootstrap.CloseSession(ctx, *worker.liveSession); err != nil {
		response.Message = "live key cleanup was not confirmed; reservation retained"
		return response
	}
	session.State = "closed-installed"
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhaseReadyToReboot, Detail: "session closed without reboot; ephemeral live key revoked"})
	if err := worker.state.Save(session); err != nil {
		response.Message = "cleanup succeeded but persistent state could not be updated; reservation retained"
		return response
	}
	if worker.preparer == nil {
		response.Message = "cleanup succeeded but preparation cleanup is unavailable; reservation retained"
		return response
	}
	if err := worker.preparer.Discard(session.OperationID); err != nil {
		response.Message = "cleanup succeeded but preparation GC roots could not be removed; reservation retained"
		return response
	}
	if err := worker.reservation.ReleaseResolved(); err != nil {
		response.Message = "cleanup succeeded but the controller reservation could not be released"
		return response
	}
	worker.operationID = ""
	worker.liveSession = nil
	worker.reservation = nil
	response.State = session.State
	response.Session = &session
	response.Message = "installed session closed without reboot; remove local media before starting the target"
	return response
}

func remoteSessionNeverDispatched(session domain.RemoteInstallSession) bool {
	if session.TokenConsumed || session.DispatchUncertain || session.Receipt != nil || session.RebootRequested || session.BootVerified {
		return false
	}
	switch session.State {
	case "artifacts-ready", "bootstrapped", "bootstrapped-artifacts", "prepared", "review-ready", "reconciliation-required", "closed-before-apply":
		return true
	}
	return false
}

func (worker *remoteWorker) releaseClosedBeforeApplyLocked(session domain.RemoteInstallSession) error {
	if session.State != "closed-before-apply" || !remoteSessionNeverDispatched(session) || worker.reservation == nil {
		return errors.New("never-dispatched session closure is incomplete; reservation retained")
	}
	if err := worker.bootstrap.DiscardLocalSession(adapters.VerifiedLiveSession{OperationID: session.OperationID}); err != nil {
		return fmt.Errorf("discard closed session credentials; reservation retained: %w", err)
	}
	if session.Artifacts != nil || session.Preparation != nil {
		if worker.preparer == nil {
			return errors.New("closed session preparation cleanup unavailable; reservation retained")
		}
		if err := worker.preparer.Discard(session.OperationID); err != nil {
			return fmt.Errorf("discard closed session preparation; reservation retained: %w", err)
		}
	}
	if err := worker.reservation.ReleaseResolved(); err != nil {
		return fmt.Errorf("release closed session reservation: %w", err)
	}
	worker.operationID = ""
	worker.liveSession = nil
	worker.reservation = nil
	return nil
}

func (worker *remoteWorker) handlePlan(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "blocked"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.liveSession == nil || worker.reservation == nil {
		response.Message = "worker does not own the prepared live session"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || session.Preparation == nil || (session.State != "prepared" && session.State != "review-ready") || session.DispatchUncertain {
		response.Message = "remote installation session is not ready for disk review"
		return response
	}
	source, err := worker.installSource(session.Preparation.BundlePath)
	if err != nil {
		response.Message = err.Error()
		return response
	}
	manager := app.NewRemoteInstallManager(source)
	report := manager.PlanReservedWithHostKeyRotation(ctx, *session.Preparation, request.Disk, request.HostKeyRotation)
	response.Plan = &report
	if report.HasErrors() {
		response.Message = report.Message
		if response.Message == "" && len(report.Issues) > 0 {
			response.Message = report.Issues[0].Message
		}
		return response
	}
	session.Plan = report.Plan
	session.ReviewTokenDigest = domain.RemoteInstallTokenDigest(report.ReviewToken)
	session.ReviewExpiresAt = report.ExpiresAt
	session.TokenConsumed = false
	session.State = "review-ready"
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhaseReview, Detail: "published content-bound destructive disk review"})
	if err := worker.state.Save(session); err != nil {
		response.Plan = nil
		response.Message = "could not persist the reviewed remote installation plan"
		return response
	}
	response.State = session.State
	response.Session = &session
	response.Message = report.Message
	return response
}

func (worker *remoteWorker) handleApply(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "blocked"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.liveSession == nil || worker.reservation == nil {
		response.Message = "worker does not own the reviewed live session"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || session.Preparation == nil || session.State != "review-ready" || session.TokenConsumed || session.ReviewExpiresAt.IsZero() {
		response.Message = "remote installation review is absent, consumed, or no longer applicable"
		return response
	}
	expectedDigest := domain.RemoteInstallTokenDigest(request.ReviewToken)
	if subtle.ConstantTimeCompare([]byte(expectedDigest), []byte(session.ReviewTokenDigest)) != 1 {
		response.Message = "remote installation review token differs"
		return response
	}
	expectedConfirmation := fmt.Sprintf("ERASE %s FOR %s", session.Plan.Disk.Path, session.Plan.Host.Name)
	if request.Confirmation != expectedConfirmation {
		response.Message = "destructive confirmation text differs from the reviewed disk"
		return response
	}
	report := frozenWorkerPlan(session, request.ReviewToken)
	if domain.RemoteInstallReviewToken(report) != request.ReviewToken {
		response.Message = "persistent review no longer matches its content-bound token"
		return response
	}
	session.TokenConsumed = true
	session.State = "dispatching"
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhaseRevalidate, Detail: "review token consumed before final revalidation and dispatch"})
	if err := worker.state.Save(session); err != nil {
		response.Message = "could not persist review consumption before dispatch"
		return response
	}
	source, err := worker.installSource(session.Preparation.BundlePath)
	if err != nil {
		response.Message = err.Error()
		return response
	}
	execution := app.NewRemoteInstallManager(source).ApplyReserved(ctx, report, request.ReviewToken)
	response.Execution = &execution
	session.DispatchUncertain = execution.DispatchUncertain
	if execution.DispatchUncertain {
		session.State = "reconciliation-required"
		session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhaseReconciliationRequired, Detail: "apply response was uncertain; automatic retry is forbidden"})
	} else if execution.State == "blocked" || execution.State == "failed" {
		session.State = "prepared"
		session.ReviewTokenDigest = ""
		session.ReviewExpiresAt = time.Time{}
		session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: execution.Phase, Detail: "apply stopped before a confirmed independent job acceptance"})
	} else {
		session.State = execution.State
		receipt := domain.RemoteInstallReceipt{
			SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: session.OperationID, State: execution.State,
			Phase: execution.Phase, MutationStarted: execution.MutationStarted, DiskMayBeModified: execution.DiskMayBeModified,
			Installed: execution.Installed, Message: execution.Message,
		}
		session.Receipt = &receipt
		session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: execution.Phase, Detail: "independent live installer job accepted"})
	}
	if err := worker.state.Save(session); err != nil {
		response.State = "reconciliation-required"
		response.Message = "apply result could not be persisted; reconcile the same operation ID"
		return response
	}
	response.State = session.State
	response.Session = &session
	response.Message = execution.Message
	return response
}

func (worker *remoteWorker) handleStatus(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "unavailable"}
	session, err := worker.state.Load(request.OperationID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.Message = "remote installation operation does not exist"
		} else {
			response.Message = "remote installation state is unavailable or unsafe"
		}
		return response
	}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	message := "remote installation state loaded"
	if worker.operationID == request.OperationID && session.State == "failed-resolved" && remoteSessionConfirmedNoMutationFailure(session) {
		if cleanupErr := worker.releaseResolvedFailureLocked(&session); cleanupErr != nil {
			message = "confirmed pre-mutation failure cleanup remains incomplete: " + cleanupErr.Error()
		} else {
			message = "confirmed pre-mutation failure was already made safe; controller reservation released"
		}
	}
	if worker.operationID == request.OperationID && worker.liveSession != nil && session.Preparation != nil &&
		(session.TokenConsumed || session.DispatchUncertain || session.Receipt != nil) {
		connection, connectionErr := worker.newStatusConnection(*worker.liveSession)
		if connectionErr != nil {
			message = "remote installation state loaded; verified live connection is unavailable: " + connectionErr.Error()
		} else {
			receipt, statusErr := connection.Status(ctx, session.Preparation.BundlePath, request.OperationID)
			if statusErr != nil {
				message = "remote installation state loaded; remote status refresh failed: " + statusErr.Error()
			} else {
				session.Receipt = &receipt
				session.DispatchUncertain = receipt.State == "unknown"
				if receipt.State == "unknown" {
					session.State = "reconciliation-required"
				} else {
					session.State = receipt.State
				}
				logMessage := ""
				if receipt.State == "failed" || receipt.State == "ready-to-reboot" || receipt.State == "reboot-requested" {
					if remoteLog, logErr := connection.OperationLog(ctx, session.Preparation.BundlePath, request.OperationID); logErr == nil {
						result := "completed"
						if receipt.State == "failed" {
							result = "failed"
							if receipt.MutationStarted || receipt.DiskMayBeModified {
								result = "partial"
							}
						}
						if logID, publishErr := worker.publishOperationLog(request.OperationID, remoteLog, result); publishErr == nil {
							session.LogID = logID
						} else {
							logMessage = "controller could not publish the remote operation log: " + publishErr.Error()
						}
					} else {
						logMessage = "controller could not retrieve the remote operation log: " + logErr.Error()
					}
				}
				saveErr := worker.state.Save(session)
				if saveErr != nil {
					message = "remote status was observed but could not be persisted: " + saveErr.Error()
				} else if remoteSessionSafelyCancellable(session) {
					if cleanupErr := worker.resolveConfirmedFailureLocked(ctx, &session); cleanupErr != nil {
						message = "remote failure occurred before disk mutation, but automatic cleanup is incomplete: " + cleanupErr.Error()
					} else {
						message = "remote failure occurred before disk mutation; live credentials were revoked and the controller reservation was released automatically"
					}
				}
				if logMessage != "" {
					message += "; " + logMessage
				}
			}
		}
	}
	response.State = session.State
	response.Session = &session
	response.Message = message
	return response
}

func frozenWorkerPlan(session domain.RemoteInstallSession, token string) domain.RemoteInstallPlanReport {
	preparation := *session.Preparation
	return domain.RemoteInstallPlanReport{
		SchemaVersion: domain.SchemaVersion, Operation: "usb-install-plan", State: "ready", Repository: preparation.Repository,
		Method: domain.RemoteInstallUSBSSH, OperationID: session.OperationID, Host: session.Plan.Host, Disk: session.Plan.Disk,
		Revision: session.Plan.DeploymentRevision, BundlePath: preparation.BundlePath, SystemPath: session.Plan.SystemPath,
		CacheURL: session.Plan.Cache.URL, HostKeyRotation: session.Plan.HostKeyRotation,
		ExpiresAt: session.ReviewExpiresAt, ReviewToken: token,
		Confirmation: fmt.Sprintf("ERASE %s FOR %s", session.Plan.Disk.Path, session.Plan.Host.Name),
		Plan:         session.Plan, Preparation: preparation, Issues: []domain.ValidationIssue{},
	}
}

type workerInstallSource struct {
	local      adapters.Local
	connection *adapters.StrictLiveSSH
	bundlePath string
}

func (worker *remoteWorker) installSource(bundlePath string) (*workerInstallSource, error) {
	if worker.liveSession == nil {
		return nil, errors.New("verified live SSH session is unavailable")
	}
	connection, err := adapters.NewStrictLiveSSH(*worker.liveSession)
	if err != nil {
		return nil, err
	}
	if !domain.ValidStorePath(bundlePath) {
		return nil, errors.New("prepared remote installer bundle path is invalid")
	}
	return &workerInstallSource{local: adapters.Local{}, connection: connection, bundlePath: bundlePath}, nil
}

func (source *workerInstallSource) LabMeta(ctx context.Context, repository string) (domain.LabMeta, error) {
	return source.local.LabMeta(ctx, repository)
}

func (source *workerInstallSource) CurrentRevision(ctx context.Context, repository string) (string, error) {
	state, err := source.local.GitState(ctx, repository)
	if err != nil || state.Dirty {
		return "", errors.New("deployment worktree is not clean")
	}
	return source.local.GitRevision(ctx, repository)
}

func (source *workerInstallSource) ClientOperationActive() (bool, error) {
	return source.local.ClientOperationActive()
}

func (source *workerInstallSource) ServiceState(ctx context.Context, name string) domain.ServiceState {
	return source.local.ServiceState(ctx, name)
}

func (*workerInstallSource) RemoteIdentityReachable(ctx context.Context, address string) (bool, error) {
	probeContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(probeContext, "tcp4", net.JoinHostPort(address, "22"))
	if err != nil {
		return false, nil
	}
	_ = connection.Close()
	return true, nil
}

func (*workerInstallSource) ReserveRemoteInstall(context.Context, domain.RemoteInstallPlan, string) (domain.RemoteInstallReservation, error) {
	return nil, errors.New("worker already owns the remote installation reservation")
}

func (source *workerInstallSource) RevalidateRemoteInstall(ctx context.Context, preparation domain.RemoteInstallPreparation, plan domain.RemoteInstallPlan) error {
	return source.connection.Revalidate(ctx, preparation, plan)
}

func (source *workerInstallSource) DispatchRemoteInstall(ctx context.Context, plan domain.RemoteInstallPlan) (app.RemoteInstallDispatch, error) {
	logID := "usb-install-" + plan.OperationID + ".log"
	receipt, err := source.connection.Dispatch(ctx, source.bundlePath, plan)
	if err != nil {
		return app.RemoteInstallDispatch{Uncertain: true, LogID: logID}, err
	}
	return app.RemoteInstallDispatch{Accepted: receipt.State != "unknown" && receipt.State != "failed", LogID: logID, Receipt: receipt}, nil
}

func (worker *remoteWorker) handlePrepare(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "blocked"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.preparer == nil {
		response.Message = "remote installation preparation is unavailable"
		return response
	}
	if request.OperationID == "" {
		return worker.handleArtifactPreparationLocked(ctx, request)
	}
	if worker.operationID != request.OperationID || worker.liveSession == nil || worker.reservation == nil {
		response.Message = "worker does not own the verified live session"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || session.Bootstrap == nil || session.Bootstrap.Host != request.Host || (session.State != "bootstrapped" && session.State != "bootstrapped-artifacts") {
		response.Message = "verified bootstrap state does not match the preparation request"
		return response
	}
	artifacts := session.Artifacts
	if artifacts == nil {
		prepared, prepareErr := worker.preparer.PrepareArtifacts(ctx, request.OperationID, request.Host)
		if prepareErr != nil {
			response.Message = prepareErr.Error()
			return response
		}
		artifacts = &prepared
		session.Artifacts = artifacts
		session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePrepare, Detail: "built pinned client and installer closures with persistent GC roots"})
		if err := worker.state.Save(session); err != nil {
			response.State = "reconciliation-required"
			response.Message = "artifacts were prepared but persistent state could not be published"
			return response
		}
	}
	preparation, err := worker.preparer.Finalize(ctx, *artifacts, *worker.liveSession)
	if err != nil {
		response.Message = err.Error()
		return response
	}
	session.Preparation = &preparation
	session.State = "prepared"
	session.Events = append(session.Events,
		domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhaseTransfer, Detail: "verified signed cache metadata and imported the immutable installer bundle"},
		domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhaseProbe, Detail: "completed final wired-NIC, resource, and disk inventory probe"},
	)
	if err := worker.state.Save(session); err != nil {
		response.State = "reconciliation-required"
		response.Message = "preparation completed remotely but persistent state could not be published"
		return response
	}
	response.State = session.State
	response.Session = &session
	response.Message = "signed installer bundle transferred and final live inventory verified"
	return response
}

func (worker *remoteWorker) handleArtifactPreparationLocked(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{State: "blocked"}
	if worker.operationID != "" || worker.reservation != nil || worker.liveSession != nil {
		if worker.operationID != "" {
			if session, err := worker.state.Load(worker.operationID); err == nil && session.Artifacts != nil &&
				session.Artifacts.HostName == request.Host && session.Bootstrap == nil && session.State == "artifacts-ready" {
				response.OperationID = session.OperationID
				response.State = session.State
				response.Session = &session
				response.Message = "target-independent installation artifacts are already prepared"
				return response
			}
		}
		response.Message = "another remote installation session is already owned by this worker"
		return response
	}
	operationID, err := domain.NewRemoteOperationID()
	if err != nil {
		response.Message = "could not create remote installation identity"
		return response
	}
	reservation, err := worker.reservations.ReserveRemoteSession(operationID)
	if err != nil {
		response.Message = err.Error()
		return response
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
		LogID: "usb-install-" + operationID + ".log", State: "preparing-artifacts",
		Events: []domain.RemoteInstallProgress{{Phase: domain.RemoteInstallPhasePrepare, Detail: "started target-independent client and installer artifact preparation"}},
	}
	if err := worker.state.Save(session); err != nil {
		if releaseErr := reservation.ReleaseResolved(); releaseErr != nil {
			worker.operationID = operationID
			worker.reservation = reservation
			response.OperationID = operationID
			response.State = "reconciliation-required"
			response.Message = "artifact preparation state and reservation cleanup both failed"
			return response
		}
		response.Message = "could not publish artifact preparation state"
		return response
	}
	worker.operationID = operationID
	worker.reservation = reservation
	artifacts, err := worker.preparer.PrepareArtifacts(ctx, operationID, request.Host)
	if err != nil {
		session.State = "artifact-preparation-failed"
		session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePrepare, Detail: "target-independent artifact preparation failed before a live client was connected"})
		stateErr := worker.state.Save(session)
		discardErr := worker.preparer.Discard(operationID)
		releaseErr := reservation.ReleaseResolved()
		if stateErr != nil || discardErr != nil || releaseErr != nil {
			response.OperationID = operationID
			response.State = "reconciliation-required"
			response.Session = &session
			response.Message = "artifact preparation failed and cleanup could not be fully confirmed"
			return response
		}
		worker.operationID = ""
		worker.reservation = nil
		response.OperationID = operationID
		response.State = session.State
		response.Session = &session
		response.Message = err.Error()
		return response
	}
	session.State = "artifacts-ready"
	session.Artifacts = &artifacts
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePrepare, Detail: "built target-independent pinned client and installer closures with persistent GC roots"})
	if err := worker.state.Save(session); err != nil {
		response.State = "reconciliation-required"
		response.OperationID = operationID
		response.Message = "artifacts were prepared but persistent state could not be published"
		return response
	}
	response.OperationID = operationID
	response.State = session.State
	response.Session = &session
	response.Message = "target-independent installation artifacts are prepared; no client endpoint or disk has been accepted"
	return response
}

func (worker *remoteWorker) handleBootstrap(ctx context.Context, request domain.RemoteInstallRequest, secret *adapters.LivePassword) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{State: "blocked"}
	if secret == nil {
		response.Message = "bootstrap password frame is missing"
		return response
	}
	defer secret.Destroy()
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	var session domain.RemoteInstallSession
	operationID := worker.operationID
	reservation := worker.reservation
	reusingArtifacts := false
	if operationID != "" || reservation != nil {
		stored, err := worker.state.Load(operationID)
		if err == nil && reservation != nil && worker.liveSession == nil && stored.State == "reconciliation-required" && stored.Bootstrap != nil {
			return worker.handleRecoveryBootstrapLocked(ctx, request, secret, stored)
		}
		if err != nil || reservation == nil || worker.liveSession != nil || stored.Artifacts == nil || stored.Bootstrap != nil ||
			stored.State != "artifacts-ready" || stored.Artifacts.HostName != request.Host {
			response.Message = "another remote installation session is already owned by this worker"
			return response
		}
		session = stored
		reusingArtifacts = true
	} else {
		var err error
		operationID, err = domain.NewRemoteOperationID()
		if err != nil {
			response.Message = "could not create remote installation identity"
			return response
		}
		reservation, err = worker.reservations.ReserveRemoteSession(operationID)
		if err != nil {
			response.Message = err.Error()
			return response
		}
		session = domain.RemoteInstallSession{
			SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID,
			LogID: "usb-install-" + operationID + ".log", Events: []domain.RemoteInstallProgress{},
		}
	}
	keepReservation := reusingArtifacts
	defer func() {
		if !keepReservation {
			_ = reservation.ReleaseResolved()
		}
	}()
	liveSession, err := worker.bootstrap.Establish(ctx, operationID, request.Address, request.Fingerprint, secret)
	if err != nil {
		if adapters.BootstrapCleanupUnconfirmed(err) {
			keepReservation = true
			worker.operationID = operationID
			worker.reservation = reservation
			response.State = "reconciliation-required"
			response.OperationID = operationID
		}
		if reusingArtifacts {
			response.OperationID = operationID
			response.State = session.State
		}
		response.Message = err.Error()
		return response
	}
	session.State = "bootstrapped"
	if session.Artifacts != nil {
		session.State = "bootstrapped-artifacts"
	}
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePreflight, Detail: "verified supported live installer and replaced temporary password authentication"})
	session.Bootstrap = &domain.RemoteInstallBootstrapRecord{
		Host: request.Host, Address: request.Address, HostPublicKey: liveSession.HostPublicKey,
		HostFingerprint: liveSession.HostFingerprint, AuthorizedKeyLine: liveSession.PublicKeyLine, Facts: liveSession.Facts,
	}
	if err := worker.state.Save(session); err != nil {
		if cleanupErr := worker.bootstrap.CloseSession(ctx, liveSession); cleanupErr != nil {
			keepReservation = true
			worker.operationID = operationID
			worker.liveSession = &liveSession
			worker.reservation = reservation
			response.State = "reconciliation-required"
			response.OperationID = operationID
			response.Message = "persistent state failed and live key cleanup was not confirmed; reservation retained"
			return response
		}
		if reusingArtifacts {
			response.OperationID = operationID
			response.State = "artifacts-ready"
			response.Message = "live bootstrap state could not be published; prepared artifacts remain reserved"
			return response
		}
		response.Message = "could not publish verified bootstrap state"
		return response
	}
	keepReservation = true
	worker.operationID = operationID
	worker.liveSession = &liveSession
	worker.reservation = reservation
	response.State = session.State
	response.OperationID = operationID
	response.Session = &session
	response.Message = "live installer identity verified; password authentication is locked"
	if reusingArtifacts {
		response.Message = "live installer identity verified and attached to the prepared artifacts; password authentication is locked"
	}
	return response
}

func (worker *remoteWorker) handleRecoveryBootstrapLocked(ctx context.Context, request domain.RemoteInstallRequest, secret *adapters.LivePassword, session domain.RemoteInstallSession) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: session.OperationID, State: "reconciliation-required", Session: &session}
	previous := session.Bootstrap
	if request.Host != previous.Host || request.Address != previous.Address || request.Fingerprint != previous.HostFingerprint {
		response.Message = "recovery endpoint differs from the reserved physical session; recheck the original live address and fingerprint"
		return response
	}
	liveSession, err := worker.bootstrap.Establish(ctx, session.OperationID, request.Address, request.Fingerprint, secret)
	if err != nil {
		response.Message = err.Error()
		return response
	}
	if liveSession.Facts.BootID != previous.Facts.BootID {
		cleanupErr := worker.bootstrap.CloseSession(ctx, liveSession)
		session.Events = append(session.Events, domain.RemoteInstallProgress{
			Phase:  domain.RemoteInstallPhaseReconciliationRequired,
			Detail: "physical recovery observed a different live boot ID; prior destructive authorization remains invalid",
		})
		_ = worker.state.Save(session)
		response.Session = &session
		response.Message = "live installer boot ID changed; no prior apply authorization can be resumed"
		if cleanupErr != nil {
			response.Message += "; recovery key cleanup was not confirmed"
		}
		return response
	}
	session.Bootstrap = &domain.RemoteInstallBootstrapRecord{
		Host: request.Host, Address: request.Address, HostPublicKey: liveSession.HostPublicKey,
		HostFingerprint: liveSession.HostFingerprint, AuthorizedKeyLine: liveSession.PublicKeyLine, Facts: liveSession.Facts,
	}
	preApplyRecovery := !session.TokenConsumed && !session.DispatchUncertain && session.Receipt == nil
	if preApplyRecovery {
		session.State = "bootstrapped"
		if session.Artifacts != nil {
			session.State = "bootstrapped-artifacts"
		}
		if session.Preparation != nil {
			session.State = "prepared"
		}
	}
	session.Events = append(session.Events, domain.RemoteInstallProgress{
		Phase:  domain.RemoteInstallPhaseReconciliationRequired,
		Detail: "restored private live-session access after a physical re-pin",
	})
	if err := worker.state.Save(session); err != nil {
		_ = worker.bootstrap.CloseSession(ctx, liveSession)
		response.Message = "recovery access was verified but its non-secret binding could not be persisted"
		return response
	}
	worker.liveSession = &liveSession
	response.State = "recovery-attached"
	response.Session = &session
	response.Message = "live recovery access restored for status reconciliation; apply remains consumed and cannot be replayed"
	if preApplyRecovery {
		response.Message = "live recovery access restored; the verified pre-apply session may be cancelled or reviewed again"
	}
	return response
}

func (worker *remoteWorker) handleCancel(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "reconciliation-required"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.reservation == nil {
		response.Message = "worker no longer owns the remote installation reservation"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || !remoteSessionSafelyCancellable(session) || (session.Bootstrap != nil && worker.liveSession == nil) {
		response.Message = "session is not in a safely cancellable state"
		return response
	}
	if worker.liveSession != nil {
		if err := worker.bootstrap.CloseSession(ctx, *worker.liveSession); err != nil {
			response.Message = "live key cleanup was not confirmed; reservation retained"
			return response
		}
	}
	session.State = "cancelled"
	detail := "cancelled artifact preparation before a live client was connected"
	if worker.liveSession != nil {
		detail = "cancelled before apply; ephemeral live key revoked"
		if session.Receipt != nil {
			detail = "cancelled after a confirmed remote failure before disk mutation; ephemeral live key revoked"
		}
	}
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePreflight, Detail: detail})
	if err := worker.state.Save(session); err != nil {
		response.Message = "cleanup succeeded but persistent state could not be updated; reservation retained"
		return response
	}
	if session.Artifacts != nil || session.Preparation != nil {
		if worker.preparer == nil {
			response.Message = "cleanup succeeded but preparation GC root ownership is unavailable; reservation retained"
			return response
		}
		if err := worker.preparer.Discard(session.OperationID); err != nil {
			response.Message = "cleanup succeeded but preparation GC roots could not be removed; reservation retained"
			return response
		}
	}
	if err := worker.reservation.ReleaseResolved(); err != nil {
		response.Message = "cleanup succeeded but the controller reservation could not be released"
		return response
	}
	worker.operationID = ""
	worker.liveSession = nil
	worker.reservation = nil
	response.State = session.State
	response.Session = &session
	response.Message = "remote installation cancelled with no disk mutation"
	return response
}

func remoteSessionSafelyCancellable(session domain.RemoteInstallSession) bool {
	if session.DispatchUncertain {
		return false
	}
	if !session.TokenConsumed {
		switch session.State {
		case "artifacts-ready", "bootstrapped", "bootstrapped-artifacts", "prepared", "review-ready":
			return true
		}
	}
	return session.State == "failed" && remoteSessionConfirmedNoMutationFailure(session)
}

func remoteSessionConfirmedNoMutationFailure(session domain.RemoteInstallSession) bool {
	receipt := session.Receipt
	return receipt != nil && receipt.OperationID == session.OperationID && receipt.State == "failed" &&
		!receipt.MutationStarted && !receipt.DiskMayBeModified && !receipt.Installed && !session.DispatchUncertain
}

func (worker *remoteWorker) resolveConfirmedFailureLocked(ctx context.Context, session *domain.RemoteInstallSession) error {
	if !remoteSessionSafelyCancellable(*session) || worker.liveSession == nil || worker.reservation == nil {
		return errors.New("confirmed failure does not have a complete verified live session")
	}
	if err := worker.bootstrap.CloseSession(ctx, *worker.liveSession); err != nil {
		return fmt.Errorf("live key cleanup was not confirmed; reservation retained: %w", err)
	}
	worker.liveSession = nil
	session.State = "failed-resolved"
	session.Events = append(session.Events, domain.RemoteInstallProgress{
		Phase:  session.Receipt.Phase,
		Detail: "confirmed remote failure before disk mutation; ephemeral live key revoked",
	})
	if err := worker.state.Save(*session); err != nil {
		stateErr := fmt.Errorf("persist resolved failure state: %w", err)
		if releaseErr := worker.releaseResolvedFailureLocked(session); releaseErr != nil {
			return fmt.Errorf("%v; %w", stateErr, releaseErr)
		}
		return stateErr
	}
	return worker.releaseResolvedFailureLocked(session)
}

func (worker *remoteWorker) releaseResolvedFailureLocked(session *domain.RemoteInstallSession) error {
	if session.State != "failed-resolved" || !remoteSessionConfirmedNoMutationFailure(*session) || worker.reservation == nil {
		return errors.New("resolved failure state is incomplete")
	}
	var cleanupErr error
	if worker.preparer == nil {
		cleanupErr = errors.New("preparation cleanup is unavailable")
	} else if err := worker.preparer.Discard(session.OperationID); err != nil {
		cleanupErr = fmt.Errorf("discard preparation roots: %w", err)
	}
	if err := worker.reservation.ReleaseResolved(); err != nil {
		if cleanupErr != nil {
			return fmt.Errorf("%v; release controller reservation: %w", cleanupErr, err)
		}
		return fmt.Errorf("release controller reservation: %w", err)
	}
	worker.operationID = ""
	worker.liveSession = nil
	worker.reservation = nil
	if cleanupErr != nil {
		session.Events = append(session.Events, domain.RemoteInstallProgress{
			Phase:  session.Receipt.Phase,
			Detail: "controller reservation released after confirmed pre-mutation failure; preparation-root cleanup needs maintenance",
		})
		_ = worker.state.Save(*session)
	}
	return nil
}

func readFixedDeploymentPath(path string) (string, error) {
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
