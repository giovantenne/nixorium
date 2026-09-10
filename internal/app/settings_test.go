package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSettingsSource struct {
	data         []byte
	readErr      error
	evalErr      error
	candidateErr error
	writeErr     error
	written      *domain.LabSettingsFile
}

func (f fakeSettingsSource) ReadSettings(string) ([]byte, error) {
	return f.data, f.readErr
}

func (f fakeSettingsSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return domain.LabMeta{}, f.evalErr
}

func (f fakeSettingsSource) ValidateCandidate(context.Context, string, domain.LabSettingsFile) error {
	return f.candidateErr
}

func (f fakeSettingsSource) WriteSettingsIfUnchanged(_ string, expected []byte, settings domain.LabSettingsFile) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if !bytes.Equal(expected, f.data) {
		return domain.ErrSettingsConflict
	}
	if f.written != nil {
		*f.written = settings
	}
	return nil
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

func TestPlanAndApplySettingsRequireReviewedFingerprint(t *testing.T) {
	baseData, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate, issues := domain.DecodeLabSettings(baseData)
	if len(issues) != 0 {
		t.Fatalf("template issues = %+v", issues)
	}
	candidate.Lab.MasterDHCPIP = "192.0.2.10"
	candidate.Lab.TeacherPassword = "$6$new$teacher"
	candidateData, err := domain.MarshalLabSettings(candidate)
	if err != nil {
		t.Fatal(err)
	}
	written := domain.LabSettingsFile{}
	manager := NewSettingsManager(fakeSettingsSource{data: baseData, written: &written})
	plan := manager.Plan(context.Background(), "/repo", candidateData)
	if plan.State != "valid" || plan.BaseFingerprint == "" || len(plan.Changes) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Changes[1].Field != "lab.teacherPassword" || !plan.Changes[1].Sensitive || stringsContainHash(plan.Changes[1]) {
		t.Fatalf("password change was not redacted: %+v", plan.Changes[1])
	}

	conflict := manager.Apply(context.Background(), "/repo", candidateData, "sha256:stale")
	if conflict.State != "conflict" || !conflict.HasErrors() || written.SchemaVersion != 0 {
		t.Fatalf("conflict = %+v, written = %+v", conflict, written)
	}
	applied := manager.Apply(context.Background(), "/repo", candidateData, plan.BaseFingerprint)
	if applied.State != "applied" || applied.HasErrors() || written.Lab.MasterDHCPIP != "192.0.2.10" {
		t.Fatalf("applied = %+v, written = %+v", applied, written)
	}
}

func TestPlanRejectsCandidateBeforeWriteWhenNixFails(t *testing.T) {
	data, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	manager := NewSettingsManager(fakeSettingsSource{data: data, candidateErr: errors.New("candidate failed")})
	plan := manager.Plan(context.Background(), "/repo", data)
	if plan.State != "invalid" || len(plan.Issues) != 1 || plan.Issues[0].Field != "$nix" {
		t.Fatalf("plan = %+v", plan)
	}
}

func stringsContainHash(change domain.SettingChange) bool {
	before, _ := change.Before.(string)
	after, _ := change.After.(string)
	return bytes.Contains([]byte(before), []byte("$6$")) || bytes.Contains([]byte(after), []byte("$6$"))
}
