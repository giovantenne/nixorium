package domain

type DeploymentComputerResult struct {
	Name     string
	Outcome  string
	Guidance string
}

type DeploymentResultSummary struct {
	Headline  string
	Computers []DeploymentComputerResult
}

// ResultSummary is a conservative projection of the execution evidence. It
// never changes recovery, retry eligibility, verification or recorded history.
// A batch error alone cannot identify which individual activation failed.
func (report DeploymentExecutionReport) ResultSummary() DeploymentResultSummary {
	summary := DeploymentResultSummary{Headline: "Deployment needs attention"}
	verified := map[string]DeploymentTargetVerification{}
	for _, target := range report.Verification.Targets {
		verified[target.Name] = target
	}
	notReached, updateFailed := false, false
	for _, target := range report.Targets {
		observation := verified[target.Name]
		result := DeploymentComputerResult{Name: target.Name, Outcome: "Not verified", Guidance: "Inspect authenticated computer state and the private log before a fresh review."}
		switch {
		case !report.BuildCompleted || report.Phase == DeploymentPhasePreflight:
			result.Outcome = "Not updated"
			result.Guidance = "No update was started. Resolve the build or preflight issue first."
		case observation.HostKeyCondition == HostKeyChanged:
			result.Guidance = "SSH host key changed. Verify physical identity and review this computer's host-key plan."
		case observation.State == "verified" && !report.RecoveryRequired && report.ApplyCompleted:
			result.Outcome = "Updated"
			result.Guidance = "Authenticated at the reviewed revision."
		case observation.Reachability == ReachabilityUnreachable && observation.State != "verified" && observation.Revision == "" && observation.SystemPath == "":
			result.Outcome = "Not reached"
			result.Guidance = "Check power and networking. The update outcome is not known; this does not prove the computer is off or unchanged."
			notReached = true
		case !report.RecoveryRequired && report.Phase == DeploymentPhaseApply && !report.ApplyCompleted && observation.Revision != "" && observation.Revision != report.Revision:
			result.Outcome = "Update failed"
			result.Guidance = "The update phase failed and this computer still reports another revision. Inspect the log before a fresh review."
			updateFailed = true
		}
		if report.RecoveryRequired {
			result.Guidance += " Activation completion is unconfirmed; use reviewed recovery, not a retry."
		}
		summary.Computers = append(summary.Computers, result)
	}
	switch {
	case report.RecoveryRequired:
		summary.Headline = "Deployment requires recovery"
		if notReached {
			summary.Headline += " — some computers were not reached"
		}
	case !report.HasErrors():
		summary.Headline = "Deployment completed and verified"
	case updateFailed:
		summary.Headline = "An update failed"
	case notReached:
		summary.Headline = "Some computers were not reached"
	case report.Phase == DeploymentPhaseApply:
		summary.Headline = "An update failed; individual outcomes need checking"
	}
	return summary
}
