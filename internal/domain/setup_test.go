package domain

import "testing"

func completeObservation() SetupObservation {
	return SetupObservation{Complete: true}
}

func completeSetupFacts() SetupFacts {
	complete := completeObservation()
	return SetupFacts{
		Environment: complete,
		Network:     complete,
		Identity:    complete,
		Credentials: complete,
		Keys:        complete,
		Validation:  complete,
		Review:      complete,
		Apply:       complete,
		Artifacts:   complete,
		Readiness:   complete,
		Install:     complete,
	}
}

func TestReconcileSetupSelectsEarliestIncompleteStage(t *testing.T) {
	facts := completeSetupFacts()
	facts.Credentials = SetupObservation{Detail: "default password hashes remain"}
	facts.Keys = SetupObservation{Complete: false, Detail: "keys are missing"}
	report := ReconcileSetup("/repo", facts)
	if report.State != "action-required" || report.CurrentStage != SetupStageCredentials {
		t.Fatalf("report = %+v", report)
	}
	if report.Stages[2].State != SetupStageComplete || report.Stages[3].State != SetupStageCurrent || report.Stages[4].State != SetupStagePending {
		t.Fatalf("unexpected stage sequence: %+v", report.Stages)
	}
}

func TestReconcileSetupIsReadyOnlyWhenEveryStageIsComplete(t *testing.T) {
	report := ReconcileSetup("/repo", completeSetupFacts())
	if report.State != "ready" || report.CurrentStage != "" {
		t.Fatalf("report = %+v", report)
	}
	for _, stage := range report.Stages {
		if stage.State != SetupStageComplete {
			t.Fatalf("stage = %+v", stage)
		}
	}
}
