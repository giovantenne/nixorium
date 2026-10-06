package domain

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Deferred updates: client computers that were off or unreachable when an
// update was reviewed. The controller applies the reviewed revision to each of
// them when it can reach them again (nixorium deploy queue run). An entry
// never authorizes another revision: once the deployment repository moves on,
// the entry is stale and waits for a fresh review.

const (
	DeferredUpdateSchemaVersion = 1
	// MaxDeferredUpdates bounds the queue file; it is one entry per client.
	MaxDeferredUpdates       = 512
	deferredUpdateErrorBytes = 512
)

var (
	deferredHostPattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	deferredRevisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

type DeferredUpdate struct {
	Host        string    `json:"host"`
	IP          string    `json:"ip"`
	Revision    string    `json:"revision"`
	QueuedAt    time.Time `json:"queuedAt"`
	Attempts    int       `json:"attempts,omitempty"`
	LastAttempt time.Time `json:"lastAttempt,omitzero"`
	LastError   string    `json:"lastError,omitempty"`
}

type DeferredUpdateQueue struct {
	SchemaVersion int              `json:"schemaVersion"`
	Updates       []DeferredUpdate `json:"updates"`
}

func (queue DeferredUpdateQueue) Validate() error {
	if queue.SchemaVersion != DeferredUpdateSchemaVersion {
		return fmt.Errorf("unsupported deferred update schema %d", queue.SchemaVersion)
	}
	if len(queue.Updates) > MaxDeferredUpdates {
		return errors.New("too many deferred updates")
	}
	seen := map[string]bool{}
	for _, update := range queue.Updates {
		if !deferredHostPattern.MatchString(update.Host) || seen[update.Host] {
			return fmt.Errorf("invalid or duplicate deferred update host %q", update.Host)
		}
		seen[update.Host] = true
		if address, err := netip.ParseAddr(update.IP); err != nil || !address.Is4() {
			return fmt.Errorf("invalid address for deferred update of %s", update.Host)
		}
		if !deferredRevisionPattern.MatchString(update.Revision) {
			return fmt.Errorf("invalid revision for deferred update of %s", update.Host)
		}
		if update.QueuedAt.IsZero() || update.Attempts < 0 || len(update.LastError) > deferredUpdateErrorBytes {
			return fmt.Errorf("invalid deferred update record for %s", update.Host)
		}
	}
	return nil
}

// Put queues an update, replacing any earlier entry for the same computer:
// the newest review is the only intent for it.
func (queue *DeferredUpdateQueue) Put(update DeferredUpdate) {
	queue.Remove(update.Host)
	queue.Updates = append(queue.Updates, update)
	slices.SortFunc(queue.Updates, func(a, b DeferredUpdate) int { return strings.Compare(a.Host, b.Host) })
}

// Remove drops the entry of a computer, reporting whether one existed.
func (queue *DeferredUpdateQueue) Remove(host string) bool {
	before := len(queue.Updates)
	queue.Updates = slices.DeleteFunc(queue.Updates, func(update DeferredUpdate) bool { return update.Host == host })
	return len(queue.Updates) != before
}

func (queue DeferredUpdateQueue) Find(host string) (DeferredUpdate, bool) {
	for _, update := range queue.Updates {
		if update.Host == host {
			return update, true
		}
	}
	return DeferredUpdate{}, false
}

// Stale reports an entry whose revision is no longer the deployment's
// current revision; it is never applied.
func (update DeferredUpdate) Stale(currentRevision string) bool {
	return update.Revision != currentRevision
}

// RecordFailure keeps a bounded, single-line reason for the last attempt.
func (update *DeferredUpdate) RecordFailure(at time.Time, reason string) {
	update.Attempts++
	update.LastAttempt = at
	reason = strings.Join(strings.Fields(reason), " ")
	if len(reason) > deferredUpdateErrorBytes {
		reason = reason[:deferredUpdateErrorBytes-3] + "..."
	}
	update.LastError = reason
}

// Due tells whether a failed entry may be tried again: 1, 2, 4… minutes
// after the last attempt, at most an hour.
func (update DeferredUpdate) Due(now time.Time) bool {
	if update.Attempts == 0 || update.LastAttempt.IsZero() {
		return true
	}
	wait := time.Minute << min(update.Attempts-1, 6)
	return !now.Before(update.LastAttempt.Add(min(wait, time.Hour)))
}

// DeferredUpdateStatus is the queue as shown to the administrator.
type DeferredUpdateStatus struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Operation     string                `json:"operation"`
	State         string                `json:"state"`
	Revision      string                `json:"revision,omitempty"`
	Updates       []DeferredUpdateEntry `json:"updates"`
	Message       string                `json:"message,omitempty"`
	Issues        []ValidationIssue     `json:"issues"`
}

type DeferredUpdateEntry struct {
	DeferredUpdate
	// Stale: the configuration changed after queueing; review again.
	Stale bool `json:"stale"`
}

func (s DeferredUpdateStatus) HasErrors() bool { return len(s.Issues) > 0 }

// Waiting counts entries that will still be applied.
func (s DeferredUpdateStatus) Waiting() int {
	count := 0
	for _, entry := range s.Updates {
		if !entry.Stale {
			count++
		}
	}
	return count
}

// DeferredUpdateRunReport describes one pass of the controller's runner.
type DeferredUpdateRunReport struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Operation     string                  `json:"operation"`
	State         string                  `json:"state"`
	Results       []DeferredUpdateOutcome `json:"results"`
	Message       string                  `json:"message,omitempty"`
	Issues        []ValidationIssue       `json:"issues"`
}

type DeferredUpdateOutcome struct {
	Host   string `json:"host"`
	State  string `json:"state"` // updated, waiting, stale, busy, failed
	Detail string `json:"detail,omitempty"`
}

func (r DeferredUpdateRunReport) HasErrors() bool { return len(r.Issues) > 0 }
