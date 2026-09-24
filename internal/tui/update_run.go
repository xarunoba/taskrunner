package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) startRun(item task.Task) (tea.Model, tea.Cmd) {
	m.runTask = item
	m.runIndex = 0
	m.runValues = make(map[string]string, len(item.Fields))
	m.runHistoryIndex = -1
	m.status = ""
	m.skipReferFields()
	if m.runIndex == len(m.runTask.Fields) {
		return m.executeRun()
	}
	m.showScreen(screenRun)
	return m, m.prepareRunField()
}

// skipReferFields derives values for refer fields without prompting. It
// advances the cursor to the next field that needs user input.
func (m *model) skipReferFields() {
	for m.runIndex < len(m.runTask.Fields) {
		field := m.runTask.Fields[m.runIndex]
		if field.Type != task.FieldRefer {
			return
		}
		m.runValues[field.Key] = m.runValues[field.From]
		m.runIndex++
	}
}

func (m *model) prepareRunField() tea.Cmd {
	field := m.runTask.Fields[m.runIndex]
	m.choiceCursor = 0
	if field.Optional && m.runValues[field.Key] == "" {
		m.choiceCursor = -1
	}
	m.confirmationYes = m.runValues[field.Key] == strconv.FormatBool(true)

	switch field.Type {
	case task.FieldText:
		m.runHistory = nil
		if m.runTask.File != "" {
			history, err := m.store.ValueHistory(m.runTask.File, field.Key)
			if err != nil {
				m.setError(fmt.Errorf("load value history: %w", err))
			} else {
				m.runHistory = history
			}
		}
		m.runHistoryIndex = -1
		m.runHistoryDraft = m.runValues[field.Key]
		m.runInput = newInput(task.ResolveKnownValues(field.Label, m.runValues), 1000)
		m.runInput.SetValue(m.runHistoryDraft)
		m.runInput.Width = m.contentWidth()
		return m.runInput.Focus()
	case task.FieldChoice:
		for i, option := range field.Options {
			if task.ResolveKnownValues(option, m.runValues) == m.runValues[field.Key] {
				m.choiceCursor = i
				break
			}
		}
	case task.FieldFile:
		m.filePicker = filepicker.New()
		m.filePicker.CurrentDirectory = m.store.Workspace()
		m.filePicker.ShowHidden = true
		m.filePicker.FileAllowed = true
		m.filePicker.DirAllowed = false
		// AutoHeight would re-derive the height from WindowSizeMsg and reset
		// the scroll window on resize; the model owns sizing instead.
		m.filePicker.AutoHeight = false
		m.filePicker.Styles.Cursor = m.styles.cursor
		m.filePicker.Styles.Selected = m.styles.selected
		m.filePicker.SetHeight(m.runFormListHeight())
		m.fileCount = 0
		m.fileNames = nil
		return tea.Batch(m.filePicker.Init(), countDirEntries(m.filePicker.CurrentDirectory, m.filePicker.ShowHidden))
	}
	return nil
}

func (m model) updateRunForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.showScreen(screenList)
			m.setStatus(fmt.Sprintf("Cancelled %s", m.runTask.Name))
			return m, nil
		case "shift+tab", "ctrl+left":
			return m.previousRun()
		}
	}

	field := m.runTask.Fields[m.runIndex]
	switch field.Type {
	case task.FieldText:
		return m.updateRunText(msg, field)
	case task.FieldChoice:
		return m.updateRunChoice(msg, field)
	case task.FieldFile:
		return m.updateRunFile(msg, field)
	case task.FieldConfirm:
		return m.updateRunConfirm(msg, field)
	default:
		m.showScreen(screenList)
		m.setError(fmt.Errorf("unknown field type %q", field.Type))
		return m, nil
	}
}

func (m model) updateRunText(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "up":
			m.moveRunHistory(-1)
			return m, nil
		case "down":
			m.moveRunHistory(1)
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.runInput.Value())
			if value == "" && !field.Optional {
				m.setError(fmt.Errorf("%s is required", task.ResolveKnownValues(field.Label, m.runValues)))
				return m, nil
			}
			m.runValues[field.Key] = value
			return m.advanceRun()
		}
	}
	var cmd tea.Cmd
	m.runInput, cmd = m.runInput.Update(msg)
	return m, cmd
}

func (m model) updateRunChoice(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		minimum := 0
		if field.Optional {
			minimum = -1
		}
		if m.choiceCursor > minimum {
			m.choiceCursor--
		}
	case "down", "j":
		if m.choiceCursor < len(field.Options)-1 {
			m.choiceCursor++
		}
	case "enter":
		value := ""
		if m.choiceCursor >= 0 {
			value = task.ResolveKnownValues(field.Options[m.choiceCursor], m.runValues)
		}
		m.runValues[field.Key] = value
		return m.advanceRun()
	}
	return m, nil
}

// resizeFilePicker preserves selection through Bubbles' public navigation API;
// SetHeight alone leaves a selected row outside the window after shrinking.
func (m *model) resizeFilePicker() {
	height := m.runFormListHeight()
	if height == m.filePicker.Height {
		return
	}
	selected := 0
	if m.fileCount > 0 {
		view := m.filePicker.View()
		selected = m.runFileStart(view) + m.runFileCursorRow(view)
	}
	m.filePicker.SetHeight(height)
	if m.fileCount == 0 {
		return
	}
	m.positionFilePicker(selected)
}

func (m *model) positionFilePicker(selected int) {
	start, direction, steps := 'g', tea.KeyDown, selected
	if selected > m.fileCount/2 {
		start, direction, steps = 'G', tea.KeyUp, m.fileCount-1-selected
	}
	m.filePicker, _ = m.filePicker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{start}})
	for range steps {
		m.filePicker, _ = m.filePicker.Update(tea.KeyMsg{Type: direction})
	}
}

// filePickerCountMsg mirrors the picker's directory listing: the entry count
// for overflow detection and the sort-order names (dirs first, then lexical,
// matching the Bubbles picker's readDir) for absolute scroll position. The
// Bubbles file picker keeps its window indices unexported, so this public-API
// compatible mirror is the only way to compute the true thumb position.
type filePickerCountMsg struct {
	dir   string
	count int
	names []string
}

func countDirEntries(dir string, showHidden bool) tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return filePickerCountMsg{dir: dir}
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() == entries[j].IsDir() {
				return entries[i].Name() < entries[j].Name()
			}
			return entries[i].IsDir()
		})
		if !showHidden {
			var visible []os.DirEntry
			for _, entry := range entries {
				if hidden, _ := filepicker.IsHidden(entry.Name()); !hidden {
					visible = append(visible, entry)
				}
			}
			entries = visible
		}
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
			if entry.Type()&os.ModeSymlink != 0 {
				target, _ := filepath.EvalSymlinks(filepath.Join(dir, entry.Name()))
				names[i] += " → " + target
			}
		}
		return filePickerCountMsg{dir: dir, count: len(names), names: names}
	}
}

func (m model) updateRunFile(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	if count, ok := msg.(filePickerCountMsg); ok {
		if count.dir == m.filePicker.CurrentDirectory {
			m.fileCount = count.count
			m.fileNames = count.names
		}
		return m, nil
	}
	if field.Optional {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "enter":
				if m.choiceCursor < 0 {
					m.runValues[field.Key] = ""
					return m.advanceRun()
				}
			case "down", "j":
				if m.choiceCursor < 0 {
					m.choiceCursor = 0
					return m, nil
				}
			}
		}
	}
	previousDir := m.filePicker.CurrentDirectory
	var cmd tea.Cmd
	m.filePicker, cmd = m.filePicker.Update(msg)
	if msg, ok := msg.(tea.KeyMsg); ok && key.Matches(msg, m.filePicker.KeyMap.PageDown) {
		view := m.filePicker.View()
		row := m.runFileCursorRow(view)
		// Bubbles may include one extra row after paging at the end.
		if m.fileCount > 0 && row >= m.filePicker.Height {
			m.positionFilePicker(m.runFileStart(view) + row)
		}
	}
	var cmds []tea.Cmd
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	if m.filePicker.CurrentDirectory != previousDir {
		m.fileCount = 0
		m.fileNames = nil
		cmds = append(cmds, countDirEntries(m.filePicker.CurrentDirectory, m.filePicker.ShowHidden))
	}
	if selected, path := m.filePicker.DidSelectFile(msg); selected {
		m.runValues[field.Key] = path
		return m.advanceRun()
	}
	if disabled, path := m.filePicker.DidSelectDisabledFile(msg); disabled {
		m.setError(fmt.Errorf("cannot select %s", path))
	}
	return m, tea.Batch(cmds...)
}

func (m model) updateRunConfirm(msg tea.Msg, field task.Field) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "left", "right", "h", "l", "tab":
		m.confirmationYes = !m.confirmationYes
	case "y":
		m.confirmationYes = true
	case "n":
		m.confirmationYes = false
	case "enter":
		if !m.confirmationYes && !field.Optional {
			m.showScreen(screenList)
			m.setStatus(fmt.Sprintf("Cancelled %s", m.runTask.Name))
			return m, nil
		}
		m.runValues[field.Key] = strconv.FormatBool(m.confirmationYes)
		return m.advanceRun()
	}
	return m, nil
}

func (m *model) moveRunHistory(direction int) {
	if len(m.runHistory) == 0 {
		return
	}
	if direction < 0 {
		if m.runHistoryIndex == -1 {
			m.runHistoryDraft = m.runInput.Value()
		}
		if m.runHistoryIndex < len(m.runHistory)-1 {
			m.runHistoryIndex++
		}
		m.runInput.SetValue(m.runHistory[m.runHistoryIndex])
		return
	}
	if m.runHistoryIndex < 0 {
		return
	}
	m.runHistoryIndex--
	if m.runHistoryIndex == -1 {
		m.runInput.SetValue(m.runHistoryDraft)
		return
	}
	m.runInput.SetValue(m.runHistory[m.runHistoryIndex])
}

func (m model) advanceRun() (tea.Model, tea.Cmd) {
	m.runIndex++
	m.status = ""
	m.skipReferFields()
	if m.runIndex == len(m.runTask.Fields) {
		return m.executeRun()
	}
	return m, m.prepareRunField()
}

func (m model) previousRun() (tea.Model, tea.Cmd) {
	if m.runIndex == 0 {
		return m, nil
	}

	field := m.runTask.Fields[m.runIndex]
	switch field.Type {
	case task.FieldText:
		value := strings.TrimSpace(m.runInput.Value())
		if value != "" || field.Optional {
			m.runValues[field.Key] = value
		}
	case task.FieldChoice:
		value := ""
		if m.choiceCursor >= 0 {
			value = task.ResolveKnownValues(field.Options[m.choiceCursor], m.runValues)
		}
		m.runValues[field.Key] = value
	case task.FieldConfirm:
		m.runValues[field.Key] = strconv.FormatBool(m.confirmationYes)
	}

	m.runIndex--
	for m.runIndex > 0 && m.runTask.Fields[m.runIndex].Type == task.FieldRefer {
		m.runIndex--
	}
	m.status = ""
	return m, m.prepareRunField()
}

func (m model) executeRun() (tea.Model, tea.Cmd) {
	command, err := m.runTask.Render(m.runValues)
	if err != nil {
		m.showScreen(screenList)
		m.setError(err)
		return m, nil
	}
	m.showScreen(screenList)
	m.status = ""
	m.statusError = false
	if m.runTask.File != "" {
		if err := m.store.RecordValueHistory(m.runTask, m.runValues); err != nil {
			m.setError(fmt.Errorf("running %s; save value history: %w", m.runTask.Name, err))
		}
	}
	client := m.daemon
	item := m.runTask
	return m, func() tea.Msg {
		job, err := client.Start(item.File, item.Name, command, item.JobPolicy)
		if err != nil {
			err = fmt.Errorf("start %s: %w", item.Name, err)
		}
		return taskStartedMsg{job: job, err: err}
	}
}
