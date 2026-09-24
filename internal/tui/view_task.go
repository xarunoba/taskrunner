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
		body.WriteString(m.styles.muted.Render("No tasks yet. Press n to create one."))
	} else {
		start, end := m.listRange(len(m.tasks), m.cursor, 2)
		for i := start; i < end; i++ {
			item := m.tasks[i]
			title := strings.ReplaceAll(item.Name, "\n", " ") + m.taskActivity(item.File)
			name := "  " + title
			if i == m.cursor {
				name = m.styles.selected.Render("› " + title)
			}
			body.WriteString(name)
			body.WriteByte('\n')
			if i == m.cursor && m.contentHeight() > 1 {
				body.WriteString("  ")
				body.WriteString(m.styles.muted.Render(strings.ReplaceAll(item.Command, "\n", " ")))
				body.WriteByte('\n')
				if m.contentHeight() > 2 {
					body.WriteString("  ")
					body.WriteString(m.styles.muted.Render(fmt.Sprintf(
						"%d fields • %s",
						len(item.Fields),
						jobPolicySummary(item.JobPolicy),
					)))
					body.WriteByte('\n')
				}
			}
		}
		return m.renderWorkspacePanel(m.listWithScrollBar(body.String(), start, end, len(m.tasks)), screenList)
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
		return m.styles.selected.Render("› " + line)
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
		m.scrollBar(form.Height, lineCount > form.Height, form.ScrollPercent()),
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
	write(m.taskStepTitle("Name", m.taskFocus == 0))
	write("\n")
	write(m.taskNameInput.View())
	write(m.gap())

	write(m.taskStepTitle("Fields", m.taskFocus == 1))
	write("\n")
	if len(m.formFields) == 0 {
		if m.taskFocus == 1 {
			focusLine = line
			write(m.styles.selected.Render("› No fields. Press a or enter to add one."))
		} else {
			write(m.styles.muted.Render("No fields. Press a or enter to add one."))
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

	write(m.taskStepTitle("Command template", m.taskFocus == 2))
	write("\n")
	if m.taskFocus == 2 {
		focusLine = line + m.taskCommandCursorLine()
	}
	write(m.taskCommandView())
	write("\n")
	write(m.styles.muted.Render("Use {{field_key}} where a runtime value belongs."))
	write(m.gap())

	policy, selectedLine := m.pickerLayout(jobPolicyLabels, jobPolicyIndex(m.formJobPolicy))
	if m.taskFocus == 3 {
		focusLine = line + 1 + selectedLine
	}
	write(m.taskStepTitle("Job policy", m.taskFocus == 3))
	write("\n")
	write(policy)
	return body.String(), focusLine
}

func (m model) taskCommandCursorLine() int {
	lines := strings.Split(m.taskCommandInput.Value(), "\n")
	current := min(m.taskCommandInput.Line(), len(lines)-1)
	line := 0
	for i := range current {
		line += taskCommandLineHeight(lines[i], m.taskCommandInput.Width())
	}
	return line + m.taskCommandInput.LineInfo().RowOffset
}

func (m model) taskStepTitle(title string, active bool) string {
	if active {
		return m.styles.step.Render(title)
	}
	return title
}

func (m model) pickerRow(options []string, selected int) string {
	view, _ := m.pickerLayout(options, selected)
	return view
}

// pickerLayout returns the rendered options and the active option's row.
func (m model) pickerLayout(options []string, selected int) (string, int) {
	var (
		body      strings.Builder
		lineWidth int
		line      int
		active    int
	)
	for i, option := range options {
		token := "[ " + option + " ]"
		if i == selected {
			token = m.styles.selected.Render("› " + option)
		}
		tokenWidth := ansi.StringWidth(token)
		separator := 0
		if lineWidth > 0 {
			separator = 1
		}
		if lineWidth > 0 && lineWidth+separator+tokenWidth > m.contentWidth()-1 {
			body.WriteByte('\n')
			line++
			lineWidth = 0
			separator = 0
		}
		if separator > 0 {
			body.WriteString(" ")
		}
		if i == selected {
			active = line
		}
		body.WriteString(token)
		lineWidth += separator + tokenWidth
	}
	return body.String(), active
}
