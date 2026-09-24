package adapters

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type RemoteInstallRequestHandler func(context.Context, domain.RemoteInstallRequest, *LivePassword) domain.RemoteInstallResponse

type RemoteInstallIPCServer struct {
	socketPath string
	handler    RemoteInstallRequestHandler
}

func NewRemoteInstallIPCServer(socketPath string, handler RemoteInstallRequestHandler) *RemoteInstallIPCServer {
	return &RemoteInstallIPCServer{socketPath: socketPath, handler: handler}
}

func (server *RemoteInstallIPCServer) Serve(ctx context.Context) error {
	if server.handler == nil {
		return errors.New("remote installation IPC handler is missing")
	}
	if err := validateRemoteSocketDirectory(server.socketPath); err != nil {
		return err
	}
	if info, err := os.Lstat(server.socketPath); err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 || stat.Uid != uint32(os.Geteuid()) {
			return errors.New("existing remote installation socket is unsafe")
		}
		if err := os.Remove(server.socketPath); err != nil {
			return fmt.Errorf("remove stale remote installation socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: server.socketPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen on remote installation socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(server.socketPath)
	}()
	if err := os.Chmod(server.socketPath, 0600); err != nil {
		return fmt.Errorf("secure remote installation socket: %w", err)
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
			return fmt.Errorf("accept remote installation IPC client: %w", err)
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			defer connection.Close()
			server.serveConnection(ctx, connection)
		}()
	}
}

func (server *RemoteInstallIPCServer) serveConnection(ctx context.Context, connection *net.UnixConn) {
	if err := requireSameUIDPeer(connection); err != nil {
		return
	}
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReaderSize(connection, 4096)
	frame, err := readRemoteIPCFrameBuffered(reader, domain.RemoteInstallPlanMaxBytes)
	if err != nil {
		return
	}
	request, err := domain.DecodeRemoteInstallRequest(frame)
	if err != nil {
		return
	}
	var secret *LivePassword
	if request.Operation == domain.RemoteInstallBootstrapOperation {
		secret, err = readRemoteIPCSecret(reader)
		if err != nil {
			return
		}
		defer secret.Destroy()
	}
	response := server.handler(ctx, request, secret)
	response.SchemaVersion = domain.RemoteInstallSchemaVersion
	response.RequestID = request.RequestID
	content, err := json.Marshal(response)
	if err != nil || len(content) > domain.RemoteInstallPlanMaxBytes {
		return
	}
	content = append(content, '\n')
	_, _ = connection.Write(content)
}

func RemoteInstallIPCRequest(ctx context.Context, socketPath string, request domain.RemoteInstallRequest) (domain.RemoteInstallResponse, error) {
	if request.Operation == domain.RemoteInstallBootstrapOperation {
		return domain.RemoteInstallResponse{}, errors.New("bootstrap requires the distinct secret IPC frame")
	}
	return remoteInstallIPCRequest(ctx, socketPath, request, nil)
}

func RemoteInstallIPCSecretRequest(ctx context.Context, socketPath string, request domain.RemoteInstallRequest, secret *LivePassword) (domain.RemoteInstallResponse, error) {
	if request.Operation != domain.RemoteInstallBootstrapOperation || secret == nil {
		if secret != nil {
			secret.Destroy()
		}
		return domain.RemoteInstallResponse{}, errors.New("secret IPC frame is only valid for bootstrap")
	}
	value, err := secret.snapshot()
	secret.Destroy()
	if err != nil {
		return domain.RemoteInstallResponse{}, err
	}
	defer zeroBytes(value)
	return remoteInstallIPCRequest(ctx, socketPath, request, value)
}

func remoteInstallIPCRequest(ctx context.Context, socketPath string, request domain.RemoteInstallRequest, secret []byte) (domain.RemoteInstallResponse, error) {
	var response domain.RemoteInstallResponse
	content, err := json.Marshal(request)
	if err != nil || len(content) > domain.RemoteInstallPlanMaxBytes {
		return response, errors.New("remote installation request cannot be encoded safely")
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return response, fmt.Errorf("connect to remote installation worker: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	}
	frame := append(content, '\n')
	if secret != nil {
		length := make([]byte, 2)
		binary.BigEndian.PutUint16(length, uint16(len(secret)))
		frame = append(frame, length...)
		frame = append(frame, secret...)
	}
	if _, err := connection.Write(frame); err != nil {
		return response, fmt.Errorf("send remote installation request: %w", err)
	}
	frame, err = readRemoteIPCFrame(connection, domain.RemoteInstallPlanMaxBytes)
	if err != nil {
		return response, fmt.Errorf("read remote installation response: %w", err)
	}
	response, err = domain.DecodeRemoteInstallResponse(frame)
	if err != nil {
		return response, err
	}
	if response.SchemaVersion != domain.RemoteInstallSchemaVersion || response.RequestID != request.RequestID {
		return response, errors.New("remote installation response identity is invalid")
	}
	return response, nil
}

func readRemoteIPCFrame(reader io.Reader, maximum int) ([]byte, error) {
	return readRemoteIPCFrameBuffered(bufio.NewReaderSize(reader, 4096), maximum)
}

func readRemoteIPCFrameBuffered(buffered *bufio.Reader, maximum int) ([]byte, error) {
	result := make([]byte, 0, 4096)
	for {
		fragment, err := buffered.ReadSlice('\n')
		if len(result)+len(fragment) > maximum+1 {
			return nil, errors.New("remote installation IPC frame exceeds the size limit")
		}
		result = append(result, fragment...)
		if err == nil {
			if len(result) < 2 {
				return nil, errors.New("remote installation IPC frame is empty")
			}
			return result[:len(result)-1], nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return nil, err
		}
	}
}

func readRemoteIPCSecret(reader io.Reader) (*LivePassword, error) {
	lengthBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, lengthBytes); err != nil {
		return nil, errors.New("read remote installation secret frame length")
	}
	length := int(binary.BigEndian.Uint16(lengthBytes))
	if length < 1 || length > 1024 {
		return nil, errors.New("remote installation secret frame has an invalid size")
	}
	value := make([]byte, length)
	if _, err := io.ReadFull(reader, value); err != nil {
		zeroBytes(value)
		return nil, errors.New("read remote installation secret frame")
	}
	return NewLivePassword(value)
}

func requireSameUIDPeer(connection *net.UnixConn) error {
	raw, err := connection.SyscallConn()
	if err != nil {
		return err
	}
	var credentials *syscall.Ucred
	var controlErr error
	if err := raw.Control(func(descriptor uintptr) {
		credentials, controlErr = syscall.GetsockoptUcred(int(descriptor), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if controlErr != nil || credentials == nil || credentials.Uid != uint32(os.Geteuid()) {
		return errors.New("remote installation IPC peer is not the worker user")
	}
	return nil
}

func validateRemoteSocketDirectory(socketPath string) error {
	directory := filepathDir(socketPath)
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect remote installation runtime directory: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("remote installation runtime directory must be private and owned by the worker")
	}
	return nil
}

func filepathDir(path string) string {
	index := len(path) - 1
	for index >= 0 && path[index] != os.PathSeparator {
		index--
	}
	if index <= 0 {
		return string(os.PathSeparator)
	}
	return path[:index]
}
