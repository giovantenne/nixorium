package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	workerSocketPath = "/run/nixorium/remote-install/control.sock"
	workerStatePath  = "/var/lib/nixorium/remote-install"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nixorium remote installation worker:", err)
		os.Exit(1)
	}
}

func run() error {
	state, err := adapters.NewRemoteInstallStateStore(workerStatePath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	server := adapters.NewRemoteInstallIPCServer(workerSocketPath, workerHandler(state))
	return server.Serve(ctx)
}

func workerHandler(state *adapters.RemoteInstallStateStore) adapters.RemoteInstallRequestHandler {
	return func(_ context.Context, request domain.RemoteInstallRequest) domain.RemoteInstallResponse {
		response := domain.RemoteInstallResponse{OperationID: request.OperationID}
		switch request.Operation {
		case domain.RemoteInstallWorkerProbeOperation:
			response.State = "ready"
			response.Message = "remote installation worker protocol is ready"
		case domain.RemoteInstallStatusOperation, domain.RemoteInstallReconcileOperation:
			session, err := state.Load(request.OperationID)
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
		default:
			response.State = "unavailable"
			response.Message = "operation is not enabled before verified live-session bootstrap"
		}
		return response
	}
}
