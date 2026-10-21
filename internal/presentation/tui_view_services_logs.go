package presentation

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

func (model dashboardModel) servicesView() string {
	path := []string{"Maintenance", "Services"}
	lines := []string{
		tuiTitle("Controller services", model.isDark),
		tuiMuted("Normally no action is needed here. Use this view when Diagnostics reports a delivery or installation service problem.", model.isDark),
	}
	notices := []tuiNotice{}
	if model.busy != "" {
		lines = append(lines, "", model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.maintenance.serviceResult.Operation != "" {
		success := !model.maintenance.serviceResult.HasErrors() && model.maintenance.serviceResult.Verified
		title := "Service action needs attention"
		if success {
			title = "Binary cache restarted and verified"
		}
		lines = append(lines,
			tuiResult(title, success, model.isDark),
			"",
			fmt.Sprintf("State: %s   Verified: %t   Retry safe: %t", model.maintenance.serviceResult.State, model.maintenance.serviceResult.Verified, model.maintenance.serviceResult.RetrySafe),
		)
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: append(path, "Result"), body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Enter", label: "Maintenance"}, {key: "l", label: "Logs"}, {key: "c", label: "Restart again"}, {key: "F1", label: "Help"}}})
	}
	if model.screen == dashboardServicesRestartReview {
		body := strings.Join([]string{
			tuiTitle("Restart the software cache?", model.isDark),
			"",
			"Affects  Controller cache; active installations may be affected",
		}, "\n")
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: "The signed cache will be briefly unavailable", detail: "Active installers may retry downloads. PXE networking and listeners are unchanged; the cache is verified afterward."})
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{
			path:      append(path, "Review"),
			body:      body,
			fixedBody: tuiSection("Enter restarts the software cache.", model.isDark),
			notices:   notices,
			actions:   []tuiAction{{key: "Enter", label: "Restart cache"}, {key: "Esc", label: "Cancel"}, {key: "F1", label: "Help"}},
		})
	}
	for _, service := range model.maintenance.services.Services {
		kind := tuiStatusAttention
		if service.State == "healthy" || service.State == "active" || service.State == "standby" {
			kind = tuiStatusSuccess
		} else if service.State == "failed" || service.State == "degraded" {
			kind = tuiStatusFailure
		}
		lines = append(lines,
			fmt.Sprintf("%s — %s", tuiSection(service.Name, model.isDark), tuiStatus(service.State, kind, model.isDark)),
		)
		if service.ID == "cache" {
			lines = append(lines, "  Supplies already-built software to clients and network installers.")
		} else if service.ID == "pxe" {
			lines = append(lines, "  Provides network boot while PXE mode is active.")
		}
		lines = append(lines, tuiMuted("  "+service.Detail, model.isDark))
		if service.ID == "pxe" {
			lines = append(lines, "  Start, stop or recover it from Installation → Network boot (PXE).")
		}
		lines = append(lines, "")
	}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if model.serviceRestartAvailable() {
		actions = append(actions, tuiAction{key: "c", label: "Review cache restart"})
	}
	actions = append(actions, tuiAction{key: "r", label: "Refresh"}, tuiAction{key: "Esc", label: "Maintenance"}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) serviceRestartAvailable() bool {
	return len(model.maintenance.services.Services) > 0 && model.maintenance.services.Services[0].ID == "cache" && len(model.maintenance.services.Services[0].Units) > 0 && model.maintenance.services.Services[0].Units[0].Loaded
}

// operationLabel names a recorded operation the way the menus do.
func operationLabel(operation string) string {
	labels := map[string]string{
		"controller-apply": "Apply to controller", "setup-apply-controller": "Apply to controller",
		"deploy-apply": "Update computers", "update-apply": "Update Nixorium",
		"pxe-start": "Start network installation", "pxe-stop": "Finish network installation",
		"pxe-prepare": "Prepare installation files", "pxe-recover": "Recover network installation",
		"software-change-apply": "Software change", "software-preset-apply": "Software profile",
		"config-apply": "Save settings", "configuration-save": "Save settings",
		"usb-install-apply": "USB installation", "shutdown-apply": "Power action", "wake-apply": "Switch on",
		"workspace-apply": "Student workspace", "template-reset-recover": "Template reset recovery",
	}
	if label, found := labels[operation]; found {
		return label
	}
	return operation
}

func (model dashboardModel) logsView() string {
	path := []string{"Maintenance", "History"}
	lines := []string{tuiTitle("Operation history", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	lines = append(lines, tuiSection("Recent actions", model.isDark))
	recordLimit := len(model.maintenance.logs.Records)
	if recordLimit > 5 {
		recordLimit = 5
	}
	if recordLimit == 0 {
		lines = append(lines, "  No recorded operation outcomes.")
	}
	for _, record := range model.maintenance.logs.Records[:recordLimit] {
		lines = append(lines, fmt.Sprintf("  %s  %-30s %-10s %s", record.RecordedAt.Local().Format("2006-01-02 15:04"), operationLabel(record.Operation), record.State, record.Subject))
		// A failure says why, so the cause is not lost once its screen is gone.
		if record.State != "completed" && record.State != "saved" && strings.TrimSpace(record.Summary) != "" {
			summary := lipgloss.NewStyle().Width(max(20, model.width-12)).Render(sanitizeRemoteInstallText(record.Summary))
			for _, line := range strings.Split(summary, "\n") {
				lines = append(lines, "      "+tuiMuted(line, model.isDark))
			}
		}
	}
	lines = append(lines, "", tuiSection("Deployment logs", model.isDark))
	if len(model.maintenance.logs.Logs) == 0 {
		lines = append(lines, "No deployment operation logs are available.")
	}
	start, end := listWindow(len(model.maintenance.logs.Logs), model.maintenance.logCursor, max(1, (model.height-ctaRows(model.height)-16)/2))
	for index := start; index < end; index++ {
		entry := model.maintenance.logs.Logs[index]
		row := fmt.Sprintf("%s  %-10s %-11s %d bytes", entry.StartedAt.UTC().Format("2006-01-02 15:04Z"), entry.Kind, entry.State, entry.SizeBytes)
		lines = append(lines, tuiSelection(row, index == model.maintenance.logCursor, model.isDark))
		lines = append(lines, "    "+entry.ID)
	}
	notices := []tuiNotice{}
	if model.message != "" {
		notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
	}
	actions := []tuiAction{}
	if len(model.maintenance.logs.Logs) > 0 {
		actions = append(actions, tuiAction{key: "↑/↓", label: "Select"}, tuiAction{key: "Enter", label: "View tail"})
	}
	back := "Maintenance"
	if model.setupMode {
		back = "Setup"
	}
	actions = append(actions, tuiAction{key: "r", label: "Refresh"}, tuiAction{key: "Esc", label: back}, tuiAction{key: "F1", label: "Help"})
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: actions})
}

func (model dashboardModel) logDetailView() string {
	path := []string{"Maintenance", "History", "Log"}
	lines := []string{tuiTitle("Operation log detail", model.isDark), ""}
	if model.busy != "" {
		lines = append(lines, model.busyView())
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "F1", label: "Help"}}})
	}
	if model.maintenance.logDetail.Log == nil {
		lines = append(lines, "The selected operation log could not be read safely.")
		notices := []tuiNotice{}
		if model.message != "" {
			notices = append(notices, tuiNotice{kind: tuiStatusAttention, title: model.message})
		}
		return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), notices: notices, actions: []tuiAction{{key: "Esc", label: "History"}, {key: "F1", label: "Help"}}})
	}
	entry := model.maintenance.logDetail.Log
	contentLines := operationLogContentLines(model.maintenance.logDetail.Content)
	end := model.maintenance.logScroll + model.logDetailHeight()
	if end > len(contentLines) {
		end = len(contentLines)
	}
	lines = append(lines,
		fmt.Sprintf("%s — %s", entry.StartedAt.UTC().Format("2006-01-02 15:04:05Z"), entry.State),
		entry.ID,
		fmt.Sprintf("Showing lines %d-%d of %d%s", displayedLineStart(model.maintenance.logScroll, len(contentLines)), end, len(contentLines), truncatedLogLabel(model.maintenance.logDetail.Truncated)),
		"",
	)
	lines = append(lines, contentLines[model.maintenance.logScroll:end]...)
	return model.renderShell(tuiShell{path: path, body: strings.Join(lines, "\n"), actions: []tuiAction{{key: "↑/↓/Pg/Home/End", label: "Scroll"}, {key: "Esc", label: "History"}, {key: "F1", label: "Help"}}})
}

func (model dashboardModel) logDetailHeight() int {
	if model.height <= 0 {
		return 12
	}
	height := model.height - ctaRows(model.height) - 15
	if height < 1 {
		return 1
	}
	return height
}

func maximumLogScroll(report domain.OperationLogReport, height int) int {
	maximum := len(operationLogContentLines(report.Content)) - height
	if maximum < 0 {
		return 0
	}
	return maximum
}

func operationLogContentLines(content string) []string {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return []string{"(empty log)"}
	}
	return strings.Split(content, "\n")
}

func operationLogIssues(issues []domain.ValidationIssue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Field+": "+issue.Message)
	}
	return strings.Join(parts, "; ")
}

func displayedLineStart(offset, total int) int {
	if total == 0 {
		return 0
	}
	return offset + 1
}

func truncatedLogLabel(truncated bool) string {
	if truncated {
		return " (bounded tail; earlier bytes omitted)"
	}
	return ""
}
