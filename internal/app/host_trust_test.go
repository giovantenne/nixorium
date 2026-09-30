package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeHostTrustSource struct {
	meta                 domain.LabMeta
	revision             string
	inspection           domain.HostTrustInspection
	gateErr, replaceErr  error
	gitErr               error
	dirty                bool
	pxeActive            bool
	writes, observations int
}

func (f *fakeHostTrustSource) GitState(context.Context, string) (domain.GitState, error) {
	return domain.GitState{Available: true, Dirty: f.dirty}, f.gitErr
}

func (f *fakeHostTrustSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	return f.meta, nil
}
func (f *fakeHostTrustSource) GitRevision(context.Context, string) (string, error) {
	return f.revision, nil
}
func (f *fakeHostTrustSource) ObserveHostTrust(context.Context, domain.HostMeta) (domain.HostTrustInspection, error) {
	f.observations++
	return f.inspection, nil
}
func (f *fakeHostTrustSource) ReplaceHostTrust(context.Context, domain.HostMeta, domain.HostTrustInspection) error {
	f.writes++
	return f.replaceErr
}
func (f *fakeHostTrustSource) AcquireClientOperation() (io.Closer, error) {
	return shutdownLease{}, f.gateErr
}
func (f *fakeHostTrustSource) ServiceState(context.Context, string) domain.ServiceState {
	return domain.ServiceState{Loaded: true, Active: f.pxeActive, State: "inactive"}
}
func hostTrustFixture() *fakeHostTrustSource {
	f := &fakeHostTrustSource{revision: strings.Repeat("a", 40), inspection: domain.HostTrustInspection{Recorded: []string{"SHA256:old"}, Offered: "SHA256:new", PublicKey: "public", BaseFingerprint: "base"}}
	f.meta.Controller.Name, f.meta.Controller.StaticIP = "pc99", "10.0.0.99"
	f.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}, {Name: "pc02", IP: "10.0.0.2"}}
	return f
}
func TestHostTrustReviewAndSingleClientApply(t *testing.T) {
	f := hostTrustFixture()
	m := NewHostTrustManager(f)
	p := m.Plan(context.Background(), "/deployment", "pc01")
	if p.HasErrors() || p.Confirmation != "ROTATE HOST KEY" || p.ReviewToken == "" || f.writes != 0 {
		t.Fatalf("plan: %+v", p)
	}
	if r := m.Apply(context.Background(), p, p.ReviewToken); r.HasErrors() || f.writes != 1 {
		t.Fatalf("apply: %+v writes=%d", r, f.writes)
	}
}
func TestHostTrustRejectsStaleOrUnapprovedReviews(t *testing.T) {
	for _, change := range []string{"revision", "offered", "recorded", "base", "address", "controller", "expired", "confirmation", "token", "gate", "pxe", "dirty", "pending-reset"} {
		t.Run(change, func(t *testing.T) {
			f := hostTrustFixture()
			m := NewHostTrustManager(f)
			p := m.Plan(context.Background(), "/deployment", "pc01")
			token := p.ReviewToken
			switch change {
			case "revision":
				f.revision = strings.Repeat("b", 40)
			case "offered":
				f.inspection.Offered = "SHA256:other"
			case "recorded":
				f.inspection.Recorded = []string{"SHA256:other"}
			case "base":
				f.inspection.BaseFingerprint = "changed"
			case "address":
				f.meta.Clients.Hosts[0].IP = "10.0.0.3"
			case "controller":
				f.meta.Controller.Name = "pc01"
			case "expired":
				p.ExpiresAt = time.Now().Add(-time.Second)
			case "confirmation":
				p.Confirmation = "yes"
			case "token":
				token = "changed"
			case "gate":
				f.gateErr = errors.New("reserved")
			case "pxe":
				f.pxeActive = true
			case "dirty":
				f.dirty = true
			case "pending-reset":
				f.gitErr = errors.New("reset recovery required")
			}
			if r := m.Apply(context.Background(), p, token); !r.HasErrors() || f.writes != 0 {
				t.Fatalf("unsafe apply: %+v writes=%d", r, f.writes)
			}
		})
	}
}
func TestHostTrustRefusesUnknownMatchingAndAmbiguousIdentities(t *testing.T) {
	for _, condition := range []string{"controller", "unknown", "duplicate-name", "duplicate-address", "controller-address", "invalid-address", "same-key", "no-record"} {
		t.Run(condition, func(t *testing.T) {
			f := hostTrustFixture()
			name := "pc01"
			switch condition {
			case "controller":
				name = "pc99"
			case "unknown":
				name = "pc03"
			case "duplicate-name":
				f.meta.Clients.Hosts[1].Name = "pc01"
			case "duplicate-address":
				f.meta.Clients.Hosts[1].IP = "10.0.0.1"
			case "controller-address":
				f.meta.Controller.StaticIP = "10.0.0.1"
			case "invalid-address":
				f.meta.Clients.Hosts[0].IP = "localhost"
			case "same-key":
				f.inspection.Offered = "SHA256:old"
			case "no-record":
				f.inspection.Recorded = nil
			}
			if p := NewHostTrustManager(f).Plan(context.Background(), "/deployment", name); !p.HasErrors() || f.writes != 0 {
				t.Fatalf("unsafe plan: %+v", p)
			}
			if condition != "same-key" && condition != "no-record" && f.observations != 0 {
				t.Fatal("connected before verifying inventory")
			}
		})
	}
}
func TestHostTrustReportsUnconfirmedWriteAndSafeHistory(t *testing.T) {
	f := hostTrustFixture()
	f.replaceErr = errors.New("private raw error")
	m := NewHostTrustManager(f)
	p := m.Plan(context.Background(), "/deployment", "pc01")
	r := m.Apply(context.Background(), p, p.ReviewToken)
	if r.State != "unconfirmed" {
		t.Fatalf("result: %+v", r)
	}
	record, ok := operationRecordFor(r)
	if !ok || record.Subject != "pc01" || strings.Contains(record.Summary, "private raw error") {
		t.Fatalf("unsafe history: %+v", record)
	}
}

func TestHostTrustTokenCannotRenewAnExpiredReview(t *testing.T) {
	f := hostTrustFixture()
	m := NewHostTrustManager(f)
	now := time.Unix(1000, 0)
	m.now = func() time.Time { return now }
	p := m.Plan(context.Background(), "/deployment", "pc01")
	now = now.Add(time.Second)
	if next := m.Plan(context.Background(), "/deployment", "pc01"); next.ReviewToken != p.ReviewToken {
		t.Fatal("immediate CLI review did not match")
	}
	now = p.ExpiresAt
	next := m.Plan(context.Background(), "/deployment", "pc01")
	if next.ReviewToken == p.ReviewToken {
		t.Fatal("expired token was renewed")
	}
	p.ExpiresAt = now.Add(time.Hour)
	if r := m.Apply(context.Background(), p, p.ReviewToken); !r.HasErrors() || f.writes != 0 {
		t.Fatal("altered expiry accepted an old token")
	}
}
