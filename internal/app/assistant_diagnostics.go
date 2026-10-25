package app

import (
	"context"
	"encoding/json"
	"net"
	"regexp"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/hostdiagnostics"
)

type AssistantDiagnosticsSource interface {
	LabMeta(context.Context, string) (domain.LabMeta, error)
	HostDiagnostics(context.Context, domain.HostMeta, bool, string) (hostdiagnostics.Report, error)
}
type AssistantDiagnostics struct{ Source AssistantDiagnosticsSource }

var diagnosticHostName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,62}$`)

// Execute always reads fresh declared inventory. The candidate and caller's
// conversation metadata cannot select a different repository or SSH address.
func (diagnostics AssistantDiagnostics) Execute(ctx context.Context, repository, host, area string) string {
	failure := func(code string) string {
		data, _ := json.Marshal(map[string]any{"ok": false, "error": code})
		return string(data)
	}
	if !diagnosticHostName.MatchString(host) || !hostdiagnostics.ValidArea(area) {
		return failure("invalid_arguments")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if diagnostics.Source == nil || ctx.Err() != nil {
		return failure("diagnostics_unavailable")
	}
	meta, err := diagnostics.Source.LabMeta(ctx, repository)
	if err != nil || ctx.Err() != nil {
		return failure("inventory_unavailable")
	}
	if meta.DeploymentMode != "" && meta.DeploymentMode != "laboratory" && meta.DeploymentMode != "controller" || meta.DeploymentMode == "controller" && len(meta.Clients.Hosts) != 0 {
		return failure("inventory_unavailable")
	}
	selected := domain.HostMeta{}
	controller := false
	seen := map[string]bool{}
	all := append([]domain.HostMeta{{Name: meta.Controller.Name, IP: meta.Controller.StaticIP}}, meta.Clients.Hosts...)
	for index, declared := range all {
		if !diagnosticHostName.MatchString(declared.Name) || seen[declared.Name] || index != 0 && net.ParseIP(declared.IP).To4() == nil {
			return failure("inventory_unavailable")
		}
		seen[declared.Name] = true
		if declared.Name == host {
			selected = declared
			controller = index == 0
		}
	}
	if selected.Name == "" {
		return failure("unknown_host")
	}
	if !hostdiagnostics.Supported(area, controller) {
		return failure("unsupported_role")
	}
	report, err := diagnostics.Source.HostDiagnostics(ctx, selected, controller, area)
	if err != nil || ctx.Err() != nil {
		return failure("diagnostics_unavailable")
	}
	data, err := hostdiagnostics.Encode(report)
	if err != nil {
		return failure("diagnostics_unavailable")
	}
	// Revalidate even typed callback output before producing model-facing JSON.
	report, err = hostdiagnostics.Decode(data, area, controller)
	if err != nil {
		return failure("diagnostics_unavailable")
	}
	data, err = json.Marshal(struct {
		OK     bool                   `json:"ok"`
		Host   string                 `json:"host"`
		Report hostdiagnostics.Report `json:"diagnostics"`
	}{true, host, report})
	if err != nil || len(data) > hostdiagnostics.MaxBytes {
		return failure("diagnostics_unavailable")
	}
	return domain.RedactForModel(string(data))
}
