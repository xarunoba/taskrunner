package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) listView() string {
	var body strings.Builder

	if len(m.tasks) == 0 {
		body.WriteString(mutedStyle.Render("No tasks yet. Press n to create one."))
	} else {
		start, end := visibleRange(len(m.tasks), m.cursor, max(1, m.contentHeight()-7))
		if start > 0 {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↑ %d more", start)))
			body.WriteByte('\n')
		}
		for i := start; i < end; i++ {
			item := m.tasks[i]
			name := "  " + item.Name + m.taskActivity(item.File)
			if i == m.cursor {
				name = selectedStyle.Render("› " + item.Name + m.taskActivity(item.File))
			}
			body.WriteString(name)
			body.WriteByte('\n')
			if i == m.cursor {
				body.WriteString("  ")
				body.WriteString(mutedStyle.Render(item.Command))
				body.WriteByte('\n')
				body.WriteString("  ")
				body.WriteString(mutedStyle.Render(fmt.Sprintf(
					"%d fields • %s",
					len(item.Fields),
					jobPolicySummary(item.JobPolicy),
				)))
				body.WriteByte('\n')
			}
		}
		if end < len(m.tasks) {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↓ %d more", len(m.tasks)-end)))
			body.WriteByte('\n')
		}
	}

	return m.renderWorkspacePanel(body.String(), screenList)
}

func (m model) taskFieldLine(index int) string {
	field := m.formFields[index]
	mode := "argument"
	if field.Raw {
		mode = "RAW"
	}
	requirement := "required"
	if field.Optional {
		requirement = "optional"
	}
	if field.Type == task.FieldRefer {
		requirement = "from " + field.From
	}
	fragment := "{{" + field.Key + "}}"
	if field.Prefix != "" || field.Suffix != "" {
		if field.Type == task.FieldConfirm {
			fragment = field.Prefix + field.Suffix
		} else {
			fragment = field.Prefix + fragment + field.Suffix
		}
	}
	line := fmt.Sprintf("%d. %s (%s, %s, %s → %s)", index+1, field.Label, field.Type, requirement, mode, fragment)
	if m.taskFocus == 1 && index == m.fieldCursor {
		return selectedStyle.Render("› " + line)
	}
	return "  " + line
}

func (m model) taskFormView() string {
	content, focusLine := m.taskFormContent()
	form := m.taskViewport
	form.Width = max(1, m.contentWidth()-1)
	form.Height = m.contentHeight()
	form.SetContent(content)
	switch {
	case focusLine < form.YOffset:
		form.SetYOffset(focusLine)
	case focusLine >= form.YOffset+form.Height:
		form.SetYOffset(focusLine - form.Height + 1)
	}
	lineCount := strings.Count(content, "\n") + 1
	view := lipgloss.JoinHorizontal(
		lipgloss.Top,
		form.View(),
		scrollBar(form.Height, lineCount > form.Height, form.ScrollPercent()),
	)
	return m.renderWorkspacePanel(view, screenTask)
}

func (m model) taskFormContent() (string, int) {
	var body strings.Builder
	line := 0
	write := func(value string) {
		body.WriteString(value)
		line += strings.Count(value, "\n")
	}
	focusLine := 0

	if m.taskFocus == 0 {
		focusLine = line
	}
	write(taskStepTitle("Name", m.taskFocus == 0))
	write("\n")
	write(m.taskNameInput.View())
	write(m.gap())

	write(taskStepTitle("Fields", m.taskFocus == 1))
	write("\n")
	if len(m.formFields) == 0 {
		if m.taskFocus == 1 {
			focusLine = line
			write(selectedStyle.Render("› No fields. Press a or enter to add one."))
		} else {
			write(mutedStyle.Render("No fields. Press a or enter to add one."))
		}
	} else {
		for i := range m.formFields {
			if m.taskFocus == 1 && i == m.fieldCursor {
				focusLine = line
			}
			write(m.taskFieldLine(i))
			if i < len(m.formFields)-1 {
				write("\n")
			}
		}
	}
	write(m.gap())

	write(taskStepTitle("Command template", m.taskFocus == 2))
	write("\n")
	if m.taskFocus == 2 {
		focusLine = line + m.taskCommandCursorLine()
	}
	write(m.taskCommandView())
	write("\n")
	write(mutedStyle.Render("Use {{field_key}} where a runtime value belongs."))
	write(m.gap())

	if m.taskFocus == 3 {
		focusLine = line
	}
	write(taskStepTitle("Job policy", m.taskFocus == 3))
	write("\n")
	write(m.pickerRow(jobPolicyLabels, jobPolicyIndex(m.formJobPolicy)))
	return body.String(), focusLine
}

func (m model) taskCommandCursorLine() int {
	lines := strings.Split(m.taskCommandInput.Value(), "\n")
	current := min(m.taskCommandInput.Line(), len(lines)-1)
	line := 0
	for i := 0; i < current; i++ {
		line += taskCommandLineHeight(lines[i], m.taskCommandInput.Width())
	}
	return line + m.taskCommandInput.LineInfo().RowOffset
}

func taskStepTitle(title string, active bool) string {
	if active {
		return stepStyle.Render(title)
	}
	return title
}

func (m model) pickerRow(options []string, selected int) string {
	var (
		body      strings.Builder
		lineWidth int
	)
	for i, option := range options {
		token := "[ " + option + " ]"
		if i == selected {
			token = selectedStyle.Render("› " + option)
		}
		tokenWidth := ansi.StringWidth(token)
		separator := 0
		if lineWidth > 0 {
			separator = 1
		}
		if lineWidth > 0 && lineWidth+separator+tokenWidth > m.contentWidth() {
			body.WriteString("\n")
			lineWidth = 0
			separator = 0
		}
		if separator > 0 {
			body.WriteString(" ")
		}
		body.WriteString(token)
		lineWidth += separator + tokenWidth
	}
	return body.String()
}
