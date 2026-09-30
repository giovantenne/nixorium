package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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
	p.Revision, err = m.source.GitRevision(ctx, repository)
	if err != nil {
		p.Message = err.Error()
		return p
	}
	p.Inspection, err = m.source.ObserveHostTrust(ctx, host)
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
	// CLI apply regenerates this review. A bounded shared window lets it match
	// the displayed token without allowing an old token to renew indefinitely.
	p.ExpiresAt = m.now().UTC().Truncate(5 * time.Minute).Add(5 * time.Minute)
	data, _ := json.Marshal(struct {
		Repository, Revision string
		Host                 domain.HostMeta
		Inspection           domain.HostTrustInspection
		ExpiresAt            time.Time
	}{p.Repository, p.Revision, p.Host, p.Inspection, p.ExpiresAt})
	p.ReviewToken = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	return p
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
	gate, err := m.source.AcquireClientOperation()
	if err != nil {
		r.Message = err.Error()
		return r
	}
	defer gate.Close()
	fresh := m.Plan(ctx, plan.Repository, plan.Host.Name)
	if fresh.HasErrors() || fresh.ReviewToken != token {
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
