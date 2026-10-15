package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// WakeSource reads the laboratory identities, tells which computers answer
// on the lab network, and keeps and uses their learned hardware addresses.
type WakeSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	SSHStatus(context.Context, []domain.HostMeta, time.Duration) map[string]domain.SSHProbe
	LearnWakeAddresses([]domain.HostMeta)
	WakeAddresses() map[string]domain.WakeAddress
	SendWakePackets(context.Context, domain.WakeNetwork, []string) map[string]error
}

// WakeManager switches on students' computers with Wake-on-LAN. A magic
// packet changes no system state, so, like a screen lock, switching on takes
// neither a review token nor the administrative operation lock. Hardware
// addresses come only from the controller's own record, never from a plan.
type WakeManager struct {
	source WakeSource
	now    func() time.Time
}

func NewWakeManager(source WakeSource) *WakeManager {
	return &WakeManager{source: source, now: time.Now}
}

const (
	// wakeProbeTimeout bounds the check that tells computers already on.
	wakeProbeTimeout = 2 * time.Second
	wakeReviewWindow = 5 * time.Minute
)

func (m *WakeManager) Plan(ctx context.Context, repository, requested string) domain.WakePlan {
	p := domain.WakePlan{SchemaVersion: domain.SchemaVersion, Operation: "wake-plan", State: "blocked", Requested: requested, Targets: []domain.WakeTarget{}, Issues: []domain.ValidationIssue{}}
	fail := func(field, message string) domain.WakePlan {
		p.Issues = append(p.Issues, domain.ValidationIssue{Field: field, Message: message})
		if p.Message == "" {
			p.Message = message
		}
		return p
	}
	var err error
	p.Repository, err = filepath.Abs(repository)
	if err != nil {
		return fail("repository", err.Error())
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		return fail("configuration", fmt.Sprintf("evaluate client inventory: %v", err))
	}
	selected, normalized, issues := selectDeploymentTargets(meta.Clients.Hosts, requested)
	if len(issues) > 0 {
		// The same validation as every classroom operation: nothing is sent
		// while a requested name is not a client computer of this lab.
		p.Targets = wakeInvalidTargets(meta.Clients.Hosts, requested)
		p.Message = "Nothing was sent: " + strings.Join(issues, "; ") + "."
		for _, issue := range issues {
			p.Issues = append(p.Issues, domain.ValidationIssue{Field: "targets", Message: issue})
		}
		return p
	}
	p.Requested = normalized
	hosts := shutdownHostMeta(selected)
	for _, host := range hosts {
		if host.Name == meta.Controller.Name {
			return fail("targets", "The controller cannot be selected.")
		}
	}
	probes := m.source.SSHStatus(ctx, hosts, wakeProbeTimeout)
	on := []domain.HostMeta{}
	for _, host := range hosts {
		if probes[host.Name].Reachability == domain.ReachabilityReachable {
			on = append(on, host)
		}
	}
	// A computer that answers now teaches the controller its address.
	m.source.LearnWakeAddresses(on)
	addresses := m.source.WakeAddresses()
	for _, host := range hosts {
		target := domain.WakeTarget{HostMeta: host}
		switch {
		case probes[host.Name].Reachability == domain.ReachabilityReachable:
			target.State, target.Detail = domain.WakeTargetOn, "Already on."
		case addresses[host.Name].MAC != "":
			target.State = domain.WakeTargetSend
		default:
			target.State, target.Detail = domain.WakeTargetUnknown, domain.WakeUnknownDetail
		}
		p.Targets = append(p.Targets, target)
	}
	p.State = "ready"
	p.ExpiresAt = m.now().UTC().Add(wakeReviewWindow)
	p.Message = wakePlanMessage(p)
	return p
}

// wakeInvalidTargets lists each requested name that cannot be switched on.
func wakeInvalidTargets(hosts []domain.HostMeta, requested string) []domain.WakeTarget {
	targets := []domain.WakeTarget{}
	if requested == "@lab" {
		return targets
	}
	known := map[string]bool{}
	for _, host := range hosts {
		known[host.Name] = true
	}
	seen := map[string]bool{}
	for _, raw := range strings.Split(requested, ",") {
		name := strings.TrimSpace(raw)
		switch {
		case name == "":
		case !known[name]:
			targets = append(targets, domain.WakeTarget{HostMeta: domain.HostMeta{Name: name}, State: domain.WakeTargetInvalid, Detail: "Not a client computer of this laboratory."})
		case seen[name]:
			targets = append(targets, domain.WakeTarget{HostMeta: domain.HostMeta{Name: name}, State: domain.WakeTargetInvalid, Detail: "Selected more than once."})
		}
		seen[name] = true
	}
	return targets
}

func wakePlanMessage(p domain.WakePlan) string {
	send, on, unknown := p.Count(domain.WakeTargetSend), p.Count(domain.WakeTargetOn), p.Count(domain.WakeTargetUnknown)
	parts := []string{}
	if on > 0 {
		parts = append(parts, fmt.Sprintf("%d already on", on))
	}
	if unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d with no known address yet", unknown))
	}
	if send == 0 {
		return "No packet will be sent: " + strings.Join(parts, ", ") + "."
	}
	message := fmt.Sprintf("%s will receive a Wake-on-LAN packet", wakeComputers(send))
	if len(parts) > 0 {
		message += "; " + strings.Join(parts, ", ")
	}
	return message + "."
}

func wakeComputers(count int) string {
	if count == 1 {
		return "1 computer"
	}
	return fmt.Sprintf("%d computers", count)
}

func (m *WakeManager) Apply(ctx context.Context, p domain.WakePlan) domain.WakeReport {
	r := domain.WakeReport{SchemaVersion: domain.SchemaVersion, Operation: "wake-apply", State: "blocked", Targets: []domain.WakeOutcome{}}
	if p.HasErrors() {
		r.Message = "This review cannot be applied; review the computers again."
		return r
	}
	if !m.now().Before(p.ExpiresAt) {
		r.Message = "Review expired; review the computers again."
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
	for _, target := range p.Targets {
		valid := target.State == domain.WakeTargetSend || target.State == domain.WakeTargetOn || target.State == domain.WakeTargetUnknown
		if !valid || target.Name == meta.Controller.Name || identities[target.Name] == "" || identities[target.Name] != target.IP {
			r.Message = "Client inventory changed; review the computers again."
			return r
		}
	}
	addresses := m.source.WakeAddresses()
	macs := []string{}
	for _, target := range p.Targets {
		if mac := addresses[target.Name].MAC; target.State == domain.WakeTargetSend && mac != "" {
			macs = append(macs, mac)
		}
	}
	results := map[string]error{}
	if len(macs) > 0 {
		results = m.source.SendWakePackets(ctx, wakeNetwork(meta), macs)
	}
	for _, target := range p.Targets {
		outcome := domain.WakeOutcome{Name: target.Name}
		mac := addresses[target.Name].MAC
		switch {
		case target.State == domain.WakeTargetOn:
			outcome.State, outcome.Detail = domain.WakeOutcomeOn, "Already on; nothing was sent."
			r.AlreadyOn++
		case target.State == domain.WakeTargetUnknown || mac == "":
			outcome.State, outcome.Detail = domain.WakeOutcomeUnknown, domain.WakeUnknownDetail
			r.Unknown++
		default:
			if err, found := results[mac]; !found || err != nil {
				detail := "no result"
				if err != nil {
					detail = err.Error()
				}
				outcome.State, outcome.Detail = domain.WakeOutcomeFailed, "The packet could not be sent: "+detail+"."
				r.Failed++
			} else {
				outcome.State, outcome.Detail = domain.WakeOutcomeSent, "Packet sent."
				r.Sent++
			}
		}
		r.Targets = append(r.Targets, outcome)
	}
	r.State = "completed"
	if r.Failed > 0 || r.Unknown > 0 {
		r.State = "partial"
	}
	r.Message = wakeReportMessage(r)
	return r
}

// wakeReportMessage counts the packets sent; it never says a computer
// started, which only the computer itself can show.
func wakeReportMessage(r domain.WakeReport) string {
	parts := []string{fmt.Sprintf("Wake-on-LAN packets sent to %s", wakeComputers(r.Sent))}
	if r.AlreadyOn > 0 {
		parts = append(parts, fmt.Sprintf("%d already on", r.AlreadyOn))
	}
	if r.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d with no known address yet", r.Unknown))
	}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d not sent", r.Failed))
	}
	message := strings.Join(parts, "; ") + "."
	if r.Sent > 0 {
		message += " A computer starts only if Wake-on-LAN is allowed in its firmware; one that is still off after a few minutes needs a look."
	}
	return message
}

// wakeNetwork sends from the controller's lab address and interface to the
// lab subnet's broadcast address.
func wakeNetwork(meta domain.LabMeta) domain.WakeNetwork {
	device := meta.Controller.Interface
	if device == "" {
		device = meta.Network.Interface
	}
	return domain.WakeNetwork{
		Broadcast: domain.DirectedBroadcast(meta.Network.Base, meta.Network.PrefixLength),
		Source:    meta.Controller.StaticIP,
		Interface: device,
	}
}
