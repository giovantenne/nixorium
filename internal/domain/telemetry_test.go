package domain

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTelemetryMonthlyIdentityAndVersion(t *testing.T) {
	c := TelemetryConfig{1, "3.0.0-beta.1", "laboratory", "16-30"}
	s := TelemetryState{SchemaVersion: 1, Consent: "enabled", Machine: strings.Repeat("a", 32), Secret: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	a := NewTelemetryPayload(c, s, now)
	fixture, err := os.ReadFile("testdata/telemetry-payload.json")
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := json.Marshal(a)
	if strings.TrimSpace(string(fixture)) != string(actual) {
		t.Fatal("wire fixture differs", string(actual))
	}
	b := NewTelemetryPayload(c, s, now.Add(24*time.Hour))
	next := NewTelemetryPayload(c, s, now.AddDate(0, 1, 0))
	if a.MonthlyID != b.MonthlyID || a.MonthlyID == next.MonthlyID || len(a.MonthlyID) != 43 {
		t.Fatal("rotation failed")
	}
	if TelemetryVersion("3.0.0-private-school") != "unknown" {
		t.Fatal("free text leaked")
	}
	if a.ClientBootVerified != nil {
		t.Fatal("missing evidence misrepresented")
	}
}
