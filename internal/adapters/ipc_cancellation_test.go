package adapters

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestIPCReadsCloseTheirConnectionOnCancellation(t *testing.T) {
	for _, kind := range []string{"classroom", "remote-install"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ipc.sock")
			listener, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			connected, disconnected := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(disconnected)
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				defer connection.Close()
				close(connected)
				_, _ = io.Copy(io.Discard, connection)
			}()
			finished := make(chan error, 1)
			go func() {
				if kind == "classroom" {
					_, err := ClassroomIPCRequest(ctx, path, domain.ClassroomRequest{Operation: domain.ClassroomOverviewOperation})
					finished <- err
				} else {
					_, err := RemoteInstallIPCRequest(ctx, path, domain.RemoteInstallRequest{Operation: domain.RemoteInstallWorkerProbeOperation})
					finished <- err
				}
			}()
			select {
			case <-connected:
			case <-time.After(5 * time.Second):
				t.Fatal("client did not connect")
			}
			cancel()
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("cancelled IPC read succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("IPC read waited for its deadline instead of cancellation")
			}
			select {
			case <-disconnected:
			case <-time.After(time.Second):
				t.Fatal("cancelled IPC connection stayed open")
			}
		})
	}
}
