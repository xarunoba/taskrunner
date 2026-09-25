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
	content, focusLine, _ := m.taskFormLayout()
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

// taskFormRows maps the task form's content rows to its controls so
// rendering and click hit-testing share one geometry.
type taskFormRows struct {
	name    fieldRow
	fields  fieldRow
	command fieldRow
	policy  fieldRow
	field   []fieldRow
}

func (m model) taskFormLayout() (string, int, taskFormRows) {
	var (
		body strings.Builder
		rows taskFormRows
		line int
	)
	write := func(value string) {
		body.WriteString(value)
		line += strings.Count(value, "\n")
	}

	rows.name.start = line
	write(m.taskStepTitle("Name", m.taskFocus == 0))
	write("\n")
	write(m.taskNameInput.View())
	rows.name.end = line + 1
	write(m.gap())

	rows.fields.start = line
	write(m.taskStepTitle("Fields", m.taskFocus == 1))
	write("\n")
	if len(m.formFields) == 0 {
		if m.taskFocus == 1 {
			write(m.styles.selected.Render("› No fields. Press a or enter to add one."))
		} else {
			write(m.styles.muted.Render("No fields. Press a or enter to add one."))
		}
		rows.fields.end = line + 1
	} else {
		rows.field = make([]fieldRow, len(m.formFields))
		for i := range m.formFields {
			rows.field[i].start = line
			write(m.taskFieldLine(i))
			rows.field[i].end = line + 1
			if i < len(m.formFields)-1 {
				write("\n")
			}
		}
		rows.fields.end = line + 1
	}
	write(m.gap())

	rows.command.start = line
	write(m.taskStepTitle("Command template", m.taskFocus == 2))
	write("\n")
	commandFocus := -1
	if m.taskFocus == 2 {
		commandFocus = line + m.taskCommandCursorLine()
	}
	write(m.taskCommandView())
	write("\n")
	write(m.styles.muted.Render("Use {{field_key}} where a runtime value belongs."))
	rows.command.end = line + 1
	write(m.gap())

	rows.policy.start = line
	policy, selectedLine := m.pickerLayout(jobPolicyLabels, jobPolicyIndex(m.formJobPolicy))
	policyFocus := -1
	if m.taskFocus == 3 {
		policyFocus = line + 1 + selectedLine
	}
	write(m.taskStepTitle("Job policy", m.taskFocus == 3))
	write("\n")
	write(policy)
	rows.policy.end = line + 1

	focusLine := -1
	switch {
	case m.taskFocus == 0:
		focusLine = rows.name.start
	case m.taskFocus == 1 && len(m.formFields) == 0:
		focusLine = rows.fields.start + 1
	case m.taskFocus == 1:
		if m.fieldCursor < len(rows.field) {
			focusLine = rows.field[m.fieldCursor].start
		}
	case m.taskFocus == 2:
		focusLine = commandFocus
	case m.taskFocus == 3:
		focusLine = policyFocus
	}
	return body.String(), focusLine, rows
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
