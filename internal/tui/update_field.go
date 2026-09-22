package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) updateFieldForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.showScreen(screenTask)
			m.taskFocus = 3
			return m, m.focusTaskControl()
		case "ctrl+s", "enter":
			return m.saveFieldForm()
		case "tab", "down":
			m.moveFieldFocus(1)
			return m, m.focusFieldControl()
		case "shift+tab", "up":
			m.moveFieldFocus(-1)
			return m, m.focusFieldControl()
		case "left", "h":
			if m.fieldFocus == 2 {
				m.fieldTypeCursor = (m.fieldTypeCursor + len(fieldTypes) - 1) % len(fieldTypes)
				return m, nil
			}
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 5 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		case "right", "l":
			if m.fieldFocus == 2 {
				m.fieldTypeCursor = (m.fieldTypeCursor + 1) % len(fieldTypes)
				return m, nil
			}
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 5 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		case " ":
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 5 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		}
	}

	if m.fieldFocus == 2 || m.fieldFocus == 4 || m.fieldFocus == 5 {
		return m, nil
	}
	inputIndex := m.fieldFocus
	if inputIndex == 3 {
		inputIndex = 2
	}
	var cmd tea.Cmd
	m.fieldInputs[inputIndex], cmd = m.fieldInputs[inputIndex].Update(msg)
	return m, cmd
}

func (m *model) openFieldForm(index int) {
	m.showScreen(screenField)
	m.editingField = index
	m.fieldFocus = 0
	m.fieldTypeCursor = 0
	m.fieldRaw = false
	m.fieldOptional = false
	m.status = ""
	for i := range m.fieldInputs {
		m.fieldInputs[i].SetValue("")
	}

	if index >= 0 {
		field := m.formFields[index]
		m.fieldInputs[0].SetValue(field.Key)
		m.fieldInputs[1].SetValue(field.Label)
		m.fieldInputs[2].SetValue(strings.Join(field.Options, ", "))
		m.fieldRaw = field.Raw
		m.fieldOptional = field.Optional
		for i, fieldType := range fieldTypes {
			if field.Type == fieldType {
				m.fieldTypeCursor = i
				break
			}
		}
	}
	m.focusFieldControl()
}

func (m *model) focusFieldControl() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.fieldInputs {
		focus := i == m.fieldFocus || i == 2 && m.fieldFocus == 3
		if focus {
			cmd = m.fieldInputs[i].Focus()
			continue
		}
		m.fieldInputs[i].Blur()
	}
	return cmd
}

func (m *model) moveFieldFocus(direction int) {
	for {
		m.fieldFocus = (m.fieldFocus + direction + 6) % 6
		if m.fieldFocus != 3 || fieldTypes[m.fieldTypeCursor] == task.FieldChoice {
			return
		}
	}
}

func (m model) saveFieldForm() (tea.Model, tea.Cmd) {
	field := task.Field{
		Key:      strings.TrimSpace(m.fieldInputs[0].Value()),
		Label:    strings.TrimSpace(m.fieldInputs[1].Value()),
		Type:     fieldTypes[m.fieldTypeCursor],
		Raw:      m.fieldRaw,
		Optional: m.fieldOptional,
	}
	if field.Type == task.FieldChoice {
		for option := range strings.SplitSeq(m.fieldInputs[2].Value(), ",") {
			field.Options = append(field.Options, strings.TrimSpace(option))
		}
	}

	fields := cloneFields(m.formFields)
	if m.editingField < 0 {
		fields = append(fields, field)
	} else {
		fields[m.editingField] = field
	}
	candidate := task.Task{Name: "validate", Command: "true", Fields: fields}
	if err := candidate.Validate(); err != nil {
		m.setError(err)
		return m, nil
	}

	m.formFields = fields
	if m.editingField < 0 {
		m.fieldCursor = len(fields) - 1
	}
	m.showScreen(screenTask)
	m.taskFocus = 3
	m.status = ""
	return m, m.focusTaskControl()
}
