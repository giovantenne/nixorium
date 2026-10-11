package domain

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var updateAssistantTools = flag.Bool("update-assistant-tools", false, "Update generated Assistant tool schemas")

func TestAssistantToolSchemasGenerated(t *testing.T) {
	data, err := json.MarshalIndent(AssistantToolsV1(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := "../../docs/assistant-tools-v1.json"
	if *updateAssistantTools {
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	existing, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, existing) {
		t.Fatal("Assistant schemas differ; run go test ./internal/domain -update-assistant-tools")
	}
}
func TestAssistantToolArguments(t *testing.T) {
	for _, test := range []struct {
		name, args string
		valid      bool
	}{
		{"read_file", `{"path":"flake.nix"}`, true}, {"read_file", `{"path":"flake.nix","limit":65536}`, true}, {"read_file", `{"path":"flake.nix","offset":-1}`, false}, {"read_file", `{"path":"flake.nix","limit":65537}`, false},
		{"write_file", `{"path":"x","content":""}`, true}, {"write_file", `{"path":"x","content":"x","executable":"true"}`, false}, {"list_files", `{}`, true}, {"evaluate", `{}`, true}, {"evaluate", `{"command":"bash"}`, false}, {"bash", `{}`, false},
		{"read_file", `{"path":"a","path":"b"}`, false}, {"read_file", `{"path":"a","\u0070ath":"b"}`, false}, {"read_file", `{"path":null}`, false}, {"read_file", `{"path":"x","offset":1.5}`, false},
		{"build", `{}`, true}, {"build", `{"command":"bash"}`, false},
		{"lab_status", `{}`, true}, {"lab_status", `{"command":"bash"}`, false},
	} {
		if got := ValidateAssistantArguments(test.name, []byte(test.args)); got != test.valid {
			t.Fatalf("%s %s: %v", test.name, test.args, got)
		}
	}
}
