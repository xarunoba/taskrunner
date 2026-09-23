package tui

import (
	"fmt"
	"strings"

	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) runFormView() string {
	field := m.runTask.Fields[m.runIndex]
	var body strings.Builder
	body.WriteString(accentStyle.Render(m.runTask.Name))
	body.WriteByte('\n')
	body.WriteString(mutedStyle.Render(fmt.Sprintf("Field %d of %d", m.runIndex+1, len(m.runTask.Fields))))
	body.WriteString(m.gap())
	label := task.ResolveKnownValues(field.Label, m.runValues)
	if field.Optional {
		label += " (optional)"
	}
	body.WriteString(stepStyle.Render(label))
	body.WriteString(m.gap())
	if field.Raw {
		body.WriteString(errorStyle.Render("RAW MODE: this value will execute as shell syntax."))
		body.WriteString(m.gap())
	}

	switch field.Type {
	case task.FieldText:
		body.WriteString(m.runInput.View())
	case task.FieldChoice:
		if field.Optional {
			skip := "  Skip"
			if m.choiceCursor < 0 {
				skip = selectedStyle.Render("› Skip")
			}
			body.WriteString(skip)
			body.WriteByte('\n')
		}
		start, end := visibleRange(len(field.Options), max(0, m.choiceCursor), max(1, m.contentHeight()-9))
		if start > 0 {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↑ %d more", start)))
			body.WriteByte('\n')
		}
		for i := start; i < end; i++ {
			resolved := task.ResolveKnownValues(field.Options[i], m.runValues)
			option := "  " + resolved
			if i == m.choiceCursor {
				option = selectedStyle.Render("› " + resolved)
			}
			body.WriteString(option)
			body.WriteByte('\n')
		}
		if end < len(field.Options) {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↓ %d more", len(field.Options)-end)))
			body.WriteByte('\n')
		}
	case task.FieldFile:
		body.WriteString(m.filePicker.View())
	case task.FieldConfirm:
		selected := 0
		if m.confirmationYes {
			selected = 1
		}
		body.WriteString(m.pickerRow([]string{"No", "Yes"}, selected))
	}
	return m.renderWorkspacePanel(body.String(), screenRun)
}
