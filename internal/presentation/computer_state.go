package presentation

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// The last computer observation made in this session (Computers → Inventory
// or a check started from a selection list). It is labelled with its time and
// never presented as live state.
func (model dashboardModel) observedHost(name string) (domain.HostStatus, bool) {
	for _, host := range model.computers.hosts.Hosts {
		if host.Name == name {
			return host, true
		}
	}
	return domain.HostStatus{}, false
}

func observationHeading(at time.Time) string {
	if at.IsZero() {
		return "Not checked in this session · r checks the computers now"
	}
	return "Last checked " + at.Local().Format("15:04:05") + " · r checks again"
}

// deploymentStateLabel states whether a computer needs the saved configuration.
func deploymentStateLabel(host domain.HostStatus, found bool) string {
	switch {
	case !found:
		return "Not checked"
	case host.Reachability != domain.ReachabilityReachable:
		return "Off or not reachable at last check"
	case host.Deployment == domain.DeploymentOutdated:
		return "Needs update"
	case host.Deployment == domain.DeploymentCurrent:
		return "Up to date"
	default:
		return "Not verified"
	}
}

func powerStateLabel(host domain.HostStatus, found bool) string {
	switch {
	case !found:
		return "Not checked"
	case host.Reachability == domain.ReachabilityReachable:
		return "On at last check"
	case host.Reachability == domain.ReachabilityUnreachable:
		return "Off or not reachable at last check"
	default:
		return "State unknown"
	}
}

// internetStateWords turns observed and outcome states into plain words.
func internetStateWords(state string) string {
	switch state {
	case "enabled":
		return "Internet on"
	case "blocked":
		return "Internet blocked"
	case "verified":
		return "done and verified"
	case "unconfirmed":
		return "sent, not confirmed"
	case "not-sent":
		return "not sent"
	case "", "unknown":
		return "state unknown"
	}
	return state
}

// checkComputersThen observes the configured computers and returns to the
// current selection list instead of opening the inventory.
func (model dashboardModel) checkComputersThen() (tea.Model, tea.Cmd) {
	if model.actions.LoadHosts == nil {
		model.message = "Computer checks are not available in this session."
		return model, nil
	}
	model.computers.refreshReturn = model.screen
	model.busy = "Checking the configured computers"
	model.message = ""
	return model.loadHosts()
}

// selectComputers replaces the selection with the listed computers matching
// the predicate, using only the last observation.
func (model dashboardModel) selectComputers(chosen map[string]bool, match func(domain.HostStatus) bool, none string) (map[string]bool, string) {
	if model.computers.hosts.GeneratedAt.IsZero() {
		return chosen, "No observation yet: press r to check the computers first."
	}
	result := map[string]bool{}
	for _, host := range model.report.Meta.Clients.Hosts {
		if observed, found := model.observedHost(host.Name); found && match(observed) {
			result[host.Name] = true
		}
	}
	if len(result) == 0 {
		return chosen, none
	}
	return result, ""
}

func hostNeedsUpdate(host domain.HostStatus) bool {
	return host.Deployment == domain.DeploymentOutdated
}

func hostIsOn(host domain.HostStatus) bool {
	return host.Reachability == domain.ReachabilityReachable
}

// updateComputersThatNeedIt opens the ordinary deployment review for every
// computer the last observation found outdated.
func (model dashboardModel) updateComputersThatNeedIt() (tea.Model, tea.Cmd) {
	chosen, message := model.selectComputers(nil, hostNeedsUpdate, "No computer needed an update at the last check.")
	if message != "" {
		model.message = message
		return model, nil
	}
	model.screen = dashboardDeploy
	model.deployment.result = domain.DeploymentExecutionReport{}
	model.deployment.context = ""
	model.deployment.chosen = chosen
	model.deployment.cursor = 0
	return model.updateDeployment(tea.KeyPressMsg{Code: tea.KeyEnter})
}

type internetObserveMsg struct {
	plan domain.InternetPlan
}

// checkInternetState uses the read-only plan to observe every client. The
// plan is not kept for application; a change always needs its own review.
func (model dashboardModel) checkInternetState() (tea.Model, tea.Cmd) {
	hosts := model.report.Meta.Clients.Hosts
	if model.actions.PlanInternet == nil || len(hosts) == 0 {
		model.message = "Internet state cannot be checked in this session."
		return model, nil
	}
	names := make([]string, 0, len(hosts))
	for _, host := range hosts {
		names = append(names, host.Name)
	}
	requested, action := strings.Join(names, ","), model.internet.action
	model.busy = "Checking Internet access on all clients"
	model.message = ""
	return model.startRead(func(ctx context.Context) tea.Msg {
		return internetObserveMsg{model.actions.PlanInternet(ctx, requested, action)}
	})
}

func (model dashboardModel) finishInternetObservation(message internetObserveMsg) (tea.Model, tea.Cmd) {
	model.busy = ""
	model.internet.observed = map[string]string{}
	for _, target := range message.plan.Targets {
		state := "unknown"
		if target.Eligible && target.Observed.Valid() {
			state = target.Observed.State
		} else if !target.Eligible {
			state = "unreachable"
		}
		model.internet.observed[target.Name] = state
	}
	model.internet.observedAt = time.Now()
	if len(message.plan.Targets) == 0 && message.plan.Message != "" {
		model.message = message.plan.Message
	}
	return model, nil
}

func (model internetModel) stateLabel(name string) string {
	state, found := model.observed[name]
	switch {
	case !found:
		return "Not checked"
	case state == "unreachable":
		return "Not reachable at last check"
	}
	return internetStateWords(state)
}
