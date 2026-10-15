package app

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type assistantHostsInspector interface {
	Hosts(context.Context, string) (domain.HostsReport, error)
}
type assistantDeferredStatus interface {
	Status(context.Context, string) domain.DeferredUpdateStatus
}

// AssistantLabStatus exposes the same observations as the administrator TUI.
// It never applies configuration or runs the deferred-update queue.
type AssistantLabStatus struct {
	Inspector       assistantHostsInspector
	DeferredUpdates assistantDeferredStatus
}

type assistantObservedHost struct {
	Name                 string                    `json:"name"`
	Role                 string                    `json:"role"`
	IP                   string                    `json:"ip"`
	Reachability         domain.Reachability       `json:"reachability"`
	SSH                  domain.SSHAvailability    `json:"ssh"`
	Deployment           domain.DeploymentCurrency `json:"deployment"`
	CurrentRevision      string                    `json:"currentRevision"`
	LastSuccessfulUpdate string                    `json:"lastSuccessfulUpdate,omitempty"`
	Detail               string                    `json:"detail"`
	// Network is set during a change of laboratory network.
	Network domain.HostNetwork `json:"network,omitempty"`
}

var assistantStatusPrivateDetail = regexp.MustCompile(`(?:/[^\s"'<>]+|SHA256:[A-Za-z0-9+/=]+|(?:ssh-(?:rsa|ed25519)|ecdsa-sha2-\S+)\s+[A-Za-z0-9+/=]+)`)

func assistantStatusDetail(text string) string {
	text = domain.RedactForModel(text)
	text = assistantStatusPrivateDetail.ReplaceAllString(text, "[REDACTED]")
	// A diagnostic detail is a bounded line, never a multiline probe log.
	text = strings.SplitN(text, "\n", 2)[0]
	runes := []rune(text)
	return string(runes[:min(len(runes), 300)])
}

func (status AssistantLabStatus) Execute(ctx context.Context, repository string) string {
	const unavailable = `{"ok":false,"error":"status_unavailable"}`
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if status.Inspector == nil || status.DeferredUpdates == nil || ctx.Err() != nil {
		return unavailable
	}
	report, err := status.Inspector.Hosts(ctx, repository)
	if err != nil || ctx.Err() != nil {
		return unavailable
	}
	queue := status.DeferredUpdates.Status(ctx, repository)
	if queue.HasErrors() || queue.State == "failed" || ctx.Err() != nil {
		return unavailable
	}
	hosts := make([]assistantObservedHost, 0, len(report.Hosts))
	for _, host := range report.Hosts {
		observed := assistantObservedHost{
			Name: host.Name, Role: host.Role, IP: host.IP, Reachability: host.Reachability,
			SSH: host.SSH, Deployment: host.Deployment,
			CurrentRevision: host.CurrentRevision[:min(len(host.CurrentRevision), 12)],
			Detail:          assistantStatusDetail(host.Detail),
			Network:         host.Network,
		}
		if host.LastSuccessfulDeploy != nil {
			observed.LastSuccessfulUpdate = host.LastSuccessfulDeploy.VerifiedAt.UTC().Format(time.RFC3339)
		}
		hosts = append(hosts, observed)
	}
	pending := make([]string, 0, len(queue.Updates))
	for _, entry := range queue.Updates {
		pending = append(pending, entry.Host)
	}
	data, err := json.Marshal(struct {
		OK              bool                    `json:"ok"`
		GeneratedAt     string                  `json:"generatedAt"`
		DesiredRevision string                  `json:"desiredRevision"`
		Hosts           []assistantObservedHost `json:"hosts"`
		PendingUpdates  []string                `json:"pendingUpdates"`
	}{true, report.GeneratedAt.UTC().Format(time.RFC3339), report.DesiredRevision[:min(len(report.DesiredRevision), 12)], hosts, pending})
	if err != nil {
		return unavailable
	}
	return string(data)
}
