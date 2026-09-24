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

// withScrollBar reserves the final column even when all content fits.
func (m model) withScrollBar(content string, width, height int, scrollable bool, percent float64) string {
	contentWidth := max(0, width-1)
	lines := strings.SplitN(content, "\n", max(1, height)+1)
	lines = lines[:min(len(lines), max(1, height))]
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, contentWidth, "…")
	}
	body := lipgloss.NewStyle().Width(contentWidth).Height(max(1, height)).
		Render(strings.Join(lines, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, body,
		m.scrollBar(height, scrollable, percent))
}

func (m model) listRange(total, current, detailRows int) (int, int) {
	return visibleRange(total, current, max(1, m.contentHeight()-detailRows))
}

func (m model) listWithScrollBar(content string, start, end, total int) string {
	hidden := total - (end - start)
	percent := 0.0
	if hidden > 0 {
		percent = float64(start) / float64(hidden)
	}
	return m.withScrollBar(content, m.contentWidth(), m.contentHeight(), hidden > 0, percent)
}

type footerButton struct {
	key, descriptor string
	row, x, width   int
}

type footerLayout struct {
	buttons [3]footerButton
	count   int
	rows    int
	width   int
}

func (m model) canGoBack() bool {
	return m.screen != topLevelTab(m.screen)
}

// footerLayout is shared by rendering, content sizing, and mouse hit-testing.
func (m model) footerLayout() footerLayout {
	width := m.width
	if width <= 0 {
		width = 80
	}
	layout := footerLayout{width: max(1, width-2), rows: 1}
	if m.helpOpen || m.settingsOpen {
		layout.buttons[0] = footerButton{key: "esc", descriptor: "close"}
		layout.count = 1
	} else {
		if m.canGoBack() {
			layout.buttons[0] = footerButton{key: "esc", descriptor: "back"}
			if m.standaloneForm && m.screen == screenTask {
				layout.buttons[0].descriptor = "cancel"
			}
			layout.count++
		}
		settingsKey, helpKey := "s", "?"
		if m.acceptsTextInput() {
			settingsKey, helpKey = "f4", "f1"
		}
		layout.buttons[layout.count] = footerButton{key: settingsKey, descriptor: "settings"}
		layout.buttons[layout.count+1] = footerButton{key: helpKey, descriptor: "keybinds"}
		layout.count += 2
	}
	var rowWidths [3]int
	for i := range layout.count {
		button := &layout.buttons[i]
		button.width = min(layout.width, ansi.StringWidth(button.key)+1+ansi.StringWidth(button.descriptor)+m.styles.helpChip.GetHorizontalFrameSize())
		row := layout.rows - 1
		space := 0
		if rowWidths[row] > 0 {
			space = 1
		}
		if rowWidths[row]+space+button.width > layout.width {
			layout.rows++
			row++
			space = 0
		}
		button.row = row
		button.x = rowWidths[row] + space
		rowWidths[row] += space + button.width
	}
	for i := range layout.count {
		button := &layout.buttons[i]
		button.x += 1 + layout.width - rowWidths[button.row]
	}
	return layout
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
	layout := m.footerLayout()
	style := m.styles.statusBar
	if m.statusError {
		style = m.styles.statusBarError
	}
	var body strings.Builder
	for row := range layout.rows {
		var line strings.Builder
		column := 1
		for i := range layout.count {
			button := layout.buttons[i]
			if button.row != row {
				continue
			}
			if row == 0 && line.Len() == 0 {
				message := ansi.Truncate(m.status, max(0, button.x-3), "…")
				if message != "" {
					line.WriteByte(' ')
					line.WriteString(message)
					column += 1 + ansi.StringWidth(message)
				}
			}
			line.WriteString(strings.Repeat(" ", max(0, button.x-column)))
			label := m.styles.helpChip.Render(button.key + " " + button.descriptor)
			line.WriteString(ansi.Truncate(label, button.width, "…"))
			column = button.x + button.width
		}
		line.WriteString(strings.Repeat(" ", max(0, layout.width+1-column)))
		left, right := "│", "│"
		if row == layout.rows-1 {
			left, right = "╰", "╯"
		}
		if row > 0 {
			body.WriteByte('\n')
		}
		body.WriteString(m.styles.border.Render(left))
		body.WriteString(style.Render(line.String()))
		body.WriteString(m.styles.border.Render(right))
	}
	return body.String()
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
