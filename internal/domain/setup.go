package domain

const (
	SetupStageInspectEnvironment = "inspect-environment"
	SetupStageNetwork            = "collect-network"
	SetupStageIdentity           = "collect-identity"
	SetupStageCredentials        = "collect-credentials"
	SetupStageKeys               = "reconcile-keys"
	SetupStageValidate           = "validate-configuration"
	SetupStageReview             = "review-changes"
	SetupStageApply              = "apply-controller"
	SetupStageArtifacts          = "prepare-artifacts"
	SetupStageReadiness          = "verify-readiness"
	SetupStageInstall            = "offer-client-installation"
)

type SetupStageState string

const (
	SetupStageComplete SetupStageState = "complete"
	SetupStageCurrent  SetupStageState = "current"
	SetupStagePending  SetupStageState = "pending"
)

type SetupObservation struct {
	Complete bool
	Detail   string
}

type SetupFacts struct {
	Environment SetupObservation
	Network     SetupObservation
	Identity    SetupObservation
	Credentials SetupObservation
	Keys        SetupObservation
	Validation  SetupObservation
	Review      SetupObservation
	Apply       SetupObservation
	Artifacts   SetupObservation
	Readiness   SetupObservation
	Install     SetupObservation
}

type SetupStage struct {
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	State  SetupStageState `json:"state"`
	Detail string          `json:"detail,omitempty"`
}

type SetupReport struct {
	SchemaVersion int          `json:"schemaVersion"`
	Operation     string       `json:"operation"`
	State         string       `json:"state"`
	Repository    string       `json:"repository"`
	CurrentStage  string       `json:"currentStage,omitempty"`
	Stages        []SetupStage `json:"stages"`
}

func ReconcileSetup(repository string, facts SetupFacts) SetupReport {
	definitions := []struct {
		id          string
		title       string
		observation SetupObservation
	}{
		{SetupStageInspectEnvironment, "Inspect environment", facts.Environment},
		{SetupStageNetwork, "Collect network settings", facts.Network},
		{SetupStageIdentity, "Collect lab identity and locale", facts.Identity},
		{SetupStageCredentials, "Collect and hash credentials", facts.Credentials},
		{SetupStageKeys, "Reconcile key material", facts.Keys},
		{SetupStageValidate, "Validate candidate configuration", facts.Validation},
		{SetupStageReview, "Review and accept Git changes", facts.Review},
		{SetupStageApply, "Apply controller configuration", facts.Apply},
		{SetupStageArtifacts, "Prepare installation artifacts", facts.Artifacts},
		{SetupStageReadiness, "Verify readiness", facts.Readiness},
		{SetupStageInstall, "Offer first client installation", facts.Install},
	}
	report := SetupReport{
		SchemaVersion: SchemaVersion,
		Operation:     "setup-status",
		State:         "ready",
		Repository:    repository,
		Stages:        make([]SetupStage, 0, len(definitions)),
	}
	foundCurrent := false
	for _, definition := range definitions {
		state := SetupStageComplete
		if foundCurrent {
			state = SetupStagePending
		} else if !definition.observation.Complete {
			state = SetupStageCurrent
			foundCurrent = true
			report.State = "action-required"
			report.CurrentStage = definition.id
		}
		report.Stages = append(report.Stages, SetupStage{
			ID:     definition.id,
			Title:  definition.title,
			State:  state,
			Detail: definition.observation.Detail,
		})
	}
	return report
}

type KeyMaterialState struct {
	Name           string `json:"name"`
	PrivatePresent bool   `json:"privatePresent"`
	PublicPresent  bool   `json:"publicPresent"`
	PrivateMode    uint32 `json:"privateMode,omitempty"`
	Safe           bool   `json:"safe"`
	Verified       bool   `json:"verified"`
	Matches        bool   `json:"matches"`
	Problem        string `json:"problem,omitempty"`
}

func (k KeyMaterialState) Ready() bool {
	return k.PrivatePresent && k.PublicPresent && k.Safe && k.Verified && k.Matches && k.Problem == ""
}

type KeyReconcileReport struct {
	SchemaVersion int                `json:"schemaVersion"`
	Operation     string             `json:"operation"`
	State         string             `json:"state"`
	Repository    string             `json:"repository"`
	Keys          []KeyMaterialState `json:"keys"`
}
