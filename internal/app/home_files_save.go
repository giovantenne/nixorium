package app

import (
	"context"
	"github.com/giovantenne/nixorium/internal/domain"
)

type HomeFilesSaveSource interface {
	HomeFilesSource
	RecordHomeFilesCandidate(context.Context, string, string, domain.HomeFilesDraft) (string, error)
}
type HomeFilesSaveManager struct {
	source HomeFilesSaveSource
	review *GitReviewManager
}

func NewHomeFilesSaveManager(source HomeFilesSaveSource, review *GitReviewManager) HomeFilesSaveManager {
	return HomeFilesSaveManager{source, review}
}
func (m HomeFilesSaveManager) Save(ctx context.Context, repository string, draft domain.HomeFilesDraft, token string) domain.HomeFilesApplyReport {
	check := m.review.Review(ctx, repository)
	if check.HasErrors() {
		return domain.HomeFilesApplyReport{SchemaVersion: 1, State: "failed", Issues: []domain.ValidationIssue{{Field: "storage", Message: "Inspect Git review before saving initial files; CLI: nixorium git review."}}}
	}
	for _, c := range check.Changes {
		if c.Path == domain.HomeFilesFileName || len(c.Path) > len(domain.HomeFilesDirectoryName) && c.Path[:len(domain.HomeFilesDirectoryName)+1] == domain.HomeFilesDirectoryName+"/" {
			return domain.HomeFilesApplyReport{SchemaVersion: 1, State: "conflict", Issues: []domain.ValidationIssue{{Field: "storage", Message: "Initial files already have local changes; inspect Git review before saving."}}}
		}
	}
	plan := NewHomeFilesManager(m.source).Plan(ctx, repository, draft)
	if plan.HasErrors() || plan.Snapshot == nil {
		return domain.HomeFilesApplyReport{SchemaVersion: 1, State: plan.State, Issues: plan.Issues, Message: plan.Message}
	}
	r := NewHomeFilesManager(m.source).Apply(ctx, repository, draft, token)
	r.Operation = "home-files-save"
	if r.HasErrors() || r.State != "saved" {
		return r
	}
	revision, err := m.source.RecordHomeFilesCandidate(ctx, repository, plan.Snapshot.Revision, draft)
	if err != nil {
		r.State = "partial"
		r.RecoveryRequired = true
		r.Issues = append(r.Issues, domain.ValidationIssue{Field: "record", Message: "Files were saved but their local commit is unconfirmed."})
		r.Message = "Inspect Maintenance → Git review (CLI: nixorium git review); do not replay this save. No system was applied."
		return r
	}
	r.Recorded = true
	r.Revision = revision
	r.Message = "Initial files saved and recorded locally. Apply the controller and clients separately; the baseline takes effect at their next normal boot."
	return r
}
