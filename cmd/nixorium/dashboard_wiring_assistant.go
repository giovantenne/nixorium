package main

import (
	"context"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/presentation"
)

// wireAssistantActions wires the Assistant conversation, account and workspace callbacks of the dashboard.
func wireAssistantActions(actions *presentation.DashboardActions, deps dashboardDeps) {
	local := deps.local
	inspector := deps.inspector
	repository := deps.repository
	assistantConfig, _ := adapters.LoadAssistantConfig("/etc/nixorium/assistant.json")
	assistantClient := adapters.AssistantClient{Endpoint: assistantConfig.Endpoint, Version: nixoriumVersion}
	assistantStore := adapters.AssistantStateStore{Directory: "/var/lib/nixorium/assistant"}
	assistantWorkspace := adapters.AssistantWorkspaceStore{Directory: assistantStore.Directory, Version: nixoriumVersion}
	assistantReporter := app.AssistantConversationReporter{Client: assistantClient, Store: assistantWorkspace, Version: nixoriumVersion}
	assistantLabStatus := app.AssistantLabStatus{Inspector: inspector, DeferredUpdates: newDeferredUpdateManager()}
	assistantDiagnostics := app.AssistantDiagnostics{Source: local}
	assistantSystemFiles := app.AssistantSystemFiles{Source: local, Errors: map[error]string{
		adapters.ErrSystemFileRefused: "refused_path", adapters.ErrSystemFileMissing: "not_found",
		adapters.ErrSystemFileUnreadable: "not_readable", adapters.ErrSystemFileOldComputer: "computer_needs_update",
	}}
	assistantRunner := app.AssistantTurnRunner{Client: assistantClient, Tools: adapters.AssistantWorkspaceTools{Store: assistantWorkspace, LabStatus: assistantLabStatus.Execute, Diagnostics: assistantDiagnostics.Execute, SystemFiles: assistantSystemFiles.Execute, Marketplace: app.AssistantMarketplace{Resolver: app.NewWorkspaceMarketplace(local)}.Execute}, Store: assistantWorkspace, Version: nixoriumVersion, Sharing: &assistantReporter}
	actions.Assistant = assistantConfig
	actions.AssistantService = &presentation.AssistantActions{
		ReadToken: assistantStore.ReadToken, WriteToken: assistantStore.WriteToken,
		DeleteToken: assistantStore.DeleteToken, ReadConsent: assistantStore.ReadConsent,
		WriteConsent: assistantStore.WriteConsent, WriteConsentSharing: assistantStore.WriteConsentSharing,
		ReadSharing: assistantStore.ReadSharing, WriteSharing: assistantStore.WriteSharing, Report: assistantReporter.Report, Activate: assistantClient.Activate,
		Account:       assistantClient.Account,
		SetLabName:    assistantClient.SetLabName,
		BillingPortal: assistantClient.BillingPortal,
		Offer:         assistantClient.Offer,
		Cleanup:       func(current string) error { return assistantWorkspace.Cleanup(current) },
		ListConversations: func(ctx context.Context, current string) ([]domain.AssistantConversationHistory, error) {
			return assistantWorkspace.ListConversations(ctx, repository, current)
		},
		ConversationHistory: func(ctx context.Context, id string) (domain.AssistantConversationHistory, error) {
			return assistantWorkspace.ConversationHistory(ctx, repository, id)
		},
		PrepareConversation: func(id string) (domain.AssistantConversation, error) {
			return assistantWorkspace.PrepareConversation(repository, id)
		},
		CarryOverProposal: assistantWorkspace.CarryOverProposal,
		Run:               assistantRunner.Run,
		ChangedFiles:      assistantWorkspace.ChangedFiles,
		PlanSave: func(ctx context.Context, id string) domain.AssistantSavePlan {
			return adapters.PlanAssistantSave(ctx, assistantWorkspace, id)
		},
		Save: func(ctx context.Context, plan domain.AssistantSavePlan) domain.TemplateResetResult {
			return adapters.SaveAssistant(ctx, assistantWorkspace, plan)
		},
	}
}
