package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
)

// dashboardDeps holds the managers and values the dashboard callbacks capture.
// The wire<Area>Actions functions copy what they need into locals of the same
// name, so each callback keeps the body it had in runDashboardProgram.
type dashboardDeps struct {
	local                     adapters.Local
	inspector                 *app.Inspector
	setupManager              app.SetupManager
	lifecycle                 app.PXELifecycle
	deploymentManager         *app.DeploymentManager
	controllerManager         *app.ControllerManager
	configurationStateManager *app.ConfigurationStateManager
	serviceManager            *app.ServiceManager
	operationLogManager       *app.OperationLogManager
	supportManager            *app.SupportManager
	gitReviewManager          *app.GitReviewManager
	gitCommitManager          *app.GitCommitManager
	updateManager             *app.UpdateManager
	updateNotifications       *app.UpdateNotificationManager
	baseManager               *app.UpdateManager
	settingsManager           app.SettingsManager
	settingsSaveManager       app.SettingsSaveManager
	configurationSaveManager  app.ManagedConfigurationSaveManager
	updateSaveManager         app.UpdateSaveManager
	baseSaveManager           app.UpdateSaveManager
	softwareManager           app.SoftwareManager
	workspaceManager          app.WorkspaceManager
	homeFilesManager          app.HomeFilesManager
	workspaceMarketplace      app.WorkspaceMarketplace
	templateResetManager      app.TemplateResetManager
	softwareSaveManager       app.SoftwareSaveManager
	softwarePresetSaveManager app.SoftwarePresetSaveManager
	shutdownManager           *app.ShutdownManager
	wakeManager               *app.WakeManager
	cleanupManager            *app.CleanupManager
	internetManager           *app.InternetManager
	lockManager               *app.LockManager
	progressManager           app.OperationProgressManager
	ctx                       context.Context
	repository                string
	setupMode                 bool
}

// newDashboardDeps builds the managers the dashboard callbacks share.
func newDashboardDeps(ctx context.Context, repository string, setupMode bool) dashboardDeps {
	local := adapters.Local{}
	inspector := app.NewInspector(local)
	setupManager := app.NewSetupManager(local)
	lifecycle := app.NewPXELifecycle(local)
	deploymentManager := app.NewDeploymentManager(local)
	controllerManager := app.NewControllerManager(local)
	configurationStateManager := app.NewConfigurationStateManager(inspector, controllerManager)
	serviceManager := app.NewServiceManager(local)
	operationLogManager := app.NewOperationLogManager(local)
	supportManager := app.NewSupportManager(local)
	gitReviewManager := app.NewGitReviewManager(local)
	gitCommitManager := app.NewGitCommitManager(local)
	updateManager := app.NewUpdateManager(local)
	updateManager.RunningVersion = nixoriumVersion
	updateNotifications := app.NewUpdateNotificationManager(local)
	baseSource := adapters.PackageBase{}
	baseManager := app.NewPackageBaseManager(baseSource)
	settingsManager := app.NewSettingsManager(local).WithNetworkObserver(local)
	settingsSaveManager := app.NewSettingsSaveManager(settingsManager, gitReviewManager, gitCommitManager)
	configurationSaveManager := app.NewManagedConfigurationSaveManager(gitReviewManager, gitCommitManager)
	updateSaveManager := app.NewUpdateSaveManager(updateManager, local, gitReviewManager, configurationSaveManager)
	baseSaveManager := app.NewUpdateSaveManager(baseManager, baseSource, gitReviewManager, configurationSaveManager)
	softwareManager := app.NewSoftwareManager(local)
	workspaceManager := app.NewWorkspaceManager(local)
	homeFilesManager := app.NewHomeFilesManager(local)
	workspaceMarketplace := app.NewWorkspaceMarketplace(local)
	templateResetManager := app.NewTemplateResetManager(adapters.TemplateReset{})
	softwareSaveManager := app.NewSoftwareSaveManager(softwareManager, gitReviewManager, configurationSaveManager)
	softwarePresetSaveManager := app.NewSoftwarePresetSaveManager(softwareManager, gitReviewManager, configurationSaveManager)
	shutdownManager := app.NewShutdownManager(local)
	wakeManager := app.NewWakeManager(local)
	cleanupManager := app.NewCleanupManager(local)
	internetManager := app.NewInternetManager(local)
	lockManager := app.NewLockManager(lockSource{Local: local})
	progressManager := app.NewOperationProgressManager(local)
	return dashboardDeps{
		local:                     local,
		inspector:                 inspector,
		setupManager:              setupManager,
		lifecycle:                 lifecycle,
		deploymentManager:         deploymentManager,
		controllerManager:         controllerManager,
		configurationStateManager: configurationStateManager,
		serviceManager:            serviceManager,
		operationLogManager:       operationLogManager,
		supportManager:            supportManager,
		gitReviewManager:          gitReviewManager,
		gitCommitManager:          gitCommitManager,
		updateManager:             updateManager,
		updateNotifications:       updateNotifications,
		baseManager:               baseManager,
		settingsManager:           settingsManager,
		settingsSaveManager:       settingsSaveManager,
		configurationSaveManager:  configurationSaveManager,
		updateSaveManager:         updateSaveManager,
		baseSaveManager:           baseSaveManager,
		softwareManager:           softwareManager,
		workspaceManager:          workspaceManager,
		homeFilesManager:          homeFilesManager,
		workspaceMarketplace:      workspaceMarketplace,
		templateResetManager:      templateResetManager,
		softwareSaveManager:       softwareSaveManager,
		softwarePresetSaveManager: softwarePresetSaveManager,
		shutdownManager:           shutdownManager,
		wakeManager:               wakeManager,
		cleanupManager:            cleanupManager,
		internetManager:           internetManager,
		lockManager:               lockManager,
		progressManager:           progressManager,
		ctx:                       ctx,
		repository:                repository,
		setupMode:                 setupMode,
	}
}
