package presentation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantInputHint = "Enter send · Alt+Enter new line · / commands · Ctrl+R review · Esc back"
const assistantSavedChatText = "Saved. Your next message starts a new chat on the saved configuration."
const assistantLongChatText = "This chat is getting long. /new starts a fresh one, and answers will be faster."

type assistantEntry struct {
	role, text string
	repeat     int // consecutive identical activity lines collapsed into this one
}

func (model *assistantModel) noteTranscriptSize(bytes int) {
	if !model.longChatShown && model.maxRequestBytes > 0 && bytes > model.maxRequestBytes/2 {
		model.longChatShown = true
		model.entries = append(model.entries, assistantEntry{role: "notice", text: assistantLongChatText})
	}
}

func (model *assistantModel) noteAccessError(code string) {
	if code == "subscription_inactive" || code == "quota_exhausted" {
		model.accessError = code
	}
}

func (model *assistantModel) appendText(text string) {
	if len(model.entries) > 0 && model.entries[len(model.entries)-1].role == "assistant" {
		model.entries[len(model.entries)-1].text += text
		return
	}
	model.entries = append(model.entries, assistantEntry{role: "assistant", text: text})
}

// Only the current turn is considered: a later turn may repeat an answer.
func assistantSummaryShown(entries []assistantEntry, summary string) bool {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return true
	}
	text := ""
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].role == "user" {
			break
		}
		if entries[i].role == "assistant" {
			text = entries[i].text + text
		}
	}
	return domain.AssistantFinalAnswer(text, summary) == text
}
func (model *assistantModel) appendFinal(text, summary string) {
	answer := domain.AssistantFinalAnswer(text, summary)
	if len(model.entries) > 0 && model.entries[len(model.entries)-1].role == "assistant" && model.entries[len(model.entries)-1].text == answer {
		return
	}
	if text != "" && len(model.entries) > 0 && model.entries[len(model.entries)-1].role == "assistant" && model.entries[len(model.entries)-1].text == text {
		model.entries[len(model.entries)-1].text = answer
	} else if answer != "" {
		model.entries = append(model.entries, assistantEntry{role: "assistant", text: answer})
	}
}
func (model *assistantModel) appendSummary(summary string) {
	if !assistantSummaryShown(model.entries, summary) {
		model.entries = append(model.entries, assistantEntry{role: "assistant", text: summary})
	}
}

// appendActivity collapses consecutive identical activity lines, so a long
// series of searches shows as one line with a count.
func (model *assistantModel) appendActivity(line string) {
	if last := len(model.entries) - 1; last >= 0 && model.entries[last].role == "activity" && model.entries[last].text == line {
		model.entries[last].repeat = max(model.entries[last].repeat, 1) + 1
		return
	}
	model.entries = append(model.entries, assistantEntry{role: "activity", text: line})
}

func assistantActivityLine(text string) string {
	text = assistantDisplayText(text)
	if strings.HasPrefix(text, "Searching") {
		return "● Searching the configuration"
	}
	if strings.HasPrefix(text, "Reading Nixorium") {
		return "● Reading Nixorium source"
	}
	return "● " + text
}
func assistantCompactHosts(hosts []string, controller string) string {
	hosts = append([]string(nil), hosts...)
	sort.Strings(hosts)
	type group struct {
		label string
		count int
	}
	groups := []group{}
	number := func(host string) (int, bool) {
		if host == controller || !strings.HasPrefix(host, "pc") {
			return 0, false
		}
		n, err := strconv.Atoi(strings.TrimPrefix(host, "pc"))
		return n, err == nil
	}
	for i := 0; i < len(hosts); {
		end := i
		n, ok := number(hosts[i])
		if ok {
			for end+1 < len(hosts) {
				next, valid := number(hosts[end+1])
				if !valid || next != n+1 || len(hosts[end+1]) != len(hosts[i]) {
					break
				}
				end++
				n = next
			}
		}
		label := hosts[i]
		if end > i {
			label += "–" + hosts[end]
		}
		groups = append(groups, group{label, end - i + 1})
		i = end + 1
	}
	labels := []string{}
	for _, g := range groups {
		labels = append(labels, g.label)
	}
	if len([]rune(strings.Join(labels, ", "))) <= 60 {
		return strings.Join(labels, ", ")
	}
	shown := 0
	for n := len(groups) - 1; n >= 0; n-- {
		shown = 0
		for _, g := range groups[:n] {
			shown += g.count
		}
		suffix := fmt.Sprintf(", +%d more", len(hosts)-shown)
		prefix := strings.Join(labels[:n], ", ")
		if len([]rune(prefix+suffix)) <= 60 {
			return prefix + suffix
		}
	}
	return ""
}
func assistantToolResultLine(name, data, controller string) string {
	var result struct {
		OK           *bool             `json:"ok"`
		Error        string            `json:"error"`
		ChangedHosts []string          `json:"changedHosts"`
		Built        int               `json:"built"`
		Blocked      int               `json:"blocked"`
		Skipped      []json.RawMessage `json:"skipped"`
		Failed       []struct {
			Name string `json:"name"`
		} `json:"failed"`
	}
	err := json.Unmarshal([]byte(data), &result)
	if name == "evaluate" {
		if result.Error == "timeout" {
			return "✗ Evaluation timed out"
		}
		if err == nil && result.OK != nil && *result.OK {
			if len(result.ChangedHosts) == 0 {
				return "✓ Evaluation passed · no computer changes"
			}
			return "✓ Evaluation passed · changes " + assistantCompactHosts(result.ChangedHosts, controller)
		}
		return "✗ Evaluation failed"
	}
	if err != nil {
		return "● Build check finished"
	}
	switch result.Error {
	case "evaluate_first":
		return "● Build check needs a new evaluation"
	case "timeout":
		return "✗ Build check timed out"
	case "too_large", "build_unavailable":
		return "● Build check not available"
	}
	if result.OK != nil && *result.OK {
		line := fmt.Sprintf("✓ Build check passed · %d parts built", result.Built)
		if n := result.Blocked + len(result.Skipped); n > 0 {
			line += fmt.Sprintf(" · %d not checked", n)
		}
		return line
	}
	if result.OK != nil && !*result.OK && len(result.Failed) > 0 {
		line := "✗ Build check failed · " + assistantDisplayText(result.Failed[0].Name)
		if len(result.Failed) > 1 {
			line += fmt.Sprintf(" +%d more", len(result.Failed)-1)
		}
		return line
	}
	return "● Build check finished"
}
func assistantMarkdown(text string, dark bool) string {
	text = assistantDisplayText(text)
	lines := strings.Split(text, "\n")
	content := make([]string, 0, len(lines))
	fenced := false
	bold := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle().Foreground(newTUITheme(dark).muted)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			content = append(content, muted.Render("  "+line))
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			content = append(content, bold.Render(strings.TrimSpace(strings.TrimLeft(trimmed, "#"))))
			continue
		}
		rendered := ""
		for len(line) > 0 {
			a, b := strings.Index(line, "**"), strings.Index(line, "`")
			marker := "**"
			start := a
			style := bold
			if b >= 0 && (a < 0 || b < a) {
				marker = "`"
				start = b
				style = muted
			}
			if start < 0 {
				rendered += line
				break
			}
			tail := line[start+len(marker):]
			end := strings.Index(tail, marker)
			if end < 0 {
				rendered += line
				break
			}
			rendered += line[:start] + style.Render(tail[:end])
			line = tail[end+len(marker):]
		}
		content = append(content, rendered)
	}
	return assistantLinkify(strings.Join(content, "\n"))
}
func (model dashboardModel) assistantQuotaText() (string, bool) {
	right := ""
	account := model.assistant.account
	warn := false
	if model.assistant.token == "" && account.Plan == "" {
		right = "Not activated"
	}
	if account.Plan != "" {
		right = fmt.Sprintf("%d requests left", account.TurnsRemaining)
		warn = account.TurnsRemaining <= 50
		if account.Plan == "trial" {
			right = fmt.Sprintf("Free trial · %d of 10 left", account.TurnsRemaining)
			warn = account.TurnsRemaining <= 1
			if account.Status == "inactive" || account.TurnsRemaining <= 0 || model.assistant.accessError == "subscription_inactive" || model.assistant.accessError == "quota_exhausted" {
				right, warn = "Free trial ended", true
			}
		} else if account.TurnsRemaining <= 0 || model.assistant.accessError == "quota_exhausted" {
			right, warn = "No requests left", true
		}
	}
	return right, warn
}
func assistantHeaderRight(model dashboardModel) string {
	right, _ := model.assistantQuotaText()
	return right
}
func (model dashboardModel) assistantHeader(width int) string {
	name := strings.ReplaceAll(assistantDisplayText(model.assistant.account.Name), "\n", " ")
	lab := strings.TrimSpace(strings.ReplaceAll(assistantDisplayText(model.assistant.account.LabName), "\n", " "))
	left := "Assistant"
	if name != "" {
		left += " · " + name
		if lab != "" {
			// The school stays readable: the laboratory name gives way first.
			room := max(1, width-lipgloss.Width(assistantHeaderRight(model))-1) - lipgloss.Width(left) - lipgloss.Width(" · ")
			if room >= 4 {
				left += " · " + ansi.Truncate(lab, room, "…")
			}
		}
	}
	right, warn := model.assistantQuotaText()
	theme := newTUITheme(model.isDark)
	tone := theme.muted
	if warn {
		tone = theme.attention
	}
	left = ansi.Truncate(left, max(1, width-lipgloss.Width(right)-1), "…")
	return tuiSection(left, model.isDark) + strings.Repeat(" ", max(1, width-lipgloss.Width(left)-lipgloss.Width(right))) + lipgloss.NewStyle().Foreground(tone).Render(right)
}
func (model dashboardModel) assistantBox(text string, width int, kind tuiStatusKind) string {
	theme := newTUITheme(model.isDark)
	tone := theme.muted
	switch kind {
	case tuiStatusSuccess:
		tone = theme.success
	case tuiStatusAttention:
		tone = theme.attention
	case tuiStatusFailure:
		tone = theme.failure
	}
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(tone).Render(assistantLinkify(assistantDisplayText(text)))
}
func (model dashboardModel) assistantView() string {
	if model.assistant.state == "resume" {
		return model.assistantResumeView()
	}
	if model.assistant.state == "sharing" || model.assistant.state == "report" || model.assistant.state == "report-sending" {
		return model.assistantSharingView()
	}
	if model.assistant.state == "name" || model.assistant.state == "name-saving" {
		return model.assistantNameView()
	}
	if model.assistant.state == "welcome" || model.assistant.state == "consent" || model.assistant.state == "activate" {
		return model.assistantGateView()
	}
	return model.assistantChatView()
}

// assistantMessage keeps speaker identity visible with and without terminal colors.
func assistantMessage(entry assistantEntry, width int, dark, noColors bool) string {
	text := assistantDisplayText(entry.text)
	if entry.role == "user" && !noColors {
		// The opposite theme's normal text is the existing light/dark grey token.
		return lipgloss.NewStyle().Width(width).Padding(0, 1).
			Foreground(newTUITheme(dark).text).Background(newTUITheme(!dark).text).
			Render(assistantLinkify(text))
	}
	prefix := "> "
	if entry.role == "assistant" {
		text = assistantMarkdown(entry.text, dark)
		prefix = lipgloss.NewStyle().Foreground(tuiAccent(dark)).Render("●") + " "
	}
	wrapped := strings.Split(ansi.Wrap(assistantLinkify(text), max(1, width-2), ""), "\n")
	for i := range wrapped {
		if i == 0 {
			wrapped[i] = prefix + wrapped[i]
		} else {
			wrapped[i] = "  " + wrapped[i]
		}
	}
	return strings.Join(wrapped, "\n")
}

func (model dashboardModel) assistantChatLayout() (bodyLines []string, fixedText string, width, capacity int) {
	width = max(20, model.width-4)
	if model.width == 0 {
		width = 100
	}
	input := model.assistant.input
	input.SetWidth(width - 2)
	styles := textarea.DefaultStyles(model.isDark)
	styles.Focused.CursorLine = styles.Focused.CursorLine.UnsetBackground()
	for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
		state.Text = lipgloss.NewStyle().Foreground(tuiAccent(model.isDark))
		state.Prompt = lipgloss.NewStyle().Foreground(tuiAccent(model.isDark))
	}
	input.SetStyles(styles)
	inputBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(tuiAccent(model.isDark)).Render(input.View())
	fixed := []string{}
	if model.assistant.saved {
		fixed = append(fixed, model.assistantBox(assistantSavedChatText, width, tuiStatusSuccess))
	}
	if model.assistant.state == "billing-loading" {
		fixed = append(fixed, model.activitySpinner.View()+" Opening Stripe… · Esc to cancel")
	}
	if model.assistant.state == "running" {
		seconds := 0
		if !model.assistant.started.IsZero() {
			seconds = max(0, int(time.Since(model.assistant.started).Seconds()))
		}
		fixed = append(fixed, model.activitySpinner.View()+fmt.Sprintf(" Working… %ds · Esc to stop", seconds))
	}
	if model.assistant.changedFiles > 0 {
		left := "Proposal ready · " + countNoun(model.assistant.changedFiles, "file") + " changed"
		right := "Ctrl+R to review"
		fixed = append(fixed, model.assistantBox(left+strings.Repeat(" ", max(1, width-4-lipgloss.Width(left)-len(right)))+right, width, tuiStatusSuccess))
	}
	fixed = append(fixed, inputBox)
	if model.assistant.state == "history" {
		choices := []string{"Continue in a new chat", "Back"}
		fixed = []string{model.assistantBox(assistantReadOnlyText, width, tuiStatusAttention)}
		for i, choice := range choices {
			fixed = append(fixed, tuiSelection(choice, i == model.assistant.readOnlyCursor, model.isDark))
		}
	}
	lines := []string{}
	for _, entry := range model.assistant.archivedEntries {
		text := assistantDisplayText(entry.text)
		if entry.role == "user" || entry.role == "assistant" {
			lines = append(lines, assistantMessage(entry, width, model.isDark, model.assistantNoColors), "")
		} else {
			lines = append(lines, tuiMuted(text, model.isDark), "")
		}
	}
	if len(model.assistant.archivedEntries) > 0 {
		lines = append(lines, tuiMuted("New chat", model.isDark), "")
	}
	entries := model.assistant.entries
	if model.assistant.state == "history" {
		entries = nil
		lines = nil
		for _, message := range model.assistant.readOnlyHistory.Messages {
			if message.Role == "user" || message.Role == "assistant" && message.Content != "" {
				entries = append(entries, assistantEntry{role: message.Role, text: message.Content})
			}
		}
	}
	if len(entries) == 0 && model.assistant.state != "history" {
		for _, message := range model.assistant.messages {
			if message.Role == "user" || (message.Role == "assistant" && message.Content != "") {
				entries = append(entries, assistantEntry{role: message.Role, text: message.Content})
			}
		}
		if model.assistant.summary != "" && !assistantSummaryShown(entries, model.assistant.summary) {
			entries = append(entries, assistantEntry{role: "assistant", text: model.assistant.summary})
		}
	}
	for _, entry := range entries {
		switch entry.role {
		case "user", "assistant":
			if entry.role == "assistant" && len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			lines = append(lines, assistantMessage(entry, width, model.isDark, model.assistantNoColors), "")
		case "notice":
			lines = append(lines, tuiMuted(assistantDisplayText(entry.text), model.isDark))
		case "activity":
			tone := newTUITheme(model.isDark).muted
			if strings.HasPrefix(entry.text, "✓") {
				tone = newTUITheme(model.isDark).success
			}
			if strings.HasPrefix(entry.text, "✗") {
				tone = newTUITheme(model.isDark).failure
			}
			text := assistantDisplayText(entry.text)
			if entry.repeat > 1 {
				text += fmt.Sprintf(" (%d×)", entry.repeat)
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(tone).Render(text))
		}
	}
	if len(entries) == 0 && model.assistant.state != "history" {
		lines = append(lines, tuiMuted("Try asking, for example:", model.isDark))
		for _, example := range assistantExamples {
			lines = append(lines, tuiMuted("  "+example, model.isDark))
		}
	}
	if model.assistant.state == "chat" && model.assistant.token != "" && model.assistant.account.Plan != "" && model.assistant.account.LabName == "" {
		lines = append(lines, "", tuiMuted(assistantNameHint, model.isDark))
	}
	if choices := model.assistant.commandChoices(); len(choices) > 0 && model.assistant.state != "history" {
		for i, command := range choices {
			lines = append(lines, tuiSelection(command, i == model.assistant.commandCursor%len(choices), model.isDark)+"  "+tuiMuted(assistantCommandDescriptions[command], model.isDark))
		}
	}
	if model.message != "" {
		if strings.HasPrefix(model.message, "Subscription active · ") {
			lines = append(lines, model.assistantBox("✓ "+model.message, width, tuiStatusSuccess))
		} else {
			lines = append(lines, model.assistantBox("! "+model.message, width, tuiStatusAttention))
		}
	}
	body := lipgloss.NewStyle().Width(width).Render(assistantLinkify(strings.Join(lines, "\n")))
	bodyLines = strings.Split(body, "\n")
	fixedText = strings.Join(fixed, "\n")
	capacity = max(1, model.height-2-1-2-lipgloss.Height(fixedText)-1)
	if model.height == 0 {
		capacity = len(bodyLines)
	}
	return
}

func (model dashboardModel) scrollAssistantHistory(delta int) dashboardModel {
	lines, _, _, capacity := model.assistantChatLayout()
	model.assistant.historyScroll = min(max(0, len(lines)-capacity), max(0, model.assistant.historyScroll+delta))
	return model
}

func (model dashboardModel) assistantChatView() string {
	bodyLines, fixedText, width, capacity := model.assistantChatLayout()
	end := max(1, len(bodyLines)-min(model.assistant.historyScroll, max(0, len(bodyLines)-capacity)))
	start := max(0, end-capacity)
	body := strings.Join(bodyLines[start:end], "\n")
	body = lipgloss.NewStyle().Height(capacity).Render(body)
	hint := assistantInputHint
	if model.assistant.state == "history" {
		hint = "Enter select · Esc back · PgUp/PgDn scroll"
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(model.assistantHeader(width) + "\n\n" + body + "\n" + fixedText + "\n" + tuiMuted(hint, model.isDark))
}
