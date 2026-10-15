package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// migrationLab is a laboratory of three clients on 10.0.0.0/24 and its
// controller at 10.0.0.99, with every observation configurable.
type migrationLab struct {
	t          *testing.T
	settings   domain.LabSettingsFile
	revision   string
	dirty      bool
	pxeActive  bool
	answers    map[string]domain.HostNetwork
	revisions  map[string]string
	addresses  []string
	conditions []domain.BlockingCondition
	queued     []string
	saved      []domain.LabSettingsFile
	rekeyed    [][]domain.KnownHostMove
	rekeyErr   error
	planned    int
}

func newMigrationLab(t *testing.T) *migrationLab {
	_, settings := networkSettingsFixture(t)
	settings.Lab.NetworkBase, settings.Lab.NetworkPrefix, settings.Lab.MasterHostNumber = "10.0.0.0", 24, 99
	settings.Lab.ControllerInterfaceName = "eth1"
	return &migrationLab{
		t: t, settings: settings, revision: strings.Repeat("a", 40),
		answers:   map[string]domain.HostNetwork{"pc01": domain.HostNetworkCurrent, "pc02": domain.HostNetworkCurrent, "pc03": domain.HostNetworkCurrent},
		revisions: map[string]string{},
		addresses: []string{"10.0.0.99"},
	}
}

func (lab *migrationLab) manager() *NetworkMigrationManager {
	manager := NewNetworkMigrationManager(lab, lab, lab, lab, lab, lab)
	manager.now = func() time.Time { return time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC) }
	return manager
}

func (lab *migrationLab) LabMeta(context.Context, string) (domain.LabMeta, error) {
	var meta domain.LabMeta
	meta.Controller.Name = "pc99"
	meta.Controller.StaticIP, _ = domain.ControllerStaticAddress(lab.settings.Lab)
	meta.Network.Interface = "eth1"
	meta.Network.Base, meta.Network.PrefixLength = lab.settings.Lab.NetworkBase, lab.settings.Lab.NetworkPrefix
	previous := lab.settings.Lab
	if transition := lab.settings.Lab.NetworkTransition; transition != nil {
		previous.NetworkBase, previous.NetworkPrefix = transition.PreviousBase, transition.PreviousPrefixLength
		controller, _ := domain.ControllerStaticAddress(previous)
		meta.Network.Transition = &domain.NetworkTransitionMeta{Base: transition.PreviousBase, PrefixLength: transition.PreviousPrefixLength, ControllerIP: controller, StartedAt: transition.StartedAt}
	}
	for number := 1; number <= lab.settings.Lab.PCCount; number++ {
		address, _ := domain.LabHostAddress(lab.settings.Lab, number)
		meta.Clients.Hosts = append(meta.Clients.Hosts, domain.HostMeta{Name: domain.ClientName(number), IP: address})
		if meta.Network.Transition != nil {
			old, _ := domain.LabHostAddress(previous, number)
			meta.Network.Transition.Clients = append(meta.Network.Transition.Clients, domain.HostMeta{Name: domain.ClientName(number), IP: old})
		}
	}
	// Decode as the adapters do, which attaches the previous addresses.
	data, _ := json.Marshal(meta)
	var decoded domain.LabMeta
	if err := json.Unmarshal(data, &decoded); err != nil {
		lab.t.Fatal(err)
	}
	return decoded, nil
}

func (lab *migrationLab) GitState(context.Context, string) (domain.GitState, error) {
	if lab.dirty {
		return domain.GitState{Available: true, Dirty: true, Changes: 1}, nil
	}
	return domain.GitState{Available: true}, nil
}

func (lab *migrationLab) GitRevision(context.Context, string) (string, error) {
	return lab.revision, nil
}

func (lab *migrationLab) ServiceState(_ context.Context, name string) domain.ServiceState {
	return domain.ServiceState{Name: name, Loaded: true, Active: lab.pxeActive, State: map[bool]string{true: "active", false: "inactive"}[lab.pxeActive]}
}

func (lab *migrationLab) InterfaceAddresses(string) ([]string, error) { return lab.addresses, nil }

func (lab *migrationLab) SSHStatus(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.SSHProbe {
	probes := map[string]domain.SSHProbe{}
	for _, host := range hosts {
		probe := domain.SSHProbe{Reachability: domain.ReachabilityUnreachable, SSH: domain.SSHUnknown}
		network, answers := lab.answers[host.Name]
		if answers {
			probe = domain.SSHProbe{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
		}
		if host.PreviousIP != "" {
			probe.Network = domain.HostNetworkUnknown
			if answers {
				probe.Network = network
			}
		}
		probes[host.Name] = probe
	}
	return probes
}

func (lab *migrationLab) CurrentSystems(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.HostSystemProbe {
	systems := map[string]domain.HostSystemProbe{}
	for _, host := range hosts {
		revision := lab.revisions[host.Name]
		if revision == "" {
			revision = lab.revision
		}
		systems[host.Name] = domain.HostSystemProbe{Revision: revision, SystemPath: "/nix/store/system"}
	}
	return systems
}

func (lab *migrationLab) Current(string) (domain.LabSettingsFile, error) { return lab.settings, nil }

func (lab *migrationLab) PlanSettings(_ context.Context, _ string, candidate domain.LabSettingsFile) domain.ConfigPlanReport {
	lab.planned++
	report := domain.ConfigPlanReport{State: "valid", BaseFingerprint: "base", CandidateFingerprint: "candidate"}
	report.Issues = append(candidate.Validate(), domain.SettingsTransitionIssues(lab.settings, candidate)...)
	return report
}

func (lab *migrationLab) Save(_ context.Context, _ string, candidate domain.LabSettingsFile, reviewed domain.ConfigPlanReport) domain.ConfigurationSaveReport {
	if reviewed.BaseFingerprint != "base" {
		return domain.ConfigurationSaveReport{State: "blocked", Issues: []domain.ValidationIssue{{Field: "review", Message: "not reviewed"}}}
	}
	lab.saved = append(lab.saved, candidate)
	lab.settings = candidate
	lab.revision = strings.Repeat("b", 40)
	return domain.ConfigurationSaveReport{State: "saved", Revision: lab.revision}
}

func (lab *migrationLab) Observe(context.Context, string) domain.RecoveryReport {
	return domain.RecoveryReport{Conditions: lab.conditions}
}

func (lab *migrationLab) Status(context.Context, string) domain.DeferredUpdateStatus {
	status := domain.DeferredUpdateStatus{}
	for _, host := range lab.queued {
		status.Updates = append(status.Updates, domain.DeferredUpdateEntry{DeferredUpdate: domain.DeferredUpdate{Host: host}})
	}
	return status
}

func (lab *migrationLab) RekeyMoves(moves []domain.KnownHostMove) (domain.KnownHostRekey, error) {
	lab.rekeyed = append(lab.rekeyed, moves)
	if lab.rekeyErr != nil {
		return domain.KnownHostRekey{}, lab.rekeyErr
	}
	names := []string{}
	for _, move := range moves {
		names = append(names, move.Name)
	}
	return domain.KnownHostRekey{Added: names}, nil
}

func startRequest() NetworkMigrationRequest {
	return NetworkMigrationRequest{Action: domain.NetworkMigrationStart, Base: "10.1.0.0", Prefix: 24}
}

func TestNetworkMigrationStartReviewListsEveryAddress(t *testing.T) {
	lab := newMigrationLab(t)
	plan := lab.manager().Plan(context.Background(), "/lab", startRequest())
	if plan.HasErrors() || plan.ReviewToken == "" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.From != "10.0.0.0/24" || plan.To != "10.1.0.0/24" || plan.ControllerFrom != "10.0.0.99" || plan.ControllerTo != "10.1.0.99" {
		t.Fatalf("networks = %s → %s, controller %s → %s", plan.From, plan.To, plan.ControllerFrom, plan.ControllerTo)
	}
	if len(plan.Computers) != 3 || plan.Computers[2] != (domain.NetworkMigrationComputer{Name: "pc03", From: "10.0.0.3", To: "10.1.0.3", Reachability: domain.ReachabilityReachable, State: domain.MigrationComputerReachable}) {
		t.Fatalf("computers = %+v", plan.Computers)
	}
	if len(plan.Steps) != 4 || !strings.Contains(plan.Steps[1], "both networks (10.0.0.99 and 10.1.0.99)") {
		t.Fatalf("steps = %q", plan.Steps)
	}
	transition := plan.Candidate.Lab.NetworkTransition
	if transition == nil || transition.PreviousBase != "10.0.0.0" || plan.Candidate.Lab.NetworkBase != "10.1.0.0" {
		t.Fatalf("candidate = %+v", plan.Candidate.Lab)
	}
	// A fresh review of the same situation matches, whatever its start time.
	later := lab.manager()
	later.now = func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC) }
	if again := later.Plan(context.Background(), "/lab", startRequest()); again.ReviewToken != plan.ReviewToken {
		t.Fatal("the review token depends on the start time")
	}
}

func TestNetworkMigrationStartNeedsOffComputersAcknowledged(t *testing.T) {
	lab := newMigrationLab(t)
	delete(lab.answers, "pc02")
	plan := lab.manager().Plan(context.Background(), "/lab", startRequest())
	if !plan.HasErrors() || plan.ReviewToken != "" || len(plan.Issues) != 1 || !strings.Contains(plan.Issues[0].Message, "pc02 did not answer") {
		t.Fatalf("plan = %+v", plan.Issues)
	}
	request := startRequest()
	request.AcknowledgeOff = true
	acknowledged := lab.manager().Plan(context.Background(), "/lab", request)
	if acknowledged.HasErrors() || strings.Join(acknowledged.Acknowledged, ",") != "pc02" || acknowledged.Computers[1].State != domain.MigrationComputerAcknowledged {
		t.Fatalf("acknowledged = %+v", acknowledged)
	}
	// Another computer going off after the review changes its token.
	delete(lab.answers, "pc03")
	if again := lab.manager().Plan(context.Background(), "/lab", request); again.ReviewToken == acknowledged.ReviewToken {
		t.Fatal("the token does not bind the computers that are off")
	}
}

func TestNetworkMigrationRefusesBlockedLaboratories(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*migrationLab, *NetworkMigrationRequest)
		message string
	}{
		{"change in progress", func(lab *migrationLab, _ *NetworkMigrationRequest) {
			lab.settings.Lab.NetworkTransition = &domain.NetworkTransition{PreviousBase: "10.9.0.0", PreviousPrefixLength: 24, StartedAt: "2026-10-15T08:00:00Z"}
		}, "already in progress"},
		{"network installation", func(lab *migrationLab, _ *NetworkMigrationRequest) { lab.pxeActive = true }, "network installation"},
		{"unsaved changes", func(lab *migrationLab, _ *NetworkMigrationRequest) { lab.dirty = true }, "unsaved change"},
		{"interrupted update", func(lab *migrationLab, _ *NetworkMigrationRequest) {
			step, _ := domain.NextStepByCode("DEPLOY-PENDING")
			lab.conditions = []domain.BlockingCondition{{Kind: domain.RecoveryDeploymentPending, Title: step.Title, Next: step}}
		}, "interrupted"},
		{"same network", func(_ *migrationLab, request *NetworkMigrationRequest) { request.Base = "10.0.0.0" }, "already the laboratory network"},
		{"unaligned network", func(_ *migrationLab, request *NetworkMigrationRequest) { request.Base = "10.1.0.7" }, "use 10.1.0.0"},
		{"overlapping network", func(_ *migrationLab, request *NetworkMigrationRequest) { request.Base, request.Prefix = "10.0.0.0", 16 }, "must not overlap"},
	} {
		t.Run(test.name, func(t *testing.T) {
			lab := newMigrationLab(t)
			request := startRequest()
			test.change(lab, &request)
			plan := lab.manager().Plan(context.Background(), "/lab", request)
			text := ""
			for _, issue := range plan.Issues {
				text += issue.Message + "\n"
			}
			if !plan.HasErrors() || plan.ReviewToken != "" || !strings.Contains(text, test.message) {
				t.Fatalf("issues = %s", text)
			}
		})
	}
	// Reminders do not block.
	lab := newMigrationLab(t)
	step, _ := domain.NextStepByCode("BACKUP-DUE")
	lab.conditions = []domain.BlockingCondition{{Kind: domain.RecoveryBackupDue, Next: step}}
	if plan := lab.manager().Plan(context.Background(), "/lab", startRequest()); plan.HasErrors() {
		t.Fatalf("a due backup blocked the change: %+v", plan.Issues)
	}
}

func TestNetworkMigrationStartRecordsTrustThenSaves(t *testing.T) {
	lab := newMigrationLab(t)
	manager := lab.manager()
	plan := manager.Plan(context.Background(), "/lab", startRequest())
	if result := manager.Apply(context.Background(), plan, "sha256:other"); !result.HasErrors() || len(lab.saved) != 0 {
		t.Fatalf("a different token saved: %+v", result)
	}
	result := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if result.HasErrors() || result.State != "saved" || len(lab.saved) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if moves := lab.rekeyed[0]; len(moves) != 3 || moves[0] != (domain.KnownHostMove{Name: "pc01", From: "10.0.0.1", To: "10.1.0.1"}) {
		t.Fatalf("moves = %+v", lab.rekeyed)
	}
	if transition := lab.saved[0].Lab.NetworkTransition; transition == nil || transition.PreviousBase != "10.0.0.0" || transition.StartedAt != "2026-10-15T08:00:00Z" {
		t.Fatalf("saved = %+v", lab.saved[0].Lab)
	}
	if len(result.Next) != 3 || !strings.Contains(result.Next[0], "Apply to controller") || !strings.Contains(result.Next[1], "Update computers") {
		t.Fatalf("next = %q", result.Next)
	}

	// Trust that cannot be recorded stops the save.
	lab = newMigrationLab(t)
	lab.rekeyErr = errors.New("known hosts are unsafe")
	manager = lab.manager()
	plan = manager.Plan(context.Background(), "/lab", startRequest())
	if result := manager.Apply(context.Background(), plan, plan.ReviewToken); !result.HasErrors() || len(lab.saved) != 0 || !strings.Contains(result.Message, "known hosts are unsafe") {
		t.Fatalf("result = %+v", result)
	}

	// A commit after the review makes it stale.
	lab = newMigrationLab(t)
	manager = lab.manager()
	plan = manager.Plan(context.Background(), "/lab", startRequest())
	lab.revision = strings.Repeat("c", 40)
	if result := manager.Apply(context.Background(), plan, plan.ReviewToken); !result.HasErrors() || len(lab.saved) != 0 {
		t.Fatalf("stale review saved: %+v", result)
	}
}

// startedLab is the laboratory after start: settings on 10.1.0.0/24 with the
// record, the controller on both addresses.
func startedLab(t *testing.T) *migrationLab {
	lab := newMigrationLab(t)
	manager := lab.manager()
	plan := manager.Plan(context.Background(), "/lab", startRequest())
	if result := manager.Apply(context.Background(), plan, plan.ReviewToken); result.HasErrors() {
		t.Fatalf("start = %+v", result)
	}
	lab.addresses = []string{"10.1.0.99", "10.0.0.99"}
	return lab
}

func TestNetworkMigrationStatusFollowsEveryComputer(t *testing.T) {
	lab := startedLab(t)
	lab.addresses = []string{"10.1.0.99"}
	status := lab.manager().Status(context.Background(), "/lab")
	if !status.InProgress() || status.ControllerReady || !strings.HasPrefix(status.Next, "Apply to controller") || status.CanFinish {
		t.Fatalf("status = %+v", status)
	}
	if len(lab.rekeyed) != 2 || lab.rekeyed[1][2] != (domain.KnownHostMove{Name: "pc03", From: "10.0.0.3", To: "10.1.0.3"}) {
		t.Fatalf("status did not record trust for new addresses: %+v", lab.rekeyed)
	}

	lab.addresses = []string{"10.1.0.99", "10.0.0.99"}
	lab.answers = map[string]domain.HostNetwork{"pc01": domain.HostNetworkCurrent, "pc02": domain.HostNetworkPrevious}
	lab.revisions = map[string]string{"pc02": strings.Repeat("a", 40)}
	lab.queued = []string{"pc03"}
	status = lab.manager().Status(context.Background(), "/lab")
	states := []string{}
	for _, computer := range status.Computers {
		states = append(states, computer.Name+"="+computer.State)
	}
	if strings.Join(states, ",") != "pc01=migrated,pc02=previous,pc03=waiting" || status.Migrated != 1 || status.Previous != 1 || status.Waiting != 1 {
		t.Fatalf("states = %v, status = %+v", states, status)
	}
	if !strings.Contains(status.Next, "Update computers") || status.Computers[1].From != "10.0.0.2" || status.Computers[1].To != "10.1.0.2" {
		t.Fatalf("next = %q, %+v", status.Next, status.Computers[1])
	}

	// A computer running the saved configuration but still on its previous
	// address needs a restart.
	lab.revisions = map[string]string{}
	lab.queued = nil
	lab.answers["pc03"] = domain.HostNetworkCurrent
	status = lab.manager().Status(context.Background(), "/lab")
	if !strings.HasPrefix(status.Next, "Restart 1 computer") || !strings.Contains(status.Computers[1].Detail, "restart it") {
		t.Fatalf("next = %q, %+v", status.Next, status.Computers[1])
	}

	lab.answers["pc02"] = domain.HostNetworkCurrent
	status = lab.manager().Status(context.Background(), "/lab")
	if !status.CanFinish || status.Migrated != 3 || !strings.Contains(status.Next, "finish") {
		t.Fatalf("status = %+v", status)
	}

	// Without a change in progress nothing is probed.
	idle := newMigrationLab(t)
	idle.answers = nil
	if status := idle.manager().Status(context.Background(), "/lab"); status.InProgress() || !strings.Contains(status.Message, "No change") {
		t.Fatalf("status = %+v", status)
	}
}

func TestNetworkMigrationFinishWaitsForEveryComputer(t *testing.T) {
	lab := startedLab(t)
	finish := NetworkMigrationRequest{Action: domain.NetworkMigrationFinish}
	lab.answers["pc02"] = domain.HostNetworkPrevious
	plan := lab.manager().Plan(context.Background(), "/lab", finish)
	if !plan.HasErrors() || plan.ReviewToken != "" || !strings.Contains(plan.Issues[0].Message, "pc02 is not yet on the new network") {
		t.Fatalf("plan = %+v", plan.Issues)
	}
	lab.answers["pc02"] = domain.HostNetworkCurrent
	lab.revisions["pc03"] = strings.Repeat("a", 40)
	if plan := lab.manager().Plan(context.Background(), "/lab", finish); !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "pc03") {
		t.Fatalf("an outdated computer did not block: %+v", plan.Issues)
	}
	delete(lab.revisions, "pc03")
	delete(lab.answers, "pc01")
	if plan := lab.manager().Plan(context.Background(), "/lab", finish); !plan.HasErrors() || !strings.Contains(plan.Issues[0].Message, "pc01") {
		t.Fatalf("a computer that is off did not block: %+v", plan.Issues)
	}
	lab.answers["pc01"] = domain.HostNetworkCurrent

	manager := lab.manager()
	plan = manager.Plan(context.Background(), "/lab", finish)
	if plan.HasErrors() || plan.From != "10.0.0.0/24" || plan.To != "10.1.0.0/24" || !strings.Contains(plan.Steps[3], "network installation again") {
		t.Fatalf("plan = %+v", plan)
	}
	result := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if result.HasErrors() || lab.settings.Lab.NetworkTransition != nil || lab.settings.Lab.NetworkBase != "10.1.0.0" || !strings.Contains(result.Next[2], "network installation") {
		t.Fatalf("result = %+v, settings = %+v", result, lab.settings.Lab)
	}
}

func TestNetworkMigrationUndoIsAlwaysPossibleBeforeFinish(t *testing.T) {
	lab := startedLab(t)
	// Computers in any state, even off, do not block the undo.
	lab.answers = map[string]domain.HostNetwork{"pc01": domain.HostNetworkCurrent, "pc02": domain.HostNetworkPrevious}
	manager := lab.manager()
	plan := manager.Plan(context.Background(), "/lab", NetworkMigrationRequest{Action: domain.NetworkMigrationUndo})
	if plan.HasErrors() || plan.From != "10.1.0.0/24" || plan.To != "10.0.0.0/24" || plan.ControllerFrom != "10.1.0.99" || plan.ControllerTo != "10.0.0.99" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Computers[0].From != "10.1.0.1" || plan.Computers[0].To != "10.0.0.1" || plan.Computers[2].State != domain.MigrationComputerUnknown {
		t.Fatalf("computers = %+v", plan.Computers)
	}
	if !strings.HasPrefix(plan.Steps[1], "Update computers") || !strings.HasPrefix(plan.Steps[2], "Apply to controller") {
		t.Fatalf("steps = %q", plan.Steps)
	}
	result := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if result.HasErrors() || lab.settings.Lab.NetworkTransition != nil || lab.settings.Lab.NetworkBase != "10.0.0.0" {
		t.Fatalf("result = %+v, settings = %+v", result, lab.settings.Lab)
	}
	if !strings.HasPrefix(result.Next[0], "Update computers") {
		t.Fatalf("next = %q", result.Next)
	}
	// With nothing in progress there is nothing to finish or undo.
	if plan := lab.manager().Plan(context.Background(), "/lab", NetworkMigrationRequest{Action: domain.NetworkMigrationUndo}); !plan.HasErrors() {
		t.Fatal("undo without a change in progress")
	}
}
