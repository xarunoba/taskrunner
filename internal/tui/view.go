package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func scrollBar(height int, scrollable bool, percent float64) string {
	lines := make([]string, max(1, height))
	if !scrollable {
		return strings.Join(lines, "\n")
	}
	thumb := int(percent*float64(len(lines)-1) + 0.5)
	for i := range lines {
		lines[i] = mutedStyle.Render("│")
	}
	lines[thumb] = accentStyle.Render("█")
	return strings.Join(lines, "\n")
}

const helpChipText = "? keybinds"

func (m model) helpChipWidth() int {
	return ansi.StringWidth(helpChipStyle.Render(helpChipText))
}

func (m model) View() string {
	base := m.baseView()
	if !m.helpOpen {
		return base
	}
	m.syncHelpViewport()
	help := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.helpViewport.View(),
		scrollBar(
			m.helpViewport.Height,
			!m.helpViewport.AtTop() || !m.helpViewport.AtBottom(),
			m.helpViewport.ScrollPercent(),
		),
	)
	if m.compactHelp() {
		return titledPanel(m.renderPanelWithFooter(help), m.helpTitle())
	}
	return overlay(base, titledPanel(helpModalStyle.Render(help), m.helpTitle()), m.width, m.height)
}

func titledPanel(panel, title string) string {
	lines := strings.Split(panel, "\n")
	width := ansi.StringWidth(lines[0])
	titleWidth := max(0, width-5)
	title = ansi.Truncate(accentStyle.Render(title), titleWidth, "…")
	fillWidth := max(0, width-ansi.StringWidth(title)-5)
	lines[0] = borderStyle.Render("╭─ ") +
		title +
		borderStyle.Render(" "+strings.Repeat("─", fillWidth)+"╮")
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
	tasksStyle := inactiveTabStyle
	jobsStyle := inactiveTabStyle
	if topLevelTab(active) == screenList {
		tasksStyle = activeTabStyle
	} else {
		jobsStyle = activeTabStyle
	}

	tabs := tasksStyle.Render("Tasks") + " " + jobsStyle.Render("Jobs")
	width := m.contentWidth() + panelStyle.GetHorizontalFrameSize()
	titleWidth := max(0, width-5)
	title := ansi.Truncate(tabs, titleWidth, "")
	if pathWidth := titleWidth - ansi.StringWidth(tabs) - 2; pathWidth > 0 {
		path := ansi.Truncate(m.store.Workspace(), pathWidth, "…")
		title += "  " + mutedStyle.Render(path)
	}
	fillWidth := max(0, width-ansi.StringWidth(title)-5)
	return borderStyle.Render("╭─ ") +
		title +
		borderStyle.Render(" "+strings.Repeat("─", fillWidth)+"╮")
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
	chip := helpChipStyle.Render(helpChipText)
	chipWidth := m.helpChipWidth()
	message := ansi.Truncate(m.status, max(1, inner-chipWidth-1), "…")
	line := " " + message
	line += strings.Repeat(" ", max(0, inner-chipWidth-ansi.StringWidth(line)))
	line += chip
	line = ansi.Truncate(line, inner, "")
	line += strings.Repeat(" ", max(0, inner-ansi.StringWidth(line)))
	style := statusBarStyle
	if m.statusError {
		style = statusBarErrorStyle
	}
	return borderStyle.Render("╰") + style.Render(line) + borderStyle.Render("╯")
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
			lines = append(lines[:height-2], mutedStyle.Render("…"), lines[len(lines)-1])
		}
	}
	return panelStyle.
		Width(width + panelStyle.GetHorizontalPadding()).
		Height(height + panelStyle.GetVerticalPadding()).
		Render(strings.Join(lines, "\n"))
}
