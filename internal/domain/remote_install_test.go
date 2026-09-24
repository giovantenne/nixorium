package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validRemoteInstallPlan() RemoteInstallPlan {
	return RemoteInstallPlan{
		SchemaVersion:      RemoteInstallSchemaVersion,
		OperationID:        "0123456789abcdef0123456789abcdef",
		BootID:             "01234567-89ab-cdef-0123-456789abcdef",
		DeploymentRevision: "0123456789abcdef0123456789abcdef01234567",
		SystemPath:         "/nix/store/00000000000000000000000000000000-nixos-system-pc01-test",
		Host: RemoteInstallHost{
			Name: "pc01", Interface: "enp0s2", LiveIP: "192.0.2.20", StaticIP: "10.0.0.1",
		},
		Cache: RemoteInstallCache{URL: "http://192.0.2.10:5000", PublicKey: "cache.example:YWJjZA=="},
		Disk: RemoteDisk{
			Path: "/dev/sda", KName: "sda", MajorMinor: "8:0", SizeBytes: 16 * 1024 * 1024 * 1024,
			Serial: "serial-1", Model: "Test disk", Transport: "sata", DiskSeq: "1",
		},
		AdminPublicKey: "ssh-ed25519 YWJjZA== admin@test",
		HostKeyPublic:  "ssh-ed25519 YWJjZA== root@test",
	}
}

func TestDecodeRemoteInstallPlanStrict(t *testing.T) {
	plan := validRemoteInstallPlan()
	valid, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeRemoteInstallPlan(valid); err != nil || decoded.OperationID != plan.OperationID {
		t.Fatalf("valid plan rejected: %#v, %v", decoded, err)
	}

	tests := map[string][]byte{
		"unknown":      bytes.Replace(valid, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"unexpected":true`), 1),
		"duplicate":    bytes.Replace(valid, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"schemaVersion":1`), 1),
		"trailing":     append(append([]byte{}, valid...), []byte(` {}`)...),
		"oversized":    bytes.Repeat([]byte(" "), RemoteInstallPlanMaxBytes+1),
		"invalid-utf8": {0xff, 0xfe},
		"overflow":     bytes.Replace(valid, []byte(`"sizeBytes":17179869184`), []byte(`"sizeBytes":18446744073709551616`), 1),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRemoteInstallPlan(data); err == nil {
				t.Fatal("unsafe input accepted")
			}
		})
	}
}

func TestValidateRemoteInstallPlanRejectsChangedBindings(t *testing.T) {
	tests := map[string]func(*RemoteInstallPlan){
		"operation":       func(plan *RemoteInstallPlan) { plan.OperationID = "../escape" },
		"boot":            func(plan *RemoteInstallPlan) { plan.BootID = "new-boot" },
		"revision":        func(plan *RemoteInstallPlan) { plan.DeploymentRevision = strings.Repeat("a", 39) },
		"host-path":       func(plan *RemoteInstallPlan) { plan.Host.Name = "pc02" },
		"same-ip":         func(plan *RemoteInstallPlan) { plan.Host.LiveIP = plan.Host.StaticIP },
		"noncanonical-ip": func(plan *RemoteInstallPlan) { plan.Host.LiveIP = "192.0.2.020" },
		"link-local":      func(plan *RemoteInstallPlan) { plan.Host.LiveIP = "169.254.1.2" },
		"https-cache":     func(plan *RemoteInstallPlan) { plan.Cache.URL = "https://192.0.2.10:5000" },
		"cache-path":      func(plan *RemoteInstallPlan) { plan.Cache.URL = "http://192.0.2.10:5000/cache" },
		"disk-path":       func(plan *RemoteInstallPlan) { plan.Disk.Path = "/dev/sdb" },
		"weak-disk":       func(plan *RemoteInstallPlan) { plan.Disk.Serial, plan.Disk.WWN, plan.Disk.DiskSeq = "", "", "" },
		"small-disk":      func(plan *RemoteInstallPlan) { plan.Disk.SizeBytes = 1024 },
		"private-key":     func(plan *RemoteInstallPlan) { plan.AdminPublicKey = "PRIVATE" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			plan := validRemoteInstallPlan()
			mutate(&plan)
			if err := ValidateRemoteInstallPlan(plan); err == nil {
				t.Fatal("changed binding accepted")
			}
		})
	}
}

func TestRemoteInstallReviewTokenBindsDestructiveInputs(t *testing.T) {
	base := RemoteInstallPlanReport{
		Repository: "/deployment", Method: RemoteInstallUSBSSH,
		OperationID: "0123456789abcdef0123456789abcdef",
		Plan:        validRemoteInstallPlan(), BundlePath: "/nix/store/11111111111111111111111111111111-bundle",
		ExpiresAt: time.Unix(1000, 0).UTC(),
	}
	token := RemoteInstallReviewToken(base)
	mutations := []func(*RemoteInstallPlanReport){
		func(report *RemoteInstallPlanReport) { report.Repository = "/other" },
		func(report *RemoteInstallPlanReport) { report.Plan.BootID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" },
		func(report *RemoteInstallPlanReport) { report.Plan.Disk.Serial = "changed" },
		func(report *RemoteInstallPlanReport) { report.Plan.Cache.URL = "http://192.0.2.11:5000" },
		func(report *RemoteInstallPlanReport) {
			report.Plan.SystemPath = "/nix/store/11111111111111111111111111111111-nixos-system-pc01-other"
		},
		func(report *RemoteInstallPlanReport) { report.ExpiresAt = report.ExpiresAt.Add(time.Second) },
	}
	for _, mutate := range mutations {
		changed := base
		mutate(&changed)
		if RemoteInstallReviewToken(changed) == token {
			t.Fatal("review token did not bind a destructive input")
		}
	}
}

func TestRemoteOperationIDsAreRandomAndValid(t *testing.T) {
	first, err := NewRemoteOperationID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRemoteOperationID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !remoteOperationIDPattern.MatchString(first) || !remoteOperationIDPattern.MatchString(second) {
		t.Fatalf("invalid operation IDs %q %q", first, second)
	}
}

func TestDecodeRemoteMachineFactsLimitsAndSupport(t *testing.T) {
	facts := RemoteMachineFacts{
		SchemaVersion: RemoteInstallSchemaVersion, VariantID: "installer", VersionID: "26.05", BuildID: "26.05.test",
		Architecture: "x86_64", UEFI: true, SudoReady: true,
		BootID:     "01234567-89ab-cdef-0123-456789abcdef",
		Interfaces: []RemoteNetworkInterface{{Name: "enp0s2", Addresses: []string{"192.0.2.20"}}},
		Disks: []RemoteDisk{{
			Path: "/dev/sda", KName: "sda", MajorMinor: "8:0", SizeBytes: 16 * 1024 * 1024 * 1024,
			Serial: "serial", Model: "disk", Transport: "sata", DiskSeq: "1", Eligible: true,
		}},
	}
	data, _ := json.Marshal(facts)
	if _, err := DecodeRemoteMachineFacts(data); err != nil {
		t.Fatal(err)
	}
	facts.VariantID = "server"
	data, _ = json.Marshal(facts)
	if _, err := DecodeRemoteMachineFacts(data); err == nil {
		t.Fatal("unsupported installer accepted")
	}
	facts.VariantID = "installer"
	facts.Interfaces = make([]RemoteNetworkInterface, RemoteInstallMaximumNICs+1)
	data, _ = json.Marshal(facts)
	if _, err := DecodeRemoteMachineFacts(data); err == nil {
		t.Fatal("oversized interface inventory accepted")
	}
}
