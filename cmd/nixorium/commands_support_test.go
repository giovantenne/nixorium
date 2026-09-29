package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeSupportCommand struct {
	snapshot         domain.SupportSnapshot
	previewErr       error
	collected, saved int
	result           domain.SupportExportResult
}

func (m *fakeSupportCommand) Preview(context.Context, string, string) (domain.SupportSnapshot, error) {
	m.collected++
	return m.snapshot, m.previewErr
}
func (m *fakeSupportCommand) Export(_ context.Context, snapshot domain.SupportSnapshot) domain.SupportExportResult {
	if snapshot.JSON() != m.snapshot.JSON() {
		panic("changed snapshot")
	}
	m.saved++
	return m.result
}

func TestSupportArguments(t *testing.T) {
	for _, arguments := range []string{"support preview", "support preview --json", "support export --repo /private"} {
		if _, err := parseArguments(strings.Fields(arguments)); err != nil {
			t.Fatalf("%s: %v", arguments, err)
		}
	}
	for _, arguments := range []string{"support", "support export --yes", "support export --json", "support preview --full", "support preview --file /tmp/report", "support preview --expect token", "support preview export", "doctor export", "support --on @lab"} {
		if _, err := parseArguments(strings.Fields(arguments)); err == nil {
			t.Fatalf("accepted %s", arguments)
		}
	}
}

func TestSupportPreviewExportAndCancellation(t *testing.T) {
	snapshot, err := domain.NewSupportSnapshot(domain.SupportInput{Version: "2.0.0", Collected: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, subcommand string
		json, approve    bool
		state            string
		saved, code      int
	}{
		{"preview", "preview", false, false, "", 0, 0},
		{"json", "preview", true, false, "", 0, 0},
		{"cancel", "export", false, false, "", 0, 0},
		{"save", "export", false, true, "saved", 1, 0},
		{"partial", "export", false, true, "partial", 1, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeSupportCommand{snapshot: snapshot, result: domain.SupportExportResult{State: test.state}}
			var output, stderr bytes.Buffer
			confirmed := false
			confirm := func() (bool, error) {
				confirmed = true
				if !strings.Contains(output.String(), snapshot.JSON()) {
					t.Fatal("confirmation preceded exact preview")
				}
				return test.approve, nil
			}
			code := runSupportWithManager(context.Background(), manager, "/private", options{subcommand: test.subcommand, json: test.json}, confirm, &output, &stderr)
			if code != test.code || manager.saved != test.saved || manager.collected != 1 {
				t.Fatalf("code %d, manager %+v", code, manager)
			}
			if test.subcommand == "preview" && confirmed {
				t.Fatal("preview requested authorization")
			}
			if test.json && output.String() != snapshot.JSON() {
				t.Fatal("JSON polluted with prose")
			}
		})
	}
}

type failingSupportWriter struct{}

func (failingSupportWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestSupportNeverSavesWithoutDeliveredPreview(t *testing.T) {
	snapshot, _ := domain.NewSupportSnapshot(domain.SupportInput{Collected: time.Now()})
	manager := &fakeSupportCommand{snapshot: snapshot}
	confirm := func() (bool, error) { t.Fatal("confirmed failed preview"); return true, nil }
	if code := runSupportWithManager(context.Background(), manager, "/private", options{subcommand: "export"}, confirm, failingSupportWriter{}, io.Discard); code == 0 || manager.saved != 0 {
		t.Fatal("saved after output failure")
	}
	manager.previewErr = errors.New("SECRET evaluator error")
	var stderr bytes.Buffer
	if code := runSupportWithManager(context.Background(), manager, "/private", options{subcommand: "preview"}, confirm, io.Discard, &stderr); code == 0 || strings.Contains(stderr.String(), "SECRET") {
		t.Fatal("raw error leaked")
	}
}
