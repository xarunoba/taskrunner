package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

const settingsTitle = "Settings"

// settingKey identifies a persisted setting row in the settings modal.
type settingKey uint8

const (
	settingTheme settingKey = iota
)

// settingChoice is one modal row: a labeled choice among enumerated options.
type settingChoice struct {
	key      settingKey
	label    string
	options  []string
	selected int
}

// settingsLoadedMsg carries the enumerated theme names back to the model.
type settingsLoadedMsg struct {
	names []string
	err   error
}

// settingSelectedMsg carries the theme applied by the store back to the model.
type settingSelectedMsg struct {
	theme theme.Theme
	err   error
}

// openSettings shows the modal immediately and enumerates themes in the
// background. The command only reads the store; the model changes when the
// message returns through Update.
func (m *model) openSettings() (tea.Model, tea.Cmd) {
	m.helpOpen = false
	m.settingsOpen = true
	m.settingsLoading = true
	m.settingsSaving = false
	m.settingsCursor = 0
	m.settings = nil
	m.status = ""
	m.statusError = false
	themes := m.themes
	cmd := func() tea.Msg {
		names, err := themes.Names()
		return settingsLoadedMsg{names: names, err: err}
	}
	return *m, cmd
}

// applySettingsLoaded builds the modal rows from the enumerated names. A
// late arrival after the modal closed is dropped.
func (m *model) applySettingsLoaded(msg settingsLoadedMsg) {
	if !m.settingsOpen {
		return
	}
	m.settingsLoading = false
	if msg.err != nil {
		m.settingsOpen = false
		m.setError(msg.err)
		return
	}
	selected := 0
	for i, name := range msg.names {
		if name == m.theme.Name {
			selected = i
			break
		}
	}
	m.settings = []settingChoice{{
		key:      settingTheme,
		label:    "Theme",
		options:  msg.names,
		selected: selected,
	}}
	m.settingsCursor = 0
}

func (m model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "s":
		m.settingsOpen = false
		return m, nil
	case "q":
		return m, tea.Quit
	}
	if m.settingsLoading || m.settingsSaving || len(m.settings) == 0 {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < len(m.settings)-1 {
			m.settingsCursor++
		}
	case "left", "h":
		return m.cycleSetting(m.settingsCursor, -1)
	case "right", "l", "enter":
		return m.cycleSetting(m.settingsCursor, 1)
	}
	return m, nil
}

// cycleSetting moves a row to its previous or next option and applies it.
func (m *model) cycleSetting(row, direction int) (tea.Model, tea.Cmd) {
	if row < 0 || row >= len(m.settings) {
		return m, nil
	}
	choice := m.settings[row]
	if len(choice.options) > 1 {
		choice.selected = ((choice.selected+direction)%len(choice.options) + len(choice.options)) % len(choice.options)
	}
	m.settings[row] = choice
	return m.applySetting(choice)
}

// applySetting persists one changed row through its store call. The saving
// flag is set before the command is returned so further input cannot start a
// second write that could reorder the atomic file replacements.
func (m *model) applySetting(choice settingChoice) (tea.Model, tea.Cmd) {
	switch choice.key {
	case settingTheme:
		name := choice.options[choice.selected]
		if name == m.theme.Name {
			return *m, nil
		}
		m.settingsSaving = true
		themes := m.themes
		return *m, func() tea.Msg {
			selected, err := themes.Select(name)
			return settingSelectedMsg{theme: selected, err: err}
		}
	}
	return *m, nil
}

// applySettingSelected adopts a successfully persisted theme and refreshes
// model-owned styles; a failure keeps the previous palette, row selection,
// and modal state intact.
func (m *model) applySettingSelected(msg settingSelectedMsg) {
	m.settingsSaving = false
	if msg.err != nil {
		m.setError(msg.err)
		return
	}
	m.theme = msg.theme
	m.styles = newStyles(m.theme.Palette)
	m.syncFilePickerStyles()
	for i := range m.settings {
		if m.settings[i].key != settingTheme {
			continue
		}
		for j, name := range m.settings[i].options {
			if name == m.theme.Name {
				m.settings[i].selected = j
				break
			}
		}
	}
}

// syncFilePickerStyles re-applies the cursor and selected styles when the
// active run field is a file picker.
func (m *model) syncFilePickerStyles() {
	if m.screen == screenRun && m.runIndex < len(m.runTask.Fields) &&
		m.runTask.Fields[m.runIndex].Type == task.FieldFile {
		m.filePicker.Styles.Cursor = m.styles.cursor
		m.filePicker.Styles.Selected = m.styles.selected
	}
}

func (m model) settingsView() string {
	content := m.settingsContent()
	if m.compactModal() {
		return m.titledPanel(m.renderPanelWithFooter(content), settingsTitle)
	}
	return overlay(m.baseView(), m.titledPanel(m.styles.helpModal.Render(content), settingsTitle), m.width, m.height)
}

func (m model) settingsContent() string {
	var body strings.Builder
	switch {
	case m.settingsLoading:
		body.WriteString(m.styles.muted.Render("Loading themes…"))
	case m.settingsSaving:
		body.WriteString(m.styles.muted.Render("Saving theme…"))
	default:
		for i, choice := range m.settings {
			if i > 0 {
				body.WriteByte('\n')
			}
			body.WriteString(m.settingsRow(i, choice))
		}
	}
	return body.String()
}

func (m model) settingsRow(index int, choice settingChoice) string {
	width := m.contentWidth()
	if index == m.settingsCursor {
		width -= 2 // Selection style padding.
	}
	prefix := "  "
	if index == m.settingsCursor {
		prefix = "› "
	}
	prefix += choice.label + "  "
	available := max(1, width-ansi.StringWidth(prefix)-4) // "‹ " and " ›" markers.
	name := ansi.Truncate(choice.options[choice.selected], available, "…")
	row := prefix + "‹ " + name + " ›"
	if index == m.settingsCursor {
		return m.styles.selected.Render(row)
	}
	return row
}

// clickSettings applies a clicked settings row: the label side cycles to the
// previous option and the value side to the next.
func (m model) clickSettings(lines []string, y, x int) (tea.Model, tea.Cmd) {
	if m.settingsLoading || m.settingsSaving {
		return m, nil
	}
	top, left, width, ok := modalGeometry(lines, settingsTitle)
	if !ok {
		return m, nil
	}
	for i := range m.settings {
		row := top + 2 + i // Border and padding above the rows.
		if y != row || row >= len(lines) {
			continue
		}
		contentStart := left + 3 // Border and padding.
		if x < contentStart || x >= left+width-2 {
			return m, nil
		}
		rowLine := ansi.Cut(lines[row], contentStart, left+width-2)
		valueStart := strings.Index(rowLine, "‹")
		if valueStart < 0 {
			return m, nil
		}
		m.settingsCursor = i
		direction := 1
		if x-contentStart < valueStart+1 {
			direction = -1
		}
		return m.cycleSetting(i, direction)
	}
	return m, nil
}

// modalGeometry locates a titled modal panel in the rendered view and returns
// the row of its top border with the border column and full frame width.
func modalGeometry(lines []string, title string) (top, left, width int, ok bool) {
	marker := "╭─ " + title
	for i, line := range lines {
		start := strings.Index(line, marker)
		if start < 0 {
			continue
		}
		end := strings.Index(line[start:], "╮")
		if end < 0 {
			return 0, 0, 0, false
		}
		return i, ansi.StringWidth(line[:start]), ansi.StringWidth(line[start:start+end]) + 1, true
	}
	return 0, 0, 0, false
}
