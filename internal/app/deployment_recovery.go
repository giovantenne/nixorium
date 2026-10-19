package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

const deploymentRecoveryWindow = 10 * time.Minute
const deploymentActivityTimeout = 8 * time.Second

// DeploymentRecoverySource observes an interrupted client update and archives
// its record once the operator has reviewed every computer.
type DeploymentRecoverySource interface {
	PendingDeployment() (domain.PendingDeployment, bool, error)
	ProcessRunning(pid int, start string) bool
	ObserveDeploymentActivity(context.Context, []domain.HostMeta, time.Duration) map[string]domain.DeploymentActivity
	ArchivePendingDeployment(domain.PendingDeployment) (string, error)
}

// ownApplySource reports whether this process is still applying a client
// update itself (the record it wrote is then not yet abandoned).
type ownApplySource interface {
	OwnDeploymentApplyRunning() bool
}

// pxeStateSource observes the managed network installation units.
type pxeStateSource interface {
	ServiceState(context.Context, string) domain.ServiceState
}

type DeploymentRecoveryManager struct {
	source DeploymentRecoverySource
	now    func() time.Time
	ownPID int
}

func NewDeploymentRecoveryManager(source DeploymentRecoverySource) *DeploymentRecoveryManager {
	return &DeploymentRecoveryManager{source: source, now: time.Now}
}

// ForProcess names the caller's own PID. A record written by this process is
// not refused as "still running" once its apply has returned: the caller is
// the process that would otherwise be waited for. Any other live PID is
// still refused, and so is this one while it is applying.
func (m *DeploymentRecoveryManager) ForProcess(pid int) *DeploymentRecoveryManager {
	m.ownPID = pid
	return m
}

func (m *DeploymentRecoveryManager) recorderRunning(pid int, start string) bool {
	if pid <= 0 || !m.source.ProcessRunning(pid, start) {
		return false
	}
	if pid != m.ownPID {
		return true
	}
	// Fail closed when the source cannot tell whether this process is
	// still applying.
	own, ok := m.source.(ownApplySource)
	return !ok || own.OwnDeploymentApplyRunning()
}

func (m *DeploymentRecoveryManager) Plan(ctx context.Context, repository string, acknowledgeUnreachable bool) domain.DeploymentRecoveryPlan {
	plan := domain.DeploymentRecoveryPlan{
		SchemaVersion: domain.SchemaVersion, Operation: "deploy-recover-plan", State: "blocked",
		AcknowledgeUnreachable: acknowledgeUnreachable, Targets: []domain.DeploymentRecoveryTarget{}, Issues: []domain.ValidationIssue{},
	}
	if root, err := filepath.Abs(repository); err == nil {
		plan.Repository = root
	}
	issue := func(field, message string) domain.DeploymentRecoveryPlan {
		plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: field, Message: message})
		if plan.Message == "" {
			plan.Message = message
		}
		return plan
	}
	pending, present, err := m.source.PendingDeployment()
	if !present {
		plan.State = "unchanged"
		plan.Message = "No interrupted client update is recorded."
		return plan
	}
	if err != nil {
		plan.Issues = append(plan.Issues, domain.BlockedIssue("record", "DEPLOY-PENDING", err.Error()+". Keep the record and collect a support report; do not delete it."))
		plan.Message = plan.Issues[0].Message
		return plan
	}
	plan.Pending = pending
	if m.recorderRunning(pending.ControllerPID, pending.ControllerPIDStart) {
		return issue("process", fmt.Sprintf("the Nixorium process that started the update (PID %d) is still running; wait for it to finish or close it first", pending.ControllerPID))
	}
	hosts := make([]domain.HostMeta, 0, len(pending.Targets))
	for _, target := range pending.Targets {
		hosts = append(hosts, domain.HostMeta{Name: target.Name, IP: target.IP, PreviousIP: target.PreviousIP})
	}
	activity := m.source.ObserveDeploymentActivity(ctx, hosts, deploymentActivityTimeout)
	activating := 0
	for _, host := range hosts {
		plan.Targets = append(plan.Targets, recoveryTarget(host, activity[host.Name], pending.Revision))
		switch plan.Targets[len(plan.Targets)-1].State {
		case domain.RecoveryTargetActivating:
			activating++
		case domain.RecoveryTargetUnreachable:
			plan.Unreachable++
		}
	}
	if pxe, ok := m.source.(pxeStateSource); ok {
		switch mode := ObservePXELifecycle(pxe.ServiceState(ctx, PXEListenerUnit), pxe.ServiceState(ctx, PXENetworkUnit), domain.PXEPreparationState{}); mode.Mode {
		case "active", "degraded", "recovery-required":
			plan.PXEMode = mode.Mode
		}
	}
	if activating > 0 {
		return issue("targets", fmt.Sprintf("%d computer(s) are still applying the update; wait until they finish, then check again", activating))
	}
	if plan.Unreachable > 0 && !acknowledgeUnreachable {
		plan.Message = fmt.Sprintf("%d computer(s) could not be checked. Check them at the console; then acknowledge them explicitly to continue.", plan.Unreachable)
		return issue("unreachable", plan.Message)
	}
	plan.State = "ready"
	plan.ExpiresAt = m.now().UTC().Truncate(deploymentRecoveryWindow).Add(deploymentRecoveryWindow)
	plan.ReviewToken = domain.DeploymentRecoveryToken(plan)
	plan.Message = "Every reachable computer has finished. Confirming archives the record and unblocks operations; the interrupted update is not declared successful."
	return plan
}

func (m *DeploymentRecoveryManager) Apply(ctx context.Context, plan domain.DeploymentRecoveryPlan, expectedToken string) domain.DeploymentRecoveryResult {
	result := domain.DeploymentRecoveryResult{SchemaVersion: domain.SchemaVersion, Operation: "deploy-recover", State: "blocked", Issues: []domain.ValidationIssue{}}
	refuse := func(field, message string) domain.DeploymentRecoveryResult {
		code := "REVIEW-CHANGED"
		if field == "expiry" {
			code = "REVIEW-EXPIRED"
		}
		result.Issues = append(result.Issues, domain.BlockedIssue(field, code, message))
		result.Message = message
		return result
	}
	if plan.State != "ready" || plan.ReviewToken == "" || expectedToken != plan.ReviewToken || plan.ReviewToken != domain.DeploymentRecoveryToken(plan) {
		return refuse("review", "the recovery review token does not match the frozen plan; create a fresh review")
	}
	if !m.now().UTC().Before(plan.ExpiresAt) {
		return refuse("expiry", "the recovery review expired; create a fresh review")
	}
	pending, present, err := m.source.PendingDeployment()
	if !present || err != nil || !samePendingDeployment(pending, plan.Pending) {
		return refuse("record", "the interrupted update record changed after review; create a fresh review")
	}
	if m.recorderRunning(pending.ControllerPID, pending.ControllerPIDStart) {
		return refuse("process", "the Nixorium process that started the update is running again; create a fresh review")
	}
	previous := map[string]string{}
	for _, target := range pending.Targets {
		previous[target.Name] = target.PreviousIP
	}
	recheck := []domain.HostMeta{}
	for _, target := range plan.Targets {
		if target.State != domain.RecoveryTargetUnreachable {
			recheck = append(recheck, domain.HostMeta{Name: target.Name, IP: target.IP, PreviousIP: previous[target.Name]})
		}
	}
	activity := m.source.ObserveDeploymentActivity(ctx, recheck, deploymentActivityTimeout)
	for _, host := range recheck {
		if activity[host.Name].Activating {
			return refuse("targets", host.Name+" started applying again after review; create a fresh review")
		}
	}
	archive, err := m.source.ArchivePendingDeployment(plan.Pending)
	if err != nil {
		return refuse("archive", "the record could not be archived: "+err.Error())
	}
	result.State, result.Archive = "completed", archive
	for _, target := range plan.Targets {
		result.Targets = append(result.Targets, target.Name)
	}
	result.Message = "The interrupted update is recorded as reviewed and operations are unblocked. Create a fresh review to update these computers."
	return result
}

func recoveryTarget(host domain.HostMeta, activity domain.DeploymentActivity, reviewed string) domain.DeploymentRecoveryTarget {
	target := domain.DeploymentRecoveryTarget{Name: host.Name, IP: host.IP, Revision: activity.Revision, Detail: activity.Detail}
	switch {
	case !activity.Reachable:
		target.State = domain.RecoveryTargetUnreachable
		if target.Detail == "" {
			target.Detail = "off or not reachable; check it at the console"
		}
	case activity.Activating:
		target.State = domain.RecoveryTargetActivating
		target.Detail = "still applying the update"
	case activity.Revision != "" && activity.Revision == reviewed:
		target.State = domain.RecoveryTargetReviewed
		target.Detail = "finished on the reviewed revision"
	default:
		target.State = domain.RecoveryTargetOther
		target.Detail = "finished on another revision; it will be updated by a new review"
	}
	return target
}

func samePendingDeployment(left, right domain.PendingDeployment) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && string(a) == string(b)
}
