package adapters

import "testing"

func TestInternetObservationRejectsUntrustedOrOldHelperResponses(t *testing.T) {
	valid := `{"schemaVersion":1,"bootId":"11111111-1111-4111-8111-111111111111","state":"enabled"}`
	if !decodeInternetObservation([]byte(valid)).Valid() {
		t.Fatal("valid response rejected")
	}
	for _, input := range []string{valid + `{}`, `{"state":"enabled"}`, `{"schemaVersion":2,"bootId":"11111111-1111-4111-8111-111111111111","state":"blocked"}`, `{"schemaVersion":1,"bootId":"; touch /tmp/bad","state":"enabled"}`, `{"schemaVersion":1,"bootId":"11111111-1111-4111-8111-111111111111","state":"enabled","unexpected":true}`} {
		if decodeInternetObservation([]byte(input)).Valid() {
			t.Fatalf("accepted %q", input)
		}
	}
}
