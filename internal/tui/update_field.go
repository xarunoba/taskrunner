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
			m.taskFocus = 1
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
			if m.fieldFocus == 3 && fieldTypes[m.fieldTypeCursor] == task.FieldRefer {
				if keys := m.referSourceKeys(); len(keys) > 0 {
					m.fieldFromCursor = (m.fieldFromCursor + len(keys) - 1) % len(keys)
				}
				return m, nil
			}
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 7 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		case "right", "l":
			if m.fieldFocus == 2 {
				m.fieldTypeCursor = (m.fieldTypeCursor + 1) % len(fieldTypes)
				return m, nil
			}
			if m.fieldFocus == 3 && fieldTypes[m.fieldTypeCursor] == task.FieldRefer {
				if keys := m.referSourceKeys(); len(keys) > 0 {
					m.fieldFromCursor = (m.fieldFromCursor + 1) % len(keys)
				}
				return m, nil
			}
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 7 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		case " ":
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 7 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		}
	}

	inputIndex, isInput := m.fieldInputForFocus(m.fieldFocus)
	if !isInput {
		return m, nil
	}
	var cmd tea.Cmd
	m.fieldInputs[inputIndex], cmd = m.fieldInputs[inputIndex].Update(msg)
	return m, cmd
}

// fieldInputForFocus maps a field form focus row to its text input. Focus
// rows that hold pickers report false; for refer fields the options row
// holds the From picker instead of a text input.
func (m model) fieldInputForFocus(focus int) (int, bool) {
	if focus == 3 && fieldTypes[m.fieldTypeCursor] == task.FieldRefer {
		return 0, false
	}
	switch focus {
	case 0:
		return 0, true
	case 1:
		return 1, true
	case 3:
		return 2, true
	case 5:
		return 3, true
	case 6:
		return 4, true
	default:
		return 0, false
	}
}

// referSourceKeys returns the keys the field being edited may reference:
// every field declared before it.
func (m model) referSourceKeys() []string {
	end := len(m.formFields)
	if m.editingField >= 0 && m.editingField < end {
		end = m.editingField
	}
	keys := make([]string, 0, end)
	for _, field := range m.formFields[:end] {
		keys = append(keys, field.Key)
	}
	return keys
}

func (m *model) openFieldForm(index int) {
	m.showScreen(screenField)
	m.editingField = index
	m.fieldFocus = 0
	m.fieldTypeCursor = 0
	m.fieldFromCursor = 0
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
		m.fieldInputs[3].SetValue(field.Prefix)
		m.fieldInputs[4].SetValue(field.Suffix)
		m.fieldRaw = field.Raw
		m.fieldOptional = field.Optional
		for i, fieldType := range fieldTypes {
			if field.Type == fieldType {
				m.fieldTypeCursor = i
				break
			}
		}
		if field.Type == task.FieldRefer {
			for i, key := range m.referSourceKeys() {
				if key == field.From {
					m.fieldFromCursor = i
					break
				}
			}
		}
	}
	m.focusFieldControl()
}

func (m *model) focusFieldControl() tea.Cmd {
	var cmd tea.Cmd
	input, isInput := m.fieldInputForFocus(m.fieldFocus)
	for i := range m.fieldInputs {
		focus := isInput && i == input
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
		m.fieldFocus = (m.fieldFocus + direction + 8) % 8
		fieldType := fieldTypes[m.fieldTypeCursor]
		if m.fieldFocus == 3 && fieldType != task.FieldChoice && fieldType != task.FieldRefer {
			continue
		}
		if m.fieldFocus == 7 && fieldType == task.FieldRefer {
			continue
		}
		return
	}
}

func (m model) saveFieldForm() (tea.Model, tea.Cmd) {
	field := task.Field{
		Key:   strings.TrimSpace(m.fieldInputs[0].Value()),
		Label: strings.TrimSpace(m.fieldInputs[1].Value()),
		Type:  fieldTypes[m.fieldTypeCursor],
		// Prefix and suffix keep their whitespace: a trailing space is how a
		// prefix separates its value from the next argument.
		Prefix:   m.fieldInputs[3].Value(),
		Suffix:   m.fieldInputs[4].Value(),
		Raw:      m.fieldRaw,
		Optional: m.fieldOptional,
	}
	if field.Type == task.FieldRefer {
		field.Optional = false
		if keys := m.referSourceKeys(); len(keys) > 0 {
			field.From = keys[min(m.fieldFromCursor, len(keys)-1)]
		}
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
	m.taskFocus = 1
	m.status = ""
	return m, m.focusTaskControl()
}
