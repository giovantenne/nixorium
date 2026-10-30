package adapters

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeUnixSocketShutdownClosesIdleClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handling := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- serveUnixSocket(ctx, path, "test", 0600, false, func(ctx context.Context, connection *net.UnixConn) {
			close(handling)
			buffer := make([]byte, 1)
			_, _ = connection.Read(buffer) // a silent client holds the handler
		})
	}()
	var connection net.Conn
	var err error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if _, statErr := os.Stat(path); statErr != nil {
			continue
		}
		if connection, err = net.Dial("unix", path); err == nil {
			break
		}
	}
	if connection == nil {
		t.Fatal(err)
	}
	defer connection.Close()
	select {
	case <-handling:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown waited for a client that never closes")
	}
}

func TestWriteIPCResponseGivesUpOnClientThatNeverReads(t *testing.T) {
	previous := ipcResponseWriteTimeout
	ipcResponseWriteTimeout = 100 * time.Millisecond
	defer func() { ipcResponseWriteTimeout = previous }()
	pair, err := socketPair()
	if err != nil {
		t.Fatal(err)
	}
	defer pair[0].Close()
	defer pair[1].Close()
	started := time.Now()
	err = writeIPCResponse(pair[0], bytes.Repeat([]byte("x"), 8<<20))
	if err == nil || time.Since(started) > 5*time.Second {
		t.Fatalf("write = %v after %v, want a timeout", err, time.Since(started))
	}
}

func socketPair() ([2]*net.UnixConn, error) {
	var pair [2]*net.UnixConn
	path := filepath.Join(os.TempDir(), "pair-"+time.Now().Format("150405.000000000")+".sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return pair, err
	}
	defer os.Remove(path)
	defer listener.Close()
	accepted := make(chan *net.UnixConn, 1)
	go func() {
		connection, _ := listener.AcceptUnix()
		accepted <- connection
	}()
	dialed, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return pair, err
	}
	pair[0], pair[1] = dialed, <-accepted
	return pair, nil
}
