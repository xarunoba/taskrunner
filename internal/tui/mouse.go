package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	event := tea.MouseEvent(msg)
	if event.IsWheel() {
		if m.settingsOpen {
			return m, nil
		}
		if m.helpOpen {
			return m.updateHelp(msg)
		}
		if m.screen == screenResult {
			return m.updateResult(msg)
		}
		keyType := tea.KeyDown
		if event.Button == tea.MouseButtonWheelUp {
			keyType = tea.KeyUp
		}
		key := tea.KeyMsg{Type: keyType}
		switch m.screen {
		case screenTask:
			return m.updateTaskForm(key)
		case screenField:
			return m.updateFieldForm(key)
		case screenRun:
			return m.updateRunForm(key)
		case screenJobs:
			return m.updateJobs(key)
		default:
			return m.updateList(key)
		}
	}
	if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft {
		return m, nil
	}
	layout := m.footerLayout()
	height := m.height
	if height <= 0 {
		height = 40
	}
	for i := range layout.count {
		button := layout.buttons[i]
		if event.Y != height-layout.rows+button.row || event.X < button.x || event.X >= button.x+button.width {
			continue
		}
		switch button.key {
		case "esc":
			if m.helpOpen || m.settingsOpen {
				m.helpOpen = false
				m.settingsOpen = false
				return m, nil
			}
			return m.goBack()
		case "s", "f4":
			return m.openSettings()
		case "?", "f1":
			m.openHelp()
			return m, nil
		}
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if m.settingsOpen {
		return m.clickSettings(lines, event.Y, event.X)
	}
	if m.helpOpen {
		return m, nil
	}
	if screen, ok := topLevelTabAt(lines, event.Y, event.X); ok {
		m.switchTab(screen)
		return m, nil
	}

	left := m.styles.panel.GetBorderLeftSize() + m.styles.panel.GetPaddingLeft()
	top := m.styles.panel.GetBorderTopSize() + m.styles.panel.GetPaddingTop()
	row := event.Y - top
	if row < 0 || row >= m.contentHeight() || event.X < left || event.X >= left+m.contentWidth()-1 {
		return m, nil
	}
	line := strings.TrimSpace(ansi.Cut(lines[event.Y], left, left+m.contentWidth()-1))
	switch m.screen {
	case screenTask:
		return m.clickTaskForm(line, event.X)
	case screenField:
		return m.clickFieldForm(row, event.X)
	case screenRun:
		return m.clickRunForm(line, event.X, row)
	case screenJobs:
		return m.clickJobs(row)
	case screenResult:
		return m, nil
	default:
		return m.clickTaskList(row)
	}
}

func (m model) goBack() (tea.Model, tea.Cmd) {
	key := tea.KeyMsg{Type: tea.KeyEsc}
	switch m.screen {
	case screenTask:
		return m.updateTaskForm(key)
	case screenField:
		return m.updateFieldForm(key)
	case screenRun:
		return m.updateRunForm(key)
	case screenResult:
		return m.updateResult(key)
	default:
		return m, nil
	}
}

// listItemAtRow uses the same item window and expanded detail rows as rendering.
func (m model) listItemAtRow(total, current, detailRows, row int) int {
	start, end := m.listRange(total, current, detailRows)
	for i := start; i < end; i++ {
		if row == 0 {
			return i
		}
		row--
		if i == current {
			row -= min(detailRows, m.contentHeight()-1)
		}
		if row < 0 {
			break
		}
	}
	return -1
}

func (m model) clickTaskList(row int) (tea.Model, tea.Cmd) {
	i := m.listItemAtRow(len(m.tasks), m.cursor, 2, row)
	if i < 0 {
		return m, nil
	}
	if m.cursor == i {
		return m.startRun(m.tasks[i])
	}
	m.cursor = i
	return m, nil
}

func (m model) clickJobs(row int) (tea.Model, tea.Cmd) {
	i := m.listItemAtRow(len(m.jobs), m.jobCursor, 1, row)
	if i < 0 {
		return m, nil
	}
	if m.jobCursor == i {
		return m, m.openJob(m.jobs[len(m.jobs)-1-i].ID)
	}
	m.jobCursor = i
	return m, nil
}

// tabRegionWidth is the fixed rendered column width of "╭─ " plus the
// padded "Tasks" and "Jobs" tab labels and their separator.
var tabRegionWidth = lipgloss.Width("╭─ ") + lipgloss.Width(" Tasks ") + 1 + lipgloss.Width(" Jobs ")

func topLevelTabAt(lines []string, y, x int) (screen, bool) {
	if y != 0 || len(lines) == 0 {
		return 0, false
	}
	// Compare in terminal columns: rune offsets match columns here, byte
	// offsets from strings.Index would shift after the multi-byte border.
	runes := []rune(lines[0])
	if len(runes) > tabRegionWidth {
		runes = runes[:tabRegionWidth]
	}
	for _, tab := range []struct {
		label  string
		screen screen
	}{
		{label: "Tasks", screen: screenList},
		{label: "Jobs", screen: screenJobs},
	} {
		start := runeIndex(runes, tab.label)
		if start >= 0 && x >= start && x < start+len(tab.label) {
			return tab.screen, true
		}
	}
	return 0, false
}

// runeIndex returns the first rune offset of s in r, or -1.
func runeIndex(r []rune, s string) int {
	target := []rune(s)
	for i := 0; i+len(target) <= len(r); i++ {
		match := true
		for j := range target {
			if r[i+j] != target[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func (m model) clickTaskForm(line string, x int) (tea.Model, tea.Cmd) {
	switch {
	case strings.HasPrefix(line, "Name") || containsNonEmpty(line, m.taskNameInput.Value()):
		m.taskFocus = 0
		m.taskNameInput.SetCursor(max(0, x-3))
		return m, m.focusTaskControl()
	case strings.HasPrefix(line, "Fields") || strings.Contains(line, "No fields."):
		m.taskFocus = 1
		return m, m.focusTaskControl()
	case strings.HasPrefix(line, "Command") || containsValueLine(line, m.taskCommandInput.Value()):
		m.taskFocus = 2
		return m, m.focusTaskControl()
	case strings.HasPrefix(line, "Job policy"):
		m.taskFocus = 3
		m.moveJobPolicy(1)
		return m, m.focusTaskControl()
	case strings.Contains(line, "Sequential") || strings.Contains(line, "Parallel") || strings.Contains(line, "Cancel previous"):
		if selected := optionAtX(line, x-3, jobPolicyLabels); selected >= 0 {
			m.formJobPolicy = jobPolicies[selected]
			m.taskFocus = 3
			return m, m.focusTaskControl()
		}
	}
	for i, field := range m.formFields {
		if strings.Contains(line, field.Label) {
			m.taskFocus = 1
			m.fieldCursor = i
			return m, m.focusTaskControl()
		}
	}
	return m, nil
}

func (m model) clickFieldForm(row, x int) (tea.Model, tea.Cmd) {
	lines, start, end, _, controls := m.fieldFormLayout()
	contentRow := start + row
	if contentRow >= end {
		return m, nil
	}
	focus := -1
	for i, control := range controls {
		if contentRow >= control.start && contentRow < control.end {
			focus = i
			break
		}
	}
	if focus < 0 {
		return m, nil
	}
	m.fieldFocus = focus
	column := x - m.styles.panel.GetBorderLeftSize() - m.styles.panel.GetPaddingLeft()
	line := ansi.Strip(lines[contentRow])
	switch focus {
	case 0:
		m.fieldInputs[0].SetCursor(max(0, column))
	case 1:
		m.fieldInputs[1].SetCursor(max(0, column))
	case 2:
		if selected := optionAtX(line, column, fieldTypeLabels); selected >= 0 {
			m.fieldTypeCursor = selected
		}
	case 3:
		if fieldTypes[m.fieldTypeCursor] == task.FieldRefer {
			if selected := optionAtX(line, column, m.referSourceKeys()); selected >= 0 {
				m.fieldFromCursor = selected
			}
		} else {
			m.fieldInputs[2].SetCursor(max(0, column))
		}
	case 4:
		if selected := optionAtX(line, column, []string{"Argument", "Raw"}); selected >= 0 {
			m.fieldRaw = selected == 1
		}
	case 5:
		m.fieldInputs[3].SetCursor(max(0, column))
	case 6:
		m.fieldInputs[4].SetCursor(max(0, column))
	case 7:
		if selected := optionAtX(line, column, []string{"Required", "Optional"}); selected >= 0 {
			m.fieldOptional = selected == 1
		}
	}
	return m, m.focusFieldControl()
}

func (m model) clickRunForm(line string, x, row int) (tea.Model, tea.Cmd) {
	field := m.runTask.Fields[m.runIndex]

	switch field.Type {
	case task.FieldText:
		m.runInput.SetCursor(max(0, x-3))
		return m, m.runInput.Focus()
	case task.FieldChoice:
		if index, ok := m.runChoiceRowAt(row); ok {
			m.choiceCursor = index
		}
	case task.FieldFile:
		if m.runFileSkipHit(row) {
			m.runValues[field.Key] = ""
			return m.advanceRun()
		}
		pickerRow := row - strings.Count(m.runFormHeader(field), "\n")
		if pickerRow < 0 || pickerRow >= min(m.fileCount, m.filePicker.Height) {
			return m, nil
		}
		m.choiceCursor = 0
		move := m.runFileClickMove(row)
		key := tea.KeyDown
		if move < 0 {
			key = tea.KeyUp
			move = -move
		}
		var cmds []tea.Cmd
		for range move {
			var cmd tea.Cmd
			m.filePicker, cmd = m.filePicker.Update(tea.KeyMsg{Type: key})
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	case task.FieldConfirm:
		if selected := optionAtX(line, x-3, []string{"No", "Yes"}); selected >= 0 {
			m.confirmationYes = selected == 1
		}
	}
	return m, nil
}

func containsNonEmpty(text, value string) bool {
	return value != "" && strings.Contains(text, value)
}

func containsValueLine(text, value string) bool {
	for line := range strings.SplitSeq(value, "\n") {
		if containsNonEmpty(text, line) {
			return true
		}
	}
	return false
}

func optionAtX(line string, x int, options []string) int {
	for i, option := range options {
		before, _, ok := strings.Cut(line, option)
		if !ok {
			continue
		}
		startColumn := ansi.StringWidth(before)
		endColumn := startColumn + ansi.StringWidth(option)
		if x >= startColumn-2 && x <= endColumn+2 {
			return i
		}
	}
	return -1
}
