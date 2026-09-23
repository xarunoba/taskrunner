package tui

import (
	"strings"

	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) fieldFormView() string {
	var body strings.Builder
	body.WriteString(m.taskStepTitle("Key", m.fieldFocus == 0))
	body.WriteByte('\n')
	body.WriteString(m.fieldInputs[0].View())
	body.WriteString(m.gap())
	body.WriteString(m.taskStepTitle("Label", m.fieldFocus == 1))
	body.WriteByte('\n')
	body.WriteString(m.fieldInputs[1].View())
	body.WriteString(m.gap())
	body.WriteString(m.taskStepTitle("Type", m.fieldFocus == 2))
	body.WriteByte('\n')
	body.WriteString(m.pickerRow(fieldTypeLabels, m.fieldTypeCursor))
	fieldType := fieldTypes[m.fieldTypeCursor]
	if fieldType == task.FieldChoice {
		body.WriteString(m.gap())
		body.WriteString(m.taskStepTitle("Options (comma-separated)", m.fieldFocus == 3))
		body.WriteByte('\n')
		body.WriteString(m.fieldInputs[2].View())
	}
	if fieldType == task.FieldRefer {
		body.WriteString(m.gap())
		body.WriteString(m.taskStepTitle("From (earlier field)", m.fieldFocus == 3))
		body.WriteByte('\n')
		if keys := m.referSourceKeys(); len(keys) > 0 {
			body.WriteString(m.pickerRow(keys, min(m.fieldFromCursor, len(keys)-1)))
		} else {
			body.WriteString(m.styles.muted.Render("No earlier fields to reference"))
		}
	}
	body.WriteString(m.gap())
	body.WriteString(m.taskStepTitle("Interpolation", m.fieldFocus == 4))
	body.WriteByte('\n')
	interpolation := 0
	if m.fieldRaw {
		interpolation = 1
	}
	body.WriteString(m.pickerRow([]string{"Argument", "Raw"}, interpolation))
	if m.fieldRaw {
		body.WriteByte('\n')
		body.WriteString(m.styles.error.Render("Warning: raw values execute as shell syntax."))
	}
	body.WriteString(m.gap())
	body.WriteString(m.taskStepTitle("Prefix", m.fieldFocus == 5))
	body.WriteByte('\n')
	body.WriteString(m.fieldInputs[3].View())
	body.WriteString(m.gap())
	body.WriteString(m.taskStepTitle("Suffix", m.fieldFocus == 6))
	body.WriteByte('\n')
	body.WriteString(m.fieldInputs[4].View())
	if fieldType != task.FieldRefer {
		body.WriteString(m.gap())
		body.WriteString(m.taskStepTitle("Requirement", m.fieldFocus == 7))
		body.WriteByte('\n')
		requirement := 0
		if m.fieldOptional {
			requirement = 1
		}
		body.WriteString(m.pickerRow([]string{"Required", "Optional"}, requirement))
	}
	return m.renderWorkspacePanel(body.String(), screenField)
}
