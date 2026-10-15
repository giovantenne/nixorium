package npmartifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunResolver is the fixed helper inside a network and filesystem namespace.
// The only reachable network service is a broker over its mounted Unix socket.
func RunResolver(node, name, version string) error {
	if !strings.HasPrefix(node, "/nix/store/") || strings.Contains(node, "..") || !ValidName(name) || !ValidVersion(version) {
		return errors.New("invalid_arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", "/run/registry.sock")
	}}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	listener, err := net.Listen("tcp", "127.0.0.1:4873")
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != "GET" {
			http.Error(w, "unsupported", 400)
			return
		}
		upstream, err := http.NewRequestWithContext(ctx, "GET", "http://registry"+request.URL.RequestURI(), nil)
		if err != nil {
			http.Error(w, "unsupported", 400)
			return
		}
		response, err := client.Do(upstream)
		if err != nil {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(response.Body, MaxMetadataBytes+1))
	})}
	go server.Serve(listener)
	defer server.Close()
	if err = os.MkdirAll("/tmp/work", 0700); err != nil {
		return err
	}
	if err = os.MkdirAll("/tmp/home", 0700); err != nil {
		return err
	}
	data := fmt.Sprintf("{\"name\":\"nixorium-npm-baseline\",\"version\":\"1.0.0\",\"private\":true,\"dependencies\":{%q:%q}}\n", name, version)
	if err = os.WriteFile("/tmp/work/package.json", []byte(data), 0600); err != nil {
		return err
	}
	for _, p := range []string{"/tmp/userconfig", "/tmp/globalconfig"} {
		if err = os.WriteFile(p, nil, 0600); err != nil {
			return err
		}
	}
	command := exec.CommandContext(ctx, filepath.Join(node, "bin/npm"), "install", "--package-lock-only", "--ignore-scripts", "--no-audit", "--no-fund", "--engine-strict", "--loglevel=error")
	command.Dir = "/tmp/work"
	command.Env = []string{"PATH=" + node + "/bin", "HOME=/tmp/home", "NODE_OPTIONS=--max-old-space-size=512 --disable-wasm-trap-handler", "NPM_CONFIG_REGISTRY=http://127.0.0.1:4873", "NPM_CONFIG_CACHE=/tmp/cache", "NPM_CONFIG_USERCONFIG=/tmp/userconfig", "NPM_CONFIG_GLOBALCONFIG=/tmp/globalconfig", "NPM_CONFIG_UPDATE_NOTIFIER=false"}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		return errors.New("resolution_failed")
	}
	lock, err := os.ReadFile("/tmp/work/package-lock.json")
	if err != nil || len(lock) > MaxLockBytes {
		return errors.New("limit_exceeded")
	}
	_, err = os.Stdout.Write(lock)
	return err
}
