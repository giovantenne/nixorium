package main

import (
	"context"
	"io"
	"os"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireSettingsActions wires first-run setup and laboratory settings callbacks of the dashboard.
func wireSettingsActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	setupManager := deps.setupManager
	settingsManager := deps.settingsManager
	settingsSaveManager := deps.settingsSaveManager
	configurationSaveManager := deps.configurationSaveManager
	ctx := deps.ctx
	repository := deps.repository
	actions.LoadSetup = func(ctx context.Context) domain.SetupReport {
		return setupManager.Status(ctx, repository)
	}
	actions.LoadSetupKeys = func(ctx context.Context) domain.KeyReconcileReport {
		return setupManager.VerifyKeys(ctx, repository)
	}
	actions.ReconcileSetupKeys = func() (domain.KeyReconcileReport, error) {
		return setupManager.ReconcileKeys(ctx, repository)
	}
	actions.ImportSetupKey = func(name, sourcePath string) (domain.KeyImportReport, error) {
		return setupManager.ImportKey(ctx, repository, name, sourcePath)
	}
	actions.SaveSetupConfiguration = func() domain.ConfigurationSaveReport {
		return configurationSaveManager.SaveChanged(ctx, repository, []string{
			"lab-settings.json",
			"keys/cache-public-key",
			"keys/admin-ssh.pub",
		})
	}
	actions.InstallSetupSecrets = func() domain.ActionReport {
		report := app.NewSystemActions(local).InstallSecrets(ctx)
		report.Message = operationRecordMessage(report.Message, report)
		return report
	}
	actions.SettingsRepairIssues = func() []domain.ValidationIssue {
		_, issues, _ := settingsManager.CurrentForEditing(repository)
		return issues
	}
	actions.LoadSettings = func(ctx context.Context) (domain.LabSettingsFile, error) {
		settings, _, err := settingsManager.CurrentForEditing(repository)
		if err != nil {
			return domain.LabSettingsFile{}, err
		}
		if settings.Lab.MasterDHCPIP == domain.MasterDHCPPlaceholder {
			staticAddress, _ := domain.ControllerStaticAddress(settings.Lab)
			settings = applyDetectedNetworkDefaults(settings, local.DetectNetworkDefaults(staticAddress))
		}
		return settings, nil
	}
	actions.PlanSettings = func(ctx context.Context, candidate domain.LabSettingsFile) domain.ConfigPlanReport {
		return settingsManager.PlanSettings(ctx, repository, candidate)
	}
	actions.LoadControllerAddresses = func(ctx context.Context, settings domain.LabSettingsFile) domain.ControllerAddresses {
		return settingsManager.ControllerAddresses(ctx, settings)
	}
	actions.SaveSettings = func(candidate domain.LabSettingsFile, plan domain.ConfigPlanReport) domain.ConfigurationSaveReport {
		return settingsSaveManager.Save(ctx, repository, candidate, plan)
	}
	actions.ChangePassword = func(account string, settings domain.LabSettingsFile, input *os.File, output io.Writer) (domain.LabSettingsFile, error) {
		if account == "all" {
			reader := presentation.TerminalSecretReader{Input: input, Output: output}
			if err := collectSetupCredentials(ctx, reader, local, output, &settings); err != nil {
				return settings, err
			}
			return settings, nil
		}
		return collectSettingsPassword(ctx, local, input, output, account, settings)
	}
}
