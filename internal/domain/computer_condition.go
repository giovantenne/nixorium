package domain

// ComputerCondition explains observation failures without claiming that an
// unreachable computer is powered off or that an open SSH port authenticates it.
func ComputerCondition(h HostStatus) (Level, string, string) {
	if h.Reachability == ReachabilityUnreachable {
		return LevelWarning, "Could not be reached", "Check that the computer is powered on and connected to the lab network."
	}
	if h.SSH == SSHUnavailable {
		return LevelWarning, "Management unavailable", "The computer responds, but remote management is unavailable. Open diagnostics."
	}
	if h.SSH != SSHAvailable {
		return LevelWarning, "Not verified", "Refresh the observation, then check the connection details."
	}
	switch h.Deployment {
	case DeploymentCurrent:
		return LevelOK, "Up to date", "The observed configuration matches the saved revision."
	case DeploymentOutdated:
		return LevelWarning, "Update ready", "Review a deployment to apply the saved configuration."
	default:
		return LevelWarning, "Revision not verified", "Remote management responds, but configuration identity could not be verified. Open details."
	}
}
