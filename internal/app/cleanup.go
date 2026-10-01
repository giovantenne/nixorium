package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const cleanupObservationTimeout = 5 * time.Second

// A cleanup collects garbage, which can take minutes on a large store.
const cleanupDispatchTimeout = 30 * time.Minute
const cleanupReviewWindow = 10 * time.Minute

// CleanupSource observes and removes old system generations (ADR 0022).
type CleanupSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	ServiceState(context.Context, string) domain.ServiceState
	ClientOperationActive() (bool, error)
	AcquireClientOperation() (io.Closer, error)
	ObserveControllerGenerations(context.Context) domain.CleanupObservation
	ObserveClientGenerations(context.Context, []domain.HostMeta, time.Duration) map[string]domain.CleanupObservation
	CleanControllerGenerations(context.Context, string) error
	CleanClientGenerations(context.Context, map[string]string, []domain.HostMeta, time.Duration) map[string]domain.CleanupDispatchResult
}

type CleanupManager struct {
	source CleanupSource
	now    func() time.Time
}

func NewCleanupManager(source CleanupSource) *CleanupManager {
	return &CleanupManager{source: source, now: time.Now}
}

// Plan observes the requested computers read-only. requested lists client
// names, "@lab" for every client and "controller" (or its name).
func (m *CleanupManager) Plan(ctx context.Context, repository, requested string) domain.CleanupPlanReport {
	report := domain.CleanupPlanReport{
		SchemaVersion:   domain.SchemaVersion,
		Operation:       "cleanup-plan",
		State:           "blocked",
		Requested:       requested,
		KeepGenerations: domain.CleanupKeepGenerations,
		Targets:         []domain.CleanupTargetPlan{},
		Issues:          []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return cleanupPlanIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		return cleanupPlanIssue(report, "configuration", fmt.Sprintf("evaluate laboratory inventory: %v", err))
	}
	controller, clients, normalized, issues := selectCleanupTargets(meta, requested)
	report.Requested = normalized
	for _, issue := range issues {
		report = cleanupPlanIssue(report, "targets", issue)
	}
	if len(issues) > 0 {
		return report
	}
	active, err := m.source.ClientOperationActive()
	if message, refused := operationRefusal(active, err, "another Nixorium operation is running; wait for it to finish", "inspect active operations: "); refused {
		return cleanupPlanIssue(report, "operation", message)
	}
	if conflict := cleanupPXEConflict(m.source, ctx); conflict != "" {
		return cleanupPlanIssue(report, "installation", conflict)
	}
	if controller {
		report.Targets = append(report.Targets, cleanupTarget(meta.Controller.Name, "", true, m.source.ObserveControllerGenerations(ctx)))
	}
	observations := m.source.ObserveClientGenerations(ctx, clients, cleanupObservationTimeout)
	for _, host := range clients {
		observation, found := observations[host.Name]
		if !found {
			observation = domain.CleanupObservation{Reachability: domain.ReachabilityUnknown, SSH: domain.SSHUnknown, Detail: "no observation returned"}
		}
		report.Targets = append(report.Targets, cleanupTarget(host.Name, host.IP, false, observation))
	}
	for _, target := range report.Targets {
		if target.Eligible {
			report.Eligible++
		}
	}
	if report.Eligible == 0 {
		report.State = "unchanged"
		report.Message = "Nothing to remove on the selected computers that could be checked. Computers that are off are never queued."
		return report
	}
	report.State = "ready"
	report.ExpiresAt = m.now().UTC().Truncate(cleanupReviewWindow).Add(cleanupReviewWindow)
	report.ReviewToken = domain.CleanupReviewToken(report)
	report.Confirmation = domain.CleanupConfirmation
	report.Message = fmt.Sprintf("Old system versions can be removed on %d of %d selected computer(s). The space freed is known only afterwards.", report.Eligible, len(report.Targets))
	return report
}

func (m *CleanupManager) Apply(ctx context.Context, plan domain.CleanupPlanReport, expectedToken string) domain.CleanupApplyReport {
	report := domain.CleanupApplyReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "cleanup-apply",
		State:         "blocked",
		Repository:    plan.Repository,
		Requested:     plan.Requested,
		Targets:       []domain.CleanupTargetOutcome{},
		Issues:        []domain.ValidationIssue{},
	}
	if plan.HasErrors() || plan.State != "ready" || plan.ReviewToken == "" {
		report.Message = "Nothing was removed because the reviewed plan is not ready."
		return report
	}
	if expectedToken == "" || expectedToken != plan.ReviewToken || plan.ReviewToken != domain.CleanupReviewToken(plan) {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "review", Message: "cleanup review token does not match the frozen plan"})
		report.Message = "Nothing was removed; create and review a fresh plan."
		return report
	}
	if !m.now().UTC().Before(plan.ExpiresAt) {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "expiry", Message: "cleanup review expired"})
		report.Message = "Nothing was removed because the review is too old; create a fresh plan."
		return report
	}
	if conflict := cleanupPXEConflict(m.source, ctx); conflict != "" {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "installation", Message: conflict})
		report.Message = "Nothing was removed because network installation is active or needs recovery."
		return report
	}
	meta, err := m.source.LabMeta(ctx, plan.Repository)
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "configuration", Message: err.Error()})
		report.Message = "Nothing was removed because the laboratory inventory could not be rechecked."
		return report
	}
	if !cleanupInventoryMatches(meta, plan.Targets) {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "targets", Message: "computer identities or addresses changed after review"})
		report.Message = "Nothing was removed; create and review a fresh plan."
		return report
	}

	outcomes := map[string]domain.CleanupTargetOutcome{}
	// The controller unit takes the operation lock itself, so it runs before
	// this process holds the lock for the clients.
	for _, target := range plan.Targets {
		if target.Controller && target.Eligible {
			outcomes[target.Name] = m.cleanController(ctx, target)
		}
	}
	clients := []domain.HostMeta{}
	expect := map[string]string{}
	for _, target := range plan.Targets {
		if !target.Controller && target.Eligible {
			clients = append(clients, domain.HostMeta{Name: target.Name, IP: target.IP})
			expect[target.Name] = target.Expect
		}
	}
	if len(clients) > 0 {
		lease, err := acquireOperation(m.source, "Free disk space")
		if err != nil {
			for _, host := range clients {
				outcomes[host.Name] = domain.CleanupTargetOutcome{Name: host.Name, State: "not-sent", Detail: "another Nixorium operation is running", TechnicalDetail: err.Error()}
			}
		} else {
			results := m.source.CleanClientGenerations(ctx, expect, clients, cleanupDispatchTimeout)
			lease.Close()
			for _, host := range clients {
				outcomes[host.Name] = cleanupOutcome(host.Name, results[host.Name])
			}
		}
	}
	for _, target := range plan.Targets {
		outcome, found := outcomes[target.Name]
		if !found {
			outcome = domain.CleanupTargetOutcome{Name: target.Name, State: "not-sent", Detail: cleanupSkipReason(target)}
		}
		switch outcome.State {
		case "cleaned":
			report.Cleaned++
		case "unchanged":
			report.Unchanged++
		case "not-sent":
			report.NotSent++
		default:
			report.Unconfirmed++
		}
		report.Targets = append(report.Targets, outcome)
	}
	switch {
	case report.Cleaned == 0 && report.Unconfirmed == 0:
		report.State = "blocked"
		report.Message = "Nothing was removed. Create a fresh review."
	case report.Unconfirmed > 0 || report.NotSent > 0:
		report.State = "partial"
		report.Message = fmt.Sprintf("Old versions removed on %d computer(s); %d not sent and %d not confirmed. Check those computers before trying again.", report.Cleaned, report.NotSent, report.Unconfirmed)
	default:
		report.State = "completed"
		report.Message = fmt.Sprintf("Old versions removed on %d computer(s).", report.Cleaned)
	}
	return report
}

func (m *CleanupManager) cleanController(ctx context.Context, target domain.CleanupTargetPlan) domain.CleanupTargetOutcome {
	before := m.source.ObserveControllerGenerations(ctx)
	if !before.Valid || before.Expect != target.Expect {
		return domain.CleanupTargetOutcome{Name: target.Name, State: "not-sent", Detail: "the controller changed after review; create a fresh review"}
	}
	runErr := m.source.CleanControllerGenerations(ctx, target.Expect)
	after := m.source.ObserveControllerGenerations(ctx)
	outcome := domain.CleanupTargetOutcome{Name: target.Name, FreeBefore: before.FreeBytes, FreeAfter: after.FreeBytes}
	switch {
	case after.Valid && len(after.Remove) == 0 && runErr == nil:
		outcome.State = "cleaned"
		outcome.Removed = before.Remove
		outcome.Detail = "old versions removed"
	case after.Valid && len(after.Remove) == 0:
		outcome.State = "cleaned"
		outcome.Removed = before.Remove
		outcome.Detail = "old versions removed, but the job reported an error: the boot menu may still list them until the next application"
		outcome.TechnicalDetail = runErr.Error()
	default:
		outcome.State = "unconfirmed"
		outcome.Detail = "the controller job did not confirm the removal; inspect nixorium-clean-generations@*.service before trying again"
		if runErr != nil {
			outcome.TechnicalDetail = runErr.Error()
		}
	}
	return outcome
}

func cleanupOutcome(name string, result domain.CleanupDispatchResult) domain.CleanupTargetOutcome {
	outcome := domain.CleanupTargetOutcome{Name: name, Removed: result.Removed, FreeBefore: result.FreeBefore, FreeAfter: result.FreeAfter}
	switch result.State {
	case "cleaned":
		outcome.State = "cleaned"
		outcome.Detail = "old versions removed"
		if result.BootMenuFailed {
			outcome.Detail = "old versions removed; the boot menu could not be rewritten and may list them until the next update"
		}
	case "unchanged":
		outcome.State = "unchanged"
		outcome.Detail = "nothing to remove"
	case "changed":
		outcome.State = "not-sent"
		outcome.Detail = "the computer changed after review; nothing was removed"
	default:
		outcome.State = "unconfirmed"
		outcome.Detail = "the result could not be confirmed; check the computer before trying again"
		outcome.TechnicalDetail = result.Detail
	}
	return outcome
}

func cleanupTarget(name, ip string, controller bool, observation domain.CleanupObservation) domain.CleanupTargetPlan {
	target := domain.CleanupTargetPlan{
		Name: name, IP: ip, Controller: controller,
		Reachability: observation.Reachability, SSH: observation.SSH,
		Keep: observation.Keep, Remove: observation.Remove,
		FreeBytes: observation.FreeBytes, Expect: observation.Expect, Detail: observation.Detail,
	}
	if target.Keep == nil {
		target.Keep = []domain.CleanupGeneration{}
	}
	if target.Remove == nil {
		target.Remove = []int{}
	}
	switch {
	case !observation.Valid:
		if target.Detail == "" {
			target.Detail = "could not check this computer"
		}
	case len(observation.Remove) == 0:
		target.Detail = "nothing to remove"
	default:
		target.Eligible = true
		target.Detail = fmt.Sprintf("%d old version(s) to remove", len(observation.Remove))
	}
	return target
}

func cleanupSkipReason(target domain.CleanupTargetPlan) string {
	if target.Detail != "" {
		return target.Detail
	}
	return "not eligible in the reviewed plan"
}

func selectCleanupTargets(meta domain.LabMeta, requested string) (bool, []domain.HostMeta, string, []string) {
	controller := false
	clientNames := []string{}
	issues := []string{}
	for _, raw := range strings.Split(requested, ",") {
		name := strings.TrimSpace(raw)
		switch {
		case name == "":
			issues = append(issues, "target list contains an empty name")
		case name == domain.CleanupControllerTarget || (name == meta.Controller.Name && name != ""):
			if controller {
				issues = append(issues, "the controller is selected more than once")
			}
			controller = true
		default:
			clientNames = append(clientNames, name)
		}
	}
	clients := []domain.HostMeta{}
	normalized := []string{}
	if controller {
		normalized = append(normalized, domain.CleanupControllerTarget)
	}
	if len(clientNames) > 0 {
		targets, selector, selectionIssues := selectDeploymentTargets(meta.Clients.Hosts, strings.Join(clientNames, ","))
		issues = append(issues, selectionIssues...)
		for _, target := range targets {
			clients = append(clients, domain.HostMeta{Name: target.Name, IP: target.IP})
		}
		normalized = append(normalized, selector)
	}
	if !controller && len(clientNames) == 0 && len(issues) == 0 {
		issues = append(issues, "no computers were selected")
	}
	return controller, clients, strings.Join(normalized, ","), issues
}

func cleanupInventoryMatches(meta domain.LabMeta, targets []domain.CleanupTargetPlan) bool {
	clients := map[string]string{}
	for _, host := range meta.Clients.Hosts {
		clients[host.Name] = host.IP
	}
	for _, target := range targets {
		if target.Controller {
			if target.Name != meta.Controller.Name {
				return false
			}
		} else if target.Name == meta.Controller.Name || clients[target.Name] != target.IP {
			return false
		}
	}
	return true
}

func cleanupPXEConflict(source CleanupSource, ctx context.Context) string {
	listener := source.ServiceState(ctx, PXEListenerUnit)
	network := source.ServiceState(ctx, PXENetworkUnit)
	switch ObservePXELifecycle(listener, network, domain.PXEPreparationState{}).Mode {
	case "active":
		return "network installation is active; stop it before freeing disk space"
	case "degraded", "recovery-required":
		return "controller network or installation state requires recovery first"
	}
	return ""
}

func cleanupPlanIssue(report domain.CleanupPlanReport, field, message string) domain.CleanupPlanReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	if report.Message == "" {
		report.Message = message
	}
	return report
}
