package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeMarketplace struct {
	candidate domain.WorkspaceMarketplaceCandidate
	err       error
	ids       []string
}

func (f *fakeMarketplace) ResolveMarketplaceExtension(_ context.Context, _ string, id string) (domain.WorkspaceMarketplaceCandidate, error) {
	f.ids = append(f.ids, id)
	return f.candidate, f.err
}

func marketplacePin(publisher, name string) domain.WorkspaceMarketplaceExtension {
	version, hash := "1.0.0", "sha256-"+strings.Repeat("A", 43)+"="
	return domain.WorkspaceMarketplaceExtension{Publisher: &publisher, Name: &name, Version: &version, Hash: &hash}
}

func TestWorkspaceMarketplaceResolve(t *testing.T) {
	source := &fakeMarketplace{candidate: domain.WorkspaceMarketplaceCandidate{Entry: marketplacePin("Arduino", "Vscode-Arduino")}}
	manager := NewWorkspaceMarketplace(source)
	report := manager.Resolve(t.Context(), ".", "arduino.VSCODE-arduino")
	if report.HasErrors() || report.ID != "arduino.vscode-arduino" || strings.Join(source.ids, ",") != "arduino.vscode-arduino" {
		t.Fatalf("unexpected report %+v", report)
	}
	if report := manager.Resolve(t.Context(), ".", "not an id"); !report.HasErrors() || len(source.ids) != 1 {
		t.Fatal("invalid identifier reached the Marketplace")
	}
	source.candidate.Entry = marketplacePin("other", "extension")
	if report := manager.Resolve(t.Context(), ".", "arduino.vscode-arduino"); !report.HasErrors() {
		t.Fatal("accepted a result for another extension")
	}
	source.err = errors.New("offline")
	if report := manager.Resolve(t.Context(), ".", "arduino.vscode-arduino"); report.State != "failed" || !strings.Contains(report.Issues[0].Message, "offline") {
		t.Fatalf("failure not reported: %+v", report)
	}
}
