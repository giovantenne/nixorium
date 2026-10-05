package presentation

import "github.com/giovantenne/nixorium/internal/domain"

// Overview follows the order of accepted observations, not equality with an
// older saved revision. A fresh review or verified apply checks the complete
// current deployment, which can include later commits such as generated keys.
// This state is advisory only; operation reviews and save-result evidence keep
// their own exact revision binding.
type controllerOverviewObservation uint8

const (
	controllerOverviewUnknown controllerOverviewObservation = iota
	controllerOverviewPending
	controllerOverviewCurrent
)

func (model *dashboardModel) noteControllerSave(revision string) {
	if revision != "" {
		model.controllerObservation = controllerOverviewPending
	}
}

func (model *dashboardModel) observeControllerPlan(plan domain.ControllerRebuildPlanReport) {
	if plan.Operation == "" || plan.Revision == "" || plan.HasErrors() {
		return
	}
	model.controllerObservation = controllerOverviewPending
	if controllerPlanCurrent(plan) {
		model.noteControllerCurrent()
	}
}

func (model *dashboardModel) observeControllerResult(result domain.ControllerRebuildExecutionReport) {
	if result.Operation == "" || result.Revision == "" {
		return
	}
	model.controllerObservation = controllerOverviewPending
	if controllerVerifiedForSave(result.Revision, result) {
		model.noteControllerCurrent()
	}
}

func (model *dashboardModel) observeSetupController(setup domain.SetupReport) {
	if setup.CurrentStage == domain.SetupStageApply && setup.State != "ready" && setup.State != "unchecked" {
		model.controllerObservation = controllerOverviewPending
		return
	}
	for _, stage := range setup.Stages {
		if stage.ID == domain.SetupStageApply {
			switch stage.State {
			case domain.SetupStageComplete:
				model.noteControllerCurrent()
			case domain.SetupStageCurrent:
				model.controllerObservation = controllerOverviewPending
			}
			return
		}
	}
}

func (model *dashboardModel) observeRecoveryController(recovery domain.RecoveryReport) {
	model.recovery = recovery
	for _, condition := range recovery.Conditions {
		if condition.Kind == domain.RecoveryControllerChanged {
			// Recovery checks the last activation, not the desired Git HEAD.
			// Invalidate prior success without converting this replaceable
			// condition into a sticky save reminder.
			if model.controllerObservation == controllerOverviewCurrent {
				model.controllerObservation = controllerOverviewUnknown
			}
			return
		}
	}
}

func (model *dashboardModel) noteControllerCurrent() {
	model.controllerObservation = controllerOverviewCurrent
	// Only this cached condition is superseded by the fresh verification.
	// Persistent client/reset recovery records and all other conditions remain.
	conditions := make([]domain.BlockingCondition, 0, len(model.recovery.Conditions))
	for _, condition := range model.recovery.Conditions {
		if condition.Kind != domain.RecoveryControllerChanged {
			conditions = append(conditions, condition)
		}
	}
	model.recovery.Conditions = conditions
	if len(conditions) == 0 && model.recovery.State == "attention" {
		model.recovery.State = "clear"
	}
}
