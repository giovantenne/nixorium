package adapters

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
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
	defer clients.Wait()
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept %s IPC client: %w", name, err)
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			defer connection.Close()
			handle(ctx, connection)
		}()
	}
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
