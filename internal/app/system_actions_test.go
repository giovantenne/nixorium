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
