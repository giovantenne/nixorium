package app

import (
	"context"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/domain"
)

// WorkspaceMarketplaceSource downloads one Marketplace version into the Nix
// store and inspects it. It must not write deployment files.
type WorkspaceMarketplaceSource interface {
	ResolveMarketplaceExtension(context.Context, string, string) (domain.WorkspaceMarketplaceCandidate, error)
}

type WorkspaceMarketplace struct{ source WorkspaceMarketplaceSource }

func NewWorkspaceMarketplace(source WorkspaceMarketplaceSource) WorkspaceMarketplace {
	return WorkspaceMarketplace{source: source}
}

// Resolve proposes a pin for the profile draft. Saving still requires the
// ordinary workspace review; nothing is selected, committed or deployed here.
func (m WorkspaceMarketplace) Resolve(ctx context.Context, repository, id string) domain.WorkspaceMarketplaceReport {
	report := domain.WorkspaceMarketplaceReport{
		SchemaVersion: domain.WorkspaceSchemaVersion, Operation: "workspace-marketplace", State: "invalid",
		ID: id, Issues: []domain.ValidationIssue{},
	}
	normalized, err := domain.NormalizeWorkspaceMarketplaceID(id)
	if err != nil {
		return workspaceMarketplaceIssue(report, "id", err)
	}
	report.ID = normalized
	root, err := filepath.Abs(repository)
	if err != nil {
		return workspaceMarketplaceIssue(report, "repository", err)
	}
	candidate, err := m.source.ResolveMarketplaceExtension(ctx, root, normalized)
	if err != nil {
		report.State = "failed"
		return workspaceMarketplaceIssue(report, "marketplace", err)
	}
	if candidate.Entry.Issue() != "" || candidate.Entry.ID() != normalized {
		report.State = "failed"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "marketplace", Message: "the Marketplace result does not match the requested extension"})
		report.Message = "No Marketplace extension was added."
		return report
	}
	report.State, report.Candidate = "ready", &candidate
	report.Message = "Downloaded into the controller's Nix store only. Add it to the draft, then review and save the profile."
	return report
}

func workspaceMarketplaceIssue(report domain.WorkspaceMarketplaceReport, field string, err error) domain.WorkspaceMarketplaceReport {
	report.Issues = append(report.Issues, domain.ValidationIssue{Field: field, Message: err.Error()})
	report.Message = "No Marketplace extension was added."
	return report
}
