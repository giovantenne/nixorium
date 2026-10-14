package app_test

// End-to-end contract test: the real Go Assistant client and turn runner talk
// to a real Assistant service Worker over HTTP.
//
// The test is skipped unless NIXORIUM_ASSISTANT_E2E_URL is set. It is driven by
// the private service repository (service/e2e), which starts the Worker locally
// with a simulated model provider, seeds three accounts and exports:
//
//	NIXORIUM_ASSISTANT_E2E_URL              base URL of the Worker (http://127.0.0.1:port)
//	NIXORIUM_ASSISTANT_E2E_TRIAL_CODE       activation code of a fresh 10-request trial account
//	NIXORIUM_ASSISTANT_E2E_EXHAUSTED_CODE   code of an account whose quota is used up
//	NIXORIUM_ASSISTANT_E2E_EXPIRED_CODE     code of an account whose period has ended
//	NIXORIUM_ASSISTANT_E2E_PAID_CODE        code of an active annual subscription (1000 requests)
//	NIXORIUM_ASSISTANT_E2E_CORE_REV         40-hex revision whose source the Worker can serve
//
// The simulated model chooses its answer from the user prompt ("e2e:final",
// "e2e:tools", "e2e:core", "e2e:forbidden", "e2e:unknown-tool").

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

const e2eVersion = "3.5.0"

type e2eEnv struct {
	url, trial, exhausted, expired, paid, coreRev string
}

func loadE2E(t *testing.T) e2eEnv {
	t.Helper()
	url := os.Getenv("NIXORIUM_ASSISTANT_E2E_URL")
	if url == "" {
		t.Skip("NIXORIUM_ASSISTANT_E2E_URL is not set; run service/e2e from the private service repository")
	}
	env := e2eEnv{
		url:       strings.TrimRight(url, "/") + "/v1",
		trial:     os.Getenv("NIXORIUM_ASSISTANT_E2E_TRIAL_CODE"),
		exhausted: os.Getenv("NIXORIUM_ASSISTANT_E2E_EXHAUSTED_CODE"),
		expired:   os.Getenv("NIXORIUM_ASSISTANT_E2E_EXPIRED_CODE"),
		paid:      os.Getenv("NIXORIUM_ASSISTANT_E2E_PAID_CODE"),
		coreRev:   os.Getenv("NIXORIUM_ASSISTANT_E2E_CORE_REV"),
	}
	if env.trial == "" || env.exhausted == "" || env.expired == "" || env.paid == "" || env.coreRev == "" {
		t.Fatal("incomplete NIXORIUM_ASSISTANT_E2E_* environment")
	}
	return env
}

func e2eCode(t *testing.T, err error, status int, code string) {
	t.Helper()
	var assistant *domain.AssistantError
	if !errors.As(err, &assistant) {
		t.Fatalf("expected Assistant error %s, got %v", code, err)
	}
	if assistant.Code != code || (status != 0 && assistant.Status != status) {
		t.Fatalf("expected %d %s, got %d %s (%s)", status, code, assistant.Status, assistant.Code, assistant.Message)
	}
}

// e2eRunner builds the production runner (real workspace store and tools) for
// a new local conversation that holds token, with deterministic lab tools.
func e2eRunner(t *testing.T, env e2eEnv, token string) (app.AssistantTurnRunner, domain.AssistantConversation, *[]string) {
	t.Helper()
	repo := t.TempDir()
	runnerGit(t, repo, "init", "-q")
	if err := os.WriteFile(repo+"/flake.nix", []byte("{outputs = _: {}; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runnerGit(t, repo, "add", "flake.nix")
	runnerGit(t, repo, "commit", "-qm", "synthetic base")
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	store := adapters.AssistantWorkspaceStore{Directory: state}
	local := adapters.AssistantStateStore{Directory: state}
	if err := local.WriteToken(token); err != nil {
		t.Fatal(err)
	}
	if err := local.WriteConsent(); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.CreateConversation(repo)
	if err != nil {
		t.Fatal(err)
	}
	executed := &[]string{}
	tools := adapters.AssistantWorkspaceTools{
		Store: store,
		LabStatus: func(context.Context, string) string {
			*executed = append(*executed, "lab_status")
			return `{"ok":true,"computers":[]}`
		},
		Diagnostics: func(_ context.Context, _, host, area string) string {
			*executed = append(*executed, "host_diagnostics:"+host+":"+area)
			return `{"ok":true,"summary":"synthetic"}`
		},
	}
	runner := app.AssistantTurnRunner{
		Client:  adapters.AssistantClient{Endpoint: env.url, Version: e2eVersion},
		Tools:   tools,
		Store:   store,
		Version: e2eVersion,
	}
	return runner, conversation, executed
}

type e2eRun struct {
	events  []domain.AssistantEvent
	result  domain.AssistantTurnResult
	err     error
	text    string
	final   *domain.AssistantEvent
	acts    []string
	started bool
}

func e2eAsk(t *testing.T, runner app.AssistantTurnRunner, conversation domain.AssistantConversation, prompt string) e2eRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	var run e2eRun
	run.result, run.err = runner.Run(ctx, conversation, prompt, func(event domain.AssistantEvent) {
		run.events = append(run.events, event)
		switch event.Type {
		case "turn_started":
			run.started = true
		case "text":
			run.text += event.Delta
		case "activity":
			run.acts = append(run.acts, event.Text)
		case "final":
			copy := event
			run.final = &copy
		}
	})
	return run
}

func TestAssistantServiceE2E(t *testing.T) {
	env := loadE2E(t)
	client := adapters.AssistantClient{Endpoint: env.url, Version: e2eVersion}
	ctx := t.Context()

	var token string
	t.Run("offer", func(t *testing.T) {
		// The configured trial terms are announced to clients that send their version.
		offer, err := client.Offer(ctx)
		if err != nil || offer.TrialRequests < 1 || offer.TrialDays < 1 {
			t.Fatal(err, offer)
		}
	})
	t.Run("activation returns the complete account", func(t *testing.T) {
		activation, err := client.Activate(ctx, env.trial)
		if err != nil {
			t.Fatal(err)
		}
		token = activation.Token
		// Regression: activation used to omit status and turnsRemaining, so
		// the TUI header showed "Free trial ended" for a valid trial.
		account := activation.Account
		if account.Plan != "trial" || account.Status != "active" || account.TurnsRemaining != 10 || account.TrialRequests != 10 || account.PeriodEnd == "" || account.Name == "" {
			t.Fatalf("incomplete activation account: %+v", account)
		}
		_, err = client.Activate(ctx, env.trial)
		e2eCode(t, err, 400, "invalid_code")
		_, err = client.Activate(ctx, "no-such-code")
		e2eCode(t, err, 400, "invalid_code")
	})
	t.Run("account", func(t *testing.T) {
		account, err := client.Account(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if account.Status != "active" || account.TurnsRemaining != 10 || account.TrialRequests != 10 || account.Plan != "trial" || account.UpgradeURL == "" {
			t.Fatalf("unexpected account: %+v", account)
		}
		_, err = client.Account(ctx, strings.Repeat("x", 43))
		e2eCode(t, err, 401, "invalid_token")
	})

	t.Run("laboratory name", func(t *testing.T) {
		account, err := client.SetLabName(ctx, token, "Room 12")
		if err != nil || account.LabName != "Room 12" || account.Status != "active" || account.TurnsRemaining != 10 {
			t.Fatal(err, account)
		}
		account, err = client.SetLabName(ctx, token, "")
		if err != nil || account.LabName != "" {
			t.Fatal(err, account)
		}
		_, err = client.SetLabName(ctx, token, "bad\x01name")
		e2eCode(t, err, 400, "invalid_request")
	})

	t.Run("final answer", func(t *testing.T) {
		runner, conversation, _ := e2eRunner(t, env, token)
		run := e2eAsk(t, runner, conversation, "e2e:final")
		if run.err != nil || run.result.Outcome != "final" || run.final == nil || !run.started {
			t.Fatalf("%v %+v", run.err, run.result)
		}
		if !strings.Contains(run.text, "Hello from the simulated model") || run.final.Summary == "" {
			t.Fatalf("unexpected answer %q / %q", run.text, run.final.Summary)
		}
		account, err := client.Account(ctx, token)
		if err != nil || account.TurnsRemaining != 9 {
			t.Fatal(err, account)
		}
	})

	t.Run("tool calls with continuation until final", func(t *testing.T) {
		runner, conversation, executed := e2eRunner(t, env, token)
		run := e2eAsk(t, runner, conversation, "e2e:tools")
		if run.err != nil || run.result.Outcome != "final" || run.final == nil {
			t.Fatalf("%v %+v", run.err, run.result)
		}
		if strings.Join(*executed, ",") != "lab_status,host_diagnostics:lab-pc:system" {
			t.Fatalf("tools executed: %v", *executed)
		}
		if !strings.Contains(run.text, "Both checks finished") {
			t.Fatalf("missing continuation text %q", run.text)
		}
		// Assistant tool_calls message followed by one tool message per call.
		roles := []string{}
		for _, message := range run.result.Messages {
			roles = append(roles, message.Role)
		}
		if strings.Join(roles, ",") != "user,assistant,tool,tool,assistant" {
			t.Fatalf("history roles: %v", roles)
		}
	})

	t.Run("read_core service messages followed by a local tool", func(t *testing.T) {
		runner, conversation, executed := e2eRunner(t, env, token)
		runner.CoreOverride = &domain.AssistantCore{Owner: "giovantenne", Repo: "nixorium", Rev: env.coreRev}
		run := e2eAsk(t, runner, conversation, "e2e:core")
		if run.err != nil || run.result.Outcome != "final" {
			t.Fatalf("%v %+v", run.err, run.result)
		}
		if strings.Join(*executed, ",") != "lab_status" {
			t.Fatalf("tools executed: %v", *executed)
		}
		seen := false
		for _, message := range run.result.Messages {
			for _, call := range message.ToolCalls {
				seen = seen || call.Name == "read_core"
			}
		}
		if !seen {
			t.Fatal("service read_core messages were not kept in history")
		}
	})

	t.Run("sharing and answer reports", func(t *testing.T) {
		runner, conversation, _ := e2eRunner(t, env, token)
		run := e2eAsk(t, runner, conversation, "e2e:tools")
		if run.err != nil {
			t.Fatal(run.err)
		}
		store, ok := runner.Store.(app.AssistantSharingStore)
		if !ok {
			t.Fatal("workspace store cannot share")
		}
		reporter := app.AssistantConversationReporter{Store: store, Client: client, Version: e2eVersion}
		if err := reporter.Report(ctx, conversation, "helpful", "Thanks"); err != nil {
			t.Fatal(err)
		}
		if err := reporter.Report(ctx, conversation, "wrong", ""); err != nil {
			t.Fatal(err)
		}
		if err := client.ShareConversation(ctx, token, conversation.ID, domain.AssistantStepRequest{
			Context:  &domain.AssistantContext{Nixorium: e2eVersion, Files: []string{"flake.nix"}},
			Messages: run.result.Messages,
		}); err != nil {
			t.Fatal(err)
		}
	})

	step := func(t *testing.T, turnID string, capabilities []string, prompt string) (calls []domain.AssistantToolCall, terminal string, code string, err error) {
		t.Helper()
		request := domain.AssistantStepRequest{
			Context:  &domain.AssistantContext{Nixorium: e2eVersion, Files: []string{"flake.nix"}, Capabilities: capabilities},
			Messages: []domain.AssistantMessage{{Role: "user", Content: prompt}},
		}
		err = client.Step(ctx, token, turnID, request, func(event domain.AssistantEvent) {
			switch event.Type {
			case "tool_calls":
				calls, terminal = event.Calls, event.Type
			case "final", "error":
				terminal, code = event.Type, event.Code
			}
		})
		return
	}
	openTurn := func(t *testing.T) string {
		t.Helper()
		turn, err := client.OpenTurn(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		return turn.TurnID
	}

	t.Run("capability header gates host_diagnostics", func(t *testing.T) {
		for _, capable := range []bool{false, true} {
			turnID := openTurn(t)
			capabilities := []string(nil)
			if capable {
				capabilities = []string{"prepare_npm_v1", "host_diagnostics_v1", "git_history_v1"}
			}
			calls, terminal, _, err := step(t, turnID, capabilities, "e2e:tools")
			if err != nil || terminal != "tool_calls" {
				t.Fatalf("capable=%v: %v %q", capable, err, terminal)
			}
			names := []string{}
			for _, call := range calls {
				names = append(names, call.Name)
			}
			want := "lab_status"
			if capable {
				want = "lab_status,host_diagnostics"
			}
			if strings.Join(names, ",") != want {
				t.Fatalf("capable=%v: calls %v, want %s", capable, names, want)
			}
			if err := client.Cancel(ctx, token, turnID); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("a gated tool call from the model is a model error, not a protocol error", func(t *testing.T) {
		turnID := openTurn(t)
		_, terminal, code, err := step(t, turnID, nil, "e2e:forbidden")
		if err != nil || terminal != "error" || code != "model_error" {
			t.Fatalf("%v %q %q", err, terminal, code)
		}
	})
	t.Run("an unknown tool from the model is a model error", func(t *testing.T) {
		turnID := openTurn(t)
		_, terminal, code, err := step(t, turnID, []string{"host_diagnostics_v1"}, "e2e:unknown-tool")
		if err != nil || terminal != "error" || code != "model_error" {
			t.Fatalf("%v %q %q", err, terminal, code)
		}
	})
	t.Run("step errors decode into codes", func(t *testing.T) {
		turnID := openTurn(t)
		_, _, _, err := step(t, turnID, nil, "e2e:final")
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, err = step(t, turnID, nil, "e2e:final")
		e2eCode(t, err, 410, "turn_closed")
		_, _, _, err = step(t, "00000000-0000-4000-8000-000000000000", nil, "e2e:final")
		e2eCode(t, err, 404, "turn_not_found")
		first := openTurn(t)
		_, err = client.OpenTurn(ctx, token)
		e2eCode(t, err, 409, "turn_active")
		if err := client.Cancel(ctx, token, first); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("paid subscription", func(t *testing.T) {
		paid, err := client.Activate(ctx, env.paid)
		if err != nil {
			t.Fatal(err)
		}
		if paid.Account.Plan != "annual" || paid.Account.Status != "active" || paid.Account.TurnsRemaining != 1000 {
			t.Fatalf("unexpected paid account %+v", paid.Account)
		}
		account, err := client.Account(ctx, paid.Token)
		if err != nil || account.UpgradeURL != "" || account.TurnsRemaining != 1000 {
			t.Fatal(err, account)
		}
	})

	t.Run("turn admission errors", func(t *testing.T) {
		_, err := client.OpenTurn(ctx, strings.Repeat("y", 43))
		e2eCode(t, err, 401, "invalid_token")
		old := adapters.AssistantClient{Endpoint: env.url, Version: "not-a-version"}
		_, err = old.OpenTurn(ctx, token)
		e2eCode(t, err, 426, "client_upgrade_required")

		exhausted, err := client.Activate(ctx, env.exhausted)
		if err != nil {
			t.Fatal(err)
		}
		if exhausted.Account.Status != "active" || exhausted.Account.TurnsRemaining != 0 {
			t.Fatalf("unexpected exhausted account %+v", exhausted.Account)
		}
		_, err = client.OpenTurn(ctx, exhausted.Token)
		e2eCode(t, err, 429, "quota_exhausted")
		runner, conversation, _ := e2eRunner(t, env, exhausted.Token)
		run := e2eAsk(t, runner, conversation, "e2e:final")
		e2eCode(t, run.err, 429, "quota_exhausted")
		if run.started {
			t.Fatal("a rejected turn must not start")
		}

		expired, err := client.Activate(ctx, env.expired)
		if err != nil {
			t.Fatal(err)
		}
		if expired.Account.Status != "inactive" {
			t.Fatalf("unexpected expired account %+v", expired.Account)
		}
		_, err = client.OpenTurn(ctx, expired.Token)
		e2eCode(t, err, 402, "subscription_inactive")
	})
}
