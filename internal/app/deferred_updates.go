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

// DeferredDeploy applies a reviewed revision to one computer exactly like
// Update computers. busy reports that another operation held the gate, so
// nothing was attempted.
type DeferredDeploy func(ctx context.Context, repository, host, revision string) (report domain.DeploymentExecutionReport, busy bool)

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
		queued = append(queued, domain.DeferredUpdate{Host: host, IP: target.IP, Revision: plan.Revision, QueuedAt: m.now().UTC()})
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

// Run makes one pass: every current, due entry whose computer answers is
// updated in turn; stale entries are never applied.
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
			candidates = append(candidates, domain.HostMeta{Name: update.Host, IP: update.IP})
		}
	}
	probes := m.source.SSHStatus(ctx, candidates, m.timeout)
	for _, host := range candidates {
		if probes[host.Name].SSH != domain.SSHAvailable {
			report.Results = append(report.Results, domain.DeferredUpdateOutcome{Host: host.Name, State: "waiting", Detail: "not reachable yet"})
			continue
		}
		update, _ := queue.Find(host.Name)
		result, busy := m.deploy(ctx, repository, host.Name, update.Revision)
		outcome := domain.DeferredUpdateOutcome{Host: host.Name}
		switch {
		case busy:
			outcome.State, outcome.Detail = "busy", "another operation is running; trying again on the next pass"
		case result.State == "completed":
			outcome.State = "updated"
			if err := m.store.Mutate(func(queue *domain.DeferredUpdateQueue) error {
				if current, found := queue.Find(host.Name); found && current.Revision == update.Revision {
					queue.Remove(host.Name)
				}
				return nil
			}); err != nil {
				outcome.Detail = "updated; the queue entry is removed on the next pass: " + err.Error()
			}
		default:
			outcome.State, outcome.Detail = "failed", deferredFailure(result)
			_ = m.store.Mutate(func(queue *domain.DeferredUpdateQueue) error {
				if current, found := queue.Find(host.Name); found && current.Revision == update.Revision {
					current.RecordFailure(m.now().UTC(), outcome.Detail)
					queue.Put(current)
				}
				return nil
			})
		}
		report.Results = append(report.Results, outcome)
		if result.RecoveryRequired {
			// An unconfirmed activation blocks every operation until reviewed.
			report.State = "failed"
			report.Message = "an update could not be confirmed; open Nixorium to recover before queued updates continue"
			return report
		}
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
