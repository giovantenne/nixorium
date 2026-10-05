package presentation

import (
	"fmt"
	"github.com/giovantenne/nixorium/internal/domain"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"image/color"
)

type tuiTheme struct {
	accent              color.Color
	heading             color.Color
	selectionText       color.Color
	selectionBackground color.Color
	controls            color.Color
	text                color.Color
	muted               color.Color
	notice              color.Color
	success             color.Color
	attention           color.Color
	failure             color.Color
}

func newTUITheme(dark bool) tuiTheme {
	return tuiTheme{
		accent:              lipgloss.LightDark(dark)(lipgloss.Color("#155E75"), lipgloss.Color("#7CC8D4")),
		heading:             lipgloss.LightDark(dark)(lipgloss.Color("#6B3FA0"), lipgloss.Color("#C4B5E8")),
		selectionText:       lipgloss.Color("#F8FAFC"),
		controls:            lipgloss.LightDark(dark)(lipgloss.Color("#374151"), lipgloss.Color("#CBD5E1")),
		selectionBackground: lipgloss.LightDark(dark)(lipgloss.Color("#245566"), lipgloss.Color("#285665")),
		text:                lipgloss.LightDark(dark)(lipgloss.Color("#292524"), lipgloss.Color("#E7E5E4")),
		notice:              lipgloss.LightDark(dark)(lipgloss.Color("#0369A1"), lipgloss.Color("#7DD3FC")),
		muted:               lipgloss.LightDark(dark)(lipgloss.Color("#596273"), lipgloss.Color("#A3ADBA")),
		success:             lipgloss.LightDark(dark)(lipgloss.Color("#047857"), lipgloss.Color("#86EFAC")),
		attention:           lipgloss.LightDark(dark)(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")),
		failure:             lipgloss.LightDark(dark)(lipgloss.Color("#B91C1C"), lipgloss.Color("#FDA4AF")),
	}
}

func tuiAccent(dark bool) color.Color {
	return newTUITheme(dark).accent
}

func tuiProgress(width int, dark bool) progress.Model {
	return progress.New(progress.WithColors(tuiAccent(dark)), progress.WithWidth(width))
}

func tuiListDelegate(dark bool) list.DefaultDelegate {
	theme := newTUITheme(dark)
	delegate := list.NewDefaultDelegate()
	delegate.Styles = list.NewDefaultItemStyles(dark)
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(theme.text)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(theme.muted)
	// While searching, other rows keep the theme instead of fixed greys.
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.Foreground(theme.muted)
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.Foreground(theme.muted)
	delegate.Styles.FilterMatch = lipgloss.NewStyle().Underline(true)
	delegate.Styles.SelectedTitle = tuiFocusStyle(dark).
		BorderStyle(lipgloss.Border{Left: "›"}).BorderLeft(true).BorderForeground(tuiAccent(dark)).PaddingLeft(1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(newTUITheme(dark).muted).PaddingLeft(2)
	return delegate
}

// tuiListStyles themes a list's search prompt and pagination.
func tuiListStyles(dark bool) list.Styles {
	theme := newTUITheme(dark)
	styles := list.DefaultStyles(dark)
	for _, state := range []*textinput.StyleState{&styles.Filter.Focused, &styles.Filter.Blurred} {
		state.Prompt = lipgloss.NewStyle().Foreground(theme.muted)
		state.Text = lipgloss.NewStyle().Foreground(theme.accent)
	}
	styles.ActivePaginationDot = lipgloss.NewStyle().Foreground(theme.accent).SetString("•")
	styles.InactivePaginationDot = lipgloss.NewStyle().Foreground(theme.muted).SetString("•")
	styles.DividerDot = lipgloss.NewStyle().Foreground(theme.muted).SetString(" • ")
	return styles
}

// themeList applies the theme to a list after list.New, which copies its
// default pagination and search styles when it builds the model.
func themeList(menu *list.Model, dark bool) {
	menu.Styles = tuiListStyles(dark)
	menu.Help.Styles = help.DefaultStyles(dark)
	menu.Paginator.ActiveDot = menu.Styles.ActivePaginationDot.String()
	menu.Paginator.InactiveDot = menu.Styles.InactivePaginationDot.String()
	menu.FilterInput.Prompt = "Search: "
	menu.FilterInput.SetStyles(menu.Styles.Filter)
}

type tuiStatusKind int

const (
	tuiStatusNeutral tuiStatusKind = iota
	tuiStatusSuccess
	tuiStatusAttention
	tuiStatusFailure
)

func tuiTitle(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).heading).Render(value)
}

func tuiError(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).failure).Render(value)
}

func tuiSection(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).text).Render(value)
}

func tuiMuted(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Foreground(newTUITheme(darkBackground).muted).Render(value)
}

func tuiSelectionMarker(selected bool, darkBackground bool) string {
	if !selected {
		return "  "
	}
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).accent).Render("› ")
}

func tuiSelection(value string, selected bool, darkBackground bool) string {
	if !selected {
		return "  " + value
	}
	style := tuiFocusStyle(darkBackground)
	return tuiSelectionMarker(true, darkBackground) + style.Render(value)
}

func tuiFocusStyle(darkBackground bool) lipgloss.Style {
	theme := newTUITheme(darkBackground)
	return lipgloss.NewStyle().Bold(true).Foreground(theme.selectionText).Background(theme.selectionBackground)
}

func tuiShortcut(value string, darkBackground bool) string {
	return lipgloss.NewStyle().Bold(true).Foreground(newTUITheme(darkBackground).controls).Render(value)
}

func newTUISpinner(darkBackground bool) spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(newTUITheme(darkBackground).accent)),
	)
}

func tuiStatus(value string, kind tuiStatusKind, darkBackground bool) string {
	theme := newTUITheme(darkBackground)
	color := theme.muted
	symbol := "○ "
	switch kind {
	case tuiStatusSuccess:
		symbol = "✓ "
		color = theme.success
	case tuiStatusAttention:
		symbol = "! "
		color = theme.attention
	case tuiStatusFailure:
		symbol = "× "
		color = theme.failure
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(symbol + value)
}

type tuiNotice struct {
	kind   tuiStatusKind
	title  string
	detail string
}

type tuiAction struct {
	key   string
	label string
}

type tuiShell struct {
	path      []string
	body      string
	fixedBody string
	notices   []tuiNotice
	actions   []tuiAction
}

type tuiShellRegions struct {
	header    string
	body      string
	fixedBody string
	notices   string
	actions   string
}

func buildTUIShellRegions(shell tuiShell, width int, darkBackground bool) tuiShellRegions {
	header := tuiTitle("Nixorium", darkBackground)
	if len(shell.path) > 0 {
		for index, part := range shell.path {
			header += tuiMuted("  /  ", darkBackground)
			if index == len(shell.path)-1 {
				header += tuiSection(part, darkBackground)
			} else {
				header += tuiMuted(part, darkBackground)
			}
		}
	}
	regions := tuiShellRegions{
		header:    header,
		body:      strings.TrimSpace(shell.body),
		fixedBody: strings.TrimSpace(shell.fixedBody),
	}
	if len(shell.notices) > 0 {
		// The status symbol and color mark the title; explanations stay in
		// ordinary text so long notices remain readable.
		lines := []string{}
		for index, notice := range shell.notices {
			if index > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, tuiNoticeTitle(notice.title, notice.kind, darkBackground))
			if notice.detail != "" {
				lines = append(lines, "  "+notice.detail)
			}
			if line := noticeNextStep(notice); line != "" {
				lines = append(lines, "  "+line)
			}
		}
		regions.notices = strings.Join(lines, "\n")
	}
	if len(shell.actions) > 0 {
		regions.actions = tuiActionBar(width, darkBackground, shell.actions...)
	}
	return regions
}

func renderTUIShell(shell tuiShell, width int, darkBackground bool) string {
	regions := buildTUIShellRegions(shell, width, darkBackground)
	lines := []string{regions.header, "", regions.body}
	for _, region := range []string{regions.fixedBody, regions.notices, regions.actions} {
		if region != "" {
			lines = append(lines, "", region)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func tuiActionBar(width int, darkBackground bool, actions ...tuiAction) string {
	theme := newTUITheme(darkBackground)
	labelStyle := lipgloss.NewStyle().Foreground(theme.muted)
	items := make([]string, 0, len(actions))
	for _, action := range actions {
		items = append(items, tuiShortcut(action.key, darkBackground)+" "+labelStyle.Render(action.label))
	}
	separator := tuiMuted("  ·  ", darkBackground)
	if width <= 0 {
		return strings.Join(items, separator)
	}
	limit := max(20, min(116, width-6))
	lines := []string{}
	line := ""
	for _, item := range items {
		if line != "" && lipgloss.Width(line)+lipgloss.Width(separator)+lipgloss.Width(item) > limit {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += separator
		}
		line += item
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func tuiResult(value string, success, darkBackground bool) string {
	kind := tuiStatusAttention
	if success {
		kind = tuiStatusSuccess
	}
	return tuiStatus(value, kind, darkBackground)
}

func tuiHelp(width int, darkBackground bool, bindings ...key.Binding) string {
	if len(bindings) > 4 {
		bindings = bindings[:4]
	}
	hasHelp := false
	for _, binding := range bindings {
		if binding.Help().Desc == "help" {
			hasHelp = true
		}
	}
	if !hasHelp {
		bindings = append(bindings, tuiHelpBinding([]string{"f1"}, "F1", "help"))
	}
	model := help.New()
	model.Styles = help.DefaultStyles(darkBackground)
	theme := newTUITheme(darkBackground)
	model.Styles.ShortKey = lipgloss.NewStyle().Bold(true).Foreground(theme.controls)
	model.Styles.ShortDesc = lipgloss.NewStyle().Foreground(theme.muted)
	model.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(theme.muted)
	if width > 0 {
		model.SetWidth(width)
	}
	return model.ShortHelpView(bindings)
}

func tuiHelpBinding(keys []string, label, description string) key.Binding {
	return key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(label, description),
	)
}

// noticeNextStep names the way forward for a recognized blocker, unless the
// notice already gives one.
func noticeNextStep(notice tuiNotice) string {
	if notice.kind == tuiStatusSuccess {
		return ""
	}
	text := notice.title + " " + notice.detail
	if strings.Contains(text, "Next:") {
		return ""
	}
	step, found := domain.NextStepFor(text)
	if !found {
		return ""
	}
	line := "Next: " + step.Action
	if step.TUI != "" {
		line += " (" + step.TUI + ")"
	}
	return line
}

func tuiNoticeText(value string, kind tuiStatusKind, dark bool) string {
	theme := newTUITheme(dark)
	tone := theme.notice
	switch kind {
	case tuiStatusAttention:
		tone = theme.attention
	case tuiStatusFailure:
		tone = theme.failure
	case tuiStatusSuccess:
		tone = theme.success
	}
	return lipgloss.NewStyle().Foreground(tone).Render(value)
}

// A focused input stays recognizable in monochrome through its prompt marker.
func tuiInputField(label, value string, focused, dark bool) string {
	text := tuiFieldValue(value, dark)
	if label != "" {
		text = tuiMuted(label+":", dark) + " " + text
	}
	return tuiSelectionMarker(focused, dark) + text
}

// Labels stay neutral; values use the same accent as the input cursor.
func tuiFieldValue(value string, dark bool) string {
	return lipgloss.NewStyle().Foreground(newTUITheme(dark).accent).Render(value)
}

// trimBlankLines drops the empty rows a fixed-height component adds above
// and below its content, so the page keeps its usual single blank lines.
func trimBlankLines(value string) string {
	lines := strings.Split(value, "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// tuiFields renders facts as aligned rows: muted labels, ordinary values.
// Values may carry their own status style.
func tuiFields(dark bool, rows ...[2]string) []string {
	width := 0
	for _, row := range rows {
		width = max(width, lipgloss.Width(row[0]))
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, tuiMuted(row[0]+strings.Repeat(" ", width-lipgloss.Width(row[0])+2), dark)+row[1])
	}
	return lines
}

func tuiFieldDetail(label, value string, dark bool) string {
	return tuiMuted(label+":", dark) + " " + tuiFieldValue(value, dark)
}

func tuiStepHeading(step, total int, title string, dark bool) string {
	return tuiMuted(fmt.Sprintf("Step %d of %d", step, total), dark) + "  " + tuiSection(title, dark)
}

func tuiInstruction(step int, title, detail string, dark bool) string {
	return tuiSection(fmt.Sprintf("%d. %s", step, title), dark) + "\n" + tuiMuted("   "+detail, dark)
}

func tuiNoticeTitle(title string, kind tuiStatusKind, dark bool) string {
	if kind != tuiStatusNeutral {
		return tuiStatus(title, kind, dark)
	}
	return tuiNoticeText("○ "+title, kind, dark)
}
