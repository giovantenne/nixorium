package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func validInstallationSession() InstallationSessionRecord {
	started := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	practical := started.Add(2 * time.Minute)
	return InstallationSessionRecord{
		SchemaVersion: InstallationSessionSchemaVersion,
		Repository:    "/home/admin/nixorium-deployment",
		State:         InstallationSessionActive,
		Revision:      strings.Repeat("a", 40),
		Selected:      "pc01",
		StartedAt:     started,
		UpdatedAt:     practical,
		Evidence: []InstallationEvidence{{
			Name:                 "pc01",
			Revision:             strings.Repeat("a", 40),
			SystemPath:           "/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-pc01-system",
			TechnicalVerifiedAt:  started.Add(time.Minute),
			PracticalConfirmedAt: &practical,
		}},
	}
}

func TestInstallationSessionRoundTripIsStrictAndDeterministic(t *testing.T) {
	record := validInstallationSession()
	first, err := MarshalInstallationSession(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeInstallationSession(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalInstallationSession(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || first[len(first)-1] != '\n' {
		t.Fatalf("session encoding is not deterministic:\n%s\n%s", first, second)
	}
}

func TestInstallationSessionRejectsUnboundOrImpossibleEvidence(t *testing.T) {
	mutations := []func(*InstallationSessionRecord){
		func(r *InstallationSessionRecord) { r.Repository = "relative" },
		func(r *InstallationSessionRecord) { r.Revision = "short" },
		func(r *InstallationSessionRecord) { r.Selected = "../../pc01" },
		func(r *InstallationSessionRecord) { r.UpdatedAt = r.StartedAt.Add(-time.Second) },
		func(r *InstallationSessionRecord) { r.Evidence[0].Revision = strings.Repeat("b", 40) },
		func(r *InstallationSessionRecord) { r.Evidence[0].SystemPath = "/tmp/system" },
		func(r *InstallationSessionRecord) { r.Evidence = append(r.Evidence, r.Evidence[0]) },
		func(r *InstallationSessionRecord) {
			before := r.Evidence[0].TechnicalVerifiedAt.Add(-time.Second)
			r.Evidence[0].PracticalConfirmedAt = &before
		},
	}
	for index, mutate := range mutations {
		record := validInstallationSession()
		mutate(&record)
		if _, err := MarshalInstallationSession(record); err == nil {
			t.Errorf("mutation %d was accepted", index)
		}
	}
}

func TestInstallationSessionDecoderRejectsUnknownAndTrailingData(t *testing.T) {
	data, err := MarshalInstallationSession(validInstallationSession())
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range [][]byte{
		bytes.Replace(data, []byte(`"state": "active"`), []byte(`"unknown": true, "state": "active"`), 1),
		append(append([]byte(nil), data...), []byte(`{}`)...),
	} {
		if _, err := DecodeInstallationSession(mutation); err == nil {
			t.Fatal("unsafe session JSON was accepted")
		}
	}
}
