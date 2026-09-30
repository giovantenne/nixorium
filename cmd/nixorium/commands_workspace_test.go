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

func TestWorkspaceArguments(t *testing.T) {
	for _, command := range []string{
		"workspace plan --file candidate.json",
		"workspace plan --file candidate.json --repo /deployment --json",
		"workspace apply --file candidate.json --expect sha256:review --yes --json",
		"workspace apply --file candidate.json --expect sha256:review",
		"workspace marketplace --extension platformio.platformio-ide",
		"workspace marketplace --extension platformio.platformio-ide --repo /deployment --json",
	} {
		if _, err := parseArguments(strings.Fields(command)); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	for _, command := range []string{
		"workspace", "workspace catalog", "workspace plan", "workspace apply --file candidate.json --yes",
		"workspace plan --file candidate.json --yes", "workspace plan --file candidate.json --expect token",
		"workspace plan --file candidate.json --scope shared", "workspace plan --file candidate.json --on pc01",
		"workspace plan --file candidate.json --remove", "workspace plan --file candidate.json --target master",
		"workspace apply --file candidate.json --expect token --full",
		"workspace marketplace", "workspace marketplace --file candidate.json --extension a.b",
		"workspace marketplace --extension a.b --yes", "workspace plan --file candidate.json --extension a.b",
		"software search --query code --extension a.b",
	} {
		if _, err := parseArguments(strings.Fields(command)); err == nil {
			t.Fatalf("invalid command accepted: %s", command)
		}
	}
}

type fakeWorkspaceCommands struct {
	plan           domain.WorkspacePlanReport
	result         domain.WorkspaceApplyReport
	plans, applies int
	expected       string
	data           []byte
}

func (f *fakeWorkspaceCommands) Plan(_ context.Context, _ string, data []byte) domain.WorkspacePlanReport {
	f.plans++
	f.data = append([]byte(nil), data...)
	return f.plan
}

func (f *fakeWorkspaceCommands) Apply(_ context.Context, _ string, data []byte, token string) domain.WorkspaceApplyReport {
	f.applies++
	f.data, f.expected = append([]byte(nil), data...), token
	return f.result
}

func TestWorkspaceCommandReviewAndSaveBoundaries(t *testing.T) {
	for _, item := range []struct {
		name, subcommand, planState, token, resultState string
		yes, approved                                   bool
		confirmErr                                      error
		wantCode, wantConfirm, wantApply                int
		wantState                                       string
	}{
		{name: "plan", subcommand: "plan", planState: "ready", wantState: "ready"},
		{name: "confirmed", subcommand: "apply", planState: "ready", token: "review", resultState: "saved", approved: true, wantConfirm: 1, wantApply: 1, wantState: "saved"},
		{name: "yes", subcommand: "apply", planState: "ready", token: "review", resultState: "saved", yes: true, wantApply: 1, wantState: "saved"},
		{name: "cancelled", subcommand: "apply", planState: "ready", token: "review", wantConfirm: 1, wantState: "cancelled"},
		{name: "noninteractive", subcommand: "apply", planState: "ready", token: "review", confirmErr: errors.New("terminal required"), wantCode: 1, wantConfirm: 1},
		{name: "stale before prompt", subcommand: "apply", planState: "ready", token: "stale", wantCode: 1, wantState: "conflict"},
		{name: "invalid before prompt", subcommand: "apply", planState: "invalid", token: "review", wantCode: 1, wantState: "invalid"},
		{name: "unchanged", subcommand: "apply", planState: "unchanged", token: "review", resultState: "unchanged", wantApply: 1, wantState: "unchanged"},
		{name: "drift during confirmation", subcommand: "apply", planState: "ready", token: "review", resultState: "conflict", approved: true, wantConfirm: 1, wantApply: 1, wantCode: 1, wantState: "conflict"},
		{name: "durability", subcommand: "apply", planState: "ready", token: "review", resultState: "partial", yes: true, wantApply: 1, wantCode: 1, wantState: "partial"},
	} {
		t.Run(item.name, func(t *testing.T) {
			manager := &fakeWorkspaceCommands{
				plan:   domain.WorkspacePlanReport{SchemaVersion: 1, Operation: "workspace-plan", State: item.planState, ReviewToken: "review", Confirmation: "SAVE"},
				result: domain.WorkspaceApplyReport{SchemaVersion: 1, Operation: "workspace-apply", State: item.resultState},
			}
			var stdout, stderr bytes.Buffer
			confirmations := 0
			confirm := func(domain.WorkspacePlanReport) (bool, error) { confirmations++; return item.approved, item.confirmErr }
			candidate := []byte(`{"schemaVersion":1}`)
			code := runWorkspaceWithManager(t.Context(), manager, "/deployment", candidate, options{subcommand: item.subcommand, expect: item.token, yes: item.yes, json: true}, confirm, &stdout, &stderr)
			if code != item.wantCode || confirmations != item.wantConfirm || manager.applies != item.wantApply {
				t.Fatalf("code=%d confirmations=%d applies=%d output=%s error=%s", code, confirmations, manager.applies, &stdout, &stderr)
			}
			if item.wantApply > 0 && (manager.expected != item.token || string(manager.data) != string(candidate)) {
				t.Fatal("review token/candidate not forwarded")
			}
			if item.wantState != "" {
				var report struct {
					State string `json:"state"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.State != item.wantState {
					t.Fatalf("structured stdout: %s (%v)", &stdout, err)
				}
			}
		})
	}
}
