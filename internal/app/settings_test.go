package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	written := domain.LabSettingsFile{}
	manager := NewSettingsManager(fakeSettingsSource{data: baseData, written: &written})
	current, err := manager.Current("/repo")
	if err != nil || current.SchemaVersion != domain.SettingsSchemaVersion {
		t.Fatalf("current = %+v, err = %v", current, err)
	}
	plan := manager.PlanSettings(context.Background(), "/repo", candidate)
	if plan.State != "valid" || plan.BaseFingerprint == "" || len(plan.Changes) != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Changes[1].Field != "lab.teacherPassword" || !plan.Changes[1].Sensitive || stringsContainHash(plan.Changes[1]) {
		t.Fatalf("password change was not redacted: %+v", plan.Changes[1])
	}

	conflict := manager.ApplySettings(context.Background(), "/repo", candidate, "sha256:stale")
	if conflict.State != "conflict" || !conflict.HasErrors() || written.SchemaVersion != 0 {
		t.Fatalf("conflict = %+v, written = %+v", conflict, written)
	}
	applied := manager.ApplySettings(context.Background(), "/repo", candidate, plan.BaseFingerprint)
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

func TestSettingsThatNoLongerValidateAreRepairedBySaving(t *testing.T) {
	template, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	// An unknown legacy field and an address rule added later.
	baseData := bytes.Replace(template, []byte(`"lab": {`), []byte(`"lab": {"obsoleteSetting": ["pc01"],`), 1)
	baseData = bytes.Replace(baseData, []byte(`"MASTER_DHCP_IP"`), []byte(`"10.0.0.50"`), 1)
	if _, issues := domain.DecodeLabSettings(baseData); len(issues) == 0 {
		t.Fatal("fixture is unexpectedly valid")
	}
	written := domain.LabSettingsFile{}
	manager := NewSettingsManager(fakeSettingsSource{data: baseData, written: &written})
	if _, err := manager.Current("/repo"); err == nil {
		t.Fatal("strict loading accepted invalid settings")
	}
	editing, problems, err := manager.CurrentForEditing("/repo")
	if err != nil || len(problems) < 2 {
		t.Fatalf("repair view = %+v %+v %v", editing, problems, err)
	}
	if problems[0].Field != "lab.obsoleteSetting" {
		t.Fatalf("obsolete field not listed first: %+v", problems)
	}
	editing.Lab.MasterDHCPIP = "192.0.2.10"
	plan := manager.PlanSettings(context.Background(), "/repo", editing)
	if plan.HasErrors() || plan.State != "valid" {
		t.Fatalf("plan = %+v", plan)
	}
	removed := false
	for _, change := range plan.Changes {
		removed = removed || (change.Field == "lab.obsoleteSetting" && change.After == "removed")
	}
	if !removed {
		t.Fatalf("removed field not reviewed: %+v", plan.Changes)
	}
	applied := manager.ApplySettings(context.Background(), "/repo", editing, plan.BaseFingerprint)
	if applied.State != "applied" || written.Lab.MasterDHCPIP != "192.0.2.10" {
		t.Fatalf("applied = %+v", applied)
	}
	if _, _, err := NewSettingsManager(fakeSettingsSource{data: []byte("not json")}).CurrentForEditing("/repo"); err == nil {
		t.Fatal("unparseable JSON was repaired")
	}
}

func TestSettingsAddressChangesCannotDisconnectConfiguredClients(t *testing.T) {
	template, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	base, issues := domain.DecodeLabSettings(template)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	for _, field := range []string{"lab.networkBase", "lab.networkPrefixLength", "lab.masterHostNumber"} {
		for _, removeClients := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/remove=%v", field, removeClients), func(t *testing.T) {
				candidate := base
				switch field {
				case "lab.networkBase":
					candidate.Lab.NetworkBase = "10.1.0.0"
				case "lab.networkPrefixLength":
					candidate.Lab.NetworkPrefix = 23
				case "lab.masterHostNumber":
					candidate.Lab.MasterHostNumber = 100
				}
				if removeClients {
					candidate.Lab.PCCount = 0
					candidate.Lab.DeploymentMode = "controller"
				}
				written := domain.LabSettingsFile{}
				manager := NewSettingsManager(fakeSettingsSource{data: template, written: &written, candidateErr: errors.New("must reject before Nix")})
				plan := manager.PlanSettings(context.Background(), "/repo", candidate)
				if !plan.HasErrors() || plan.State != "invalid" || plan.Issues[0].Field != field {
					t.Fatalf("plan = %+v", plan)
				}
				applied := manager.ApplySettings(context.Background(), "/repo", candidate, plan.BaseFingerprint)
				if !applied.HasErrors() || written.SchemaVersion != 0 {
					t.Fatalf("applied = %+v; written = %+v", applied, written)
				}
				// Even an old successful review cannot bypass the write-time rule.
				candidateData, err := domain.MarshalLabSettings(candidate)
				if err != nil {
					t.Fatal(err)
				}
				oldReview := domain.ConfigPlanReport{State: "valid", BaseFingerprint: domain.SettingsFingerprint(template), CandidateFingerprint: domain.SettingsFingerprint(candidateData)}
				applied = manager.ApplyReviewedSettings("/repo", candidate, oldReview)
				if !applied.HasErrors() || applied.Issues[0].Field != field || written.SchemaVersion != 0 {
					t.Fatalf("reviewed apply = %+v; written = %+v", applied, written)
				}
			})
		}
	}
}

func TestSettingsAddressChangesRemainAvailableBeforeClientConfiguration(t *testing.T) {
	template, err := os.ReadFile("../../templates/site/lab-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := domain.DecodeLabSettings(template)
	base.Lab.PCCount = 0
	base.Lab.DeploymentMode = "controller"
	baseData, err := domain.MarshalLabSettings(base)
	if err != nil {
		t.Fatal(err)
	}
	candidate := base
	candidate.Lab.DeploymentMode = "laboratory"
	candidate.Lab.PCCount = 20
	candidate.Lab.NetworkBase = "10.1.0.0"
	candidate.Lab.NetworkPrefix = 23
	candidate.Lab.MasterHostNumber = 100
	written := domain.LabSettingsFile{}
	manager := NewSettingsManager(fakeSettingsSource{data: baseData, written: &written})
	plan := manager.PlanSettings(context.Background(), "/repo", candidate)
	if plan.HasErrors() || plan.State != "valid" {
		t.Fatalf("initial configuration = %+v", plan)
	}
	applied := manager.ApplyReviewedSettings("/repo", candidate, plan)
	if applied.HasErrors() || written.Lab.NetworkBase != "10.1.0.0" || written.Lab.PCCount != 20 {
		t.Fatalf("initial save = %+v; written = %+v", applied, written)
	}
}
