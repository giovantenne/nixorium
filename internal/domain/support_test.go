package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func supportFixture() SupportInput {
	status := StatusReport{SchemaVersion: SchemaVersion}
	status.Meta.Version = "2.0.0"
	status.Meta.DeploymentMode = "laboratory"
	status.Deployment.Ready = true
	status.Git.Available = true
	status.PXE.Mode = "stopped"
	return SupportInput{
		Version: "2.0.0", Revision: strings.Repeat("a", 40), Collected: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
		Status:     &status,
		Doctor:     &DoctorReport{SchemaVersion: SchemaVersion, Findings: []Finding{{ID: "CACHE-HEALTH", Level: LevelWarning}}},
		Hosts:      &HostsReport{SchemaVersion: SchemaVersion, Hosts: []HostStatus{{SSH: SSHAvailable, Deployment: DeploymentCurrent}}},
		Operations: &[]OperationRecord{{Operation: "deploy-apply", State: "partial"}},
	}
}

func TestSupportSnapshotClassification(t *testing.T) {
	input := supportFixture()
	snapshot, err := NewSupportSnapshot(input)
	if err != nil || !snapshot.Valid() {
		t.Fatalf("snapshot: %v", err)
	}
	var report supportReport
	if err := json.Unmarshal([]byte(snapshot.JSON()), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || report.Version != "2.0.0" || report.Revision != input.Revision ||
		report.Doctor.Findings[0].ID != "CACHE-HEALTH" || report.Hosts.SSH.Available != 1 ||
		report.Operations.Outcomes[0].State != "partial" {
		t.Fatalf("unexpected report: %s", snapshot.JSON())
	}
	input.Status.Meta.Version = "9.9.9"
	if strings.Contains(snapshot.JSON(), "9.9.9") {
		t.Fatal("snapshot changed after collection")
	}
	repeated, _ := NewSupportSnapshot(supportFixture())
	if len(snapshot.Digest()) != 64 || snapshot.Digest() != repeated.Digest() {
		t.Fatal("unstable digest")
	}
	if (SupportSnapshot{}).Valid() {
		t.Fatal("zero snapshot is exportable")
	}
}

func TestSupportGuidanceUsesOnlyStaticRoutes(t *testing.T) {
	input := supportFixture()
	input.Doctor.Findings = nil
	for _, id := range SupportFindingIDs() {
		input.Doctor.Findings = append(input.Doctor.Findings, Finding{ID: id, Level: LevelWarning, Remediation: "SECRET"})
		if SupportFindingGuide(id) == "" {
			t.Fatalf("missing route for %s", id)
		}
	}
	if SupportFindingGuide("COMMAND-SECRET") != "" {
		t.Fatal("dynamic code produced a guide")
	}
	snapshot, err := NewSupportSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	var report supportReport
	if err := json.Unmarshal([]byte(snapshot.JSON()), &report); err != nil {
		t.Fatal(err)
	}
	if report.GuidanceVersion != SupportGuidanceVersion || len(report.Doctor.Findings) != len(SupportFindingIDs()) {
		t.Fatal("incomplete guidance catalog")
	}
	for _, finding := range report.Doctor.Findings {
		if finding.Guide != SupportFindingGuide(finding.ID) {
			t.Fatal("guide came from private remediation text")
		}
	}
}

// Populate every string leaf, including nested reports and future fields. Only
// the explicitly restored codes below may cross the sharing boundary.
func poisonSupportStrings(value reflect.Value, secret string) {
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		poisonSupportStrings(value.Elem(), secret)
	case reflect.Struct:
		if value.Type() == reflect.TypeOf(time.Time{}) {
			return
		}
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanSet() {
				poisonSupportStrings(value.Field(i), secret)
			}
		}
	case reflect.String:
		value.SetString(secret)
	case reflect.Slice:
		value.Set(reflect.MakeSlice(value.Type(), 1, 1))
		poisonSupportStrings(value.Index(0), secret)
	}
}

func TestSupportSnapshotNeverCopiesPrivateText(t *testing.T) {
	for _, secret := range []string{
		"sk-or-v1-synthetic-credential", "$6$synthetic$passwordhash", "-----BEGIN OPENSSH PRIVATE KEY-----",
		"/home/private-school/student/.config", "school.example.test", "192.0.2.83", "\x1b]52;c;c2VjcmV0\a",
		"SECRET-" + strings.Repeat("x", 64*1024),
	} {
		input := supportFixture()
		poisonSupportStrings(reflect.ValueOf(&input).Elem(), secret)
		input.Doctor.Findings[0].ID = "CACHE-SIGNING-KEY"
		input.Doctor.Findings[0].Level = LevelError
		input.Operations = &[]OperationRecord{{ID: secret, Operation: "deploy-apply", State: "failed", Subject: secret, Summary: secret}}
		snapshot, err := NewSupportSnapshot(input)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(snapshot.JSON(), secret) || strings.Contains(snapshot.JSON(), "SECRET-") {
			t.Fatal("private text leaked")
		}
		var decoded any
		if err := json.Unmarshal([]byte(snapshot.JSON()), &decoded); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(decoded)
		if strings.Contains(string(encoded), "c2VjcmV0") {
			t.Fatal("terminal payload leaked")
		}
		if !strings.Contains(snapshot.JSON(), "CACHE-SIGNING-KEY") || !strings.Contains(snapshot.JSON(), "failed") {
			t.Fatal("safe evidence was lost")
		}
	}
}

func TestSupportSnapshotBoundsAndUnknowns(t *testing.T) {
	input := supportFixture()
	input.Doctor.Findings = make([]Finding, SupportMaxFindings+10)
	for i := range input.Doctor.Findings {
		input.Doctor.Findings[i] = Finding{ID: "COMMAND-SECRET", Level: LevelError}
	}
	input.Doctor.Findings[0] = Finding{ID: "COMMAND-NIX", Level: Level("SECRET")}
	input.Hosts.Hosts = make([]HostStatus, SupportMaxHosts+10)
	input.Operations = &[]OperationRecord{{Operation: "deploy-apply", State: "SECRET"}, {Operation: "SECRET", State: "failed"}}
	snapshot, err := NewSupportSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	var report supportReport
	if err := json.Unmarshal([]byte(snapshot.JSON()), &report); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(snapshot.JSON(), "SECRET") || len(snapshot.JSON()) > SupportMaxBytes {
		t.Fatal("unsafe output")
	}
	if report.Doctor.Omitted != SupportMaxFindings+10 || report.Hosts.Omitted != 10 || report.Hosts.Deployment.Unknown != SupportMaxHosts || report.Operations.Omitted != 2 {
		t.Fatal("missing omission counts")
	}
	input.Status.SchemaVersion = 999
	input.Doctor.SchemaVersion = 999
	input.Hosts.SchemaVersion = 999
	input.Operations = nil
	snapshot, err = NewSupportSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	report = supportReport{}
	if err := json.Unmarshal([]byte(snapshot.JSON()), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != nil || report.Doctor != nil || report.Hosts != nil || report.Operations != nil {
		t.Fatal("unknown schema was trusted")
	}
}

func TestSupportSnapshotOperationLimitAndStableOrder(t *testing.T) {
	input := supportFixture()
	records := make([]OperationRecord, SupportMaxOperations+1)
	for i := range records {
		records[i] = OperationRecord{Operation: "deploy-apply", State: "completed"}
	}
	records[0].Operation = "git-commit"
	input.Operations = &records
	first, err := NewSupportSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		next, err := NewSupportSnapshot(input)
		if err != nil || first.JSON() != next.JSON() {
			t.Fatal("unstable serialization")
		}
	}
	var report supportReport
	if err := json.Unmarshal([]byte(first.JSON()), &report); err != nil {
		t.Fatal(err)
	}
	if report.Operations.Omitted != 1 || report.Operations.Outcomes[0].Count != SupportMaxOperations-1 {
		t.Fatal("wrong bounded aggregation")
	}
	input.Collected = time.Time{}
	if _, err := NewSupportSnapshot(input); err == nil {
		t.Fatal("missing timestamp accepted")
	}
}

func FuzzSupportSnapshotPrivateStrings(f *testing.F) {
	f.Add("secret-token")
	f.Add("PRIVATE-192.0.2.83")
	f.Fuzz(func(t *testing.T, value string) {
		input := supportFixture()
		input.Status.Repository = value
		input.Status.Warnings = []string{value}
		input.Doctor.Findings[0].Summary = value
		input.Doctor.Findings[0].Evidence = value
		input.Doctor.Findings[0].Remediation = value
		input.Hosts.Hosts[0].Name = value
		input.Hosts.Hosts[0].CurrentRevision = value
		(*input.Operations)[0].Summary = value
		actual, err := NewSupportSnapshot(input)
		if err != nil {
			t.Fatal(err)
		}
		expected, _ := NewSupportSnapshot(supportFixture())
		if actual.JSON() != expected.JSON() {
			t.Fatal("private text changed shared output")
		}
	})
}
