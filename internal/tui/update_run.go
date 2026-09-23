package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) startRun(item task.Task) (tea.Model, tea.Cmd) {
	m.runTask = item
	m.runIndex = 0
	m.runValues = make(map[string]string, len(item.Fields))
	m.runHistoryIndex = -1
	m.status = ""
	m.skipReferFields()
	if m.runIndex == len(m.runTask.Fields) {
		return m.executeRun()
	}
	m.showScreen(screenRun)
	return m, m.prepareRunField()
}

// skipReferFields derives values for refer fields without prompting. It
// advances the cursor to the next field that needs user input.
func (m *model) skipReferFields() {
	for m.runIndex < len(m.runTask.Fields) {
		field := m.runTask.Fields[m.runIndex]
		if field.Type != task.FieldRefer {
			return
		}
		m.runValues[field.Key] = m.runValues[field.From]
		m.runIndex++
	}
}

func (m *model) prepareRunField() tea.Cmd {
	field := m.runTask.Fields[m.runIndex]
	m.choiceCursor = 0
	if field.Optional && m.runValues[field.Key] == "" {
		m.choiceCursor = -1
	}
	m.confirmationYes = m.runValues[field.Key] == strconv.FormatBool(true)

	switch field.Type {
	case task.FieldText:
		m.runHistory = nil
		if m.runTask.File != "" {
			history, err := m.store.ValueHistory(m.runTask.File, field.Key)
			if err != nil {
				m.setError(fmt.Errorf("load value history: %w", err))
			} else {
				m.runHistory = history
			}
		}
		m.runHistoryIndex = -1
		m.runHistoryDraft = m.runValues[field.Key]
		m.runInput = newInput(task.ResolveKnownValues(field.Label, m.runValues), 1000)
		m.runInput.SetValue(m.runHistoryDraft)
		m.runInput.Width = m.contentWidth()
		return m.runInput.Focus()
	case task.FieldChoice:
		for i, option := range field.Options {
			if task.ResolveKnownValues(option, m.runValues) == m.runValues[field.Key] {
				m.choiceCursor = i
				break
			}
		}
	case task.FieldFile:
		m.filePicker = filepicker.New()
		m.filePicker.CurrentDirectory = m.store.Workspace()
		m.filePicker.ShowHidden = true
		m.filePicker.FileAllowed = true
		m.filePicker.DirAllowed = false
		m.filePicker.Styles.Cursor = m.styles.cursor
		m.filePicker.Styles.Selected = m.styles.selected
		m.filePicker.SetHeight(m.filePickerHeight())
		return m.filePicker.Init()
	}
	return nil
}

func (m model) updateRunForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.showScreen(screenList)
			m.setStatus(fmt.Sprintf("Cancelled %s", m.runTask.Name))
			return m, nil
		case "shift+tab", "ctrl+left":
			return m.previousRun()
		}
	}

	field := m.runTask.Fields[m.runIndex]
	switch field.Type {
	case task.FieldText:
		return m.updateRunText(msg, field)
	case task.FieldChoice:
		return m.updateRunChoice(msg, field)
	case task.FieldFile:
		return m.updateRunFile(msg, field)
	case task.FieldConfirm:
		return m.updateRunConfirm(msg, field)
	default:
		m.showScreen(screenList)
		m.setError(fmt.Errorf("unknown field type %q", field.Type))
		return m, nil
	}
}

func (m model) updateRunText(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up":
			m.moveRunHistory(-1)
			return m, nil
		case "down":
			m.moveRunHistory(1)
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.runInput.Value())
			if value == "" && !field.Optional {
				m.setError(fmt.Errorf("%s is required", task.ResolveKnownValues(field.Label, m.runValues)))
				return m, nil
			}
			m.runValues[field.Key] = value
			return m.advanceRun()
		}
	}
	var cmd tea.Cmd
	m.runInput, cmd = m.runInput.Update(msg)
	return m, cmd
}

func (m model) updateRunChoice(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		minimum := 0
		if field.Optional {
			minimum = -1
		}
		if m.choiceCursor > minimum {
			m.choiceCursor--
		}
	case "down", "j":
		if m.choiceCursor < len(field.Options)-1 {
			m.choiceCursor++
		}
	case "enter":
		value := ""
		if m.choiceCursor >= 0 {
			value = task.ResolveKnownValues(field.Options[m.choiceCursor], m.runValues)
		}
		m.runValues[field.Key] = value
		return m.advanceRun()
	}
	return m, nil
}

func (m model) updateRunFile(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	if field.Optional {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "enter":
				if m.choiceCursor < 0 {
					m.runValues[field.Key] = ""
					return m.advanceRun()
				}
			case "down", "j":
				if m.choiceCursor < 0 {
					m.choiceCursor = 0
					return m, nil
				}
			}
		}
	}
	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)
	if selected, path := m.filePicker.DidSelectFile(msg); selected {
		m.runValues[field.Key] = path
		return m.advanceRun()
	}
	if disabled, path := m.filePicker.DidSelectDisabledFile(msg); disabled {
		m.setError(fmt.Errorf("cannot select %s", path))
	}
	return m, cmd
}

func (m model) updateRunConfirm(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "left", "right", "h", "l", "tab":
		m.confirmationYes = !m.confirmationYes
	case "y":
		m.confirmationYes = true
	case "n":
		m.confirmationYes = false
	case "enter":
		if !m.confirmationYes && !field.Optional {
			m.showScreen(screenList)
			m.setStatus(fmt.Sprintf("Cancelled %s", m.runTask.Name))
			return m, nil
		}
		m.runValues[field.Key] = strconv.FormatBool(m.confirmationYes)
		return m.advanceRun()
	}
	return m, nil
}

func (m *model) moveRunHistory(direction int) {
	if len(m.runHistory) == 0 {
		return
	}
	if direction < 0 {
		if m.runHistoryIndex == -1 {
			m.runHistoryDraft = m.runInput.Value()
		}
		if m.runHistoryIndex < len(m.runHistory)-1 {
			m.runHistoryIndex++
		}
		m.runInput.SetValue(m.runHistory[m.runHistoryIndex])
		return
	}
	if m.runHistoryIndex < 0 {
		return
	}
	m.runHistoryIndex--
	if m.runHistoryIndex == -1 {
		m.runInput.SetValue(m.runHistoryDraft)
		return
	}
	m.runInput.SetValue(m.runHistory[m.runHistoryIndex])
}

func (m model) advanceRun() (tea.Model, tea.Cmd) {
	m.runIndex++
	m.status = ""
	m.skipReferFields()
	if m.runIndex == len(m.runTask.Fields) {
		return m.executeRun()
	}
	return m, m.prepareRunField()
}

func (m model) previousRun() (tea.Model, tea.Cmd) {
	if m.runIndex == 0 {
		return m, nil
	}

	field := m.runTask.Fields[m.runIndex]
	switch field.Type {
	case task.FieldText:
		value := strings.TrimSpace(m.runInput.Value())
		if value != "" || field.Optional {
			m.runValues[field.Key] = value
		}
	case task.FieldChoice:
		value := ""
		if m.choiceCursor >= 0 {
			value = task.ResolveKnownValues(field.Options[m.choiceCursor], m.runValues)
		}
		m.runValues[field.Key] = value
	case task.FieldConfirm:
		m.runValues[field.Key] = strconv.FormatBool(m.confirmationYes)
	}

	m.runIndex--
	for m.runIndex > 0 && m.runTask.Fields[m.runIndex].Type == task.FieldRefer {
		m.runIndex--
	}
	m.status = ""
	return m, m.prepareRunField()
}

func (m model) executeRun() (tea.Model, tea.Cmd) {
	command, err := m.runTask.Render(m.runValues)
	if err != nil {
		m.showScreen(screenList)
		m.setError(err)
		return m, nil
	}
	m.showScreen(screenList)
	m.status = ""
	m.statusError = false
	if m.runTask.File != "" {
		if err := m.store.RecordValueHistory(m.runTask, m.runValues); err != nil {
			m.setError(fmt.Errorf("running %s; save value history: %w", m.runTask.Name, err))
		}
	}
	client := m.daemon
	item := m.runTask
	return m, func() tea.Msg {
		job, err := client.Start(item.File, item.Name, command, item.JobPolicy)
		if err != nil {
			err = fmt.Errorf("start %s: %w", item.Name, err)
		}
		return taskStartedMsg{job: job, err: err}
	}
}
