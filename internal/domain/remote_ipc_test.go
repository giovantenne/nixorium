package domain

import (
	"strings"
	"testing"
)

func TestDecodeRemoteInstallRequestIsStrictAndBounded(t *testing.T) {
	valid := `{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"prepare","operationId":"abcdefabcdefabcdefabcdefabcdefab","host":"pc01"}`
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

func TestDecodeRemoteInstallPrepareAllowsTargetIndependentStart(t *testing.T) {
	request, err := DecodeRemoteInstallRequest([]byte(`{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","operation":"prepare","host":"pc01"}`))
	if err != nil || request.OperationID != "" || request.Host != "pc01" {
		t.Fatalf("target-independent prepare request=%+v error=%v", request, err)
	}
}

func TestDecodeRemoteInstallRequestValidatesReviewFields(t *testing.T) {
	operationID := "abcdefabcdefabcdefabcdefabcdefab"
	requestID := "0123456789abcdef0123456789abcdef"
	valid := []string{
		`{"schemaVersion":1,"requestId":"` + requestID + `","operation":"plan","operationId":"` + operationID + `","disk":"/dev/sda"}`,
		`{"schemaVersion":1,"requestId":"` + requestID + `","operation":"plan","operationId":"` + operationID + `","disk":"/dev/sda","hostKeyRotation":true}`,
		`{"schemaVersion":1,"requestId":"` + requestID + `","operation":"apply","operationId":"` + operationID + `","reviewToken":"sha256:` + strings.Repeat("a", 64) + `","confirmation":"ERASE pc01 /dev/sda"}`,
	}
	for _, value := range valid {
		if _, err := DecodeRemoteInstallRequest([]byte(value)); err != nil {
			t.Fatalf("valid request rejected: %v", err)
		}
	}
	invalid := []string{
		strings.Replace(valid[0], `"disk":"/dev/sda"`, `"disk":"../../dev/sda"`, 1),
		strings.Replace(valid[0], `"disk":"/dev/sda"`, `"disk":"/dev/sda","reviewToken":"sha256:`+strings.Repeat("a", 64)+`"`, 1),
		strings.Replace(valid[2], `"confirmation":"ERASE pc01 /dev/sda"`, `"confirmation":""`, 1),
		strings.Replace(valid[2], `"reviewToken":"sha256:`+strings.Repeat("a", 64)+`"`, `"reviewToken":"sha256:short"`, 1),
		strings.Replace(valid[2], `"confirmation":"ERASE pc01 /dev/sda"`, `"hostKeyRotation":true,"confirmation":"ERASE pc01 /dev/sda"`, 1),
	}
	for _, value := range invalid {
		if _, err := DecodeRemoteInstallRequest([]byte(value)); err == nil {
			t.Fatalf("invalid review request accepted: %s", value)
		}
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
	badLog := strings.Replace(session, `"state":"prepared"`, `"logId":"../operation.log","state":"prepared"`, 1)
	if _, err := DecodeRemoteInstallSession([]byte(badLog)); err == nil {
		t.Fatal("invalid log identity was accepted")
	}
}

func TestDecodeRemoteInstallResponseValidatesNestedSession(t *testing.T) {
	response := `{"schemaVersion":1,"requestId":"0123456789abcdef0123456789abcdef","state":"prepared","operationId":"abcdefabcdefabcdefabcdefabcdefab","session":{"schemaVersion":1,"operationId":"abcdefabcdefabcdefabcdefabcdefab","logId":"../operation.log","state":"prepared","plan":{},"tokenConsumed":false,"dispatchUncertain":false,"events":[]}}`
	if _, err := DecodeRemoteInstallResponse([]byte(response)); err == nil {
		t.Fatal("response with invalid nested session was accepted")
	}
}
