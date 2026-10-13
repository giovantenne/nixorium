package hostdiagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJournalIsBoundedAndScrubbedBeforeEncoding(t *testing.T) {
	messages := []string{"Failed with result 'exit-code'.", "No space left on device /home/student/private.txt", "message from student@example.org https://private.example/report", "password=syntheticsecret", "ExecStart=/bin/tool --secret=synthetic", "ordinary failed connection"}
	var input strings.Builder
	for i := 0; i < 80; i++ {
		data, _ := json.Marshal(map[string]string{"UNIT": "home-reset.service", "_SYSTEMD_UNIT": "init.scope", "__REALTIME_TIMESTAMP": "1780000000000000", "PRIORITY": "3", "MESSAGE": messages[i%len(messages)], "ENVIRONMENT": "must never copy"})
		input.Write(data)
		input.WriteByte('\n')
	}
	entries, _ := parseJournal([]byte(input.String()), "home_reset", false)
	if len(entries) != 50 {
		t.Fatal(len(entries))
	}
	data, _ := json.Marshal(entries)
	for _, secret := range []string{"student", "private.txt", "private.example", "syntheticsecret", "ExecStart=", "must never copy"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("leaked %q: %s", secret, data)
		}
	}
	if !strings.Contains(string(data), "No space left on device") || !strings.Contains(string(data), "ordinary failed connection") {
		t.Fatal("useful evidence removed", string(data))
	}
	if got, _ := parseJournal([]byte(`{"_SYSTEMD_UNIT":"foreign.service","MESSAGE":"private","PRIORITY":"3","__REALTIME_TIMESTAMP":"1"}`), "system", false); len(got) != 0 {
		t.Fatal("foreign unit allowed")
	}
}
func TestCollectUsesFixedReadOnlyCommandsAndHonestMissingEvidence(t *testing.T) {
	calls := 0
	r := collect(t.Context(), Config{Systemctl: "fixed-systemctl", Journalctl: "fixed-journalctl"}, "home_reset", func(ctx context.Context, name string, args ...string) ([]byte, bool, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing deadline")
		}
		joined := strings.Join(args, " ")
		if name == "/run/current-system/sw/bin/nixos-version" {
			if joined != "--configuration-revision" {
				t.Fatal(joined)
			}
			return []byte(strings.Repeat("a", 40)), false, nil
		}
		for _, forbidden := range []string{" start ", " restart ", " stop ", "--all", "--user"} {
			if strings.Contains(joined, forbidden) {
				t.Fatal(joined)
			}
		}
		if name == "fixed-systemctl" {
			return []byte("Id=home-reset.service\nLoadState=loaded\nActiveState=failed\nResult=exit-code\nExecMainStatus=1\nStateChangeTimestamp=Tue 2026-10-13 09:00:00 UTC\n"), false, nil
		}
		if name != "fixed-journalctl" || !strings.Contains(joined, "--boot=") || !strings.Contains(joined, "--lines=50") {
			t.Fatal(name, joined)
		}
		if len(strings.TrimPrefix(args[0], "--boot=")) != 32 {
			t.Fatal("journalctl needs compact boot descriptor", args[0])
		}
		return nil, false, errors.New("permission denied syntheticsecret")
	})
	if calls != 3 || len(r.Units) != 1 || r.Units[0].Result != "exit-code" || r.Units[0].ChangedAt == "" || !strings.Contains(strings.Join(r.Unavailable, ","), "journal") {
		t.Fatal(r, calls)
	}
	data, err := Encode(r)
	if err != nil || strings.Contains(string(data), "syntheticsecret") {
		t.Fatal(string(data), err)
	}
	if _, err := Decode(data, "home_reset", false); err != nil {
		t.Fatal(err)
	}
}
func TestDecodeRefusesForeignOrPrivateWireValues(t *testing.T) {
	base := Report{SchemaVersion: 1, Area: "system", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Units: []Unit{}, Journal: []Entry{}, Space: []Space{}, Unavailable: []string{}}
	for _, modify := range []func(*Report){func(r *Report) { r.Controller = true }, func(r *Report) { r.Revision = "secret" }, func(r *Report) { r.Unavailable = []string{"private journal"} }, func(r *Report) {
		r.Units = []Unit{{Name: "foreign.service", LoadState: "loaded", ActiveState: "active"}}
	}, func(r *Report) { r.Space = []Space{{"/home/student", 0, 1}} }} {
		r := base
		modify(&r)
		data, _ := json.Marshal(r)
		if _, err := Decode(data, "system", false); err == nil {
			t.Fatal(string(data))
		}
	}
	if _, err := Decode([]byte(`{"schemaVersion":1,"schemaVersion":1}`), "system", false); err == nil {
		t.Fatal("duplicates allowed")
	}
}
func TestRunMarksPermissionWarningsAndBoundsPipes(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "observe")
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\nprintf '[]'\necho 'no permission to see system journal' >&2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t.Context(), fixture); err == nil {
		t.Fatal("permission warning became empty success")
	}
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\nwhile :; do printf 'fixed observation text'; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	data, truncated, err := run(ctx, fixture)
	if err == nil || !truncated || len(data) > 64*1024 || time.Since(start) > 3*time.Second {
		t.Fatal(len(data), truncated, err, time.Since(start))
	}
}

func TestServiceParsingDoesNotHidePartialOrMalformedEvidence(t *testing.T) {
	valid := "Id=home-reset.service\nLoadState=loaded\nActiveState=failed\nResult=exit-code\nExecMainStatus=1\nStateChangeTimestamp=Tue 2026-10-13 09:00:00.123456 +0000\n"
	units, incomplete := parseUnits([]byte(valid), "home_reset", false)
	if len(units) != 1 || !incomplete || units[0].ChangedAt != "2026-10-13T09:00:00Z" {
		t.Fatal(units, incomplete)
	}
	for _, text := range []string{strings.ReplaceAll(valid, "ExecMainStatus=1\n", ""), strings.ReplaceAll(valid, "ExecMainStatus=1", "ExecMainStatus=bogus"), valid + "ExecMainStatus=0\n"} {
		units, incomplete := parseUnits([]byte(text), "home_reset", false)
		if len(units) != 0 || !incomplete {
			t.Fatal(units, incomplete)
		}
	}
	for _, text := range []string{`{"UNIT":"home-reset.service","MESSAGE":[1,2],"__REALTIME_TIMESTAMP":"1780000000000000","PRIORITY":"3"}`, `{"UNIT":"home-reset.service","MESSAGE":"ordinary","__REALTIME_TIMESTAMP":"1","PRIORITY":"3"}`} {
		entries, incomplete := parseJournal([]byte(text), "home_reset", false)
		if len(entries) != 0 || !incomplete {
			t.Fatal(entries, incomplete)
		}
	}
}
func TestRestartNoticeMustContainTypedBooleanAndActualObservationTime(t *testing.T) {
	now := time.Unix(1780000000, 0)
	for _, text := range []string{`{"updatedAt":1780000000}`, `{"updatedAt":1780000000,"restart":null}`, `{"updatedAt":1780000000,"restart":false,"extra":true}`, `{"updatedAt":1780000000,"restart":"false"}`, `{"updatedAt":1780000000,"restart":true,"restart":false}`} {
		if _, _, valid := parseRestartNotice([]byte(text), now); valid {
			t.Fatal(text)
		}
	}
	value, at, valid := parseRestartNotice([]byte(`{"updatedAt":1770000000,"restart":true}`), now)
	if !value || !valid || at != time.Unix(1770000000, 0).UTC().Format(time.RFC3339) {
		t.Fatal(value, at, valid)
	}
}
