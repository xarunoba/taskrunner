package tui

import (
	"strings"

	"github.com/xarunoba/taskrunner/internal/task"
)

// fieldRow is the content-line range [start, end) a form control occupies,
// titles included. end == start means the control is not rendered.
type fieldRow struct {
	start, end int
}

// covers reports whether a content row lands inside the control.
func (r fieldRow) covers(row int) bool {
	return row >= r.start && row < r.end
}

// fieldFormLayout renders the full field form content and computes the
// focus-following window shown at the current terminal size.
//
// It returns every content line, the visible slice [start, end), the content
// row of the focused control (for pickers, the row of the active selection),
// and the row range of each control by field focus. Mouse handling uses the
// same layout so clicks map to the rows that are actually on screen.
func (m model) fieldFormLayout() (lines []string, start, end, focusLine int, rows [8]fieldRow) {
	var body []string
	add := func(s string) {
		for line := range strings.SplitSeq(s, "\n") {
			body = append(body, line)
		}
	}
	addGap := func() {
		if m.gap() == "\n\n" {
			body = append(body, "")
		}
	}
	// addTitle writes a step title and records its row for the focus.
	addTitle := func(title string, focus int) {
		rows[focus].start = len(body)
		rows[focus].end = len(body) + 1
		add(m.taskStepTitle(title, m.fieldFocus == focus))
	}
	// addInput writes a single-line input and marks its row when focused.
	addInput := func(view string, focus int) {
		add(view)
		if m.fieldFocus == focus {
			focusLine = len(body) - 1
		}
		rows[focus].end = len(body)
	}
	// addPicker writes a (possibly multi-line) picker block and records its
	// rows; when focused, focusLine is the row of the active selection.
	addPicker := func(options []string, selected, focus int) {
		view, selectedLine := m.pickerLayout(options, selected)
		controlStart := len(body)
		add(view)
		rows[focus].end = len(body)
		if m.fieldFocus == focus {
			focusLine = controlStart + selectedLine
		}
	}

	addTitle("Key", 0)
	addInput(m.fieldInputs[0].View(), 0)
	addGap()

	addTitle("Label", 1)
	addInput(m.fieldInputs[1].View(), 1)
	addGap()

	addTitle("Type", 2)
	addPicker(fieldTypeLabels, m.fieldTypeCursor, 2)
	fieldType := fieldTypes[m.fieldTypeCursor]

	switch fieldType {
	case task.FieldChoice:
		addGap()
		addTitle("Options (comma-separated)", 3)
		addInput(m.fieldInputs[2].View(), 3)
	case task.FieldRefer:
		addGap()
		addTitle("From (earlier field)", 3)
		if keys := m.referSourceKeys(); len(keys) > 0 {
			addPicker(keys, min(m.fieldFromCursor, len(keys)-1), 3)
		} else {
			add(m.styles.muted.Render("No earlier fields to reference"))
			rows[3].end = len(body)
			if m.fieldFocus == 3 {
				focusLine = len(body) - 1
			}
		}
	}

	addGap()
	addTitle("Interpolation", 4)
	interpolation := 0
	if m.fieldRaw {
		interpolation = 1
	}
	addPicker([]string{"Argument", "Raw"}, interpolation, 4)
	if m.fieldRaw {
		add(m.styles.error.Render("Warning: raw values execute as shell syntax."))
		rows[4].end = len(body)
	}
	addGap()

	addTitle("Prefix", 5)
	addInput(m.fieldInputs[3].View(), 5)
	addGap()

	addTitle("Suffix", 6)
	addInput(m.fieldInputs[4].View(), 6)

	if fieldType != task.FieldRefer {
		addGap()
		addTitle("Requirement", 7)
		requirement := 0
		if m.fieldOptional {
			requirement = 1
		}
		addPicker([]string{"Required", "Optional"}, requirement, 7)
	}

	lines = body
	total := len(lines)
	height := max(1, m.contentHeight())
	start = 0
	if total > height && focusLine >= height {
		start = min(focusLine-height+1, total-height)
	}
	end = min(total, start+height)
	return lines, start, end, focusLine, rows
}

func (m model) fieldFormView() string {
	lines, start, end, _, _ := m.fieldFormLayout()
	content := strings.Join(lines[start:end], "\n")
	visible := end - start
	scrollable := len(lines) > visible
	percent := 0.0
	if scrollable {
		percent = float64(start) / float64(len(lines)-visible)
	}
	return m.renderWorkspacePanel(
		m.withScrollBar(content, m.contentWidth(), m.contentHeight(), scrollable, percent),
		screenField,
	)
}
