package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAssistantErrorJSONOmitsEmptyDetail(t *testing.T) {
	data, err := json.Marshal(&AssistantError{Code: "protocol_error"})
	if err != nil || strings.Contains(string(data), "detail") {
		t.Fatal(string(data), err)
	}
	data, err = json.Marshal(&AssistantError{Code: "service_interrupted", Detail: "stream ended before a terminal event"})
	if err != nil || !strings.Contains(string(data), `"detail":"stream ended before a terminal event"`) {
		t.Fatal(string(data), err)
	}
	var decoded AssistantError
	if json.Unmarshal(data, &decoded) != nil || decoded.Detail != "stream ended before a terminal event" || decoded.Code != "service_interrupted" {
		t.Fatalf("%+v", decoded)
	}
}
