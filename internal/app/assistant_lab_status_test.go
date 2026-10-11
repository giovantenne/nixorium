package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type assistantStatusInspectorFunc func(context.Context, string) (domain.HostsReport, error)

func (f assistantStatusInspectorFunc) Hosts(ctx context.Context, repo string) (domain.HostsReport, error) {
	return f(ctx, repo)
}

type assistantStatusQueueFunc func(context.Context, string) domain.DeferredUpdateStatus

func (f assistantStatusQueueFunc) Status(ctx context.Context, repo string) domain.DeferredUpdateStatus {
	return f(ctx, repo)
}

func TestAssistantLabStatusObservation(t *testing.T) {
	at := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	var deadline time.Time
	status := AssistantLabStatus{
		Inspector: assistantStatusInspectorFunc(func(ctx context.Context, repo string) (domain.HostsReport, error) {
			if repo != "/deployment" {
				t.Fatal(repo)
			}
			var ok bool
			deadline, ok = ctx.Deadline()
			if !ok || time.Until(deadline) > 60*time.Second || time.Until(deadline) < 59*time.Second {
				t.Fatal("missing overall deadline")
			}
			return domain.HostsReport{GeneratedAt: at, DesiredRevision: strings.Repeat("a", 40), Repository: "/private/repository", HistoryDetail: "private log", Hosts: []domain.HostStatus{
				{Name: "pc01", Role: "client", IP: "10.42.0.1", Reachability: "reachable", SSH: "available", Deployment: "current", CurrentRevision: strings.Repeat("b", 40), CurrentSystem: "/nix/store/private-system", DeploymentDetail: "private SSH log", LastSuccessfulDeploy: &domain.LastSuccessfulDeployment{VerifiedAt: at}, Detail: "Bearer abcdef_123 /nix/store/private SHA256:abcd ssh-ed25519 YWJj\nprivate log"},
				{Name: "pc02", Role: "client", Detail: strings.Repeat("è", 400)},
			}}, nil
		}),
		DeferredUpdates: assistantStatusQueueFunc(func(ctx context.Context, repo string) domain.DeferredUpdateStatus {
			got, _ := ctx.Deadline()
			if got != deadline || repo != "/deployment" {
				t.Fatal("queue must share repository and deadline")
			}
			return domain.DeferredUpdateStatus{State: "ready", Updates: []domain.DeferredUpdateEntry{{DeferredUpdate: domain.DeferredUpdate{Host: "pc02", LastError: "private queue log"}}}}
		}),
	}
	result := status.Execute(t.Context(), "/deployment")
	for _, private := range []string{"abcdef_123", "/nix/store", "/private", "SHA256:", "YWJj", "private log", "private SSH", "private queue"} {
		if strings.Contains(result, private) {
			t.Fatalf("leaked %q: %s", private, result)
		}
	}
	var decoded struct {
		OK                           bool
		GeneratedAt, DesiredRevision string
		Hosts                        []assistantObservedHost
		PendingUpdates               []string
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK || decoded.GeneratedAt != at.Format(time.RFC3339) || decoded.DesiredRevision != strings.Repeat("a", 12) || len(decoded.Hosts) != 2 || !reflect.DeepEqual(decoded.PendingUpdates, []string{"pc02"}) {
		t.Fatal(result)
	}
	host := decoded.Hosts[0]
	if host.Name != "pc01" || host.Role != "client" || host.IP != "10.42.0.1" || host.Reachability != "reachable" || host.SSH != "available" || host.Deployment != "current" || host.CurrentRevision != strings.Repeat("b", 12) || host.LastSuccessfulUpdate != at.Format(time.RFC3339) || len([]rune(decoded.Hosts[1].Detail)) != 300 {
		t.Fatal(result)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(result), &fields)
	if len(fields) != 5 {
		t.Fatal("unexpected report fields", result)
	}
	var hosts []map[string]json.RawMessage
	_ = json.Unmarshal(fields["hosts"], &hosts)
	if len(hosts[0]) != 9 || len(hosts[1]) != 8 {
		t.Fatal("unexpected host fields", result)
	}
}

func TestAssistantLabStatusErrorsAndCancellation(t *testing.T) {
	const failure = `{"ok":false,"error":"status_unavailable"}`
	queueCalls := 0
	status := AssistantLabStatus{
		Inspector: assistantStatusInspectorFunc(func(ctx context.Context, _ string) (domain.HostsReport, error) {
			return domain.HostsReport{}, errors.New("private failure /nix/store/secret")
		}),
		DeferredUpdates: assistantStatusQueueFunc(func(context.Context, string) domain.DeferredUpdateStatus {
			queueCalls++
			return domain.DeferredUpdateStatus{State: "failed"}
		}),
	}
	if got := status.Execute(t.Context(), "repo"); got != failure || queueCalls != 0 {
		t.Fatal(got, queueCalls)
	}
	status.Inspector = assistantStatusInspectorFunc(func(context.Context, string) (domain.HostsReport, error) { return domain.HostsReport{}, nil })
	if got := status.Execute(t.Context(), "repo"); got != failure || queueCalls != 1 {
		t.Fatal(got, queueCalls)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	status.Inspector = assistantStatusInspectorFunc(func(ctx context.Context, _ string) (domain.HostsReport, error) {
		<-ctx.Done()
		return domain.HostsReport{}, ctx.Err()
	})
	if got := status.Execute(ctx, "repo"); got != failure || queueCalls != 1 {
		t.Fatal(got, queueCalls)
	}
	if got := (AssistantLabStatus{}).Execute(t.Context(), "repo"); got != failure {
		t.Fatal(got)
	}
}
