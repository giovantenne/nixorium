package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const ClassroomSocketPath = "/run/nixorium-classroom/control.sock"

type ClassroomRequestHandler func(context.Context, domain.ClassroomRequest) domain.ClassroomResponse

type ClassroomIPCServer struct {
	socketPath string
	handler    ClassroomRequestHandler
}

func NewClassroomIPCServer(socketPath string, handler ClassroomRequestHandler) *ClassroomIPCServer {
	return &ClassroomIPCServer{socketPath: socketPath, handler: handler}
}

func (server *ClassroomIPCServer) Serve(ctx context.Context) error {
	if server.handler == nil {
		return errors.New("classroom IPC handler is missing")
	}
	if err := validateClassroomSocketDirectory(server.socketPath); err != nil {
		return err
	}
	if info, err := os.Lstat(server.socketPath); err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0660 || stat.Uid != uint32(os.Geteuid()) || stat.Gid != uint32(os.Getegid()) {
			return errors.New("existing classroom socket is unsafe")
		}
		if err := os.Remove(server.socketPath); err != nil {
			return fmt.Errorf("remove stale classroom socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: server.socketPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen on classroom socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(server.socketPath)
	}()
	if err := os.Chmod(server.socketPath, 0660); err != nil {
		return fmt.Errorf("secure classroom socket: %w", err)
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	var clients sync.WaitGroup
	defer clients.Wait()
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept classroom IPC client: %w", err)
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			defer connection.Close()
			server.serveConnection(ctx, connection)
		}()
	}
}

func (server *ClassroomIPCServer) serveConnection(ctx context.Context, connection *net.UnixConn) {
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	frame, err := readRemoteIPCFrameBuffered(bufio.NewReaderSize(connection, 4096), domain.ClassroomMessageMaxBytes)
	if err != nil {
		return
	}
	request, err := domain.DecodeClassroomRequest(frame)
	if err != nil {
		return
	}
	_ = connection.SetDeadline(time.Time{})
	response := server.handler(ctx, request)
	response.SchemaVersion = domain.ClassroomProtocolVersion
	response.RequestID = request.RequestID
	content, err := json.Marshal(response)
	if err != nil || len(content) > domain.ClassroomMessageMaxBytes {
		return
	}
	_ = writeRemoteIPCFrame(connection, append(content, '\n'))
}

func ClassroomIPCRequest(ctx context.Context, socketPath string, request domain.ClassroomRequest) (domain.ClassroomResponse, error) {
	var response domain.ClassroomResponse
	content, err := json.Marshal(request)
	if err != nil || len(content) > domain.ClassroomMessageMaxBytes {
		return response, errors.New("classroom request cannot be encoded safely")
	}
	dialer := net.Dialer{Timeout: 3 * time.Second}
	connection, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return response, fmt.Errorf("connect to classroom worker: %w", err)
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancellation()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(2 * time.Minute))
	}
	if err := writeRemoteIPCFrame(connection, append(content, '\n')); err != nil {
		return response, fmt.Errorf("send classroom request: %w", err)
	}
	frame, err := readRemoteIPCFrame(connection, domain.ClassroomMessageMaxBytes)
	if err != nil {
		return response, fmt.Errorf("read classroom response: %w", err)
	}
	response, err = domain.DecodeClassroomResponse(frame)
	if err != nil {
		return response, err
	}
	if response.RequestID != request.RequestID {
		return domain.ClassroomResponse{}, errors.New("classroom response does not match the request")
	}
	if response.State == "failed" {
		return response, errors.New(response.Message)
	}
	return response, nil
}

func validateClassroomSocketDirectory(socketPath string) error {
	directory := filepathDir(socketPath)
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect classroom runtime directory: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0770 || stat.Uid != uint32(os.Geteuid()) || stat.Gid != uint32(os.Getegid()) {
		return errors.New("classroom runtime directory must be group-private and owned by the worker")
	}
	return nil
}
