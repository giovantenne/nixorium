package app

import (
	"context"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/hostdiagnostics"
	"strings"
	"testing"
	"time"
)

type diagnosticSource struct {
	meta       domain.LabMeta
	calls      int
	repository string
}

func (s *diagnosticSource) LabMeta(_ context.Context, repo string) (domain.LabMeta, error) {
	s.repository = repo
	return s.meta, nil
}
func (s *diagnosticSource) HostDiagnostics(_ context.Context, host domain.HostMeta, controller bool, area string) (hostdiagnostics.Report, error) {
	s.calls++
	return hostdiagnostics.Report{SchemaVersion: 1, Area: area, Controller: controller, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Units: []hostdiagnostics.Unit{}, Journal: []hostdiagnostics.Entry{}, Space: []hostdiagnostics.Space{}, Unavailable: []string{"journal", "boot_id", "revision", "disk:/", "disk:/home", "disk:/nix/store", "service_state", "home_reset_pending"}}, nil
}
func TestAssistantDiagnosticsOnlyDeclaredIdentitiesAndRoleBoundAreas(t *testing.T) {
	s := &diagnosticSource{}
	s.meta.Controller.Name = "pc99"
	s.meta.Controller.StaticIP = "192.0.2.99"
	s.meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "192.0.2.1"}}
	d := AssistantDiagnostics{Source: s}
	for _, test := range []struct{ host, area, code string }{{"root@192.0.2.1", "system", "invalid_arguments"}, {"pc02", "system", "unknown_host"}, {"pc01", "cache", "unsupported_role"}, {"pc01", "service arbitrary", "invalid_arguments"}} {
		if got := d.Execute(t.Context(), "/declared", test.host, test.area); !strings.Contains(got, test.code) {
			t.Fatal(got)
		}
	}
	if s.calls != 0 {
		t.Fatal("rejected target observed")
	}
	if got := d.Execute(t.Context(), "/declared", "pc01", "system"); !strings.Contains(got, `"ok":true`) || !strings.Contains(got, `"unavailable":["journal"`) {
		t.Fatal(got)
	}
	if s.calls != 1 || s.repository != "/declared" {
		t.Fatal(s)
	}
	s.meta.Clients.Hosts = append(s.meta.Clients.Hosts, s.meta.Clients.Hosts[0])
	if got := d.Execute(t.Context(), "/declared", "pc01", "system"); !strings.Contains(got, "inventory_unavailable") {
		t.Fatal(got)
	}
}
