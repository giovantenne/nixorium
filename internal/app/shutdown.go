package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const shutdownObservationTimeout = 3 * time.Second
const shutdownDispatchTimeout = 8 * time.Second
const shutdownReviewWindow = 5 * time.Minute

type ShutdownSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	ServiceState(context.Context, string) domain.ServiceState
	ClientOperationActive() (bool, error)
	AcquireClientOperation() (io.Closer, error)
	ShutdownObservations(context.Context, []domain.HostMeta, time.Duration) map[string]domain.ShutdownObservation
	DispatchShutdowns(context.Context, []domain.HostMeta, time.Duration) map[string]domain.ShutdownDispatchResult
}

type ShutdownManager struct {
	source ShutdownSource
	now    func() time.Time
}

func NewShutdownManager(source ShutdownSource) *ShutdownManager {
	return &ShutdownManager{source: source, now: time.Now}
}

func (m *ShutdownManager) Plan(ctx context.Context, repository, requested string, policy domain.ShutdownSessionPolicy) domain.ShutdownPlanReport {
	report := domain.ShutdownPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "shutdown-plan",
		State:         "blocked",
		Requested:     requested,
		Policy:        policy,
		Targets:       []domain.ShutdownTargetPlan{},
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return shutdownPlanIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	if policy != domain.ShutdownRequireIdle && policy != domain.ShutdownAcknowledgeUnknown {
		return shutdownPlanIssue(report, "policy", "session policy must be require-idle or acknowledge-unknown")
	}
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return shutdownPlanIssue(report, "configuration", fmt.Sprintf("evaluate client inventory: %v", err))
	}
	selected, normalized, selectionIssues := selectDeploymentTargets(meta.Clients.Hosts, requested)
	report.Requested = normalized
	for _, issue := range selectionIssues {
		report = shutdownPlanIssue(report, "targets", issue)
	}
	if len(selectionIssues) > 0 {
		return report
	}
	active, err := m.source.ClientOperationActive()
	if err != nil {
		return shutdownPlanIssue(report, "operation", "inspect active client operations: "+err.Error())
	}
	if active {
		return shutdownPlanIssue(report, "operation", "another client deployment or shutdown is already running")
	}
	if conflict := shutdownPXEConflict(m.source, ctx); conflict != "" {
		return shutdownPlanIssue(report, "installation", conflict)
	}
	hosts := shutdownHostMeta(selected)
	report.Targets, report.Eligible = shutdownTargetsFromObservations(hosts, m.source.ShutdownObservations(ctx, hosts, shutdownObservationTimeout), policy)
	if report.Eligible == 0 {
		report.Message = "No selected computer is currently eligible for shutdown. Nothing will be queued for later."
		return shutdownPlanIssue(report, "targets", "power on a selected computer, restore management access, or review unknown-session risk")
	}
	report.State = "ready"
	report.ExpiresAt = m.now().UTC().Truncate(shutdownReviewWindow).Add(shutdownReviewWindow)
	report.ReviewToken = domain.ShutdownReviewToken(report)
	report.Confirmation = fmt.Sprintf("SHUTDOWN %d CLIENTS %s", report.Eligible, report.ReviewToken[len("sha256:"):len("sha256:")+12])
	report.Message = fmt.Sprintf("%d of %d selected computer(s) are eligible; checks will run again before requests are sent.", report.Eligible, len(report.Targets))
	return report
}

func (m *ShutdownManager) ApplyPlan(ctx context.Context, plan domain.ShutdownPlanReport, expectedToken string) domain.ShutdownApplyReport {
	report := shutdownApplyFromPlan(plan)
	if plan.HasErrors() || plan.State != "ready" || plan.ReviewToken == "" {
		report.Message = "Shutdown was not started because the reviewed plan is not ready."
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken || plan.ReviewToken != domain.ShutdownReviewToken(plan) {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "review", Message: "shutdown review token does not match the frozen plan"})
		report.Message = "Shutdown was not started; create and review a fresh plan."
		return report
	}
	if !m.now().UTC().Before(plan.ExpiresAt) {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "expiry", Message: "shutdown review expired before dispatch"})
		report.Message = "Shutdown was not started because computer and session observations are stale."
		return report
	}
	lease, err := m.source.AcquireClientOperation()
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "operation", Message: err.Error()})
		report.Message = "Shutdown was not started because another client operation is running."
		return report
	}
	defer lease.Close()
	if conflict := shutdownPXEConflict(m.source, ctx); conflict != "" {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "installation", Message: conflict})
		report.Message = "Shutdown was not started because installation or controller-network recovery is active."
		return report
	}
	meta, err := m.source.LabMeta(ctx, plan.Repository)
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "configuration", Message: err.Error()})
		report.Message = "Shutdown was not started because the client inventory could not be rechecked."
		return report
	}
	if !shutdownInventoryMatches(meta, plan.Targets) {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "targets", Message: "evaluated client identities or addresses changed after review"})
		report.Message = "Shutdown was not started; create and review a fresh plan."
		return report
	}

	eligible := []domain.HostMeta{}
	outcomes := map[string]domain.ShutdownTargetOutcome{}
	observations := m.source.ShutdownObservations(ctx, shutdownPlanHosts(plan.Targets, true), shutdownObservationTimeout)
	for _, target := range plan.Targets {
		if !target.Eligible {
			outcomes[target.Name] = domain.ShutdownTargetOutcome{Name: target.Name, State: "not-sent", Detail: "not eligible in the reviewed plan"}
			report.NotSent++
			continue
		}
		observed := observations[target.Name]
		if ok, detail := shutdownObservationEligible(observed, plan.Policy); !ok {
			outcomes[target.Name] = domain.ShutdownTargetOutcome{Name: target.Name, State: "not-sent", Detail: "pre-dispatch check: " + detail}
			report.NotSent++
			continue
		}
		eligible = append(eligible, domain.HostMeta{Name: target.Name, IP: target.IP})
	}

	dispatched := m.source.DispatchShutdowns(ctx, eligible, shutdownDispatchTimeout)
	for _, host := range eligible {
		result, found := dispatched[host.Name]
		if found && result.Accepted {
			outcomes[host.Name] = domain.ShutdownTargetOutcome{Name: host.Name, State: "accepted", Detail: "the operating system accepted the power-off request"}
			report.Accepted++
		} else {
			technicalDetail := "no dispatch result was returned"
			if found && result.Detail != "" {
				technicalDetail = result.Detail
			}
			outcomes[host.Name] = domain.ShutdownTargetOutcome{
				Name:            host.Name,
				State:           "unconfirmed",
				Detail:          "request result could not be confirmed; inspect the computer before retrying",
				TechnicalDetail: technicalDetail,
			}
			report.Unconfirmed++
		}
	}
	for _, target := range plan.Targets {
		report.Targets = append(report.Targets, outcomes[target.Name])
	}
	report.RetrySafe = len(eligible) == 0
	switch {
	case len(eligible) == 0:
		report.State = "blocked"
		report.Message = "No shutdown request was sent because every reviewed target became ineligible. Create a fresh plan."
	case report.Unconfirmed > 0 || report.NotSent > 0:
		report.State = "partial"
		report.Message = fmt.Sprintf("Requests accepted for %d computer(s); %d not sent and %d unconfirmed. Do not retry blindly.", report.Accepted, report.NotSent, report.Unconfirmed)
	default:
		report.State = "completed"
		report.Message = fmt.Sprintf("Power-off requests were accepted for all %d reviewed computer(s). Physical power state is not inferred from network loss.", report.Accepted)
	}
	return report
}

func shutdownPXEConflict(source ShutdownSource, ctx context.Context) string {
	listener := source.ServiceState(ctx, PXEListenerUnit)
	network := source.ServiceState(ctx, PXENetworkUnit)
	observed := ObservePXELifecycle(listener, network, domain.PXEPreparationState{})
	switch observed.Mode {
	case "active":
		return "network installation is active; stop it before shutting down clients"
	case "degraded", "recovery-required":
		return "controller network or installation state requires recovery before shutting down clients"
	default:
		return ""
	}
}

func shutdownTargetsFromObservations(hosts []domain.HostMeta, observations map[string]domain.ShutdownObservation, policy domain.ShutdownSessionPolicy) ([]domain.ShutdownTargetPlan, int) {
	result := make([]domain.ShutdownTargetPlan, 0, len(hosts))
	eligible := 0
	for _, host := range hosts {
		observation, found := observations[host.Name]
		if !found {
			observation = domain.ShutdownObservation{Reachability: domain.ReachabilityUnknown, SSH: domain.SSHUnknown, Session: domain.ShutdownSessionUnknown, Detail: "no observation returned"}
		}
		ok, detail := shutdownObservationEligible(observation, policy)
		if ok {
			eligible++
		}
		result = append(result, domain.ShutdownTargetPlan{Name: host.Name, IP: host.IP, Reachability: observation.Reachability, SSH: observation.SSH, Session: observation.Session, Eligible: ok, Detail: detail})
	}
	return result, eligible
}

func shutdownObservationEligible(observation domain.ShutdownObservation, policy domain.ShutdownSessionPolicy) (bool, string) {
	if observation.Reachability != domain.ReachabilityReachable || observation.SSH != domain.SSHAvailable {
		if observation.Detail != "" {
			return false, observation.Detail
		}
		return false, "authenticated management access is unavailable"
	}
	switch observation.Session {
	case domain.ShutdownSessionIdle:
		return true, "no interactive user session detected"
	case domain.ShutdownSessionActive:
		return false, "an interactive user session is active"
	case domain.ShutdownSessionUnknown:
		if policy == domain.ShutdownAcknowledgeUnknown {
			return true, "session state is unknown; risk explicitly acknowledged"
		}
		if observation.Detail != "" {
			return false, observation.Detail
		}
		return false, "session state is unknown"
	default:
		return false, "session observation is invalid"
	}
}

func shutdownHostMeta(targets []domain.DeploymentTarget) []domain.HostMeta {
	hosts := make([]domain.HostMeta, 0, len(targets))
	for _, target := range targets {
		hosts = append(hosts, domain.HostMeta{Name: target.Name, IP: target.IP})
	}
	return hosts
}

func shutdownPlanHosts(targets []domain.ShutdownTargetPlan, eligibleOnly bool) []domain.HostMeta {
	hosts := []domain.HostMeta{}
	for _, target := range targets {
		if !eligibleOnly || target.Eligible {
			hosts = append(hosts, domain.HostMeta{Name: target.Name, IP: target.IP})
		}
	}
	return hosts
}

func shutdownInventoryMatches(meta domain.LabMeta, targets []domain.ShutdownTargetPlan) bool {
	clients := map[string]string{}
	for _, host := range meta.Clients.Hosts {
		clients[host.Name] = host.IP
	}
	for _, target := range targets {
		if target.Name == meta.Controller.Name || clients[target.Name] != target.IP {
			return false
		}
	}
	return true
}

func shutdownPlanIssue(report domain.ShutdownPlanReport, field, message string) domain.ShutdownPlanReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	if report.Message == "" {
		report.Message = message
	}
	return report
}

func shutdownApplyFromPlan(plan domain.ShutdownPlanReport) domain.ShutdownApplyReport {
	return domain.ShutdownApplyReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "shutdown-apply",
		State:         "blocked",
		Repository:    plan.Repository,
		Requested:     plan.Requested,
		Policy:        plan.Policy,
		Targets:       []domain.ShutdownTargetOutcome{},
		RetrySafe:     true,
		Issues:        []domain.ValidationIssue{},
	}
}
