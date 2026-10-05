package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"github.com/giovantenne/nixorium/internal/domain"
	"time"
)

type TelemetrySource interface {
	TelemetryConfig() (domain.TelemetryConfig, string, error)
	TelemetryTransaction(context.Context, bool, bool, func(*domain.TelemetryState, func() error) (bool, error)) error
	SendTelemetry(context.Context, domain.TelemetryPayload) string
}
type TelemetryManager struct {
	source TelemetrySource
	now    func() time.Time
}

func NewTelemetryManager(source TelemetrySource) *TelemetryManager {
	return &TelemetryManager{source: source, now: time.Now}
}
func (m *TelemetryManager) Run(ctx context.Context, action string) (domain.TelemetryReport, error) {
	var report domain.TelemetryReport
	c, machine, err := m.source.TelemetryConfig()
	if err != nil {
		return report, errors.New("telemetry is unavailable; use the installed controller administrator account")
	}
	err = m.source.TelemetryTransaction(ctx, true, action == "disable", func(s *domain.TelemetryState, checkpoint func() error) (bool, error) {
		if s.Machine != "" && s.Machine != machine {
			*s = domain.TelemetryState{SchemaVersion: 1, Consent: "undecided"}
		}
		changed := false
		switch action {
		case "status", "preview":
		case "dismiss":
			s.Prompted = true
			changed = true
		case "enable":
			if s.Consent != "enabled" {
				secret := make([]byte, 32)
				if _, e := rand.Read(secret); e != nil {
					return false, e
				}
				*s = domain.TelemetryState{SchemaVersion: 1, Consent: "enabled", Prompted: true, Machine: machine, Secret: base64.RawURLEncoding.EncodeToString(secret)}
			}
			changed = true
		case "disable":
			*s = domain.TelemetryState{SchemaVersion: 1, Consent: "disabled", Prompted: true, Machine: machine}
			changed = true
		default:
			return false, errors.New("unknown telemetry action")
		}
		report = domain.TelemetryReport{Consent: s.Consent, Prompted: s.Prompted, Endpoint: domain.TelemetryEndpoint, Payload: domain.NewTelemetryPayload(c, *s, m.now()), LastAttempt: s.LastAttempt, LastSuccess: s.LastSuccess, Result: s.Result}
		return changed, nil
	})
	return report, err
}

// Send retains the consent lock through the bounded request. Disable waits for
// this request to finish, so no dispatch can begin after disable has returned.
func (m *TelemetryManager) Send(ctx context.Context) error {
	c, machine, err := m.source.TelemetryConfig()
	if err != nil {
		return err
	}
	return m.source.TelemetryTransaction(ctx, false, false, func(s *domain.TelemetryState, checkpoint func() error) (bool, error) {
		now := m.now().UTC()
		last, _ := time.Parse(time.RFC3339, s.LastAttempt)
		if s.Consent != "enabled" || s.Machine != machine || !last.IsZero() && last.UTC().Format("2006-01-02") == now.Format("2006-01-02") {
			return false, nil
		}
		s.LastAttempt = now.Format(time.RFC3339)
		s.Result = "unconfirmed"
		if err := checkpoint(); err != nil {
			return false, err
		}
		s.Result = m.source.SendTelemetry(ctx, domain.NewTelemetryPayload(c, *s, now))
		if s.Result == "sent" {
			s.LastSuccess = s.LastAttempt
		}
		return true, nil
	})
}
