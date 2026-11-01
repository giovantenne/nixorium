package main

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireBackupActions wires backup and restore callbacks of the dashboard.
func wireBackupActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	setupManager := deps.setupManager
	ctx := deps.ctx
	repository := deps.repository
	actions.CreateBackup = func(destination string, passphrase []byte) domain.BackupReport {
		return local.CreateBackup(ctx, repository, destination, passphrase, nixoriumVersion)
	}
	actions.PlanGitBackup = func(readCtx context.Context, remote, branch string) (domain.GitBackupPlan, error) {
		return (adapters.GitBackup{}).Plan(readCtx, repository, remote, branch)
	}
	actions.PublishGitBackup = func(plan domain.GitBackupPlan, passphrase []byte) domain.BackupReport {
		return (adapters.GitBackup{}).Publish(ctx, plan, passphrase)
	}
	actions.RestoreLab = func(remote, branch, target string, passphrase []byte) domain.BackupReport {
		return (adapters.GitBackup{}).Restore(ctx, remote, branch, target, passphrase)
	}
	actions.GitBackupDestination = func() (string, string) { return (adapters.GitBackup{}).Destination(repository) }
	actions.BackupFileFolder = func() string {
		// A connected USB drive keeps the backup away from this controller.
		if current, err := user.Current(); err == nil {
			if drives := adapters.MountedDrives(current.Username); len(drives) > 0 {
				return drives[0]
			}
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return home
	}
	actions.SyncGitBackup = func(syncCtx context.Context, skip string) domain.BackupReport {
		return (adapters.GitBackup{}).Sync(syncCtx, repository, skip)
	}
	actions.RestoreLabArchive = func(source, target string, passphrase []byte) domain.BackupReport {
		return local.RestoreLab(ctx, source, passphrase, target)
	}
	actions.RestoreFolder = func() string { return adapters.RestoreFolder(repository) }
	actions.FindBackupFiles = func() []string {
		home, _ := os.UserHomeDir()
		name := ""
		if current, err := user.Current(); err == nil {
			name = current.Username
		}
		return adapters.FindBackupFiles(home, name)
	}
	actions.UseRestoredLab = func(restored domain.BackupReport, backupFile string) domain.BackupReport {
		// The controller installs the keys of its configured deployment;
		// using a restored lab elsewhere would install other keys.
		if fixed, err := os.ReadFile(remoteInstallDeploymentPathFile); err == nil && strings.TrimSpace(string(fixed)) != repository {
			message := "Nixorium is open on " + repository + ", but this controller manages " + strings.TrimSpace(string(fixed)) + ". Open Nixorium without --repo, then use the restored lab."
			return domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-use-restored", State: "blocked", Message: message, PrivateKeys: []string{}, Issues: []domain.ValidationIssue{{Field: "repository", Message: message}}}
		}
		adminKey := ""
		if home, err := os.UserHomeDir(); err == nil {
			adminKey = filepath.Join(home, ".ssh", "id_ed25519")
		}
		return app.RestoredLab{
			Source:     local,
			VerifyKeys: setupManager.VerifyKeys,
			InstallSecrets: func(installCtx context.Context) domain.ActionReport {
				return app.NewSystemActions(local).InstallSecrets(installCtx)
			},
		}.Use(ctx, repository, adminKey, backupFile, restored)
	}
}
