package app

import (
	"context"
	"encoding/json"

	"github.com/giovantenne/nixorium/internal/domain"
)

// AssistantMarketplace is the vscode_marketplace tool: it resolves one VS Code
// extension in the public Marketplace for the deployment's pinned editor,
// downloads it into the controller's Nix store and returns the pin for
// vscode.marketplace. It writes no deployment file; Assistant adds the pin and
// the extension to workspace-profile.json in its candidate.
type AssistantMarketplace struct{ Resolver WorkspaceMarketplace }

func (tool AssistantMarketplace) Execute(ctx context.Context, repository, extension string) string {
	report := tool.Resolver.Resolve(ctx, repository, extension)
	if report.State != "ready" || report.Candidate == nil {
		detail := report.Message
		if len(report.Issues) > 0 {
			detail = report.Issues[0].Message
		}
		data, _ := json.Marshal(map[string]any{"ok": false, "error": "not_resolved", "extension": report.ID, "detail": detail})
		return string(data)
	}
	candidate := report.Candidate
	data, _ := json.Marshal(struct {
		OK            bool                                 `json:"ok"`
		Extension     string                               `json:"extension"`
		Pin           domain.WorkspaceMarketplaceExtension `json:"pin"`
		DisplayName   string                               `json:"displayName"`
		Description   string                               `json:"description"`
		Engine        string                               `json:"engine"`
		EditorVersion string                               `json:"editorVersion"`
		Dependencies  []string                             `json:"dependencies"`
		Pack          []string                             `json:"pack"`
		Native        bool                                 `json:"native"`
	}{true, report.ID, candidate.Entry, candidate.DisplayName, candidate.Description, candidate.Engine, candidate.EditorVersion, candidate.Dependencies, candidate.Pack, candidate.Native})
	return string(data)
}
