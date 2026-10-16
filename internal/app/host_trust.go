package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type HostTrustSource interface {
	GitState(context.Context, string) (domain.GitState, error)
	LabMeta(context.Context, string) (domain.LabMeta, error)
	GitRevision(context.Context, string) (string, error)
	ObserveHostTrust(context.Context, domain.HostMeta) (domain.HostTrustInspection, error)
	ReplaceHostTrust(context.Context, domain.HostMeta, domain.HostTrustInspection) error
	AcquireClientOperation() (io.Closer, error)
	ServiceState(context.Context, string) domain.ServiceState
}

const hostTrustReviewLifetime = 5 * time.Minute

type HostTrustManager struct {
	source HostTrustSource
	now    func() time.Time
}

func NewHostTrustManager(source HostTrustSource) HostTrustManager {
	return HostTrustManager{source: source, now: time.Now}
}

func (m HostTrustManager) Plan(ctx context.Context, repository, name string) domain.HostTrustPlan {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	p := domain.HostTrustPlan{SchemaVersion: domain.SchemaVersion, Operation: "host-key-plan", State: "blocked", Repository: repository}
	// Capture the revision before reading inventory, then check it again after
	// network observation so a slow handshake cannot authorize stale identity.
	var err error
	p.Revision, err = m.source.GitRevision(ctx, repository)
	if err != nil {
		p.Message = err.Error()
		return p
	}
	git, err := m.source.GitState(ctx, repository)
	if err != nil {
		p.Message = err.Error()
		return p
	}
	if !git.Available || git.Dirty {
		p.Message = "Save and commit the declared computer inventory before reviewing host trust."
		return p
	}
	meta, err := m.source.LabMeta(ctx, repository)
	if err != nil {
		p.Message = err.Error()
		return p
	}
	host, found := hostTrustClient(meta, name)
	if !found {
		p.Message = "Choose exactly one declared client computer; the controller cannot be re-trusted here."
		return p
	}
	p.Host = host
	pxe := ObservePXELifecycle(m.source.ServiceState(ctx, PXEListenerUnit), m.source.ServiceState(ctx, PXENetworkUnit), domain.PXEPreparationState{})
	if pxe.Mode != "stopped" {
		p.Message = "Stop network installation and resolve any PXE recovery before reviewing host trust."
		return p
	}
	p.Inspection, err = m.source.ObserveHostTrust(ctx, host)
	if err == nil {
		git, err = m.source.GitState(ctx, repository)
		if err == nil && (!git.Available || git.Dirty) {
			err = errors.New("Deployment changed during host-key inspection; save and commit the inventory, then create a fresh plan")
		}
	}
	if err == nil {
		var revision string
		revision, err = m.source.GitRevision(ctx, repository)
		if err == nil && revision != p.Revision {
			err = errors.New("Deployment revision changed during host-key inspection; create a fresh plan")
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		p.Message = err.Error()
		return p
	}
	if len(p.Inspection.Recorded) == 0 {
		p.Message = "No recorded key exists for this address; rotation is not needed."
		return p
	}
	conflict := false
	for _, fingerprint := range p.Inspection.Recorded {
		if fingerprint != p.Inspection.Offered {
			conflict = true
		}
	}
	if !conflict {
		p.Message = "The offered key already matches the recorded trust; no change is needed."
		return p
	}
	p.State = "ready"
	p.Confirmation = "ROTATE HOST KEY"
	p.Message = "Trust this changed key only after deliberately reinstalling this exact computer and comparing the offered fingerprint with its physical console. Other entries are preserved."
	// Apply rebuilds this review. The expiry sits on the shared review grid so
	// the rebuilt review can match the confirmed token, which keeps it valid
	// for four to five minutes without letting an old token renew.
	p.ExpiresAt = reviewExpiry(m.now(), hostTrustReviewLifetime)
	p.ReviewToken = hostTrustToken(p)
	return p
}

func hostTrustToken(p domain.HostTrustPlan) string {
	data, _ := json.Marshal(struct {
		Repository, Revision string
		Host                 domain.HostMeta
		Inspection           domain.HostTrustInspection
		ExpiresAt            time.Time
	}{p.Repository, p.Revision, p.Host, p.Inspection, p.ExpiresAt})
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}

// Reviewed reports whether token confirms a still-valid review of the same
// repository revision, computer, recorded trust and offered key as fresh, and
// returns fresh carrying that review's expiry and token. A review created in
// an earlier grid step therefore still matches a rebuilt one until it expires.
func (m HostTrustManager) Reviewed(fresh domain.HostTrustPlan, token string) (domain.HostTrustPlan, bool) {
	if fresh.HasErrors() || token == "" {
		return fresh, false
	}
	for _, expiry := range reviewExpiries(m.now(), hostTrustReviewLifetime) {
		candidate := fresh
		candidate.ExpiresAt = expiry
		if hostTrustToken(candidate) == token {
			candidate.ReviewToken = token
			return candidate, true
		}
	}
	return fresh, false
}

// A changed key is never evidence of inventory identity. Reject ambiguous
// inventories before connecting, including an address shared with the controller.
func hostTrustClient(meta domain.LabMeta, name string) (domain.HostMeta, bool) {
	if !domain.ValidRemoteHostName(meta.Controller.Name) || !domain.ValidRemoteHostName(name) || name == meta.Controller.Name {
		return domain.HostMeta{}, false
	}
	names, addresses := map[string]bool{}, map[string]bool{}
	var selected domain.HostMeta
	for _, host := range meta.Clients.Hosts {
		address, err := netip.ParseAddr(host.IP)
		if !domain.ValidRemoteHostName(host.Name) || host.Name == meta.Controller.Name ||
			err != nil || !address.Is4() || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() || address.IsLinkLocalUnicast() ||
			address.String() != host.IP || host.IP == meta.Controller.StaticIP || host.IP == meta.Controller.DHCPIP ||
			names[host.Name] || addresses[host.IP] {
			return domain.HostMeta{}, false
		}
		names[host.Name], addresses[host.IP] = true, true
		if host.Name == name {
			selected = host
		}
	}
	return selected, selected.Name != ""
}

func (m HostTrustManager) Apply(ctx context.Context, plan domain.HostTrustPlan, token string) domain.HostTrustResult {
	r := domain.HostTrustResult{Operation: "host-key-apply", State: "blocked", Host: plan.Host.Name}
	if plan.HasErrors() || token == "" || token != plan.ReviewToken || plan.Confirmation != "ROTATE HOST KEY" || !m.now().Before(plan.ExpiresAt) {
		r.Message = "Host-key review is invalid or expired; create a fresh plan."
		return r
	}
	gate, err := acquireOperation(m.source, "Host key review")
	if err != nil {
		r.Message = err.Error()
		return r
	}
	defer gate.Close()
	fresh, matched := m.Reviewed(m.Plan(ctx, plan.Repository, plan.Host.Name), token)
	if !matched {
		r.Message = "Computer identity, recorded key or offered key changed; create a fresh plan."
		return r
	}
	if err := m.source.ReplaceHostTrust(ctx, fresh.Host, fresh.Inspection); err != nil {
		r.State = "unconfirmed"
		r.Message = "Host trust update was not confirmed: " + err.Error()
		return r
	}
	r.State = "saved"
	r.Message = "The reviewed key is saved for this computer only. Refresh inventory before updating computers."
	return r
}
