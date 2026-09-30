package presentation

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func HostTrustPlanText(writer io.Writer, p domain.HostTrustPlan) {
	fmt.Fprintf(writer, "Review SSH trust for %s (%s)\nRecorded fingerprints: %s\nOffered fingerprint: %s\n%s\n", p.Host.Name, p.Host.IP, strings.Join(p.Inspection.Recorded, ", "), p.Inspection.Offered, sanitizeRemoteInstallText(p.Message))
	if p.ReviewToken != "" {
		fmt.Fprintln(writer, "Review token:", p.ReviewToken)
		fmt.Fprintln(writer, "Review expires:", p.ExpiresAt.UTC().Format(time.RFC3339))
	}
}
func ConfirmHostTrust(input io.Reader, output io.Writer, p domain.HostTrustPlan) (bool, error) {
	if p.HasErrors() {
		return false, nil
	}
	HostTrustPlanText(output, p)
	fmt.Fprint(output, "Type ROTATE HOST KEY: ")
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == "ROTATE HOST KEY", nil
}

type hostTrustModel struct {
	plan         domain.HostTrustPlan
	result       domain.HostTrustResult
	confirmation string
	id           uint64
	cancel       context.CancelFunc
	started      time.Time
	applying     bool
}
type hostTrustPlanMsg struct {
	id   uint64
	plan domain.HostTrustPlan
}
type hostTrustResultMsg struct {
	id     uint64
	result domain.HostTrustResult
}

func (model dashboardModel) openHostTrust(name string) (tea.Model, tea.Cmd) {
	if model.actions.ClassroomMode || model.actions.PlanHostTrust == nil || model.actions.ApplyHostTrust == nil {
		return model, nil
	}
	model.hostTrust.id++
	ctx, activityID := model.beginRead(dashboardReadTimeout)
	model.hostTrust.cancel = model.read.cancel
	model.hostTrust.confirmation = ""
	model.hostTrust.result = domain.HostTrustResult{}
	model.hostTrust.plan = domain.HostTrustPlan{}
	model.hostTrust.applying = false
	model.message = ""
	model.hostTrust.started = time.Now()
	model.screen = dashboardHostTrust
	model.busy = "Reading the recorded and offered SSH fingerprints"
	id := model.hostTrust.id
	return model, boundedReadCommand(ctx, activityID, func(ctx context.Context) tea.Msg {
		return hostTrustPlanMsg{id: id, plan: model.actions.PlanHostTrust(ctx, name)}
	})
}

func (model dashboardModel) updateHostTrustKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	trust := &model.hostTrust
	if trust.applying {
		model.message = "The trust update cannot be interrupted."
		return model, nil
	}
	switch key.String() {
	case "esc":
		completed := trust.result.Operation != ""
		if trust.cancel != nil {
			trust.cancel()
			trust.cancel = nil
			trust.id++
		}
		model.busy = ""
		model.screen = dashboardHosts
		model.message = "Cancelled; nothing was changed."
		if completed {
			model.message = trust.result.Message
		}
	case "enter":
		if trust.result.Operation != "" {
			model.screen = dashboardHosts
			model.message = trust.result.Message
			return model, nil
		}
		if trust.cancel != nil || trust.plan.HasErrors() {
			return model, nil
		}
		if trust.confirmation != "ROTATE HOST KEY" {
			trust.confirmation = ""
			model.message = "Type ROTATE HOST KEY exactly, after checking the physical fingerprint."
			return model, nil
		}
		plan := trust.plan
		trust.applying = true
		trust.started = time.Now()
		model.busy = "Rechecking and saving this computer's SSH trust"
		id := trust.id
		return model, func() tea.Msg { return hostTrustResultMsg{id: id, result: model.actions.ApplyHostTrust(plan)} }
	case "backspace":
		runes := []rune(trust.confirmation)
		if len(runes) > 0 {
			trust.confirmation = string(runes[:len(runes)-1])
		}
	case "space":
		if trust.cancel == nil && trust.result.Operation == "" && len(trust.confirmation) < 64 {
			trust.confirmation += " "
		}
	default:
		if trust.cancel == nil && trust.result.Operation == "" {
			if len(trust.confirmation)+len(key.Text) <= 64 {
				trust.confirmation += sanitizeRemoteInstallText(key.Text)
			}
		}
	}
	return model, nil
}

func (model dashboardModel) hostTrustView() string {
	trust := model.hostTrust
	lines := []string{tuiTitle("Review this computer's changed SSH key", model.isDark)}
	actions := []tuiAction{{key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}}
	fixed := ""
	if model.busy != "" {
		lines = append(lines, model.busyView())
	} else if trust.result.Operation != "" {
		lines = append(lines, sanitizeRemoteInstallText(trust.result.Message))
		actions[0].label = "Back"
		actions = append([]tuiAction{{key: "Enter", label: "Computer details"}}, actions...)
	} else {
		p := trust.plan
		lines = append(lines, fmt.Sprintf("Computer: %s (%s)", p.Host.Name, p.Host.IP), "Recorded: "+strings.Join(p.Inspection.Recorded, ", "), "Offered: "+p.Inspection.Offered, "", sanitizeRemoteInstallText(p.Message))
		if !p.HasErrors() {
			fixed = "Type ROTATE HOST KEY: " + trust.confirmation + "_"
			actions = append([]tuiAction{{key: "Enter", label: "Save reviewed trust"}}, actions...)
		}
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	return model.renderShell(tuiShell{path: []string{"Computers", "SSH trust"}, body: strings.Join(lines, "\n"), fixedBody: fixed, actions: actions, notices: notices})
}
