package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The guided change of laboratory network moves every client and the
// controller from one IPv4 network to another without losing contact. It is
// a sequence of the existing reviewed operations, and the settings record
// (lab.networkTransition) carries the state between them:
//
//	start  saves the new network with the record. Apply to controller then
//	       gives the controller both addresses, and Update computers moves
//	       each client, over whichever address answers; computers that are
//	       off are queued.
//	finish removes the record once every client runs the saved
//	       configuration on its new address; Apply to controller drops the
//	       previous address and Update computers makes clients trust only
//	       the new controller address.
//	undo   restores the previous network and removes the record; Update
//	       computers returns the clients, then Apply to controller drops the
//	       address on the network being left.
//
// Nothing here applies a system: each step saves and records the settings,
// and the operator continues with the ordinary reviews.

// NetworkMigrationSource observes the laboratory for the change of network.
type NetworkMigrationSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	GitState(context.Context, string) (domain.GitState, error)
	GitRevision(context.Context, string) (string, error)
	ServiceState(context.Context, string) domain.ServiceState
	InterfaceAddresses(string) ([]string, error)
	SSHStatus(context.Context, []domain.HostMeta, time.Duration) map[string]domain.SSHProbe
	CurrentSystems(context.Context, []domain.HostMeta, time.Duration) map[string]domain.HostSystemProbe
}

type networkMigrationSettings interface {
	Current(string) (domain.LabSettingsFile, error)
	PlanSettings(context.Context, string, domain.LabSettingsFile) domain.ConfigPlanReport
}

type networkMigrationSaver interface {
	Save(context.Context, string, domain.LabSettingsFile, domain.ConfigPlanReport) domain.ConfigurationSaveReport
}

type networkMigrationRecovery interface {
	Observe(context.Context, string) domain.RecoveryReport
}

type networkMigrationQueue interface {
	Status(context.Context, string) domain.DeferredUpdateStatus
}

// networkMigrationTrust records trusted host keys for new addresses; the
// host trust manager provides it.
type networkMigrationTrust interface {
	RekeyMoves([]domain.KnownHostMove) (domain.KnownHostRekey, error)
}

// NetworkMigrationRequest is what the operator chose for a review.
type NetworkMigrationRequest struct {
	Action string
	// Base and Prefix name the new network (start only).
	Base   string
	Prefix int
	// AcknowledgeOff continues without the computers that do not answer;
	// they move when they are next updated.
	AcknowledgeOff bool
}

type NetworkMigrationManager struct {
	source   NetworkMigrationSource
	settings networkMigrationSettings
	saver    networkMigrationSaver
	recovery networkMigrationRecovery
	queue    networkMigrationQueue
	trust    networkMigrationTrust
	now      func() time.Time
}

func NewNetworkMigrationManager(source NetworkMigrationSource, settings networkMigrationSettings, saver networkMigrationSaver, recovery networkMigrationRecovery, queue networkMigrationQueue, trust networkMigrationTrust) *NetworkMigrationManager {
	return &NetworkMigrationManager{source: source, settings: settings, saver: saver, recovery: recovery, queue: queue, trust: trust, now: time.Now}
}

// blockingRecoveryKinds stop a change of laboratory network; reminders such
// as a due backup or queued updates do not.
var blockingRecoveryKinds = []string{
	domain.RecoveryDeploymentPending, domain.RecoveryUSBReserved, domain.RecoveryResetPending,
	domain.RecoveryPXE, domain.RecoveryOperationBusy, domain.RecoverySettingsInvalid,
}

func newNetworkMigrationPlan(repository, action string) domain.NetworkMigrationPlan {
	return domain.NetworkMigrationPlan{
		SchemaVersion: domain.SchemaVersion, Operation: "network-migrate-plan", Action: action, State: "blocked",
		Repository: repository, Computers: []domain.NetworkMigrationComputer{}, Acknowledged: []string{},
		Steps: []string{}, Issues: []domain.ValidationIssue{},
	}
}

// Plan reviews one step. It never changes anything.
func (m *NetworkMigrationManager) Plan(ctx context.Context, repository string, request NetworkMigrationRequest) domain.NetworkMigrationPlan {
	root, err := filepath.Abs(repository)
	if err != nil {
		root = repository
	}
	plan := newNetworkMigrationPlan(root, request.Action)
	issue := func(field, message string) {
		plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: field, Message: message})
	}
	if request.Action != domain.NetworkMigrationStart && request.Action != domain.NetworkMigrationFinish && request.Action != domain.NetworkMigrationUndo {
		issue("action", "choose start, finish or undo")
		return plan
	}
	settings, err := m.settings.Current(root)
	if err != nil {
		plan.Issues = append(plan.Issues, domain.BlockedIssue("settings", "SETTINGS-INVALID", "the laboratory settings cannot be read: "+err.Error()))
		return plan
	}
	transition := settings.Lab.NetworkTransition
	switch {
	case request.Action == domain.NetworkMigrationStart && transition != nil:
		plan.Issues = append(plan.Issues, domain.BlockedIssue("network", "NETWORK-CHANGING", "a change of laboratory network is already in progress; finish or undo it before starting another"))
	case request.Action != domain.NetworkMigrationStart && transition == nil:
		issue("network", "no change of laboratory network is in progress")
	case settings.Lab.PCCount <= 0:
		issue("network", "no client computers are configured; change the laboratory network in Settings instead")
	}
	base, prefix := request.Base, request.Prefix
	if request.Action == domain.NetworkMigrationStart && len(plan.Issues) == 0 {
		if base, prefix, err = domain.CheckLabNetwork(request.Base, request.Prefix); err != nil {
			issue("network", err.Error())
		}
	}
	if len(plan.Issues) > 0 {
		return plan
	}
	m.checkPreconditions(ctx, root, &plan)
	if len(plan.Issues) > 0 {
		return plan
	}

	var candidate domain.LabSettingsFile
	switch request.Action {
	case domain.NetworkMigrationStart:
		candidate, err = domain.StartNetworkCandidate(settings, base, prefix, m.now())
		if err == nil {
			plan.ControllerFrom, plan.ControllerTo, plan.Computers, err = domain.MigrationAddresses(settings.Lab, base, prefix)
		}
		plan.From = domain.LabNetworkText(settings.Lab.NetworkBase, settings.Lab.NetworkPrefix)
		plan.To = domain.LabNetworkText(base, prefix)
	case domain.NetworkMigrationFinish:
		candidate, err = domain.FinishNetworkCandidate(settings)
		if err == nil {
			previous := settings.Lab
			previous.NetworkBase, previous.NetworkPrefix = transition.PreviousBase, transition.PreviousPrefixLength
			plan.ControllerFrom, plan.ControllerTo, plan.Computers, err = domain.MigrationAddresses(previous, settings.Lab.NetworkBase, settings.Lab.NetworkPrefix)
		}
		plan.From = domain.LabNetworkText(transition.PreviousBase, transition.PreviousPrefixLength)
		plan.To = domain.LabNetworkText(settings.Lab.NetworkBase, settings.Lab.NetworkPrefix)
	case domain.NetworkMigrationUndo:
		candidate, err = domain.UndoNetworkCandidate(settings)
		if err == nil {
			plan.ControllerFrom, plan.ControllerTo, plan.Computers, err = domain.MigrationAddresses(settings.Lab, transition.PreviousBase, transition.PreviousPrefixLength)
		}
		plan.From = domain.LabNetworkText(settings.Lab.NetworkBase, settings.Lab.NetworkPrefix)
		plan.To = domain.LabNetworkText(transition.PreviousBase, transition.PreviousPrefixLength)
	}
	if err != nil {
		issue("network", err.Error())
		return plan
	}

	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		plan.Issues = append(plan.Issues, domain.EvaluationIssue("configuration", "labMeta", err))
		return plan
	}
	m.observeComputers(ctx, &plan, meta, request)
	if err := ctx.Err(); err != nil {
		issue("computers", "the computer check was cancelled or timed out")
		return plan
	}

	review := m.settings.PlanSettings(ctx, root, candidate)
	if review.HasErrors() || (review.State != "valid" && review.State != "unchanged") {
		plan.Issues = append(plan.Issues, review.Issues...)
		if len(review.Issues) == 0 {
			issue("settings", "the new settings were not accepted; review them again")
		}
		return plan
	}
	plan.Candidate, plan.Settings = &candidate, review
	plan.Steps = networkMigrationSteps(plan)
	if len(plan.Issues) > 0 {
		return plan
	}
	plan.State = "ready"
	plan.ReviewToken = networkMigrationToken(plan, review)
	plan.Message = networkMigrationReadyMessage(plan)
	return plan
}

// checkPreconditions refuses while a catalogued situation blocks laboratory
// changes, network installation runs, or the configuration has unsaved
// changes; it also reads the revision the review binds.
func (m *NetworkMigrationManager) checkPreconditions(ctx context.Context, root string, plan *domain.NetworkMigrationPlan) {
	if m.recovery != nil {
		for _, condition := range m.recovery.Observe(ctx, root).Conditions {
			if slices.Contains(blockingRecoveryKinds, condition.Kind) {
				step := condition.Next
				message := sentenceText(condition.Title)
				if condition.Detail != "" {
					message += " " + condition.Detail
				}
				plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: "situation", Message: message, Next: &step})
			}
		}
	}
	pxe := ObservePXELifecycle(m.source.ServiceState(ctx, PXEListenerUnit), m.source.ServiceState(ctx, PXENetworkUnit), domain.PXEPreparationState{})
	if pxe.Mode != "stopped" && pxe.Mode != "ready" && pxe.Mode != "unavailable" {
		plan.Issues = append(plan.Issues, domain.BlockedIssue("pxe", "PXE-ACTIVE", "network installation is active or was interrupted; finish it or recover the controller network before changing the laboratory network"))
	}
	git, err := m.source.GitState(ctx, root)
	switch {
	case err != nil:
		plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: "git", Message: "inspect the configuration repository: " + err.Error()})
	case git.Dirty:
		plan.Issues = append(plan.Issues, domain.BlockedIssue("git", "GIT-DIRTY", fmt.Sprintf("the configuration has %d unsaved change(s); save or inspect them in Maintenance → Review Git changes first", git.Changes)))
	}
	if revision, err := m.source.GitRevision(ctx, root); err != nil {
		plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: "git", Message: "resolve the configuration revision: " + err.Error()})
	} else {
		plan.Revision = revision
	}
}

func sentenceText(text string) string {
	if text == "" || strings.HasSuffix(text, ".") {
		return text
	}
	return text + "."
}

// observeComputers checks every client. Starting requires each one to answer
// or to be acknowledged as off; finishing requires each one to run the saved
// configuration on its new address; undoing only reports them.
func (m *NetworkMigrationManager) observeComputers(ctx context.Context, plan *domain.NetworkMigrationPlan, meta domain.LabMeta, request NetworkMigrationRequest) {
	hosts := meta.Clients.Hosts
	configured := map[string]bool{}
	for _, host := range hosts {
		configured[host.Name] = true
	}
	probes := m.source.SSHStatus(ctx, hosts, sshProbeTimeout)
	var systems map[string]domain.HostSystemProbe
	if plan.Action == domain.NetworkMigrationFinish {
		answering := []domain.HostMeta{}
		for _, host := range hosts {
			if probes[host.Name].SSH == domain.SSHAvailable {
				answering = append(answering, host)
			}
		}
		systems = m.source.CurrentSystems(ctx, answering, hostSystemProbeTimeout)
	}
	off, notMoved := []string{}, []string{}
	for index := range plan.Computers {
		computer := &plan.Computers[index]
		probe, probed := probes[computer.Name]
		if !configured[computer.Name] || !probed {
			probe = domain.SSHProbe{Reachability: domain.ReachabilityUnknown, SSH: domain.SSHUnknown}
		}
		computer.Reachability, computer.Network = probe.Reachability, probe.Network
		answers := probe.SSH == domain.SSHAvailable
		switch plan.Action {
		case domain.NetworkMigrationStart:
			computer.State = domain.MigrationComputerReachable
			if !answers {
				computer.State = domain.MigrationComputerOff
				off = append(off, computer.Name)
				if request.AcknowledgeOff {
					computer.State = domain.MigrationComputerAcknowledged
					computer.Detail = "moves when it is next updated; it is queued if still off"
					plan.Acknowledged = append(plan.Acknowledged, computer.Name)
				}
			}
		case domain.NetworkMigrationFinish:
			system := systems[computer.Name]
			computer.Revision = system.Revision
			if system.Network != "" && system.Network != domain.HostNetworkUnknown {
				computer.Network = system.Network
			}
			computer.State = migrationComputerState(computer.Network, answers, system.Revision, plan.Revision, false)
			if computer.State != domain.MigrationComputerMigrated {
				notMoved = append(notMoved, computer.Name)
				computer.Detail = migrationComputerDetail(computer.State, system.Revision == plan.Revision)
			}
		case domain.NetworkMigrationUndo:
			computer.State = migrationComputerState(computer.Network, answers, "", "", false)
			if !answers {
				computer.Detail = "returns when it is next updated; it is queued if still off"
			}
		}
	}
	if len(off) > 0 && !request.AcknowledgeOff {
		plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: "computers", Message: fmt.Sprintf("%s did not answer. Switch them on and check again, or continue without them: they move when they are next updated, and Update computers can queue them.", strings.Join(off, ", "))})
	}
	if len(notMoved) > 0 {
		plan.Issues = append(plan.Issues, domain.ValidationIssue{Field: "computers", Message: fmt.Sprintf("%s %s not yet on the new network with the saved configuration. Update or switch them on first (see the change status); finishing now would leave them without the controller.", strings.Join(notMoved, ", "), map[bool]string{true: "is", false: "are"}[len(notMoved) == 1])})
	}
}

// migrationComputerState classifies one computer during the change.
func migrationComputerState(network domain.HostNetwork, answers bool, revision, desired string, queued bool) string {
	switch {
	case answers && network == domain.HostNetworkPrevious:
		return domain.MigrationComputerPrevious
	case answers && desired != "" && revision != desired:
		return domain.MigrationComputerOutdated
	case answers:
		return domain.MigrationComputerMigrated
	case queued:
		return domain.MigrationComputerWaiting
	default:
		return domain.MigrationComputerUnknown
	}
}

func migrationComputerDetail(state string, runsSaved bool) string {
	switch state {
	case domain.MigrationComputerPrevious:
		if runsSaved {
			return "runs the saved configuration but still answers on its previous address; restart it to move it"
		}
		return "still on the previous network; update it"
	case domain.MigrationComputerOutdated:
		return "answers on its new address but runs an older configuration; update it"
	case domain.MigrationComputerWaiting:
		return "off; queued, it moves when switched on"
	case domain.MigrationComputerUnknown:
		return "does not answer on either address; switch it on, then update it"
	}
	return ""
}

func networkMigrationSteps(plan domain.NetworkMigrationPlan) []string {
	switch plan.Action {
	case domain.NetworkMigrationStart:
		return []string{
			fmt.Sprintf("Save %s as the laboratory network; %s stays recorded as the previous one until you finish.", plan.To, plan.From),
			fmt.Sprintf("Apply to controller: it then answers on both networks (%s and %s).", plan.ControllerFrom, plan.ControllerTo),
			"Update computers: each one moves to its new address and is checked there; computers that are off are queued and move when switched on.",
			"Finish when every computer is on the new network, or undo the change at any time before.",
		}
	case domain.NetworkMigrationFinish:
		return []string{
			fmt.Sprintf("Save the end of the change: %s stays the laboratory network and %s is forgotten.", plan.To, plan.From),
			fmt.Sprintf("Apply to controller: it drops its previous address %s.", plan.ControllerFrom),
			fmt.Sprintf("Update computers: they then accept the controller only on %s.", plan.ControllerTo),
			"Prepare network installation again (Installation → Network boot) before installing a computer by PXE.",
		}
	default:
		return []string{
			fmt.Sprintf("Save %s as the laboratory network again and forget %s.", plan.To, plan.From),
			"Update computers: each one returns to its previous address; computers that are off are queued.",
			fmt.Sprintf("Apply to controller: it then drops its address %s on the network you are leaving.", plan.ControllerFrom),
		}
	}
}

func networkMigrationReadyMessage(plan domain.NetworkMigrationPlan) string {
	switch plan.Action {
	case domain.NetworkMigrationStart:
		return fmt.Sprintf("Ready to move the laboratory from %s to %s. Saving changes no computer yet.", plan.From, plan.To)
	case domain.NetworkMigrationFinish:
		return fmt.Sprintf("Every computer is on %s. Finishing removes the previous network %s.", plan.To, plan.From)
	default:
		return fmt.Sprintf("Ready to return the laboratory to %s. Saving changes no computer yet.", plan.To)
	}
}

// networkMigrationToken binds the review: action, networks, saved settings,
// revision, and every computer with its observed state. The start time of
// the record is left out, so a fresh review of the same situation matches.
func networkMigrationToken(plan domain.NetworkMigrationPlan, review domain.ConfigPlanReport) string {
	type computer struct{ Name, From, To, State string }
	computers := make([]computer, 0, len(plan.Computers))
	for _, entry := range plan.Computers {
		computers = append(computers, computer{entry.Name, entry.From, entry.To, entry.State})
	}
	data, _ := json.Marshal(struct {
		Action, Repository, Revision, BaseFingerprint, From, To string
		Computers                                               []computer
		Acknowledged                                            []string
	}{plan.Action, plan.Repository, plan.Revision, review.BaseFingerprint, plan.From, plan.To, computers, plan.Acknowledged})
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}

// Apply saves the reviewed step: the settings are written and recorded
// locally; no computer changes. Starting first records the trusted host key
// of every client for its new address, so SSH never has to trust a key it
// has not seen on the previous address.
func (m *NetworkMigrationManager) Apply(ctx context.Context, plan domain.NetworkMigrationPlan, token string) domain.NetworkMigrationResult {
	result := domain.NetworkMigrationResult{SchemaVersion: domain.SchemaVersion, Operation: "network-migrate-" + plan.Action, Action: plan.Action, State: "blocked", Repository: plan.Repository, Next: []string{}, Issues: []domain.ValidationIssue{}}
	refuse := func(message string) domain.NetworkMigrationResult {
		result.Issues = append(result.Issues, domain.ValidationIssue{Field: "review", Message: message})
		result.Message = "Nothing was saved. " + upperFirst(message) + "."
		return result
	}
	if plan.HasErrors() || plan.Candidate == nil || token == "" || token != plan.ReviewToken {
		return refuse("the review is missing or changed; review the change of laboratory network again")
	}
	check := newNetworkMigrationPlan(plan.Repository, plan.Action)
	m.checkPreconditions(ctx, plan.Repository, &check)
	if len(check.Issues) > 0 {
		result.Issues = append(result.Issues, check.Issues...)
		result.Message = "Nothing was saved: " + check.Issues[0].Message
		return result
	}
	if check.Revision != plan.Revision {
		return refuse("the configuration changed after the review; review the change again")
	}
	if plan.Action == domain.NetworkMigrationStart && m.trust != nil {
		moves := make([]domain.KnownHostMove, 0, len(plan.Computers))
		for _, computer := range plan.Computers {
			moves = append(moves, domain.KnownHostMove{Name: computer.Name, From: computer.From, To: computer.To})
		}
		rekey, err := m.trust.RekeyMoves(moves)
		if err != nil {
			return refuse("the trusted computer keys could not be recorded for the new addresses (" + err.Error() + ")")
		}
		result.Rekeyed = rekey.Added
	}
	saved := m.saver.Save(ctx, plan.Repository, *plan.Candidate, plan.Settings)
	result.State, result.Revision, result.RecoveryRequired = saved.State, saved.Revision, saved.RecoveryRequired
	result.Issues = append(result.Issues, saved.Issues...)
	if saved.HasErrors() || (saved.State != "saved" && saved.State != "unchanged") {
		result.Message = saved.Message
		return result
	}
	result.State = "saved"
	result.Next = networkMigrationNext(plan)
	switch plan.Action {
	case domain.NetworkMigrationStart:
		result.Message = fmt.Sprintf("The change to %s is saved locally; no computer was changed yet.", plan.To)
	case domain.NetworkMigrationFinish:
		result.Message = fmt.Sprintf("The end of the change is saved locally; %s is the only laboratory network once the controller and the computers are updated.", plan.To)
	default:
		result.Message = fmt.Sprintf("The return to %s is saved locally; no computer was changed yet.", plan.To)
	}
	return result
}

func networkMigrationNext(plan domain.NetworkMigrationPlan) []string {
	controller := "Apply to controller (Maintenance → Apply to controller; nixorium controller plan, then controller apply --expect <revision>)"
	computers := "Update computers (Computers → Update computers; nixorium deploy plan --on @lab, then deploy apply --on @lab --expect <revision> --queue-unreachable)"
	switch plan.Action {
	case domain.NetworkMigrationStart:
		return []string{controller + ": it then answers on both networks.", computers + ": each one moves to its new address.", "Follow the change in Maintenance → Change network addressing (nixorium network migrate status), then finish it."}
	case domain.NetworkMigrationFinish:
		return []string{controller + ": it drops its previous address.", computers + ": they then trust only the new controller address.", "Prepare network installation again (Installation → Network boot) before installing a computer by PXE."}
	default:
		return []string{computers + ": each one returns to its previous address.", controller + ": it then drops the address on the network you left."}
	}
}

// Status reports the change in progress computer by computer. It records
// trusted host keys for new addresses that are still missing, and never
// changes a computer.
func (m *NetworkMigrationManager) Status(ctx context.Context, repository string) domain.NetworkMigrationStatus {
	root, err := filepath.Abs(repository)
	if err != nil {
		root = repository
	}
	status := domain.NetworkMigrationStatus{SchemaVersion: domain.SchemaVersion, Operation: "network-migrate-status", State: "none", Repository: root, Computers: []domain.NetworkMigrationComputer{}, Issues: []domain.ValidationIssue{}}
	settings, err := m.settings.Current(root)
	if err != nil {
		status.Issues = append(status.Issues, domain.BlockedIssue("settings", "SETTINGS-INVALID", "the laboratory settings cannot be read: "+err.Error()))
		return status
	}
	status.To = domain.LabNetworkText(settings.Lab.NetworkBase, settings.Lab.NetworkPrefix)
	transition := settings.Lab.NetworkTransition
	if transition == nil {
		status.Message = fmt.Sprintf("No change of the laboratory network is in progress. The laboratory network is %s.", status.To)
		return status
	}
	status.State = "in-progress"
	status.From = domain.LabNetworkText(transition.PreviousBase, transition.PreviousPrefixLength)
	status.StartedAt = transition.StartedAt
	meta, err := m.source.LabMeta(ctx, root)
	if err != nil {
		status.Issues = append(status.Issues, domain.EvaluationIssue("configuration", "labMeta", err))
		return status
	}
	if revision, err := m.source.GitRevision(ctx, root); err == nil {
		status.Revision = revision
	}
	if m.trust != nil {
		if rekey, err := m.trust.RekeyMoves(domain.TransitionKnownHostMoves(meta)); err != nil {
			status.Issues = append(status.Issues, domain.ValidationIssue{Field: "host-trust", Message: "trusted computer keys could not be recorded for the new addresses: " + err.Error()})
		} else if len(rekey.Conflicts) > 0 {
			status.Issues = append(status.Issues, domain.ValidationIssue{Field: "host-trust", Message: strings.Join(rekey.Conflicts, ", ") + ": another key is already trusted for the new address; check these computers in Computer inventory"})
		}
	}
	status.ControllerTo = meta.Controller.StaticIP
	if meta.Network.Transition != nil {
		status.ControllerFrom = meta.Network.Transition.ControllerIP
	}
	if addresses, err := m.source.InterfaceAddresses(meta.Network.Interface); err == nil {
		status.ControllerReady = containsIP(addresses, status.ControllerTo) && (status.ControllerFrom == "" || containsIP(addresses, status.ControllerFrom))
	}
	queued := map[string]bool{}
	if m.queue != nil {
		for _, entry := range m.queue.Status(ctx, root).Updates {
			if !entry.Stale {
				queued[entry.Host] = true
			}
		}
	}
	hosts := meta.Clients.Hosts
	probes := m.source.SSHStatus(ctx, hosts, sshProbeTimeout)
	answering := []domain.HostMeta{}
	for _, host := range hosts {
		if probes[host.Name].SSH == domain.SSHAvailable {
			answering = append(answering, host)
		}
	}
	systems := m.source.CurrentSystems(ctx, answering, hostSystemProbeTimeout)
	restart := 0
	for _, host := range hosts {
		probe := probes[host.Name]
		system := systems[host.Name]
		network := probe.Network
		if system.Network != "" && system.Network != domain.HostNetworkUnknown {
			network = system.Network
		}
		answers := probe.SSH == domain.SSHAvailable
		state := migrationComputerState(network, answers, system.Revision, status.Revision, queued[host.Name])
		computer := domain.NetworkMigrationComputer{Name: host.Name, From: host.PreviousIP, To: host.IP, Reachability: probe.Reachability, Network: network, Revision: system.Revision, State: state,
			Detail: migrationComputerDetail(state, system.Revision != "" && system.Revision == status.Revision)}
		switch state {
		case domain.MigrationComputerMigrated:
			status.Migrated++
		case domain.MigrationComputerOutdated:
			status.Outdated++
		case domain.MigrationComputerPrevious:
			status.Previous++
			if system.Revision != "" && system.Revision == status.Revision {
				restart++
			}
		case domain.MigrationComputerWaiting:
			status.Waiting++
		default:
			status.Unknown++
		}
		status.Computers = append(status.Computers, computer)
	}
	status.CanFinish = status.ControllerReady && status.Migrated == len(status.Computers)
	switch {
	case !status.ControllerReady:
		status.Next = "Apply to controller (Maintenance → Apply to controller), so it answers on both networks."
	case status.Previous-restart+status.Outdated > 0:
		status.Next = fmt.Sprintf("Update computers (Computers → Update computers): %s still %s the new configuration.", countLabel(status.Previous-restart+status.Outdated, "computer"), map[bool]string{true: "needs", false: "need"}[status.Previous-restart+status.Outdated == 1])
	case restart > 0:
		status.Next = fmt.Sprintf("Restart %s still on the previous network (Computers → Power controls) so %s its new address.", countLabel(restart, "computer"), map[bool]string{true: "it takes", false: "they take"}[restart == 1])
	case status.Waiting+status.Unknown > 0:
		status.Next = "Switch on the computers that are off: queued ones move by themselves; update the others when they answer."
	default:
		status.Next = "Every computer is on the new network: finish the change."
	}
	status.Message = fmt.Sprintf("Changing the laboratory network from %s to %s: %d of %d computers moved.", status.From, status.To, status.Migrated, len(status.Computers))
	return status
}

// RekeyMoves records the trusted host keys of computers that move to new
// laboratory addresses under those addresses too, through the managed SSH
// trust store. It never contacts a computer or replaces a key.
func (m HostTrustManager) RekeyMoves(moves []domain.KnownHostMove) (domain.KnownHostRekey, error) {
	if len(moves) == 0 {
		return domain.KnownHostRekey{}, nil
	}
	rekeyer, ok := m.source.(knownHostRekeyer)
	if !ok {
		return domain.KnownHostRekey{}, errors.New("trusted computer keys cannot be updated in this session")
	}
	return rekeyer.RekeyKnownHosts(moves)
}

type knownHostRekeyer interface {
	RekeyKnownHosts([]domain.KnownHostMove) (domain.KnownHostRekey, error)
}

// rekeyDuringTransition records missing trusted keys for new addresses
// before a probe reaches a computer there, when the source can.
func rekeyDuringTransition(source any, meta domain.LabMeta) {
	if meta.Network.Transition == nil {
		return
	}
	if rekeyer, ok := source.(knownHostRekeyer); ok {
		_, _ = rekeyer.RekeyKnownHosts(domain.TransitionKnownHostMoves(meta))
	}
}
