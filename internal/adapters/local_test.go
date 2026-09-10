package adapters

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRegularFileNoFollowRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(target, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readRegularFileNoFollow(link); err == nil {
		t.Fatal("secure read accepted a symlink")
	}
}

func TestGitStateCountsChangedPaths(t *testing.T) {
	directory := t.TempDir()
	if _, err := run(context.Background(), "git", "init", "-q", directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "one"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "two"), []byte("2"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := (Local{}).GitState(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dirty || state.Changes != 2 || len(state.Paths) != 2 {
		t.Fatalf("state = %+v, want two changes", state)
	}
}

func TestParseProcNetFindsListeningPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tcp")
	content := "  sl  local_address rem_address   st\n   0: 0100007F:1F90 00000000:00000000 0A 00000000:00000000\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	uses, err := parseProcNet(path, "tcp", "0A")
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 1 || uses[0].Protocol != "tcp" || uses[0].Port != 8080 {
		t.Fatalf("uses = %+v, want tcp/8080", uses)
	}
}

func TestCacheHealthValidatesNixMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprintln(writer, "StoreDir: /nix/store")
	}))
	defer server.Close()
	address, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	if err := (Local{}).CacheHealth(context.Background(), address, port); err != nil {
		t.Fatal(err)
	}
}
