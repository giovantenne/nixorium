package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestHomeFilesArguments(t *testing.T) {
	for _, command := range []string{
		"home-files plan --file draft.json --repo /lab --json",
		"home-files apply --file draft.json --expect token --yes --json",
		"home-files apply --file profile.json --expect token",
		"home-files import plan --source /local/lesson.pdf --destination Documents/lesson.pdf --json",
		"home-files import plan --source /local/folder --destination Documents/Lessons --replace --repo /lab",
	} {
		if _, err := parseArguments(strings.Fields(command)); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	for _, command := range []string{
		"home-files", "home-files import", "home-files import apply", "home-files plan",
		"home-files apply --file draft.json --yes", "home-files plan --file draft.json --yes",
		"home-files plan --file draft.json --expect token", "home-files plan --file draft.json --scope shared",
		"home-files import plan --source /file", "home-files import plan --source /file --destination Documents/file --yes",
		"home-files import plan --source /file --destination Documents/file --file draft.json",
		"home-files import plan --source /file --destination Documents/file --expect token",
		"home-files apply --file draft.json --expect token --source /file", "home-files plan --file draft.json --replace",
		"home-files plan --file draft.json --on pc01", "home-files apply --file draft.json --expect token --full",
	} {
		if _, err := parseArguments(strings.Fields(command)); err == nil {
			t.Fatalf("invalid command accepted: %s", command)
		}
	}
}

type fakeHomeFilesCommands struct {
	plan                     domain.HomeFilesPlanReport
	result                   domain.HomeFilesApplyReport
	capture                  domain.HomeFilesCaptureReport
	plans, applies, captures int
	draft                    domain.HomeFilesDraft
	expected                 string
}

func (f *fakeHomeFilesCommands) Load(context.Context, string) domain.HomeFilesPlanReport {
	return f.plan
}
func (f *fakeHomeFilesCommands) Capture(context.Context, string, string, string) domain.HomeFilesCaptureReport {
	f.captures++
	return f.capture
}
func (f *fakeHomeFilesCommands) Plan(_ context.Context, _ string, draft domain.HomeFilesDraft) domain.HomeFilesPlanReport {
	f.plans++
	f.draft = draft
	return f.plan
}
func (f *fakeHomeFilesCommands) Apply(_ context.Context, _ string, draft domain.HomeFilesDraft, token string) domain.HomeFilesApplyReport {
	f.applies++
	f.draft, f.expected = draft, token
	return f.result
}

func TestHomeFilesCLIReviewSaveBoundaries(t *testing.T) {
	for _, item := range []struct {
		name, action, state, token, result string
		yes, approved                      bool
		confirmError                       error
		wantCode, wantConfirm, wantApply   int
		wantState                          string
	}{
		{name: "plan", action: "plan", state: "ready", wantState: "ready"},
		{name: "confirmed", action: "apply", state: "ready", token: "review", result: "saved", approved: true, wantConfirm: 1, wantApply: 1, wantState: "saved"},
		{name: "explicit yes", action: "apply", state: "ready", token: "review", result: "saved", yes: true, wantApply: 1, wantState: "saved"},
		{name: "cancelled", action: "apply", state: "ready", token: "review", wantConfirm: 1, wantState: "cancelled"},
		{name: "noninteractive", action: "apply", state: "ready", token: "review", confirmError: errors.New("terminal required"), wantCode: 1, wantConfirm: 1},
		{name: "stale review", action: "apply", state: "ready", token: "old", wantCode: 1, wantState: "conflict"},
		{name: "invalid", action: "apply", state: "invalid", token: "review", wantCode: 1, wantState: "invalid"},
		{name: "unchanged", action: "apply", state: "unchanged", token: "review", result: "unchanged", wantApply: 1, wantState: "unchanged"},
		{name: "conflict on save", action: "apply", state: "ready", token: "review", result: "conflict", approved: true, wantConfirm: 1, wantApply: 1, wantCode: 1, wantState: "conflict"},
		{name: "partial", action: "apply", state: "ready", token: "review", result: "partial", yes: true, wantApply: 1, wantCode: 1, wantState: "partial"},
	} {
		t.Run(item.name, func(t *testing.T) {
			draft := domain.HomeFilesDraft{Profile: domain.HomeFilesProfile{SchemaVersion: 1}, CaptureIDs: []string{"immutable-capture"}, BaseFingerprint: "base"}
			m := &fakeHomeFilesCommands{plan: domain.HomeFilesPlanReport{State: item.state, ReviewToken: "review", Candidate: &draft, Confirmation: "SAVE"}, result: domain.HomeFilesApplyReport{State: item.result}}
			var stdout, stderr bytes.Buffer
			confirmations := 0
			confirm := func(domain.HomeFilesPlanReport) (bool, error) {
				confirmations++
				return item.approved, item.confirmError
			}
			code := runHomeFilesWithManager(t.Context(), m, "/lab", draft, options{subcommand: item.action, expect: item.token, yes: item.yes, json: true}, confirm, &stdout, &stderr)
			if code != item.wantCode || confirmations != item.wantConfirm || m.applies != item.wantApply || m.captures != 0 {
				t.Fatalf("code=%d confirmations=%d applies=%d captures=%d output=%s errors=%s", code, confirmations, m.applies, m.captures, &stdout, &stderr)
			}
			if m.applies > 0 && (m.expected != item.token || m.draft.CaptureIDs[0] != "immutable-capture" || m.draft.BaseFingerprint != "base") {
				t.Fatal("exact draft/token lost")
			}
			if item.wantState != "" {
				var report struct {
					State string `json:"state"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.State != item.wantState {
					t.Fatalf("output=%s %v", &stdout, err)
				}
			}
		})
	}
}

func TestHomeFilesCLIImportCapturesOnceAndPlansWithoutSaving(t *testing.T) {
	source := "imports/" + strings.Repeat("1", 64)
	empty := domain.HomeFilesDraft{Profile: domain.HomeFilesProfile{SchemaVersion: 1}}
	m := &fakeHomeFilesCommands{plan: domain.HomeFilesPlanReport{State: "ready", Candidate: &empty, Snapshot: &domain.HomeFilesSnapshot{BaseFingerprint: "base"}}, capture: domain.HomeFilesCaptureReport{State: "captured", CaptureID: "copy", Destination: "Documents/lesson.pdf", Files: []domain.HomeFile{{Destination: "Documents/lesson.pdf", Source: &source}}}}
	var stdout, stderr bytes.Buffer
	code := runHomeFilesImport(t.Context(), m, "/lab", options{subcommand: "import-plan", homeFilesSource: "/local/lesson.pdf", homeFilesDestination: "Documents/lesson.pdf", json: true}, &stdout, &stderr)
	if code != 0 || m.captures != 1 || m.plans != 1 || m.applies != 0 || len(m.draft.Profile.Files) != 1 || m.draft.BaseFingerprint != "base" || m.draft.CaptureIDs[0] != "copy" {
		t.Fatalf("invalid import boundary: %+v %s %s", m, &stdout, &stderr)
	}
	if strings.Contains(stdout.String(), "/local/lesson.pdf") {
		t.Fatal("source path copied into candidate report")
	}
	m.capture.State = "invalid"
	m.capture.Issues = []domain.ValidationIssue{{Field: "source", Message: "source contains a link"}}
	m.plans = 0
	stdout.Reset()
	if runHomeFilesImport(t.Context(), m, "/lab", options{json: true}, &stdout, &stderr) != 1 || m.plans != 0 || m.applies != 0 {
		t.Fatal("failed capture continued")
	}
}
