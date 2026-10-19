package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type DeploymentSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	DeploymentStatus(context.Context, string) (domain.DeploymentStatus, error)
	GitState(context.Context, string) (domain.GitState, error)
	GitRevision(context.Context, string) (string, error)
	InterfaceAddresses(string) ([]string, error)
	RunDeploymentPhase(context.Context, domain.DeploymentPlanReport, domain.DeploymentPhase, io.Writer) error
	CurrentSystems(context.Context, []domain.HostMeta, time.Duration) map[string]domain.HostSystemProbe
	SSHStatus(context.Context, []domain.HostMeta, time.Duration) map[string]domain.SSHProbe
	RecordSuccessfulDeployments(string, map[string]domain.LastSuccessfulDeployment) error
}

const deploymentVerificationTimeout = 3 * time.Second

// A computer that has just activated its system may need a few seconds before
// it answers the host-state check, so a missing answer is asked for again.
const (
	deploymentVerificationAttempts  = 3
	deploymentVerificationRetryWait = 5 * time.Second
)

func (m *DeploymentManager) Execute(ctx context.Context, repository, requested, expectedRevision, logPath string, progress io.Writer) domain.DeploymentExecutionReport {
	return m.ExecuteWithProgress(ctx, repository, requested, expectedRevision, logPath, progress, nil)
}

func (m *DeploymentManager) ExecuteWithProgress(ctx context.Context, repository, requested, expectedRevision, logPath string, output io.Writer, observe func(domain.DeploymentProgress)) domain.DeploymentExecutionReport {
	startedAt := m.now().UTC()
	plan := m.Plan(ctx, repository, requested)
	reviewedAt := m.now().UTC()
	report := executionFromPlan(plan, logPath)
	if plan.HasErrors() {
		return report
	}
	if expectedRevision == "" {
		return deploymentExecutionIssue(report, "review", "expected revision is required; copy it from deploy plan")
	}
	if plan.Revision != expectedRevision {
		return deploymentExecutionIssue(report, "review", fmt.Sprintf("reviewed revision %s does not match current revision %s", expectedRevision, plan.Revision))
	}
	if output == nil {
		output = io.Discard
	}

	report.State = "running"
	report.Phase = domain.DeploymentPhaseBuild
	emitDeploymentProgress(observe, domain.DeploymentProgress{
		Phase: domain.DeploymentPhaseBuild, Total: 4,
		Activity: fmt.Sprintf("Building configurations for %d selected computer(s)", len(report.Targets)),
	})
	fmt.Fprintf(output, "Deployment revision: %s\nTargets: %s\n\n", report.Revision, report.ColmenaSelector)
	timing := deploymentTiming{now: m.now, output: output}
	timing.heading("==> Reviewing the selected computers", startedAt)
	timing.took("Reviewing", startedAt, reviewedAt)
	defer func() { timing.total(startedAt) }()
	timing.begin("==> Building the systems of the selected computers")
	err := m.runPhase(ctx, plan, domain.DeploymentProgress{Phase: domain.DeploymentPhaseBuild, Total: 4}, output, observe)
	timing.end("Building")
	if err != nil {
		report.State = "failed"
		report.Message = fmt.Sprintf("build failed before any deployment was started: %v", err)
		return report
	}
	report.BuildCompleted = true
	emitDeploymentProgress(observe, domain.DeploymentProgress{
		Phase: domain.DeploymentPhasePreflight, Completed: 1, Total: 4,
		Activity: "Build completed; revalidating the reviewed revision and targets",
	})

	timing.begin("\n==> Revalidating the reviewed revision and targets")
	verified := m.Plan(ctx, repository, requested)
	timing.end("Revalidating")
	if verified.HasErrors() {
		report.State = "failed"
		report.Phase = domain.DeploymentPhasePreflight
		report.Message = "post-build validation failed; no apply was started"
		report.Issues = append(report.Issues, verified.Issues...)
		return report
	}
	if verified.Revision != expectedRevision || verified.ColmenaSelector != report.ColmenaSelector || !sameDeploymentTargets(verified.Targets, report.Targets) {
		report.State = "failed"
		report.Phase = domain.DeploymentPhasePreflight
		report.Message = "reviewed revision or targets changed after build; no apply was started"
		return deploymentExecutionIssue(report, "review", "run deploy plan again and review the new revision and targets")
	}

	// A computer switched off during the build is left out of the apply and
	// reported: nothing is started on it, so it never becomes pending work
	// that blocks the laboratory. Computers that still answer are updated.
	applyTargets, skipped := answeringDeploymentTargets(verified)
	report.Skipped = skipped
	if len(applyTargets) == 0 {
		report.State = "failed"
		report.Phase = domain.DeploymentPhasePreflight
		report.Message = "no selected computer answered after the build; nothing was applied"
		return report
	}
	if len(skipped) > 0 {
		fmt.Fprintf(output, "\nNot answering after the build, left unchanged: %s\n", strings.Join(skipped, ", "))
	}

	report.Phase = domain.DeploymentPhaseApply
	emitDeploymentProgress(observe, domain.DeploymentProgress{
		Phase: domain.DeploymentPhaseApply, Completed: 2, Total: 4,
		Activity: fmt.Sprintf("Applying built configurations to %d selected computer(s)", len(applyTargets)),
	})
	timing.begin("\n==> Copying and activating the built systems")
	// Apply over the addresses that answered just now: during a change of
	// laboratory network a computer not yet moved is reached on its
	// previous address.
	applyPlan := plan
	applyPlan.Availability = verified.Availability
	if len(skipped) > 0 {
		applyPlan.Targets = applyTargets
		applyPlan.ColmenaSelector = strings.Join(deploymentTargetNames(applyTargets), ",")
	}
	applyErr := m.runPhase(ctx, applyPlan, domain.DeploymentProgress{Phase: domain.DeploymentPhaseApply, Completed: 2, Total: 4}, output, observe)
	timing.end("Applying")
	var unconfirmed *domain.DeploymentUnconfirmedError
	report.RecoveryRequired = errors.As(applyErr, &unconfirmed)
	report.RetrySafe = !report.RecoveryRequired
	if applyErr == nil {
		report.ApplyCompleted = true
	}
	emitDeploymentProgress(observe, domain.DeploymentProgress{
		Phase: domain.DeploymentPhaseVerify, Completed: 3, Total: 4,
		TargetTotal: len(applyTargets), Activity: "Verifying authenticated client state",
	})
	timing.begin("\n==> Verifying selected computers through authenticated host state")
	// Local cancellation must not skip the separately time-bounded observations.
	recordErr := m.verifySuccessfulDeployments(context.WithoutCancel(ctx), &report, applyTargets, output, observe)
	timing.end("Verifying")
	if applyErr != nil {
		report.State = "failed"
		report.Phase = domain.DeploymentPhaseApply
		report.Message = fmt.Sprintf("apply failed; authenticated reconciliation verified %d/%d targets at the reviewed revision: %v", report.Verification.Verified, report.Verification.Attempted, applyErr)
		if report.RecoveryRequired {
			report.Message = fmt.Sprintf("remote activation completion is unconfirmed; %d/%d targets expose the reviewed revision, but this is not completion evidence. New operations are blocked by the pending deployment marker; review the private log and deployment recovery guide: %v", report.Verification.Verified, report.Verification.Attempted, applyErr)
		}
		if recordErr != nil {
			report.Message += fmt.Sprintf("; recording verified targets also failed: %v", recordErr)
		}
		return report
	}
	if recordErr != nil {
		report.State = "failed"
		report.Phase = domain.DeploymentPhaseVerify
		report.Message = fmt.Sprintf("apply completed and %d/%d targets were authenticated at the reviewed revision, but recording verification failed: %v", report.Verification.Verified, report.Verification.Attempted, recordErr)
		return report
	}
	if report.Verification.Verified != report.Verification.Attempted {
		report.State = "partial"
		report.Phase = domain.DeploymentPhaseVerify
		report.Message = fmt.Sprintf("apply completed, but authenticated reconciliation verified only %d/%d targets; inspect host state and retry from a fresh plan", report.Verification.Verified, report.Verification.Attempted)
		return report
	}
	if len(report.Skipped) > 0 {
		report.State = "partial"
		report.Phase = domain.DeploymentPhaseComplete
		report.Message = fmt.Sprintf("%d computer(s) updated and verified; %s stopped answering after the build and kept their current system", report.Verification.Verified, strings.Join(report.Skipped, ", "))
		return report
	}
	report.Phase = domain.DeploymentPhaseComplete
	report.State = "completed"
	report.Message = "all selected targets were built, applied, authenticated, and recorded successfully"
	emitDeploymentProgress(observe, domain.DeploymentProgress{
		Phase: domain.DeploymentPhaseComplete, Completed: 4, Total: 4,
		TargetCurrent: report.Verification.Verified, TargetTotal: report.Verification.Attempted,
		Activity: "Deployment completed and authenticated state was recorded",
	})
	return report
}

// deploymentTiming writes the start time of each step of a client update and
// how long it took into the private deployment log, using the manager clock.
type deploymentTiming struct {
	now     func() time.Time
	output  io.Writer
	started time.Time
}

// begin writes a heading followed by the UTC start time.
func (t *deploymentTiming) begin(heading string) {
	t.started = t.now().UTC()
	t.heading(heading, t.started)
}

// end writes how long the step started by begin took.
func (t *deploymentTiming) end(phase string) {
	t.took(phase, t.started, t.now().UTC())
}

func (t *deploymentTiming) heading(heading string, at time.Time) {
	fmt.Fprintf(t.output, "%s (%s)\n", heading, at.Format(time.RFC3339))
}

func (t *deploymentTiming) took(phase string, started, ended time.Time) {
	fmt.Fprintf(t.output, "    %s took %s\n", phase, ended.Sub(started).Round(time.Second))
}

func (t *deploymentTiming) total(started time.Time) {
	fmt.Fprintf(t.output, "\nTotal time: %s\n", t.now().UTC().Sub(started).Round(time.Second))
}

func (m *DeploymentManager) verifySuccessfulDeployments(ctx context.Context, report *domain.DeploymentExecutionReport, targets []domain.DeploymentTarget, output io.Writer, observe func(domain.DeploymentProgress)) error {
	report.Verification = domain.DeploymentVerificationSummary{
		Attempted: len(targets),
		Targets:   make([]domain.DeploymentTargetVerification, 0, len(targets)),
	}
	hosts := targetHosts(targets)
	observed := m.source.CurrentSystems(ctx, hosts, deploymentVerificationTimeout)
	if observed == nil {
		observed = map[string]domain.HostSystemProbe{}
	}
	for attempt := 1; attempt < deploymentVerificationAttempts; attempt++ {
		pending := hostsWithoutState(hosts, observed)
		if len(pending) == 0 || m.sleep(ctx, deploymentVerificationRetryWait) != nil {
			break
		}
		for name, probe := range m.source.CurrentSystems(ctx, pending, deploymentVerificationTimeout) {
			observed[name] = probe
		}
	}
	availability := m.source.SSHStatus(ctx, hosts, sshProbeTimeout)
	updates := map[string]domain.LastSuccessfulDeployment{}
	verifiedAt := m.now().UTC()
	for index, target := range targets {
		probe, found := observed[target.Name]
		verification := domain.DeploymentTargetVerification{
			Reachability:     availability[target.Name].Reachability,
			SSH:              availability[target.Name].SSH,
			HostKeyCondition: probe.HostKeyCondition,
			Name:             target.Name,
			State:            "unverified",
			Revision:         probe.Revision,
			SystemPath:       probe.SystemPath,
			Detail:           probe.Detail,
			Network:          probe.Network,
		}
		switch {
		case !found:
			verification.Detail = "authenticated host state returned no result"
		case probe.HostKeyCondition == domain.HostKeyChanged:
			verification.Detail = "SSH host key changed; verify physical identity and review this computer's host-key plan before retrying."
		case probe.Revision == "" || probe.SystemPath == "":
			if verification.Detail == "" {
				verification.Detail = "authenticated host state is unavailable"
			}
		case probe.Revision != report.Revision:
			verification.Detail = "authenticated host revision differs from the reviewed revision"
		default:
			verification.State = "verified"
			verification.Detail = ""
			report.Verification.Verified++
			updates[target.Name] = domain.LastSuccessfulDeployment{
				Revision:   probe.Revision,
				SystemPath: probe.SystemPath,
				VerifiedAt: verifiedAt,
			}
		}
		report.Verification.Targets = append(report.Verification.Targets, verification)
		fmt.Fprintf(output, "  %-10s %s", target.Name, verification.State)
		if verification.Detail != "" {
			fmt.Fprintf(output, ": %s", verification.Detail)
		}
		fmt.Fprintln(output)
		emitDeploymentProgress(observe, domain.DeploymentProgress{
			Phase: domain.DeploymentPhaseVerify, Completed: 3, Total: 4,
			TargetCurrent: index + 1, TargetTotal: len(targets),
			Activity: fmt.Sprintf("Checked authenticated state for %d/%d computer(s)", index+1, len(targets)),
		})
	}
	if report.RecoveryRequired {
		report.Verification.Detail = "revision observations do not confirm activation completion; deployment history was not changed"
		return nil
	}
	if len(updates) == 0 {
		report.Verification.Detail = "no target was authenticated at the reviewed revision; deployment history was not changed"
		return nil
	}
	if err := m.source.RecordSuccessfulDeployments(report.Repository, updates); err != nil {
		report.Verification.Detail = "verified target state could not be recorded"
		return err
	}
	report.Verification.Recorded = len(updates)
	report.Verification.Detail = fmt.Sprintf("recorded %d authenticated target(s)", len(updates))
	return nil
}

func emitDeploymentProgress(observe func(domain.DeploymentProgress), progress domain.DeploymentProgress) {
	if observe != nil {
		observe(progress)
	}
}

func executionFromPlan(plan domain.DeploymentPlanReport, logPath string) domain.DeploymentExecutionReport {
	return domain.DeploymentExecutionReport{
		SchemaVersion:   domain.SchemaVersion,
		Operation:       "deploy-apply",
		State:           "blocked",
		Repository:      plan.Repository,
		Requested:       plan.Requested,
		Revision:        plan.Revision,
		ColmenaSelector: plan.ColmenaSelector,
		Targets:         plan.Targets,
		Phase:           domain.DeploymentPhasePreflight,
		RetrySafe:       true,
		LogPath:         logPath,
		Issues:          append([]domain.ValidationIssue{}, plan.Issues...),
	}
}

func deploymentExecutionIssue(report domain.DeploymentExecutionReport, field, message string) domain.DeploymentExecutionReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}

func sameDeploymentTargets(left, right []domain.DeploymentTarget) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type DeploymentManager struct {
	source DeploymentSource
	now    func() time.Time
	sleep  func(context.Context, time.Duration) error
}

func NewDeploymentManager(source DeploymentSource) *DeploymentManager {
	return &DeploymentManager{source: source, now: time.Now, sleep: sleepContext}
}

// sleepContext waits for the duration, or returns the context's error as soon
// as the context ends.
func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// hostsWithoutState are the hosts whose check returned no result or no system
// state, except those whose SSH host key changed: asking again cannot fix that.
func hostsWithoutState(hosts []domain.HostMeta, observed map[string]domain.HostSystemProbe) []domain.HostMeta {
	var pending []domain.HostMeta
	for _, host := range hosts {
		probe, found := observed[host.Name]
		if probe.HostKeyCondition == domain.HostKeyChanged {
			continue
		}
		if !found || probe.Revision == "" || probe.SystemPath == "" {
			pending = append(pending, host)
		}
	}
	return pending
}

func (m *DeploymentManager) Plan(ctx context.Context, repository, requested string) domain.DeploymentPlanReport {
	report := domain.DeploymentPlanReport{
		SchemaVersion: domain.SchemaVersion,
		Operation:     "deploy-plan",
		State:         "blocked",
		Requested:     requested,
		BuildFirst:    true,
		Targets:       []domain.DeploymentTarget{},
		Issues:        []domain.ValidationIssue{},
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return deploymentIssue(report, "repository", fmt.Sprintf("resolve path: %v", err))
	}
	report.Repository = root
	if issue, found := flakePinIssue(m.source, root); found {
		report.Issues = append(report.Issues, issue)
		return report
	}
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		report.Issues = append(report.Issues, domain.EvaluationIssue("configuration", "labMeta", err))
		return report
	}
	rekeyDuringTransition(m.source, meta)
	deployment, err := m.source.DeploymentStatus(ctx, root)
	if err != nil {
		return deploymentIssue(report, "readiness", fmt.Sprintf("evaluate deploymentStatus: %v", err))
	}
	if !deployment.Ready {
		for _, issue := range deployment.Issues {
			report = deploymentIssue(report, "readiness", issue)
		}
	}
	gitState, err := m.source.GitState(ctx, root)
	if err != nil {
		report = deploymentIssue(report, "git", fmt.Sprintf("inspect worktree: %v", err))
	} else if gitState.Dirty {
		report.Issues = append(report.Issues, domain.BlockedIssue("git", "GIT-DIRTY", fmt.Sprintf("deployment worktree has %d changed path(s); save or inspect them in Maintenance → Review Git changes (`nixorium git review`)", gitState.Changes)))
	}
	if revision, revisionErr := m.source.GitRevision(ctx, root); revisionErr != nil {
		report = deploymentIssue(report, "git", fmt.Sprintf("resolve revision: %v", revisionErr))
	} else {
		report.Revision = revision
	}
	if addresses, addressErr := m.source.InterfaceAddresses(meta.Network.Interface); addressErr != nil {
		report = deploymentIssue(report, "network", fmt.Sprintf("inspect %s: %v", meta.Network.Interface, addressErr))
	} else if !containsIP(addresses, meta.Controller.StaticIP) {
		report = deploymentIssue(report, "network", fmt.Sprintf("controller deployment address %s is not assigned to %s; stop or recover PXE mode first", meta.Controller.StaticIP, meta.Network.Interface))
	}

	targets, selector, selectionIssues := selectDeploymentTargets(meta.Clients.Hosts, requested)
	report.Targets = targets
	report.ColmenaSelector = selector
	for _, issue := range selectionIssues {
		report = deploymentIssue(report, "selector", issue)
	}
	if !report.HasErrors() {
		// During a change of laboratory network the probe also tries each
		// computer's previous address and says which one answered.
		probes := m.source.SSHStatus(ctx, targetHosts(targets), sshProbeTimeout)
		for _, target := range targets {
			probe, found := probes[target.Name]
			if !found {
				probe = domain.SSHProbe{Reachability: domain.ReachabilityUnknown, SSH: domain.SSHUnknown}
			}
			report.Availability = append(report.Availability, domain.DeploymentTargetAvailability{Name: target.Name, IP: target.IP, Reachability: probe.Reachability, SSH: probe.SSH, Network: probe.Network})
		}
		if err := ctx.Err(); err != nil {
			return deploymentIssue(report, "availability", "computer availability check cancelled or timed out")
		}
		// A probe can take time. Do not present a clean revision observed before
		// it as a current review when the repository changed during that wait.
		git, gitErr := m.source.GitState(ctx, root)
		revision, revisionErr := m.source.GitRevision(ctx, root)
		if gitErr != nil || git.Dirty || revisionErr != nil || revision != report.Revision {
			return deploymentIssue(report, "review", "deployment changed during availability checks; create a fresh plan")
		}
		reachable := reachableDeploymentNames(report)
		if len(reachable) > 0 && len(reachable) < len(targets) {
			report.ReachableRequested = strings.Join(reachable, ",")
		}
		report.State = "ready"
	}
	return report
}

// flakePinConsistencySource is implemented by sources that can compare
// flake.lock with flake.nix without Nix or network access.
type flakePinConsistencySource interface {
	FlakeLockConsistency(string) error
}

// flakePinIssue refuses early when flake.lock no longer matches flake.nix.
// The client build uses an immutable copy of the deployment, where Nix cannot
// rewrite the lock and would fail with a read-only file system error.
func flakePinIssue(source any, repository string) (domain.ValidationIssue, bool) {
	checker, ok := source.(flakePinConsistencySource)
	if !ok {
		return domain.ValidationIssue{}, false
	}
	err := checker.FlakeLockConsistency(repository)
	if err == nil {
		return domain.ValidationIssue{}, false
	}
	if code, typed := domain.BlockedCodeOf(err); typed {
		return domain.BlockedIssue("pin", code, err.Error()), true
	}
	return domain.ValidationIssue{Field: "pin", Message: err.Error()}, true
}

// PlanReachable creates a new review for an explicit subset. It neither edits
// the old plan nor starts an operation; execution still revalidates normally.
func (m *DeploymentManager) PlanReachable(ctx context.Context, repository string, reviewed domain.DeploymentPlanReport) domain.DeploymentPlanReport {
	names := reachableDeploymentNames(reviewed)
	if reviewed.HasErrors() || reviewed.State != "ready" || len(names) == 0 || len(names) >= len(reviewed.Targets) {
		return deploymentIssue(domain.DeploymentPlanReport{SchemaVersion: domain.SchemaVersion, Operation: "deploy-plan", State: "blocked", BuildFirst: true}, "selector", "no reachable-only subset is available; review the selected computers again")
	}
	return m.Plan(ctx, repository, strings.Join(names, ","))
}

// SplitDeploymentByAvailability separates a reviewed plan's computers that
// answered the SSH check from those that did not (off or unreachable).
func SplitDeploymentByAvailability(plan domain.DeploymentPlanReport) (reachable, unreachable []string) {
	reachable = reachableDeploymentNames(plan)
	for _, target := range plan.Targets {
		if !slices.Contains(reachable, target.Name) {
			unreachable = append(unreachable, target.Name)
		}
	}
	return reachable, unreachable
}

func reachableDeploymentNames(plan domain.DeploymentPlanReport) []string {
	names := []string{}
	for _, target := range plan.Targets {
		for _, observed := range plan.Availability {
			if observed.Name == target.Name && observed.IP == target.IP && observed.Reachability == domain.ReachabilityReachable && observed.SSH == domain.SSHAvailable {
				names = append(names, target.Name)
				break
			}
		}
	}
	return names
}

func selectDeploymentTargets(hosts []domain.HostMeta, requested string) ([]domain.DeploymentTarget, string, []string) {
	available := make(map[string]domain.HostMeta, len(hosts))
	for _, host := range hosts {
		available[host.Name] = host
	}
	names := []string{}
	issues := []string{}
	if requested == "@lab" {
		for _, host := range hosts {
			names = append(names, host.Name)
		}
	} else {
		seen := map[string]bool{}
		for _, raw := range strings.Split(requested, ",") {
			name := strings.TrimSpace(raw)
			if name == "" {
				issues = append(issues, "target list contains an empty hostname")
				continue
			}
			if _, found := available[name]; !found {
				issues = append(issues, fmt.Sprintf("%s is not a configured client", name))
				continue
			}
			if seen[name] {
				issues = append(issues, fmt.Sprintf("%s is selected more than once", name))
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	targets := make([]domain.DeploymentTarget, 0, len(names))
	for _, name := range names {
		host := available[name]
		targets = append(targets, domain.DeploymentTarget{Name: host.Name, IP: host.IP, PreviousIP: host.PreviousIP})
	}
	selector := strings.Join(names, ",")
	if requested == "@lab" && len(issues) == 0 {
		selector = "@lab"
	}
	if len(targets) == 0 && len(issues) == 0 {
		issues = append(issues, "no clients were selected")
	}
	return targets, selector, issues
}

// targetHosts addresses the selected computers, with their previous address
// during a change of laboratory network.
func targetHosts(targets []domain.DeploymentTarget) []domain.HostMeta {
	hosts := make([]domain.HostMeta, 0, len(targets))
	for _, target := range targets {
		hosts = append(hosts, domain.HostMeta{Name: target.Name, IP: target.IP, PreviousIP: target.PreviousIP})
	}
	return hosts
}

func deploymentIssue(report domain.DeploymentPlanReport, field, message string) domain.DeploymentPlanReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: message})
	return report
}

// answeringDeploymentTargets splits a revalidated plan's computers into those
// whose SSH port answered just before the apply and those that did not.
func answeringDeploymentTargets(plan domain.DeploymentPlanReport) ([]domain.DeploymentTarget, []string) {
	answering := reachableDeploymentNames(plan)
	targets := []domain.DeploymentTarget{}
	skipped := []string{}
	for _, target := range plan.Targets {
		if slices.Contains(answering, target.Name) {
			targets = append(targets, target)
		} else {
			skipped = append(skipped, target.Name)
		}
	}
	return targets, skipped
}

func deploymentTargetNames(targets []domain.DeploymentTarget) []string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Name)
	}
	return names
}
