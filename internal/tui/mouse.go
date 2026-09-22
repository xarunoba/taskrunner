package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	event := tea.MouseEvent(msg)
	if event.IsWheel() {
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
	if screen, ok := m.topLevelTabAt(event.Y, event.X); ok {
		m.switchTab(screen)
		return m, nil
	}

	line := m.mouseLine(event.Y)
	if m.mouseLineIsLastMatch(event.Y, "keybinds") {
		switch optionAtX(line, event.X-3, m.footerControls()) {
		case 0:
			m.switchTab(otherTopLevelTab(m.screen))
			return m, nil
		case 1:
			return m, tea.Quit
		case 2:
			m.openHelp()
			return m, nil
		}
	}
	switch m.screen {
	case screenTask:
		return m.clickTaskForm(line, event.X)
	case screenField:
		return m.clickFieldForm(line, event.X)
	case screenRun:
		return m.clickRunForm(line, event.X)
	case screenJobs:
		return m.clickJobs(line, event.X)
	case screenResult:
		return m, nil
	default:
		return m.clickTaskList(line, event.X)
	}
}

func (m model) mouseLine(y int) string {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
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

func (m model) mouseLineIsLastMatch(y int, text string) bool {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], text) {
			return y == i
		}
	}
	return false
}

func (m model) clickTaskList(line string, x int) (tea.Model, tea.Cmd) {
	if strings.Contains(line, "enter run") {
		switch optionAtX(line, x-3, []string{"enter run", "n new", "e edit", "d delete", "q quit"}) {
		case 0:
			return m.updateList(tea.KeyMsg{Type: tea.KeyEnter})
		case 1:
			return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		case 2:
			return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
		case 3:
			return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		case 4:
			return m, tea.Quit
		}
	}
	for i, item := range m.tasks {
		name := ansi.Truncate(item.Name, max(1, m.contentWidth()-2), "…")
		if strings.Contains(line, name) {
			m.cursor = i
			return m, nil
		}
	}
	return m, nil
}

func (m model) clickJobs(line string, x int) (tea.Model, tea.Cmd) {
	if strings.Contains(line, "enter log") {
		switch optionAtX(line, x-3, []string{"enter log", "c cancel", "r rerun", "d delete job", "q quit"}) {
		case 0:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyEnter})
		case 1:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		case 2:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		case 3:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		case 4:
			return m, tea.Quit
		}
	}
	for i := range m.jobs {
		job := m.jobs[len(m.jobs)-1-i]
		if !strings.Contains(line, job.ShortID()) {
			continue
		}
		if m.jobCursor == i {
			return m, m.openJob(job.ID)
		}
		m.jobCursor = i
		return m, nil
	}
	return m, nil
}

func (m model) topLevelTabAt(y, x int) (screen, bool) {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if y < 0 || y >= len(lines) {
		return 0, false
	}
	line := lines[y]
	for _, tab := range []struct {
		label  string
		screen screen
	}{
		{label: "Tasks", screen: screenList},
		{label: "Jobs", screen: screenJobs},
	} {
		start := strings.Index(line, tab.label)
		if start >= 0 && x >= start && x < start+len(tab.label) {
			return tab.screen, true
		}
	}
	return 0, false
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
	case line == "Interpolation":
		m.fieldFocus = 4
	case line == "Requirement":
		m.fieldFocus = 5
	default:
		typeNames := make([]string, len(fieldTypes))
		for i, fieldType := range fieldTypes {
			typeNames[i] = string(fieldType)
		}
		if selected := optionAtX(line, x-3, typeNames); selected >= 0 {
			m.fieldFocus = 2
			m.fieldTypeCursor = selected
		} else if selected := optionAtX(line, x-3, []string{"Argument", "Raw"}); selected >= 0 {
			m.fieldFocus = 4
			m.fieldRaw = selected == 1
		} else if selected := optionAtX(line, x-3, []string{"Required", "Optional"}); selected >= 0 {
			m.fieldFocus = 5
			m.fieldOptional = selected == 1
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
		if field.Optional && strings.Contains(line, "Skip") {
			m.choiceCursor = -1
			break
		}
		for i, option := range field.Options {
			if strings.Contains(line, task.ResolveKnownValues(option, m.runValues)) {
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
		start := strings.Index(line, option)
		if start < 0 {
			continue
		}
		if x >= start-2 && x <= start+len(option)+2 {
			return i
		}
	}
	return -1
}
