package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type InternetSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	ServiceState(context.Context, string) domain.ServiceState
	ClientOperationActive() (bool, error)
	AcquireClientOperation() (io.Closer, error)
	ObserveInternet(context.Context, domain.HostMeta) domain.InternetObservation
	ChangeInternet(context.Context, domain.HostMeta, domain.InternetAction, string) error
}
type InternetManager struct {
	source InternetSource
	now    func() time.Time
}

func NewInternetManager(source InternetSource) *InternetManager {
	return &InternetManager{source, time.Now}
}
func (m *InternetManager) conflict(ctx context.Context) bool {
	state := ObservePXELifecycle(m.source.ServiceState(ctx, PXEListenerUnit), m.source.ServiceState(ctx, PXENetworkUnit), domain.PXEPreparationState{})
	return state.Mode == "active" || state.Mode == "degraded" || state.Mode == "recovery-required"
}
func (m *InternetManager) Plan(ctx context.Context, repository, requested string, action domain.InternetAction) domain.InternetPlan {
	p := domain.InternetPlan{SchemaVersion: domain.SchemaVersion, Operation: "internet-plan", State: "blocked", Action: action, Requested: requested, Targets: []domain.InternetTarget{}, Issues: []domain.ValidationIssue{}}
	fail := func(message string) domain.InternetPlan {
		p.Message = message
		p.Issues = append(p.Issues, domain.ValidationIssue{Field: "internet", Message: message})
		return p
	}
	var err error
	p.Repository, err = filepath.Abs(repository)
	if err != nil {
		return fail(err.Error())
	}
	if !action.Valid() {
		return fail("Choose block or unblock.")
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
	active, err := m.source.ClientOperationActive()
	if err != nil {
		return fail(err.Error())
	}
	if active || m.conflict(ctx) {
		return fail("Another client operation or network installation is active; finish it first.")
	}
	eligible := 0
	for _, host := range shutdownHostMeta(selected) {
		if host.Name == meta.Controller.Name {
			return fail("The controller cannot be selected.")
		}
		observed := m.source.ObserveInternet(ctx, host)
		ok := observed.Valid() && (observed.State != "unknown" || action == domain.InternetUnblock)
		if ok {
			eligible++
		}
		p.Targets = append(p.Targets, domain.InternetTarget{HostMeta: host, Observed: observed, Eligible: ok})
	}
	if eligible == 0 {
		return fail("No selected client supports authenticated Internet control. Update or reconnect the clients and review again.")
	}
	p.State = "ready"
	p.ExpiresAt = m.now().UTC().Truncate(5 * time.Minute).Add(5 * time.Minute)
	p.Message = fmt.Sprintf("%d of %d clients available. Internet is restored on reboot; offline clients are never queued. Lab access remains available.", eligible, len(p.Targets))
	p.ReviewToken = domain.InternetReviewToken(p)
	return p
}
func (m *InternetManager) Apply(ctx context.Context, p domain.InternetPlan, token string) domain.InternetReport {
	r := domain.InternetReport{SchemaVersion: domain.SchemaVersion, Operation: "internet-apply", State: "blocked", Action: p.Action, Targets: []domain.InternetOutcome{}}
	fail := func(message string) domain.InternetReport { r.Message = message; return r }
	if p.HasErrors() || !p.Action.Valid() || token == "" || token != p.ReviewToken || token != domain.InternetReviewToken(p) || !m.now().Before(p.ExpiresAt) {
		return fail("Review expired or changed; create a fresh plan.")
	}
	lease, err := m.source.AcquireClientOperation()
	if err != nil {
		return fail(err.Error())
	}
	defer lease.Close()
	if m.conflict(ctx) {
		return fail("Network installation or recovery is active.")
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		return fail(err.Error())
	}
	identities := map[string]string{}
	for _, h := range meta.Clients.Hosts {
		identities[h.Name] = h.IP
	}
	for _, t := range p.Targets {
		if t.Name == meta.Controller.Name || identities[t.Name] != t.IP {
			return fail("Client inventory changed; review again.")
		}
	}
	confirmed := 0
	for _, t := range p.Targets {
		o := domain.InternetOutcome{Name: t.Name, State: "not-sent", Detail: "Unavailable in the reviewed plan; no request queued."}
		if t.Eligible {
			before := m.source.ObserveInternet(ctx, t.HostMeta)
			if !m.now().Before(p.ExpiresAt) || !before.Valid() || before.BootID != t.Observed.BootID || before.State != t.Observed.State {
				o.Detail = "Review expired, client rebooted or its state changed; review again."
			} else {
				var dispatchErr error
				if before.State != p.Action.DesiredState() {
					dispatchErr = m.source.ChangeInternet(ctx, t.HostMeta, p.Action, before.BootID)
				}
				after := m.source.ObserveInternet(ctx, t.HostMeta)
				if after.Valid() && after.BootID == before.BootID && after.State == p.Action.DesiredState() {
					o.State = "verified"
					o.Detail = "Internet " + after.State + "; reboot restores Internet."
					confirmed++
				} else {
					o.State = "unconfirmed"
					o.Detail = "State could not be confirmed. Refresh and review before retrying."
					if dispatchErr != nil {
						o.Detail += " " + dispatchErr.Error()
					}
				}
			}
		}
		r.Targets = append(r.Targets, o)
	}
	r.State = "partial"
	if confirmed == len(p.Targets) {
		r.State = "completed"
	}
	r.Message = fmt.Sprintf("Verified on %d of %d selected clients. No action is queued for offline computers.", confirmed, len(p.Targets))
	return r
}
