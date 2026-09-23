package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	event := tea.MouseEvent(msg)
	if event.IsWheel() {
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
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if mouseLineIsLastMatch(lines, event.Y, helpChipText) {
		if start, end := m.helpChipColumns(); event.X >= start && event.X < end {
			if m.helpOpen {
				m.helpOpen = false
			} else {
				m.openHelp()
			}
			return m, nil
		}
	}
	if m.helpOpen {
		return m, nil
	}
	if mouseLineIsLastMatch(lines, event.Y, backChipText) {
		if start, end, ok := m.backChipColumns(); ok && event.X >= start && event.X < end {
			return m.goBack()
		}
	}
	if screen, ok := topLevelTabAt(lines, event.Y, event.X); ok {
		m.switchTab(screen)
		return m, nil
	}

	line := mouseLine(lines, event.Y)
	switch m.screen {
	case screenTask:
		return m.clickTaskForm(line, event.X)
	case screenField:
		return m.clickFieldForm(line, event.X)
	case screenRun:
		return m.clickRunForm(line, event.X)
	case screenJobs:
		return m.clickJobs(line)
	case screenResult:
		return m, nil
	default:
		return m.clickTaskList(line)
	}
}

func mouseLine(lines []string, y int) string {
	if y < 0 || y >= len(lines) {
		return ""
	}
	line := strings.TrimSpace(lines[y])
	line = strings.TrimPrefix(line, "│")
	line = strings.TrimSuffix(line, "│")
	line = strings.TrimPrefix(line, "╰─ ")
	if border := strings.LastIndex(line, " ─"); border >= 0 {
		line = line[:border]
	}
	return strings.TrimSpace(line)
}

func mouseLineIsLastMatch(lines []string, y int, text string) bool {
	for i, line := range slices.Backward(lines) {
		if strings.Contains(line, text) {
			return y == i
		}
	}
	return false
}

func (m model) helpChipColumns() (int, int) {
	width := m.width
	if width <= 0 {
		width = 80
	}
	end := width - 1
	start := max(1, end-m.helpChipWidth())
	return start, end
}

func (m model) backChipColumns() (int, int, bool) {
	if !m.canGoBack() {
		return 0, 0, false
	}
	helpStart, _ := m.helpChipColumns()
	end := helpStart - 1
	start := max(1, end-m.backChipWidth())
	return start, end, true
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

func (m model) clickTaskList(line string) (tea.Model, tea.Cmd) {
	for i, item := range m.tasks {
		row := item.Name + m.taskActivity(item.File)
		if line != row && line != "› "+row {
			continue
		}
		if m.cursor == i {
			return m.startRun(item)
		}
		m.cursor = i
		return m, nil
	}
	return m, nil
}

func (m model) clickJobs(line string) (tea.Model, tea.Cmd) {
	for i := range m.jobs {
		job := m.jobs[len(m.jobs)-1-i]
		row := fmt.Sprintf("%-9s %s  %s", strings.ToUpper(string(job.Status)), job.Name, job.ShortID())
		if line == row || line == "› "+row {
			if m.jobCursor == i {
				return m, m.openJob(job.ID)
			}
			m.jobCursor = i
			return m, nil
		}
	}
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

func (m model) clickFieldForm(line string, x int) (tea.Model, tea.Cmd) {
	switch {
	case line == "Key" || containsNonEmpty(line, m.fieldInputs[0].Value()):
		m.fieldFocus = 0
		m.fieldInputs[0].SetCursor(max(0, x-3))
	case line == "Label" || containsNonEmpty(line, m.fieldInputs[1].Value()):
		m.fieldFocus = 1
		m.fieldInputs[1].SetCursor(max(0, x-3))
	case line == "Type":
		m.fieldFocus = 2
	case strings.Contains(line, "Options (comma-separated)") || containsNonEmpty(line, m.fieldInputs[2].Value()):
		m.fieldFocus = 3
		m.fieldInputs[2].SetCursor(max(0, x-3))
	case strings.Contains(line, "From (earlier field)"):
		m.fieldFocus = 3
	case line == "Interpolation":
		m.fieldFocus = 4
	case line == "Prefix" || containsNonEmpty(line, m.fieldInputs[3].Value()):
		m.fieldFocus = 5
		m.fieldInputs[3].SetCursor(max(0, x-3))
	case line == "Suffix" || containsNonEmpty(line, m.fieldInputs[4].Value()):
		m.fieldFocus = 6
		m.fieldInputs[4].SetCursor(max(0, x-3))
	case line == "Requirement":
		m.fieldFocus = 7
	default:
		if selected := optionAtX(line, x-3, fieldTypeLabels); selected >= 0 {
			m.fieldFocus = 2
			m.fieldTypeCursor = selected
		} else if selected := optionAtX(line, x-3, []string{"Argument", "Raw"}); selected >= 0 {
			m.fieldFocus = 4
			m.fieldRaw = selected == 1
		} else if selected := optionAtX(line, x-3, []string{"Required", "Optional"}); selected >= 0 {
			m.fieldFocus = 7
			m.fieldOptional = selected == 1
		} else if fieldTypes[m.fieldTypeCursor] == task.FieldRefer {
			if selected := optionAtX(line, x-3, m.referSourceKeys()); selected >= 0 {
				m.fieldFocus = 3
				m.fieldFromCursor = selected
			}
		}
	}
	return m, m.focusFieldControl()
}

func (m model) clickRunForm(line string, x int) (tea.Model, tea.Cmd) {
	field := m.runTask.Fields[m.runIndex]
	switch field.Type {
	case task.FieldText:
		m.runInput.SetCursor(max(0, x-3))
		return m, m.runInput.Focus()
	case task.FieldChoice:
		if field.Optional && (strings.HasPrefix(line, "› Skip") || line == "Skip") {
			m.choiceCursor = -1
			break
		}
		for i, option := range field.Options {
			resolved := task.ResolveKnownValues(option, m.runValues)
			if line == resolved || line == "› "+resolved {
				m.choiceCursor = i
				break
			}
		}
	case task.FieldFile:
		if field.Optional && strings.Contains(line, "skip") {
			m.runValues[field.Key] = ""
			return m.advanceRun()
		}
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
