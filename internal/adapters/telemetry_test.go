package adapters

import (
	"context"
	"encoding/json"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type telemetryRoundTrip func(*http.Request) (*http.Response, error)

func (f telemetryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func telemetryFixture(t *testing.T) (Telemetry, *app.TelemetryManager, *int) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := Telemetry{Directory: dir, ConfigPath: filepath.Join(dir, "config"), MachinePath: filepath.Join(dir, "machine")}
	os.WriteFile(a.ConfigPath, []byte(`{"schemaVersion":1,"version":"3.0.0-beta.1","deploymentMode":"laboratory","configuredClients":"16-30"}`), 0600)
	os.WriteFile(a.MachinePath, []byte(strings.Repeat("a", 32)), 0600)
	count := new(int)
	a.Client = &http.Client{Transport: telemetryRoundTrip(func(r *http.Request) (*http.Response, error) {
		*count++
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), strings.Repeat("a", 32)) || strings.Contains(string(b), "secret") || r.URL.String() != domain.TelemetryEndpoint {
			t.Error("private data or wrong destination")
		}
		var p domain.TelemetryPayload
		if e := json.Unmarshal(b, &p); e != nil {
			t.Error(e)
		}
		if len(p.MonthlyID) != 43 || p.ConfiguredClients != "16-30" {
			t.Error("invalid payload")
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	return a, app.NewTelemetryManager(a), count
}
func TestTelemetryConsentLifecycle(t *testing.T) {
	a, m, sends := telemetryFixture(t)
	ctx := context.Background()
	r, e := m.Run(ctx, "preview")
	if e != nil || r.Consent != "undecided" || r.Payload.MonthlyID != "<generated after consent>" {
		t.Fatal(r, e)
	}
	if e = m.Send(ctx); e != nil {
		t.Fatal(e)
	}
	if *sends != 0 {
		t.Fatal("sent without consent")
	}
	r, e = m.Run(ctx, "enable")
	if e != nil {
		t.Fatal(e)
	}
	id := r.Payload.MonthlyID
	if e = m.Send(ctx); e != nil {
		t.Fatal(e)
	}
	if e = m.Send(ctx); e != nil {
		t.Fatal(e)
	}
	if *sends != 1 {
		t.Fatal("daily bound", *sends)
	}
	r, e = m.Run(ctx, "status")
	if e != nil || r.LastSuccess == "" {
		t.Fatal(r, e)
	}
	if _, e = m.Run(ctx, "disable"); e != nil {
		t.Fatal(e)
	}
	m.Send(ctx)
	if *sends != 1 {
		t.Fatal("sent after disable")
	}
	data, _ := os.ReadFile(filepath.Join(a.Directory, "state.json"))
	if strings.Contains(string(data), "secret") {
		t.Fatal("secret retained")
	}
	r, e = m.Run(ctx, "enable")
	if e != nil || r.Payload.MonthlyID == id {
		t.Fatal("identity reused", e)
	}
	os.WriteFile(a.MachinePath, []byte(strings.Repeat("b", 32)), 0600)
	m.Send(ctx)
	r, e = m.Run(ctx, "status")
	if e != nil || r.Consent != "undecided" || *sends != 1 {
		t.Fatal("clone inherited consent", r, e)
	}
}
func TestTelemetryCorruptionAndSymlinksFailClosed(t *testing.T) {
	a, m, sends := telemetryFixture(t)
	path := filepath.Join(a.Directory, "state.json")
	os.WriteFile(path, []byte(`{"consent":"enabled"}`), 0600)
	if m.Send(context.Background()) == nil {
		t.Fatal("corrupt state accepted")
	}
	if _, e := m.Run(context.Background(), "disable"); e != nil {
		t.Fatal(e)
	}
	os.Remove(path)
	os.Symlink(a.ConfigPath, path)
	if _, e := m.Run(context.Background(), "enable"); e == nil {
		t.Fatal("symlink accepted")
	}
	if *sends != 0 {
		t.Fatal("sent")
	}
}
func TestTelemetryDisableWaitsForInFlightSend(t *testing.T) {
	a, _, _ := telemetryFixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	a.Client = &http.Client{Transport: telemetryRoundTrip(func(*http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	m := app.NewTelemetryManager(a)
	m.Run(context.Background(), "enable")
	sent := make(chan error, 1)
	go func() { sent <- m.Send(context.Background()) }()
	<-started
	disabled := make(chan error, 1)
	go func() { _, e := m.Run(context.Background(), "disable"); disabled <- e }()
	select {
	case e := <-disabled:
		t.Fatalf("disable returned during request: %v", e)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if e := <-sent; e != nil {
		t.Fatal(e)
	}
	if e := <-disabled; e != nil {
		t.Fatal(e)
	}
	r, e := m.Run(context.Background(), "status")
	if e != nil || r.Consent != "disabled" {
		t.Fatal(r, e)
	}
}
func TestTelemetryBackupSourcesExcludeConsent(t *testing.T) {
	for _, s := range backupSources("/deployment") {
		if strings.Contains(s.root, "/var/lib/nixorium") {
			t.Fatal("backup includes telemetry")
		}
	}
}

func TestTelemetryEvidenceRequiresAuthenticatedInstalledSystem(t *testing.T) {
	a, m, _ := telemetryFixture(t)
	ctx := context.Background()
	valid := map[string]domain.HostSystemProbe{"pc01": {SystemPath: "/nix/store/" + strings.Repeat("0", 32) + "-nixos-system-pc01-26.05", Revision: strings.Repeat("a", 40)}}
	a.rememberBoot(valid)
	r, _ := m.Run(ctx, "preview")
	if r.Payload.ClientBootVerified != nil {
		t.Fatal("collected without consent")
	}
	m.Run(ctx, "enable")
	for _, probe := range []domain.HostSystemProbe{{}, {SystemPath: "/nix/store/live-iso"}, {SystemPath: valid["pc01"].SystemPath, Revision: valid["pc01"].Revision, Detail: "unverified"}} {
		a.rememberBoot(map[string]domain.HostSystemProbe{"pc01": probe})
		r, _ = m.Run(ctx, "preview")
		if r.Payload.ClientBootVerified != nil {
			t.Fatal("unverified evidence accepted")
		}
	}
	a.rememberBoot(valid)
	r, _ = m.Run(ctx, "preview")
	if r.Payload.ClientBootVerified == nil || !*r.Payload.ClientBootVerified {
		t.Fatal("verified boot lost")
	}
}
