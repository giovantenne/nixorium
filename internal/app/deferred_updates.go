package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// DeferredUpdateStore holds the queue of client updates that wait for their
// computers; Mutate runs under the managed operation gate.
type DeferredUpdateStore interface {
	Read() (domain.DeferredUpdateQueue, error)
	Mutate(func(*domain.DeferredUpdateQueue) error) error
}

// DeferredDeploy applies a reviewed revision to the named computers exactly
// like Update computers. busy reports that another operation held the gate,
// so nothing was attempted.
type DeferredDeploy func(ctx context.Context, repository string, hosts []string, revision string) (report domain.DeploymentExecutionReport, busy bool)

// operationObserver tells whether another operation holds the gate or an
// interrupted update blocks new ones, without taking the gate.
type operationObserver interface {
	ClientOperationActive() (bool, error)
}

type DeferredUpdateManager struct {
	store   DeferredUpdateStore
	source  DeploymentSource
	deploy  DeferredDeploy
	now     func() time.Time
	timeout time.Duration
}

func NewDeferredUpdateManager(store DeferredUpdateStore, source DeploymentSource, deploy DeferredDeploy) *DeferredUpdateManager {
	return &DeferredUpdateManager{store: store, source: source, deploy: deploy, now: time.Now, timeout: sshProbeTimeout}
}

// Queue records the reviewed revision for the named computers of a
// reviewed plan. Only computers in that plan can be queued.
func (m *DeferredUpdateManager) Queue(plan domain.DeploymentPlanReport, hosts []string) error {
	if plan.HasErrors() || plan.Revision == "" {
		return fmt.Errorf("only a ready update review can queue computers")
	}
	targets := map[string]domain.DeploymentTarget{}
	for _, target := range plan.Targets {
		targets[target.Name] = target
	}
	queued := make([]domain.DeferredUpdate, 0, len(hosts))
	for _, host := range hosts {
		target, found := targets[host]
		if !found {
			return fmt.Errorf("%s is not part of the reviewed update", host)
		}
		queued = append(queued, domain.DeferredUpdate{Host: host, IP: target.IP, PreviousIP: target.PreviousIP, Revision: plan.Revision, QueuedAt: m.now().UTC()})
	}
	return m.store.Mutate(func(queue *domain.DeferredUpdateQueue) error {
		for _, update := range queued {
			queue.Put(update)
		}
		return nil
	})
}

// Status reads the queue and marks entries whose configuration changed.
func (m *DeferredUpdateManager) Status(ctx context.Context, repository string) domain.DeferredUpdateStatus {
	status := domain.DeferredUpdateStatus{SchemaVersion: domain.SchemaVersion, Operation: "deploy-queue-status", State: "ready", Updates: []domain.DeferredUpdateEntry{}, Issues: []domain.ValidationIssue{}}
	queue, err := m.store.Read()
	if err != nil {
		status.State = "failed"
		status.Issues = append(status.Issues, domain.ValidationIssue{Field: "queue", Message: err.Error()})
		return status
	}
	revision, err := m.source.GitRevision(ctx, repository)
	if err != nil {
		status.Issues = append(status.Issues, domain.ValidationIssue{Field: "git", Message: "resolve revision: " + err.Error()})
	}
	status.Revision = revision
	for _, update := range queue.Updates {
		status.Updates = append(status.Updates, domain.DeferredUpdateEntry{DeferredUpdate: update, Stale: revision != "" && update.Stale(revision)})
	}
	switch {
	case len(status.Updates) == 0:
		status.Message = "No computer is waiting for an update."
	case status.Waiting() == len(status.Updates):
		status.Message = fmt.Sprintf("%s will update when switched on.", countLabel(len(status.Updates), "computer"))
	default:
		status.Message = fmt.Sprintf("%s will update when switched on; %d changed configuration since and need a new review.", countLabel(status.Waiting(), "computer"), len(status.Updates)-status.Waiting())
	}
	return status
}

// Cancel removes the named computers, or every computer for "@all".
func (m *DeferredUpdateManager) Cancel(hosts []string) (int, error) {
	removed := 0
	err := m.store.Mutate(func(queue *domain.DeferredUpdateQueue) error {
		if slices.Contains(hosts, "@all") {
			removed = len(queue.Updates)
			queue.Updates = []domain.DeferredUpdate{}
			return nil
		}
		for _, host := range hosts {
			if queue.Remove(host) {
				removed++
			}
		}
		return nil
	})
	return removed, err
}

// Run makes one pass: the current, due entries whose computers answer are
// updated together with one ordinary deployment; stale entries are never
// applied. A busy or blocked controller is reported without trying anything
// and without counting it as a failure of the waiting computers.
func (m *DeferredUpdateManager) Run(ctx context.Context, repository string) domain.DeferredUpdateRunReport {
	report := domain.DeferredUpdateRunReport{SchemaVersion: domain.SchemaVersion, Operation: "deploy-queue-run", State: "completed", Results: []domain.DeferredUpdateOutcome{}, Issues: []domain.ValidationIssue{}}
	queue, err := m.store.Read()
	if err != nil {
		report.State = "failed"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "queue", Message: err.Error()})
		return report
	}
	if len(queue.Updates) == 0 {
		report.Message = "No computer is waiting for an update."
		return report
	}
	if observer, ok := m.source.(operationObserver); ok {
		active, activeErr := observer.ClientOperationActive()
		if message, refused := operationRefusal(active, activeErr, "another operation is running; trying again on the next pass", ""); refused {
			for _, update := range queue.Updates {
				report.Results = append(report.Results, domain.DeferredUpdateOutcome{Host: update.Host, State: "busy", Detail: message})
			}
			report.Message = deferredRunSummary(report.Results)
			return report
		}
	}
	revision, err := m.source.GitRevision(ctx, repository)
	if err != nil {
		report.State = "failed"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "git", Message: "resolve revision: " + err.Error()})
		return report
	}
	candidates := []domain.HostMeta{}
	for _, update := range queue.Updates {
		switch {
		case update.Stale(revision):
			report.Results = append(report.Results, domain.DeferredUpdateOutcome{Host: update.Host, State: "stale", Detail: "the configuration changed after queueing; review the update again"})
		case !update.Due(m.now()):
			report.Results = append(report.Results, domain.DeferredUpdateOutcome{Host: update.Host, State: "waiting", Detail: "retrying later after: " + update.LastError})
		default:
			candidates = append(candidates, domain.HostMeta{Name: update.Host, IP: update.IP, PreviousIP: update.PreviousIP})
		}
	}
	probes := m.source.SSHStatus(ctx, candidates, m.timeout)
	answering := []domain.HostMeta{}
	for _, host := range candidates {
		if probes[host.Name].SSH != domain.SSHAvailable {
			report.Results = append(report.Results, domain.DeferredUpdateOutcome{Host: host.Name, State: "waiting", Detail: "not reachable yet"})
			continue
		}
		answering = append(answering, host)
	}
	if len(answering) == 0 {
		report.Message = deferredRunSummary(report.Results)
		return report
	}

	// A computer already at the queued revision (updated by another review,
	// or by a pass whose queue change was refused) needs no second update.
	current := m.source.CurrentSystems(ctx, answering, deploymentVerificationTimeout)
	removals := map[string]bool{}
	failures := map[string]string{}
	pending := []string{}
	for _, host := range answering {
		if probe := current[host.Name]; probe.Revision == revision && probe.SystemPath != "" && probe.HostKeyCondition != domain.HostKeyChanged {
			removals[host.Name] = true
			report.Results = append(report.Results, domain.DeferredUpdateOutcome{Host: host.Name, State: "updated", Detail: "already up to date"})
			continue
		}
		pending = append(pending, host.Name)
	}

	recovery := false
	if len(pending) > 0 {
		result, busy := m.deploy(ctx, repository, pending, revision)
		recovery = result.RecoveryRequired
		verified := map[string]domain.DeploymentTargetVerification{}
		for _, target := range result.Verification.Targets {
			verified[target.Name] = target
		}
		skipped := map[string]bool{}
		for _, name := range result.Skipped {
			skipped[name] = true
		}
		for _, host := range pending {
			outcome := domain.DeferredUpdateOutcome{Host: host}
			observation := verified[host]
			switch {
			case busy:
				outcome.State, outcome.Detail = "busy", "another operation is running; trying again on the next pass"
			case skipped[host]:
				outcome.State, outcome.Detail = "waiting", "stopped answering during the update; trying again when it answers"
			case observation.State == "verified" && result.ApplyCompleted && !result.RecoveryRequired:
				outcome.State = "updated"
				removals[host] = true
			case result.RecoveryRequired:
				// Not a failure: the result is unknown until the interrupted
				// update closes itself, and the next pass then finds the
				// computer up to date or updates it.
				outcome.State, outcome.Detail = "waiting", "its result is not known yet; trying again once the interrupted update closes"
			default:
				outcome.State = "failed"
				outcome.Detail = deferredFailure(result)
				if observation.Detail != "" {
					outcome.Detail = observation.Detail + "; " + outcome.Detail
				}
				failures[host] = outcome.Detail
			}
			report.Results = append(report.Results, outcome)
		}
	}

	if len(removals) > 0 || len(failures) > 0 {
		at := m.now().UTC()
		if err := m.store.Mutate(func(queue *domain.DeferredUpdateQueue) error {
			for host := range removals {
				if entry, found := queue.Find(host); found && entry.Revision == revision {
					queue.Remove(host)
				}
			}
			for host, reason := range failures {
				if entry, found := queue.Find(host); found && entry.Revision == revision {
					entry.RecordFailure(at, reason)
					queue.Put(entry)
				}
			}
			return nil
		}); err != nil {
			// The next pass sees the updated computers at their revision and
			// removes them without updating them again.
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "queue", Message: "the queue could not be changed after this pass: " + err.Error()})
		}
	}
	if recovery {
		// An unconfirmed activation blocks every operation until reviewed.
		report.State = "failed"
		report.Message = "an update is not confirmed yet; queued updates continue once it closes itself or is reviewed in Nixorium"
		return report
	}
	report.Message = deferredRunSummary(report.Results)
	return report
}

func deferredFailure(result domain.DeploymentExecutionReport) string {
	reason := result.Message
	for _, issue := range result.Issues {
		reason += "; " + issue.Message
	}
	return strings.TrimPrefix(reason, "; ")
}

func deferredRunSummary(results []domain.DeferredUpdateOutcome) string {
	counts := map[string]int{}
	for _, result := range results {
		counts[result.State]++
	}
	parts := []string{}
	for _, state := range []string{"updated", "waiting", "busy", "failed", "stale"} {
		if counts[state] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[state], state))
		}
	}
	return "Queued updates: " + strings.Join(parts, ", ") + "."
}

func countLabel(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

func attentionLabel(count int) string {
	if count == 1 {
		return "1 computer needs attention"
	}
	return fmt.Sprintf("%d computers need attention", count)
}
