package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// LockSource reads the laboratory identities and reaches each computer's
// classroom agent over the controller's SSH access.
type LockSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	Connect(context.Context, domain.HostMeta) (classroomview.Session, error)
}

// LockManager locks and unlocks students' computers. A lock is a classroom
// aid in the user's session, not a system change, so it does not take the
// administrative operation lock and works while the administrator deploys.
type LockManager struct {
	source LockSource
	now    func() time.Time
}

func NewLockManager(source LockSource) *LockManager {
	return &LockManager{source, time.Now}
}

// lockParallel bounds simultaneous SSH channels.
const lockParallel = 8

func (m *LockManager) Plan(ctx context.Context, repository, requested string, action domain.LockAction) domain.LockPlan {
	p := domain.LockPlan{SchemaVersion: domain.SchemaVersion, Operation: "lock-plan", State: "blocked", Action: action, Requested: requested, Targets: []domain.LockTarget{}, Issues: []domain.ValidationIssue{}}
	fail := func(message string) domain.LockPlan {
		p.Message = message
		p.Issues = append(p.Issues, domain.ValidationIssue{Field: "lock", Message: message})
		return p
	}
	var err error
	p.Repository, err = filepath.Abs(repository)
	if err != nil {
		return fail(err.Error())
	}
	if !action.Valid() {
		return fail("Choose lock or unlock.")
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		return fail(err.Error())
	}
	selected, normalized, issues := selectDeploymentTargets(meta.Clients.Hosts, requested)
	if len(issues) > 0 {
		return fail(issues[0])
	}
	p.Requested = normalized
	hosts := shutdownHostMeta(selected)
	for _, host := range hosts {
		if host.Name == meta.Controller.Name {
			return fail("The controller cannot be selected.")
		}
	}
	p.Targets = make([]domain.LockTarget, len(hosts))
	eachHost(hosts, lockParallel, func(index int, host domain.HostMeta) {
		target := domain.LockTarget{HostMeta: host}
		session, err := m.source.Connect(ctx, host)
		if err != nil {
			target.Detail = lockUnavailable(err)
		} else {
			target.Reachable, target.Eligible, target.Locked = true, true, session.Locked()
			_ = session.Close()
		}
		p.Targets[index] = target
	})
	eligible := 0
	for _, target := range p.Targets {
		if target.Eligible {
			eligible++
		}
	}
	if eligible == 0 {
		return fail("No selected computer has someone signed in with the classroom view. Check that the computers are on and updated.")
	}
	p.State = "ready"
	// A fixed five-minute window, as for Internet control, so that a fresh
	// plan of the same computers keeps the reviewed token.
	p.ExpiresAt = m.now().UTC().Truncate(5 * time.Minute).Add(5 * time.Minute)
	p.Message = fmt.Sprintf("%d of %d computers available. Logging out or restarting always unlocks a computer.", eligible, len(p.Targets))
	p.ReviewToken = domain.LockReviewToken(p)
	return p
}

func (m *LockManager) Apply(ctx context.Context, p domain.LockPlan, token string) domain.LockReport {
	r := domain.LockReport{SchemaVersion: domain.SchemaVersion, Operation: "lock-apply", State: "blocked", Action: p.Action, Targets: []domain.LockOutcome{}}
	if p.HasErrors() || !p.Action.Valid() || token == "" || token != p.ReviewToken || token != domain.LockReviewToken(p) || !m.now().Before(p.ExpiresAt) {
		r.Message = "Review expired or changed; create a fresh plan."
		return r
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		r.Message = err.Error()
		return r
	}
	identities := map[string]string{}
	for _, host := range meta.Clients.Hosts {
		identities[host.Name] = host.IP
	}
	hosts := make([]domain.HostMeta, len(p.Targets))
	for index, target := range p.Targets {
		if target.Name == meta.Controller.Name || identities[target.Name] != target.IP {
			r.Message = "Client inventory changed; review again."
			return r
		}
		hosts[index] = target.HostMeta
	}
	outcomes := make([]domain.LockOutcome, len(hosts))
	eachHost(hosts, lockParallel, func(index int, host domain.HostMeta) {
		outcome := domain.LockOutcome{Name: host.Name, State: "not-sent", Detail: "Unavailable in the reviewed plan; nothing was sent."}
		if p.Targets[index].Eligible {
			outcome = m.change(ctx, host, p.Action.Locked())
		}
		outcomes[index] = outcome
	})
	confirmed := 0
	for _, outcome := range outcomes {
		if outcome.State == "verified" {
			confirmed++
		}
	}
	r.Targets = outcomes
	r.State = "partial"
	if confirmed == len(outcomes) {
		r.State = "completed"
	}
	verb := "Locked"
	if !p.Action.Locked() {
		verb = "Unlocked"
	}
	r.Message = fmt.Sprintf("%s %d of %d selected computers.", verb, confirmed, len(outcomes))
	return r
}

func (m *LockManager) change(ctx context.Context, host domain.HostMeta, locked bool) domain.LockOutcome {
	outcome := domain.LockOutcome{Name: host.Name, State: "not-sent"}
	session, err := m.source.Connect(ctx, host)
	if err != nil {
		outcome.Detail = lockUnavailable(err)
		return outcome
	}
	defer session.Close()
	state, err := session.SetLocked(locked)
	switch {
	case err == nil && state == locked:
		outcome.State = "verified"
		outcome.Detail = "Unlocked."
		if locked {
			outcome.Detail = "Locked."
		}
	case err != nil && errors.As(err, new(classroomview.AgentError)):
		outcome.Detail = lockUnavailable(err)
	default:
		outcome.State = "unconfirmed"
		outcome.Detail = "The computer did not confirm; check it before trying again."
	}
	return outcome
}

func lockUnavailable(err error) string {
	var agentError classroomview.AgentError
	if errors.As(err, &agentError) {
		switch agentError.Code {
		case classroomview.CodeNoSession:
			return "Nobody is signed in."
		case classroomview.CodeNoAgent, classroomview.CodeLockUnavailable:
			return "The classroom view is not running on this computer."
		}
	}
	return "Switched off or not reachable."
}
