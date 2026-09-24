package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/giovantenne/nixorium/internal/adapters"
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
	server := adapters.NewRemoteInstallIPCServer(workerSocketPath, worker.handle)
	return server.Serve(ctx)
}

type remoteState interface {
	Load(string) (domain.RemoteInstallSession, error)
	Save(domain.RemoteInstallSession) error
}

type liveBootstrapper interface {
	Establish(context.Context, string, string, string, *adapters.LivePassword) (adapters.VerifiedLiveSession, error)
	CloseSession(context.Context, adapters.VerifiedLiveSession) error
}

type remoteReservationSource interface {
	ReserveRemoteSession(string) (domain.RemoteInstallReservation, error)
}

type remotePreparer interface {
	Prepare(context.Context, string, string, adapters.VerifiedLiveSession) (domain.RemoteInstallPreparation, error)
}

type remoteWorker struct {
	mutex        sync.Mutex
	state        remoteState
	bootstrap    liveBootstrapper
	preparer     remotePreparer
	reservations remoteReservationSource
	operationID  string
	liveSession  *adapters.VerifiedLiveSession
	reservation  domain.RemoteInstallReservation
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
	case domain.RemoteInstallBootstrapOperation:
		return worker.handleBootstrap(ctx, request, secret)
	case domain.RemoteInstallPrepareOperation:
		if secret != nil {
			secret.Destroy()
		}
		return worker.handlePrepare(ctx, request)
	case domain.RemoteInstallStatusOperation, domain.RemoteInstallReconcileOperation:
		if secret != nil {
			secret.Destroy()
		}
		session, err := worker.state.Load(request.OperationID)
		if err != nil {
			response.State = "unavailable"
			if errors.Is(err, os.ErrNotExist) {
				response.Message = "remote installation operation does not exist"
			} else {
				response.Message = "remote installation state is unavailable or unsafe"
			}
			break
		}
		response.State = session.State
		response.Session = &session
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

func (worker *remoteWorker) handlePrepare(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "blocked"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.liveSession == nil || worker.reservation == nil || worker.preparer == nil {
		response.Message = "worker does not own the verified live session"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || session.Bootstrap == nil || session.Bootstrap.Host != request.Host || session.State != "bootstrapped" {
		response.Message = "verified bootstrap state does not match the preparation request"
		return response
	}
	preparation, err := worker.preparer.Prepare(ctx, request.OperationID, request.Host, *worker.liveSession)
	if err != nil {
		response.Message = err.Error()
		return response
	}
	session.Preparation = &preparation
	session.State = "prepared"
	session.Events = append(session.Events,
		domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePrepare, Detail: "built pinned client and installer closures with persistent GC roots"},
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

func (worker *remoteWorker) handleBootstrap(ctx context.Context, request domain.RemoteInstallRequest, secret *adapters.LivePassword) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{State: "blocked"}
	if secret == nil {
		response.Message = "bootstrap password frame is missing"
		return response
	}
	defer secret.Destroy()
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != "" || worker.reservation != nil {
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
	keepReservation := false
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
		response.Message = err.Error()
		return response
	}
	session := domain.RemoteInstallSession{
		SchemaVersion: domain.RemoteInstallSchemaVersion, OperationID: operationID, State: "bootstrapped",
		Events: []domain.RemoteInstallProgress{{Phase: domain.RemoteInstallPhasePreflight, Detail: "verified supported live installer and replaced temporary password authentication"}},
		Bootstrap: &domain.RemoteInstallBootstrapRecord{
			Host: request.Host, Address: request.Address, HostPublicKey: liveSession.HostPublicKey,
			HostFingerprint: liveSession.HostFingerprint, AuthorizedKeyLine: liveSession.PublicKeyLine, Facts: liveSession.Facts,
		},
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
	return response
}

func (worker *remoteWorker) handleCancel(ctx context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
	response := domain.RemoteInstallResponse{OperationID: request.OperationID, State: "reconciliation-required"}
	worker.mutex.Lock()
	defer worker.mutex.Unlock()
	if worker.operationID != request.OperationID || worker.liveSession == nil || worker.reservation == nil {
		response.Message = "worker no longer owns the live credentials; reconcile the persistent reservation"
		return response
	}
	session, err := worker.state.Load(request.OperationID)
	if err != nil || session.TokenConsumed || session.DispatchUncertain || (session.State != "bootstrapped" && session.State != "prepared") {
		response.Message = "session is not safely cancellable before apply"
		return response
	}
	if err := worker.bootstrap.CloseSession(ctx, *worker.liveSession); err != nil {
		response.Message = "live key cleanup was not confirmed; reservation retained"
		return response
	}
	session.State = "cancelled"
	session.Events = append(session.Events, domain.RemoteInstallProgress{Phase: domain.RemoteInstallPhasePreflight, Detail: "cancelled before apply; ephemeral live key revoked"})
	if err := worker.state.Save(session); err != nil {
		response.Message = "cleanup succeeded but persistent state could not be updated; reservation retained"
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
	response.Message = "remote installation cancelled before apply"
	return response
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
