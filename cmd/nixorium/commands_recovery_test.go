package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

type cliRecoverySource struct {
	pending  domain.PendingDeployment
	present  bool
	archived int
}

func (s *cliRecoverySource) PendingDeployment() (domain.PendingDeployment, bool, error) {
	return s.pending, s.present, nil
}
func (s *cliRecoverySource) ProcessRunning(int) bool { return false }
func (s *cliRecoverySource) ObserveDeploymentActivity(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.DeploymentActivity {
	result := map[string]domain.DeploymentActivity{}
	for _, host := range hosts {
		result[host.Name] = domain.DeploymentActivity{Reachable: true, Revision: s.pending.Revision}
	}
	return result
}
func (s *cliRecoverySource) ArchivePendingDeployment(domain.PendingDeployment) (string, error) {
	s.archived++
	s.present = false
	return "/var/lib/nixorium/coordination/recovered/deployment-x.json", nil
}

func TestDeployRecoverApplyAsksYesOrNo(t *testing.T) {
	newSource := func() *cliRecoverySource {
		return &cliRecoverySource{present: true, pending: domain.PendingDeployment{
			SchemaVersion: 1, Repository: "/srv/lab", Revision: strings.Repeat("a", 40), StartedAt: time.Now().UTC(),
			Targets: []domain.DeploymentTarget{{Name: "pc01", IP: "10.0.0.1"}},
		}}
	}
	record := func(message string, _ any) string { return message }
	token := func(source *cliRecoverySource) string {
		return app.NewDeploymentRecoveryManager(source).Plan(context.Background(), "/srv/lab", false).ReviewToken
	}
	run := func(source *cliRecoverySource, answer string, yes, interactive bool) (int, string, string) {
		var stdout, stderr bytes.Buffer
		opts := options{subcommand: "recover-apply", expect: token(source), yes: yes}
		code := deploymentRecoveryCommand(context.Background(), app.NewDeploymentRecoveryManager(source), "/srv/lab", opts, strings.NewReader(answer), interactive, record, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	source := newSource()
	code, _, stderr := run(source, "\n", false, true)
	if code != 1 || source.archived != 0 || !strings.Contains(stderr, "Mark this update as reviewed? [y/N]") || strings.Contains(stderr, "Type RECOVERED") {
		t.Fatalf("empty answer: code %d, archived %d, stderr:\n%s", code, source.archived, stderr)
	}
	code, stdout, _ := run(source, "y\n", false, true)
	if code != 0 || source.archived != 1 || !strings.Contains(stdout, "marked as reviewed") {
		t.Fatalf("yes: code %d, archived %d, stdout:\n%s", code, source.archived, stdout)
	}
	source = newSource()
	if code, _, _ := run(source, "", false, false); code != 2 || source.archived != 0 {
		t.Fatalf("non-interactive without --yes: code %d, archived %d", code, source.archived)
	}
	if code, _, stderr := run(source, "", true, false); code != 0 || source.archived != 1 || strings.Contains(stderr, "[y/N]") {
		t.Fatalf("--yes: code %d, archived %d, stderr %s", code, source.archived, stderr)
	}
}
