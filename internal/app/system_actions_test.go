package app

import (
	"context"
	"errors"
	"testing"
)

type fakeSystemActionSource struct {
	unit string
	err  error
}

func (f *fakeSystemActionSource) StartSystemUnit(_ context.Context, unit string) error {
	f.unit = unit
	return f.err
}

func TestInstallSecretsStartsOnlyFixedUnit(t *testing.T) {
	source := &fakeSystemActionSource{}
	report := NewSystemActions(source).InstallSecrets(context.Background())
	if report.HasErrors() || source.unit != InstallSecretsUnit || report.Operation != "setup-install-secrets" {
		t.Fatalf("report = %+v, unit = %q", report, source.unit)
	}
	source.err = errors.New("denied")
	if report := NewSystemActions(source).InstallSecrets(context.Background()); !report.HasErrors() {
		t.Fatalf("failure report = %+v", report)
	}
}

func TestApplyControllerStartsOnlyFixedUnit(t *testing.T) {
	source := &fakeSystemActionSource{}
	report := NewSystemActions(source).ApplyController(context.Background())
	if report.HasErrors() || source.unit != ApplyControllerUnit || report.Operation != "setup-apply-controller" {
		t.Fatalf("report = %+v, unit = %q", report, source.unit)
	}
}

func TestPreparePXEStartsOnlyFixedUnit(t *testing.T) {
	source := &fakeSystemActionSource{}
	report := NewSystemActions(source).PreparePXE(context.Background())
	if report.HasErrors() || source.unit != PreparePXEUnit || report.Operation != "pxe-prepare" {
		t.Fatalf("report = %+v, unit = %q", report, source.unit)
	}
	source.err = errors.New("not ready")
	if report := NewSystemActions(source).PreparePXE(context.Background()); !report.HasErrors() {
		t.Fatalf("failure report = %+v", report)
	}
}
