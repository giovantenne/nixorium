package app

import (
	"context"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSettingsSource struct {
	data    []byte
	readErr error
	evalErr error
}

func (f fakeSettingsSource) ReadSettings(string) ([]byte, error) {
	return f.data, f.readErr
}

func (f fakeSettingsSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return domain.LabMeta{}, f.evalErr
}

func TestValidateSettingsRequiresManagedFile(t *testing.T) {
	report := NewSettingsManager(fakeSettingsSource{readErr: errors.New("missing")}).Validate(context.Background(), "/repo")
	if !report.HasErrors() || report.State != "invalid" || len(report.Issues) != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestValidateSettingsUsesNixAsFinalAuthority(t *testing.T) {
	data := []byte(`{"schemaVersion":1,"lab":{}}`)
	report := NewSettingsManager(fakeSettingsSource{data: data, evalErr: errors.New("should not run")}).Validate(context.Background(), "/repo")
	if !report.HasErrors() || report.Issues[0].Field == "$nix" {
		t.Fatalf("management validation should run before Nix: %+v", report)
	}
}
