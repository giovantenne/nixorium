package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"time"
)

const TelemetryEndpoint = "https://telemetry.nixorium.org/v1/heartbeat"
const TelemetryNotice = "Optional daily adoption statistics to telemetry.nixorium.org: monthly pseudonym, installed version, mode, configured client-size band and historical verified client boot. No hostnames, IPs, users, files, configuration or logs in the payload. Cloudflare receives the connection IP. Raw records: 90-day retention target plus provider recovery copies; monthly aggregates: 24 months. Disable stops future sends, not previously received records."

type TelemetryPayload struct {
	SchemaVersion      int    `json:"schemaVersion"`
	Month              string `json:"month"`
	MonthlyID          string `json:"monthlyId"`
	Version            string `json:"version"`
	DeploymentMode     string `json:"deploymentMode"`
	ConfiguredClients  string `json:"configuredClients"`
	ClientBootVerified *bool  `json:"clientBootVerified"`
}
type TelemetryConfig struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Version           string `json:"version"`
	DeploymentMode    string `json:"deploymentMode"`
	ConfiguredClients string `json:"configuredClients"`
}

// TelemetryState is private controller state, never an exportable report.
type TelemetryState struct {
	SchemaVersion int    `json:"schemaVersion"`
	Consent       string `json:"consent"`
	Prompted      bool   `json:"prompted"`
	Secret        string `json:"secret,omitempty"`
	Machine       string `json:"machine,omitempty"`
	BootVerified  bool   `json:"bootVerified"`
	LastAttempt   string `json:"lastAttempt,omitempty"`
	LastSuccess   string `json:"lastSuccess,omitempty"`
	Result        string `json:"result,omitempty"`
}
type TelemetryReport struct {
	Consent     string           `json:"consent"`
	Prompted    bool             `json:"prompted"`
	Endpoint    string           `json:"endpoint"`
	Payload     TelemetryPayload `json:"payload"`
	LastAttempt string           `json:"lastAttempt,omitempty"`
	LastSuccess string           `json:"lastSuccess,omitempty"`
	Result      string           `json:"result,omitempty"`
}

var telemetryVersion = regexp.MustCompile(`^(unknown|[0-9]+\.[0-9]+\.[0-9]+(?:-(?:alpha|beta|rc)\.[0-9]+)?)$`)

func TelemetryVersion(v string) string {
	if len(v) <= 48 && telemetryVersion.MatchString(v) {
		return v
	}
	return "unknown"
}
func DecodeTelemetry(data []byte, value any) error { return decodeRemoteJSON(data, 8192, value) }
func (c TelemetryConfig) Valid() bool {
	if c.SchemaVersion != 1 || c.Version != TelemetryVersion(c.Version) {
		return false
	}
	if c.DeploymentMode != "controller" && c.DeploymentMode != "laboratory" && c.DeploymentMode != "unknown" {
		return false
	}
	switch c.ConfiguredClients {
	case "0", "1-5", "6-15", "16-30", "31-60", "61+", "unknown":
		return true
	}
	return false
}
func (s TelemetryState) Validate() error {
	if s.SchemaVersion != 1 || (s.Consent != "undecided" && s.Consent != "enabled" && s.Consent != "disabled") {
		return errors.New("invalid telemetry state")
	}
	if s.Consent == "enabled" {
		b, e := base64.RawURLEncoding.DecodeString(s.Secret)
		if e != nil || len(b) != 32 || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(s.Machine) {
			return errors.New("invalid telemetry identity")
		}
	} else if s.Secret != "" {
		return errors.New("unexpected telemetry identity")
	}
	for _, t := range []string{s.LastAttempt, s.LastSuccess} {
		if t != "" {
			if _, e := time.Parse(time.RFC3339, t); e != nil {
				return errors.New("invalid telemetry time")
			}
		}
	}
	switch s.Result {
	case "", "sent", "unconfirmed", "network-unavailable", "server-unavailable", "rejected", "rate-limited":
	default:
		return errors.New("invalid telemetry result")
	}
	return nil
}
func NewTelemetryPayload(c TelemetryConfig, s TelemetryState, now time.Time) TelemetryPayload {
	month := now.UTC().Format("2006-01")
	p := TelemetryPayload{1, month, "<generated after consent>", TelemetryVersion(c.Version), c.DeploymentMode, c.ConfiguredClients, nil}
	if s.Consent == "enabled" {
		secret, _ := base64.RawURLEncoding.DecodeString(s.Secret)
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte("nixorium-telemetry:v1:" + month))
		p.MonthlyID = base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	if s.BootVerified {
		v := true
		p.ClientBootVerified = &v
	}
	return p
}
