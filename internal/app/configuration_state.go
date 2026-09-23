package app

import (
	"context"
	"fmt"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type configurationHosts interface {
	Hosts(context.Context, string) (domain.HostsReport, error)
}

type configurationController interface {
	Plan(context.Context, string) domain.ControllerRebuildPlanReport
}

type ConfigurationStateManager struct {
	hosts      configurationHosts
	controller configurationController
	now        func() time.Time
}

func NewConfigurationStateManager(hosts configurationHosts, controller configurationController) *ConfigurationStateManager {
	return &ConfigurationStateManager{hosts: hosts, controller: controller, now: time.Now}
}

func (m *ConfigurationStateManager) Load(ctx context.Context, repository string) domain.ConfigurationStateReport {
	report := domain.ConfigurationStateReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "configuration-state",
		GeneratedAt:   m.now().UTC(),
		State:         "available",
		Issues:        []domain.ValidationIssue{},
	}

	clients, clientsErr := m.hosts.Hosts(ctx, repository)
	if clientsErr != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "clients", Message: clientsErr.Error()})
	} else {
		report.Clients = clients
		report.Repository = clients.Repository
		report.DesiredRevision = clients.DesiredRevision
		if !clients.GeneratedAt.IsZero() {
			report.GeneratedAt = clients.GeneratedAt.UTC()
		}
	}

	report.Controller = m.controller.Plan(ctx, repository)
	if report.Repository == "" {
		report.Repository = report.Controller.Repository
	}
	for _, issue := range report.Controller.Issues {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "controller." + issue.Field, Message: issue.Message})
	}
	if report.Controller.Operation == "" {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "controller", Message: "controller state is unavailable"})
	} else if report.Controller.HasErrors() && len(report.Controller.Issues) == 0 {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "controller", Message: "controller state could not be verified"})
	}

	controllerRevision := report.Controller.Revision
	switch {
	case report.DesiredRevision == "":
		report.DesiredRevision = controllerRevision
	case controllerRevision == "":
	case report.DesiredRevision != controllerRevision:
		report.Issues = append(report.Issues, domain.ValidationIssue{
			Field:   "revision",
			Message: fmt.Sprintf("deployment revision changed while state was observed: clients %s, controller %s", report.DesiredRevision, controllerRevision),
		})
		report.DesiredRevision = ""
	}

	if report.HasErrors() {
		report.State = "partial"
	}
	return report
}
