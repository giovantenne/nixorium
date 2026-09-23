package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type configurationHostsFake struct {
	report domain.HostsReport
	err    error
}

func (fake configurationHostsFake) Hosts(context.Context, string) (domain.HostsReport, error) {
	return fake.report, fake.err
}

type configurationControllerFake struct {
	report domain.ControllerRebuildPlanReport
}

func (fake configurationControllerFake) Plan(context.Context, string) domain.ControllerRebuildPlanReport {
	return fake.report
}

func TestConfigurationStateUsesCurrentEvidence(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	observedAt := time.Date(2026, 9, 23, 10, 30, 0, 0, time.UTC)
	manager := NewConfigurationStateManager(
		configurationHostsFake{report: domain.HostsReport{
			Operation:       "hosts",
			GeneratedAt:     observedAt,
			Repository:      "/lab",
			DesiredRevision: revision,
			Deployment:      domain.HostDeploymentSummary{Current: 1, Unknown: 1},
			Hosts: []domain.HostStatus{
				{Name: "pc01", Deployment: domain.DeploymentCurrent, CurrentRevision: revision, DesiredRevision: revision},
				{
					Name: "pc02", Deployment: domain.DeploymentUnknown, DesiredRevision: revision,
					LastSuccessfulDeploy: &domain.LastSuccessfulDeployment{Revision: revision, VerifiedAt: observedAt.Add(-time.Hour)},
				},
			},
		}},
		configurationControllerFake{report: domain.ControllerRebuildPlanReport{
			Operation: "controller-plan", State: "current", Repository: "/lab", Revision: revision, Current: true,
		}},
	)

	report := manager.Load(context.Background(), "/lab")
	if report.HasErrors() || report.State != "available" || report.DesiredRevision != revision || !report.GeneratedAt.Equal(observedAt) {
		t.Fatalf("configuration state = %+v", report)
	}
	if !report.ControllerVerified() {
		t.Fatal("matching durable controller plan was not reported as verified")
	}
	if report.Clients.Hosts[1].Deployment != domain.DeploymentUnknown {
		t.Fatal("historical client success was promoted to current evidence")
	}
}

func TestConfigurationStateRejectsMixedRevisionSnapshot(t *testing.T) {
	clientRevision := "0123456789abcdef0123456789abcdef01234567"
	controllerRevision := "89abcdef0123456789abcdef0123456789abcdef"
	manager := NewConfigurationStateManager(
		configurationHostsFake{report: domain.HostsReport{Operation: "hosts", DesiredRevision: clientRevision}},
		configurationControllerFake{report: domain.ControllerRebuildPlanReport{
			Operation: "controller-plan", State: "current", Revision: controllerRevision, Current: true,
		}},
	)

	report := manager.Load(context.Background(), "/lab")
	if report.State != "partial" || report.DesiredRevision != "" || report.ControllerVerified() {
		t.Fatalf("mixed revision report = %+v", report)
	}
	if len(report.Issues) != 1 || report.Issues[0].Field != "revision" {
		t.Fatalf("mixed revision issues = %+v", report.Issues)
	}
}

func TestConfigurationStateKeepsPartialResults(t *testing.T) {
	revision := "0123456789abcdef0123456789abcdef01234567"
	manager := NewConfigurationStateManager(
		configurationHostsFake{err: errors.New("client observation failed")},
		configurationControllerFake{report: domain.ControllerRebuildPlanReport{
			Operation: "controller-plan", State: "current", Repository: "/lab", Revision: revision, Current: true,
		}},
	)

	report := manager.Load(context.Background(), "/lab")
	if report.State != "partial" || report.DesiredRevision != revision || len(report.Issues) != 1 {
		t.Fatalf("partial configuration state = %+v", report)
	}
	if !report.ControllerVerified() {
		t.Fatal("available controller evidence was discarded with the client failure")
	}
}
