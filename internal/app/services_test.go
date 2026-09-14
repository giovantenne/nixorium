package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestServiceStatusTreatsInactivePXEAsHealthyStandby(t *testing.T) {
	source := readyPXESource()
	report := NewServiceManager(source).Status(context.Background(), ".")
	if report.HasErrors() || report.State != "healthy" || len(report.Services) != 2 {
		t.Fatalf("unexpected service report: %+v", report)
	}
	if cache := report.Services[0]; cache.ID != CacheServiceID || !cache.Healthy || cache.State != "healthy" {
		t.Fatalf("unexpected cache state: %+v", cache)
	}
	if pxe := report.Services[1]; pxe.ID != "pxe" || !pxe.Healthy || pxe.State != "ready" || len(pxe.Actions) != 0 {
		t.Fatalf("unexpected PXE state: %+v", pxe)
	}
}

func TestServiceStatusSurfacesCacheAndPXEFailures(t *testing.T) {
	source := readyPXESource()
	source.cacheErr = errors.New("connection refused")
	network := source.services[PXENetworkUnit]
	network.State = "failed"
	source.services[PXENetworkUnit] = network
	report := NewServiceManager(source).Status(context.Background(), ".")
	if !report.HasErrors() || report.Services[0].State != "unhealthy" || report.Services[1].State != "degraded" {
		t.Fatalf("failures were not surfaced: %+v", report)
	}
}

func TestRestartCacheUsesExactUnitAndVerifiesHealth(t *testing.T) {
	source := readyPXESource()
	report := NewServiceManager(source).Restart(context.Background(), ".", CacheServiceID)
	if report.HasErrors() || !report.Verified || report.State != "completed" {
		t.Fatalf("unexpected restart report: %+v", report)
	}
	if len(source.controls) != 1 || source.controls[0] != "start "+CacheRestartUnit {
		t.Fatalf("controls = %v", source.controls)
	}
}

func TestRestartRejectsPXEAndUnknownServices(t *testing.T) {
	for _, service := range []string{"pxe", "sshd"} {
		source := readyPXESource()
		report := NewServiceManager(source).Restart(context.Background(), ".", service)
		if !report.HasErrors() || len(source.controls) != 0 || !strings.Contains(report.Message, "not started") {
			t.Fatalf("unsafe restart %q was not rejected: %+v controls=%v", service, report, source.controls)
		}
	}
}

func TestRestartReportsControlAndVerificationFailures(t *testing.T) {
	source := readyPXESource()
	source.controlErr["start "+CacheRestartUnit] = errors.New("restart failed")
	report := NewServiceManager(source).Restart(context.Background(), ".", CacheServiceID)
	if report.State != "failed" || !strings.Contains(report.Message, "restart failed") {
		t.Fatalf("control failure not preserved: %+v", report)
	}

	source = readyPXESource()
	transition := &serviceHealthTransitionSource{fakePXELifecycleSource: source}
	report = NewServiceManager(transition).Restart(context.Background(), ".", CacheServiceID)
	if report.State != "failed" || report.Verified || !strings.Contains(report.Message, "verification failed") {
		t.Fatalf("verification failure not preserved: %+v", report)
	}
}

type serviceHealthTransitionSource struct {
	*fakePXELifecycleSource
	calls int
}

func (f *serviceHealthTransitionSource) CacheHealth(context.Context, string, int) error {
	f.calls++
	if f.calls > 1 {
		return errors.New("unhealthy after restart")
	}
	return nil
}
