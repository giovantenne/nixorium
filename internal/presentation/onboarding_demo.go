package presentation

import "github.com/giovantenne/nixorium/internal/domain"

// RenderOnboardingDemo supplies documentation with the actual screen renderer,
// synthetic local state and no filesystem or network callbacks.
func RenderOnboardingDemo(revision string, width, height int) DemoScenario {
	r := newDemoRecorder(DashboardActions{}, revision, width, height)
	r.model.screen = dashboardDisclaimer
	r.capture("Read the operational disclaimer", 2500)
	r.model.screen = dashboardTelemetry
	r.model.telemetry.firstOffer = true
	r.model.telemetry.report = domain.TelemetryReport{Consent: "undecided"}
	r.capture("Choose optional adoption statistics", 2500)
	r.model.screen = dashboardComputersArea
	r.model.computerSetup = "missing"
	r.capture("Configure clients before managing them", 2500)
	r.model.screen = dashboardHome
	r.model.updateNotification = domain.UpdateNotification{Key: "demo-next-stable", Target: "v3.1.0", Channel: domain.UpdateChannelStable}
	r.capture("A dismissible update on the chosen channel", 2500)
	return DemoScenario{ID: "onboarding", Title: "First administrator launch", Description: "Separate acknowledgement, optional consent and actionable notices.", Frames: r.frames}
}
