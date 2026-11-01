package app

import (
	"context"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// RestoredLabSource moves folders and records backups for UseRestoredLab.
type RestoredLabSource interface {
	AdoptRestoredLab(repository, restored, adminKey string, now time.Time) (string, error)
	RecordRestoredBackup(repository, source string, from domain.BackupReport)
}

// RestoredLab makes a lab restored by Restore lab the one this controller
// manages, the step a technician needs on a new or replaced controller:
// check the restored keys, put the restored folder in place of the current
// one (kept aside, never deleted) and install its keys on the controller.
// Nothing is built or activated; applying the controller is the next,
// separately reviewed step.
type RestoredLab struct {
	Source         RestoredLabSource
	VerifyKeys     func(context.Context, string) domain.KeyReconcileReport
	InstallSecrets func(context.Context) domain.ActionReport
	Now            func() time.Time
}

func (r RestoredLab) Use(ctx context.Context, repository, adminKey, backupFile string, restored domain.BackupReport) domain.BackupReport {
	report := domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-use-restored", State: "blocked", Path: repository, Revision: restored.Revision, PrivateKeys: []string{}, Issues: []domain.ValidationIssue{}}
	block := func(message string) domain.BackupReport {
		report.Message = message
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "restore", Message: message})
		return report
	}
	if restored.HasErrors() || restored.Path == "" {
		return block("Restore the backup first; nothing was changed.")
	}
	if keys := r.VerifyKeys(ctx, restored.Path); keys.State != "ready" {
		return block("The keys in the restored lab do not match its configuration, so it was not put in use. Nothing was changed; restore the backup again.")
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	previous, err := r.Source.AdoptRestoredLab(repository, restored.Path, adminKey, now())
	report.Previous = previous
	if err != nil {
		return block(upperFirst(err.Error()) + ".")
	}
	r.Source.RecordRestoredBackup(repository, backupFile, restored)
	if install := r.InstallSecrets(ctx); install.HasErrors() {
		report.State = "partial"
		report.Message = "The restored lab is now the one this controller manages, but its keys could not be installed on the controller (" + install.Message + "). Install them again before applying the controller: Enter here, or the command nixorium setup install-secrets."
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "keys", Message: install.Message})
		return report
	}
	report.State = "completed"
	report.Message = "This controller now manages the restored lab and has its keys."
	return report
}
