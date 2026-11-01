package adapters

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

// serveUnixSocket listens on a private Unix socket until ctx ends and runs
// handle for every client. A stale socket owned by this process is replaced;
// anything else at the path is refused. name labels the socket in errors.
func serveUnixSocket(ctx context.Context, socketPath, name string, mode os.FileMode, requireGroup bool, handle func(context.Context, *net.UnixConn)) error {
	if info, err := os.Lstat(socketPath); err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != mode || stat.Uid != uint32(os.Geteuid()) || (requireGroup && stat.Gid != uint32(os.Getegid())) {
			return fmt.Errorf("existing %s socket is unsafe", name)
		}
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("remove stale %s socket: %w", name, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen on %s socket: %w", name, err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()
	if err := os.Chmod(socketPath, mode); err != nil {
		return fmt.Errorf("secure %s socket: %w", name, err)
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	var clients sync.WaitGroup
	var live sync.Mutex
	open := map[*net.UnixConn]struct{}{}
	closing := false
	// Handlers may run for hours, so shutdown closes their connections instead
	// of waiting for peers that never read or write.
	go func() {
		<-ctx.Done()
		live.Lock()
		closing = true
		for connection := range open {
			_ = connection.Close()
		}
		live.Unlock()
	}()
	defer clients.Wait()
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept %s IPC client: %w", name, err)
		}
		live.Lock()
		if closing {
			live.Unlock()
			_ = connection.Close()
			continue
		}
		open[connection] = struct{}{}
		live.Unlock()
		clients.Add(1)
		go func() {
			defer clients.Done()
			defer func() {
				live.Lock()
				delete(open, connection)
				live.Unlock()
				_ = connection.Close()
			}()
			handle(ctx, connection)
		}()
	}
}

// ipcResponseWriteTimeout bounds how long a client may take to accept a
// response; the handler itself is not bounded.
var ipcResponseWriteTimeout = 30 * time.Second

// writeIPCResponse sends a response frame with a fresh write deadline, so a
// client that never reads cannot pin the worker goroutine.
func writeIPCResponse(connection *net.UnixConn, content []byte) error {
	_ = connection.SetWriteDeadline(time.Now().Add(ipcResponseWriteTimeout))
	return writeRemoteIPCFrame(connection, content)
}

// peerCredentials reads the kernel's record of the connecting process.
func peerCredentials(connection *net.UnixConn) (*syscall.Ucred, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return nil, err
	}
	var credentials *syscall.Ucred
	var credentialsErr error
	if err := raw.Control(func(descriptor uintptr) {
		credentials, credentialsErr = syscall.GetsockoptUcred(int(descriptor), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	if credentialsErr != nil {
		return nil, credentialsErr
	}
	if credentials == nil {
		return nil, errors.New("peer credentials are unavailable")
	}
	return credentials, nil
}
