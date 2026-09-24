package domain

import (
	"strings"
	"testing"
)

func TestDecodeRemoteInstallRequestIsStrictAndBounded(t *testing.T) {
	valid := `{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"prepare","host":"pc01"}`
	request, err := DecodeRemoteInstallRequest([]byte(valid))
	if err != nil || request.Host != "pc01" {
		t.Fatalf("request=%+v error=%v", request, err)
	}
	invalid := []string{
		`{"schemaVersion":1,"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"prepare","host":"pc01"}`,
		valid + `{}`,
		`{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"shell"}`,
		`{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"status"}`,
		`{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"prepare","host":"../../etc"}`,
	}
	for _, value := range invalid {
		if _, err := DecodeRemoteInstallRequest([]byte(value)); err == nil {
			t.Fatalf("invalid request accepted: %s", value)
		}
	}
	if _, err := DecodeRemoteInstallRequest([]byte(strings.Repeat("x", RemoteInstallPlanMaxBytes+1))); err == nil {
		t.Fatal("oversized request accepted")
	}
}

func TestDecodeRemoteInstallSessionBindsNestedIdentities(t *testing.T) {
	session := `{"schemaVersion":1,"operationId":"0123456789abcdef0123456789abcdef","state":"prepared","plan":{},"tokenConsumed":false,"dispatchUncertain":false,"events":[]}`
	if _, err := DecodeRemoteInstallSession([]byte(session)); err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(session, `"plan":{}`, `"plan":{"operationId":"ffffffffffffffffffffffffffffffff"}`, 1)
	if _, err := DecodeRemoteInstallSession([]byte(changed)); err == nil {
		t.Fatal("mismatched nested plan identity was accepted")
	}
}
