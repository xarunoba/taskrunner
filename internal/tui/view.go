package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) scrollBar(height int, scrollable bool, percent float64) string {
	lines := make([]string, max(1, height))
	if !scrollable {
		return strings.Join(lines, "\n")
	}
	thumb := int(percent*float64(len(lines)-1) + 0.5)
	for i := range lines {
		lines[i] = m.styles.muted.Render("│")
	}
	lines[thumb] = m.styles.accent.Render("█")
	return strings.Join(lines, "\n")
}

const (
	settingsChipText = "s settings"
	backChipText     = "← back"
	helpChipText     = "? keybinds"
)

func (m model) canGoBack() bool {
	return m.screen != topLevelTab(m.screen)
}

func (m model) settingsChipWidth() int {
	return ansi.StringWidth(m.styles.helpChip.Render(settingsChipText))
}

func (m model) backChipWidth() int {
	return ansi.StringWidth(m.styles.helpChip.Render(backChipText))
}

func (m model) helpChipWidth() int {
	return ansi.StringWidth(m.styles.helpChip.Render(helpChipText))
}

func (m model) footerControls() string {
	settings := m.styles.helpChip.Render(settingsChipText)
	help := m.styles.helpChip.Render(helpChipText)
	if !m.canGoBack() {
		return settings + " " + help
	}
	return m.styles.helpChip.Render(backChipText) + " " + settings + " " + help
}

func (m model) View() string {
	if m.settingsOpen {
		return m.settingsView()
	}
	base := m.baseView()
	if !m.helpOpen {
		return base
	}
	m.syncHelpViewport()
	help := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.helpViewport.View(),
		m.scrollBar(
			m.helpViewport.Height,
			!m.helpViewport.AtTop() || !m.helpViewport.AtBottom(),
			m.helpViewport.ScrollPercent(),
		),
	)
	if m.compactModal() {
		return m.titledPanel(m.renderPanelWithFooter(help), m.helpTitle())
	}
	return overlay(base, m.titledPanel(m.styles.helpModal.Render(help), m.helpTitle()), m.width, m.height)
}

func (m model) titledPanel(panel, title string) string {
	lines := strings.Split(panel, "\n")
	width := ansi.StringWidth(lines[0])
	titleWidth := max(0, width-5)
	title = ansi.Truncate(m.styles.accent.Render(title), titleWidth, "…")
	fillWidth := max(0, width-ansi.StringWidth(title)-5)
	lines[0] = m.styles.border.Render("╭─ ") +
		title +
		m.styles.border.Render(" "+strings.Repeat("─", fillWidth)+"╮")
	return strings.Join(lines, "\n")
}

func (m model) baseView() string {
	switch m.screen {
	case screenTask:
		return m.taskFormView()
	case screenField:
		return m.fieldFormView()
	case screenRun:
		return m.runFormView()
	case screenJobs:
		return m.jobsView()
	case screenResult:
		return m.resultView()
	default:
		return m.listView()
	}
}

func overlay(background, foreground string, width, height int) string {
	backgroundLines := strings.Split(background, "\n")
	foregroundLines := strings.Split(foreground, "\n")
	foregroundWidth := lipgloss.Width(foreground)
	x := max(0, (width-foregroundWidth)/2)
	y := max(0, (height-len(foregroundLines))/2)
	lines := make([]string, height)
	for i := range lines {
		if i < len(backgroundLines) {
			lines[i] = ansi.Truncate(backgroundLines[i], width, "")
		}
		lines[i] += strings.Repeat(" ", max(0, width-ansi.StringWidth(lines[i])))
	}
	for i, foregroundLine := range foregroundLines {
		row := y + i
		if row >= len(lines) {
			break
		}
		left := ansi.Cut(lines[row], 0, x)
		left += strings.Repeat(" ", max(0, x-ansi.StringWidth(left)))
		rightStart := min(width, x+foregroundWidth)
		right := ansi.Cut(lines[row], rightStart, width)
		line := left + foregroundLine + right
		lines[row] = ansi.Truncate(line, width, "")
		lines[row] += strings.Repeat(" ", max(0, width-ansi.StringWidth(lines[row])))
	}
	return strings.Join(lines, "\n")
}

func topLevelTab(current screen) screen {
	switch current {
	case screenJobs, screenResult:
		return screenJobs
	default:
		return screenList
	}
}

func otherTopLevelTab(current screen) screen {
	if topLevelTab(current) == screenList {
		return screenJobs
	}
	return screenList
}

func (m *model) switchTab(target screen) {
	currentTab := topLevelTab(m.screen)
	targetTab := topLevelTab(target)
	if currentTab == targetTab {
		return
	}

	if currentTab == screenList {
		m.taskScreen = m.screen
	} else {
		m.jobScreen = m.screen
	}
	if targetTab == screenList {
		m.showScreen(m.taskScreen)
	} else {
		m.showScreen(m.jobScreen)
	}
}

func (m model) workspaceHeader(active screen) string {
	tasksStyle := m.styles.inactiveTab
	jobsStyle := m.styles.inactiveTab
	if topLevelTab(active) == screenList {
		tasksStyle = m.styles.activeTab
	} else {
		jobsStyle = m.styles.activeTab
	}

	tabs := tasksStyle.Render("Tasks") + " " + jobsStyle.Render("Jobs")
	width := m.contentWidth() + m.styles.panel.GetHorizontalFrameSize()
	titleWidth := max(0, width-5)
	title := ansi.Truncate(tabs, titleWidth, "")
	if pathWidth := titleWidth - ansi.StringWidth(tabs) - 2; pathWidth > 0 {
		path := ansi.Truncate(m.store.Workspace(), pathWidth, "…")
		title += "  " + m.styles.muted.Render(path)
	}
	fillWidth := max(0, width-ansi.StringWidth(title)-5)
	return m.styles.border.Render("╭─ ") +
		title +
		m.styles.border.Render(" "+strings.Repeat("─", fillWidth)+"╮")
}

func (m model) renderWorkspacePanel(content string, active screen) string {
	panel := m.renderPanelWithFooter(content)
	_, rest, ok := strings.Cut(panel, "\n")
	view := m.workspaceHeader(active)
	if ok {
		view += "\n" + rest
	}
	return view
}

func visibleRange(total, current, limit int) (int, int) {
	if total <= limit {
		return 0, total
	}
	start := max(current-limit/2, 0)
	if start+limit > total {
		start = total - limit
	}
	return start, start + limit
}

func (m model) statusBar() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	inner := max(1, width-2)
	controls := m.footerControls()
	controlsWidth := ansi.StringWidth(controls)
	messageWidth := max(0, inner-controlsWidth-1)
	message := ansi.Truncate(m.status, messageWidth, "…")
	line := ""
	if messageWidth > 0 {
		line = " " + message
	}
	line += strings.Repeat(" ", max(0, inner-controlsWidth-ansi.StringWidth(line)))
	line += controls
	line = ansi.Truncate(line, inner, "")
	line += strings.Repeat(" ", max(0, inner-ansi.StringWidth(line)))
	style := m.styles.statusBar
	if m.statusError {
		style = m.styles.statusBarError
	}
	return m.styles.border.Render("╰") + style.Render(line) + m.styles.border.Render("╯")
}

func (m model) renderPanelWithFooter(content string) string {
	lines := strings.Split(m.renderPanel(content), "\n")
	lines[len(lines)-1] = m.statusBar()
	return strings.Join(lines, "\n")
}

func (m model) renderPanel(content string) string {
	width := m.contentWidth()
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}

	height := m.contentHeight()
	if len(lines) > height {
		switch height {
		case 1:
			lines = lines[:1]
		case 2:
			lines = []string{lines[0], lines[len(lines)-1]}
		default:
			lines = append(lines[:height-2], m.styles.muted.Render("…"), lines[len(lines)-1])
		}
	}
	return m.styles.panel.
		Width(width + m.styles.panel.GetHorizontalPadding()).
		Height(height + m.styles.panel.GetVerticalPadding()).
		Render(strings.Join(lines, "\n"))
}
