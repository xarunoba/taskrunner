package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/xarunoba/taskrunner/internal/task"
)

// runFormHeader keeps optional Skip rows and the input visible by collapsing
// gaps, then omitting the task name and counter when the terminal is short.
func (m model) runFormHeader(field task.Field) string {
	label := task.ResolveKnownValues(field.Label, m.runValues)
	if field.Optional {
		label += " (optional)"
	}
	hasSkip := field.Optional && (field.Type == task.FieldChoice || field.Type == task.FieldFile)
	gap := m.gap()
	gapCount := 2
	if field.Raw {
		gapCount++
	}
	headerRows := 1 + strings.Count(m.runTask.Name, "\n") + strings.Count(label, "\n") + gapCount*strings.Count(gap, "\n")
	if hasSkip {
		headerRows++
	}
	limit := max(1, m.contentHeight()-1)
	if headerRows > limit {
		headerRows -= gapCount * (strings.Count(gap, "\n") - 1)
		gap = "\n"
	}
	showName := headerRows <= limit
	if !showName {
		headerRows -= 1 + strings.Count(m.runTask.Name, "\n")
	}
	showCounter := headerRows <= limit

	var body strings.Builder
	if showName {
		body.WriteString(m.styles.accent.Render(m.runTask.Name))
		body.WriteByte('\n')
	}
	if showCounter {
		body.WriteString(m.styles.muted.Render(fmt.Sprintf("Field %d of %d", m.runIndex+1, len(m.runTask.Fields))))
		body.WriteString(gap)
	}
	body.WriteString(m.styles.step.Render(label))
	body.WriteString(gap)
	if field.Raw {
		body.WriteString(m.styles.error.Render("RAW MODE: this value will execute as shell syntax."))
		body.WriteString(gap)
	}
	if hasSkip {
		skip := "  Skip"
		if m.choiceCursor < 0 {
			skip = m.styles.selected.Render("› Skip")
		}
		body.WriteString(skip)
		body.WriteByte('\n')
	}
	return body.String()
}

// runFormListHeight is the number of rows left for the scrollable list after
// the dynamically sized header, so short terminals and raw/optional fields
// size the window from actual content.
func (m model) runFormListHeight() int {
	if m.runIndex >= len(m.runTask.Fields) {
		return m.contentHeight()
	}
	header := m.runFormHeader(m.runTask.Fields[m.runIndex])
	// The header always ends with a newline, so the count equals its row total.
	return max(1, m.contentHeight()-strings.Count(header, "\n"))
}

// runChoiceWindow returns the visible slice bounds for the current choice.
func (m model) runChoiceWindow(field task.Field) (int, int) {
	return visibleRange(len(field.Options), max(0, m.choiceCursor), m.runFormListHeight())
}

// runChoiceRowAt maps a click row (event.Y minus the panel content top) to an
// option index so mouse handling can hit-test rendered rows. A click on the
// optional Skip row returns (-1, true).
func (m model) runChoiceRowAt(row int) (int, bool) {
	if m.runIndex >= len(m.runTask.Fields) {
		return 0, false
	}
	field := m.runTask.Fields[m.runIndex]
	// The header always ends with a newline, so the count equals its row total.
	headerRows := strings.Count(m.runFormHeader(field), "\n")
	if field.Optional && row == headerRows-1 {
		return -1, true
	}
	start, end := m.runChoiceWindow(field)
	idx := row - headerRows + start
	return idx, idx >= start && idx < end
}

// runFileCursorRow returns the row index of the picker cursor within the
// rendered picker window.
func (m model) runFileCursorRow(view string) int {
	i := 0
	for line := range strings.SplitSeq(view, "\n") {
		if strings.HasPrefix(ansi.Strip(line), m.filePicker.Cursor) {
			return i
		}
		i++
	}
	return 0
}

// runFileClickMove maps a click row (event.Y minus the panel content top) to
// the number of down/up key presses (positive = down, negative = up) needed
// to move the picker cursor onto the clicked row, using public state only.
// It returns 0 when the click is outside the picker window or on the cursor.
func (m model) runFileClickMove(row int) int {
	if m.runIndex >= len(m.runTask.Fields) {
		return 0
	}
	field := m.runTask.Fields[m.runIndex]
	pickerRow := row - strings.Count(m.runFormHeader(field), "\n")
	if pickerRow < 0 || pickerRow >= m.filePicker.Height {
		return 0
	}
	return pickerRow - m.runFileCursorRow(m.filePicker.View())
}

// runFileSkipHit reports whether a click row lands on the optional Skip row of
// a file field, mirroring runChoiceRowAt's Skip mapping.
func (m model) runFileSkipHit(row int) bool {
	if m.runIndex >= len(m.runTask.Fields) {
		return false
	}
	field := m.runTask.Fields[m.runIndex]
	return field.Optional && row == strings.Count(m.runFormHeader(field), "\n")-1
}

func (m model) runChoiceList(field task.Field) string {
	start, end := m.runChoiceWindow(field)
	lines := make([]string, 0, max(0, end-start))
	for i := start; i < end; i++ {
		resolved := task.ResolveKnownValues(field.Options[i], m.runValues)
		option := "  " + resolved
		if i == m.choiceCursor {
			option = m.styles.selected.Render("› " + resolved)
		}
		lines = append(lines, option)
	}
	return strings.Join(lines, "\n")
}

func (m model) runWithScrollBar(content string, height, start, total, shown int) string {
	hidden := total - shown
	percent := 0.0
	if hidden > 0 {
		percent = float64(start) / float64(hidden)
	}
	return m.withScrollBar(content, m.contentWidth(), height, hidden > 0, percent)
}

// runFilePickerBody keeps the header static and puts the shared right-edge
// position bar on the picker viewport only.
func (m model) runFilePickerBody(header string, visible int) string {
	view := m.filePicker.View()
	scrollable := m.fileCount > visible
	percent := 0.0
	if scrollable {
		percent = m.runFilePickerPercent(view)
	}
	return header + m.withScrollBar(view, m.contentWidth(), visible, scrollable, percent)
}

// Bubbles exposes neither its entry list nor its scroll offset. Match the
// first rendered entry against the asynchronously mirrored directory listing.
func (m model) runFilePickerPercent(view string) float64 {
	hidden := m.fileCount - m.filePicker.Height
	if hidden <= 0 {
		return 0
	}
	return min(1, float64(m.runFileStart(view))/float64(hidden))
}

func (m model) runFileStart(view string) int {
	first, _, _ := strings.Cut(view, "\n")
	name := m.pickerRowName(first)
	for i, candidate := range m.fileNames {
		if candidate == name {
			return i
		}
	}
	return 0
}

func (m model) pickerRowName(row string) string {
	row = ansi.Strip(row)
	selected := strings.HasPrefix(row, m.filePicker.Cursor)
	row = ansi.Cut(row, ansi.StringWidth(m.filePicker.Cursor), ansi.StringWidth(row))
	if selected {
		style := m.filePicker.Styles.Selected
		row = ansi.Cut(row, style.GetPaddingLeft(), ansi.StringWidth(row)-style.GetPaddingRight())
	}
	if m.filePicker.ShowPermissions {
		row = strings.TrimPrefix(row, " ")
		if end := strings.IndexByte(row, ' '); end >= 0 {
			row = row[end:]
		}
	}
	if m.filePicker.ShowSize {
		row = ansi.Cut(row, m.filePicker.Styles.FileSize.GetWidth(), ansi.StringWidth(row))
	}
	return strings.TrimPrefix(row, " ")
}

func (m model) runFormView() string {
	field := m.runTask.Fields[m.runIndex]
	header := m.runFormHeader(field)
	switch field.Type {
	case task.FieldText:
		return m.renderWorkspacePanel(header+m.runInput.View(), screenRun)
	case task.FieldChoice:
		start, end := m.runChoiceWindow(field)
		list := m.runWithScrollBar(
			m.runChoiceList(field), m.runFormListHeight(),
			start, len(field.Options), end-start,
		)
		return m.renderWorkspacePanel(header+list, screenRun)
	case task.FieldFile:
		return m.renderWorkspacePanel(m.runFilePickerBody(header, m.runFormListHeight()), screenRun)
	default:
		selected := 0
		if m.confirmationYes {
			selected = 1
		}
		return m.renderWorkspacePanel(header+m.pickerRow([]string{"No", "Yes"}, selected), screenRun)
	}
}
