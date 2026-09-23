package tui

import (
	"strings"

	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) fieldFormView() string {
	var body strings.Builder
	body.WriteString(taskStepTitle("Key", m.fieldFocus == 0))
	body.WriteByte('\n')
	body.WriteString(m.fieldInputs[0].View())
	body.WriteString(m.gap())
	body.WriteString(taskStepTitle("Label", m.fieldFocus == 1))
	body.WriteByte('\n')
	body.WriteString(m.fieldInputs[1].View())
	body.WriteString(m.gap())
	body.WriteString(taskStepTitle("Type", m.fieldFocus == 2))
	body.WriteByte('\n')
	typeNames := make([]string, len(fieldTypes))
	for i, fieldType := range fieldTypes {
		typeNames[i] = string(fieldType)
	}
	body.WriteString(m.pickerRow(typeNames, m.fieldTypeCursor))
	if fieldTypes[m.fieldTypeCursor] == task.FieldChoice {
		body.WriteString(m.gap())
		body.WriteString(taskStepTitle("Options (comma-separated)", m.fieldFocus == 3))
		body.WriteByte('\n')
		body.WriteString(m.fieldInputs[2].View())
	}
	body.WriteString(m.gap())
	body.WriteString(taskStepTitle("Interpolation", m.fieldFocus == 4))
	body.WriteByte('\n')
	interpolation := 0
	if m.fieldRaw {
		interpolation = 1
	}
	body.WriteString(m.pickerRow([]string{"Argument", "Raw"}, interpolation))
	if m.fieldRaw {
		body.WriteString("\n")
		body.WriteString(errorStyle.Render("Warning: raw values execute as shell syntax."))
	}
	body.WriteString(m.gap())
	body.WriteString(taskStepTitle("Requirement", m.fieldFocus == 5))
	body.WriteByte('\n')
	requirement := 0
	if m.fieldOptional {
		requirement = 1
	}
	body.WriteString(m.pickerRow([]string{"Required", "Optional"}, requirement))
	return m.renderWorkspacePanel(body.String(), screenField)
}
