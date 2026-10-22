package app

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestAssistantMarketplaceReturnsAPinForTheProfile(t *testing.T) {
	source := &fakeMarketplace{candidate: domain.WorkspaceMarketplaceCandidate{
		Entry: marketplacePin("Arduino", "Vscode-Arduino"), DisplayName: "Arduino",
		Engine: "^1.90.0", EditorVersion: "1.105.0", Dependencies: []string{"ms-vscode.cpptools"}, Pack: []string{}, Native: true,
	}}
	tool := AssistantMarketplace{Resolver: NewWorkspaceMarketplace(source)}
	var result struct {
		OK           bool
		Extension    string
		Pin          domain.WorkspaceMarketplaceExtension
		Dependencies []string
		Native       bool
	}
	if err := json.Unmarshal([]byte(tool.Execute(t.Context(), ".", "Arduino.vscode-arduino")), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Extension != "arduino.vscode-arduino" || result.Pin.ID() != "arduino.vscode-arduino" || *result.Pin.Version != "1.0.0" || len(result.Dependencies) != 1 || !result.Native {
		t.Fatalf("result = %+v", result)
	}

	source.err = errors.New("no compatible version")
	var failed struct {
		OK            bool
		Error, Detail string
	}
	if err := json.Unmarshal([]byte(tool.Execute(t.Context(), ".", "arduino.vscode-arduino")), &failed); err != nil {
		t.Fatal(err)
	}
	if failed.OK || failed.Error != "not_resolved" || failed.Detail != "no compatible version" {
		t.Fatalf("failure = %+v", failed)
	}
}
