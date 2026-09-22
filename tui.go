package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/xarunoba/taskrunner/internal/task"
)

type screen uint8

const (
	screenList screen = iota
	screenTask
	screenField
	screenRun
)

var (
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Bold(true).Padding(0, 1)
	stepStyle     = selectedStyle
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1, 2)
)

var fieldTypes = []task.FieldType{
	task.FieldText,
	task.FieldChoice,
	task.FieldFile,
	task.FieldConfirm,
}

type commandDoneMsg struct {
	name string
	err  error
}

type model struct {
	store       *task.Store
	tasks       []task.Task
	cursor      int
	screen      screen
	status      string
	statusError bool
	width       int
	height      int

	taskInputs  [2]textinput.Model
	taskFocus   int
	formFields  []task.Field
	fieldCursor int
	editingFile string

	fieldInputs     [3]textinput.Model
	fieldFocus      int
	fieldTypeCursor int
	fieldRaw        bool
	fieldOptional   bool
	editingField    int

	runTask         task.Task
	runIndex        int
	runValues       map[string]string
	runInput        textinput.Model
	choiceCursor    int
	confirmationYes bool
	filePicker      filepicker.Model
}

func newModel(store *task.Store, tasks []task.Task) model {
	name := newInput("Build project", 100)
	command := newInput("go build ./...", 1000)
	key := newInput("environment", 50)
	label := newInput("Environment", 100)
	options := newInput("development, staging, production", 1000)

	return model{
		store:        store,
		tasks:        tasks,
		taskInputs:   [2]textinput.Model{name, command},
		fieldInputs:  [3]textinput.Model{key, label, options},
		editingField: -1,
	}
}

func newInput(placeholder string, limit int) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.Prompt = ""
	input.CharLimit = limit
	return input
}

func (m *model) resize(width, height int) {
	m.width = width
	m.height = height

	inputWidth := max(1, m.contentWidth())
	for i := range m.taskInputs {
		m.taskInputs[i].Width = inputWidth
	}
	for i := range m.fieldInputs {
		m.fieldInputs[i].Width = inputWidth
	}
	m.runInput.Width = inputWidth
	m.filePicker.SetHeight(m.filePickerHeight())
}

func (m model) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	width -= panelStyle.GetHorizontalFrameSize()
	if width > 90 {
		return 90
	}
	return max(1, width)
}

func (m model) contentHeight() int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	return max(1, height-panelStyle.GetVerticalFrameSize())
}

func (m model) filePickerHeight() int {
	return max(1, m.contentHeight()-8)
}

func (m model) gap() string {
	if m.height > 0 && m.height < 24 {
		return "\n"
	}
	return "\n\n"
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case commandDoneMsg:
		m.statusError = msg.err != nil
		if msg.err != nil {
			m.status = fmt.Sprintf("%s failed: %v", msg.name, msg.err)
		} else {
			m.status = fmt.Sprintf("%s finished", msg.name)
		}
		return m, nil
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	switch m.screen {
	case screenTask:
		return m.updateTaskForm(msg)
	case screenField:
		return m.updateFieldForm(msg)
	case screenRun:
		return m.updateRunForm(msg)
	default:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		return m.updateList(key)
	}
}

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	event := tea.MouseEvent(msg)
	if event.IsWheel() {
		keyType := tea.KeyDown
		if event.Button == tea.MouseButtonWheelUp {
			keyType = tea.KeyUp
		}
		key := tea.KeyMsg{Type: keyType}
		switch m.screen {
		case screenTask:
			return m.updateTaskForm(key)
		case screenField:
			return m.updateFieldForm(key)
		case screenRun:
			return m.updateRunForm(key)
		default:
			return m.updateList(key)
		}
	}
	if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft {
		return m, nil
	}

	line := m.mouseLine(event.Y)
	switch m.screen {
	case screenTask:
		return m.clickTaskForm(line, event.X)
	case screenField:
		return m.clickFieldForm(line, event.X)
	case screenRun:
		return m.clickRunForm(line, event.X)
	default:
		return m.clickTaskList(line)
	}
}

func (m model) mouseLine(y int) string {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if y < 0 || y >= len(lines) {
		return ""
	}
	line := strings.TrimSpace(lines[y])
	line = strings.TrimPrefix(line, "│")
	line = strings.TrimSuffix(line, "│")
	return strings.TrimSpace(line)
}

func (m model) clickTaskList(line string) (tea.Model, tea.Cmd) {
	if line == "" {
		return m, nil
	}
	for i, item := range m.tasks {
		name := ansi.Truncate(item.Name, max(1, m.contentWidth()-2), "…")
		if strings.Contains(line, name) {
			m.cursor = i
			return m, nil
		}
	}
	return m, nil
}

func (m model) clickTaskForm(line string, x int) (tea.Model, tea.Cmd) {
	switch {
	case line == "Name" || containsNonEmpty(line, m.taskInputs[0].Value()):
		m.taskFocus = 0
		m.taskInputs[0].SetCursor(max(0, x-3))
		return m, m.focusTaskControl()
	case line == "Command template" || containsNonEmpty(line, m.taskInputs[1].Value()):
		m.taskFocus = 1
		m.taskInputs[1].SetCursor(max(0, x-3))
		return m, m.focusTaskControl()
	case line == "Fields" || strings.Contains(line, "No fields."):
		m.taskFocus = 2
		return m, m.focusTaskControl()
	}
	for i, field := range m.formFields {
		if strings.Contains(line, field.Label) {
			m.taskFocus = 2
			m.fieldCursor = i
			return m, m.focusTaskControl()
		}
	}
	return m, nil
}

func (m model) clickFieldForm(line string, x int) (tea.Model, tea.Cmd) {
	switch {
	case line == "Key" || containsNonEmpty(line, m.fieldInputs[0].Value()):
		m.fieldFocus = 0
		m.fieldInputs[0].SetCursor(max(0, x-3))
	case line == "Label" || containsNonEmpty(line, m.fieldInputs[1].Value()):
		m.fieldFocus = 1
		m.fieldInputs[1].SetCursor(max(0, x-3))
	case line == "Type":
		m.fieldFocus = 2
	case strings.Contains(line, "Options (comma-separated)") || containsNonEmpty(line, m.fieldInputs[2].Value()):
		m.fieldFocus = 3
		m.fieldInputs[2].SetCursor(max(0, x-3))
	case line == "Interpolation":
		m.fieldFocus = 4
	case line == "Requirement":
		m.fieldFocus = 5
	default:
		typeNames := make([]string, len(fieldTypes))
		for i, fieldType := range fieldTypes {
			typeNames[i] = string(fieldType)
		}
		if selected := optionAtX(line, x-3, typeNames); selected >= 0 {
			m.fieldFocus = 2
			m.fieldTypeCursor = selected
		} else if selected := optionAtX(line, x-3, []string{"Argument", "Raw"}); selected >= 0 {
			m.fieldFocus = 4
			m.fieldRaw = selected == 1
		} else if selected := optionAtX(line, x-3, []string{"Required", "Optional"}); selected >= 0 {
			m.fieldFocus = 5
			m.fieldOptional = selected == 1
		}
	}
	return m, m.focusFieldControl()
}

func (m model) clickRunForm(line string, x int) (tea.Model, tea.Cmd) {
	field := m.runTask.Fields[m.runIndex]
	switch field.Type {
	case task.FieldText:
		m.runInput.SetCursor(max(0, x-3))
		return m, m.runInput.Focus()
	case task.FieldChoice:
		if field.Optional && strings.Contains(line, "Skip") {
			m.choiceCursor = -1
			break
		}
		for i, option := range field.Options {
			if strings.Contains(line, option) {
				m.choiceCursor = i
				break
			}
		}
	case task.FieldFile:
		if field.Optional && strings.Contains(line, "skip") {
			m.runValues[field.Key] = ""
			return m.advanceRun()
		}
	case task.FieldConfirm:
		if selected := optionAtX(line, x-3, []string{"No", "Yes"}); selected >= 0 {
			m.confirmationYes = selected == 1
		}
	}
	return m, nil
}

func containsNonEmpty(text, value string) bool {
	return value != "" && strings.Contains(text, value)
}

func optionAtX(line string, x int, options []string) int {
	for i, option := range options {
		start := strings.Index(line, option)
		if start < 0 {
			continue
		}
		if x >= start-2 && x <= start+len(option)+2 {
			return i
		}
	}
	return -1
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.tasks)-1 {
			m.cursor++
		}
	case "n":
		m.openTaskForm(task.Task{})
	case "e":
		if len(m.tasks) > 0 {
			m.openTaskForm(m.tasks[m.cursor])
		}
	case "d":
		if len(m.tasks) == 0 {
			break
		}
		name := m.tasks[m.cursor].Name
		if err := m.store.Delete(m.tasks[m.cursor]); err != nil {
			m.setError(err)
			break
		}
		if err := m.reload(); err != nil {
			m.setError(err)
			break
		}
		m.setStatus(fmt.Sprintf("Deleted %s", name))
	case "enter":
		if len(m.tasks) == 0 {
			break
		}
		return m.startRun(m.tasks[m.cursor])
	}
	return m, nil
}

func (m model) updateTaskForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.closeTaskForm()
			return m, nil
		case "ctrl+s":
			return m.saveTaskForm()
		case "tab":
			m.taskFocus = (m.taskFocus + 1) % 3
			return m, m.focusTaskControl()
		case "shift+tab":
			m.taskFocus = (m.taskFocus + 2) % 3
			return m, m.focusTaskControl()
		case "up":
			if m.taskFocus == 2 && m.fieldCursor > 0 {
				m.fieldCursor--
				return m, nil
			}
			if m.taskFocus > 0 {
				m.taskFocus--
				return m, m.focusTaskControl()
			}
		case "down":
			if m.taskFocus == 2 {
				if m.fieldCursor < len(m.formFields)-1 {
					m.fieldCursor++
				}
				return m, nil
			}
			m.taskFocus++
			return m, m.focusTaskControl()
		case "enter":
			if m.taskFocus < 2 {
				m.taskFocus++
				return m, m.focusTaskControl()
			}
			if len(m.formFields) == 0 {
				m.openFieldForm(-1)
			} else {
				m.openFieldForm(m.fieldCursor)
			}
			return m, nil
		}

		if m.taskFocus == 2 {
			switch key.String() {
			case "k":
				if m.fieldCursor > 0 {
					m.fieldCursor--
				} else {
					m.taskFocus = 1
					return m, m.focusTaskControl()
				}
			case "j":
				if m.fieldCursor < len(m.formFields)-1 {
					m.fieldCursor++
				}
			case "a":
				m.openFieldForm(-1)
			case "e":
				if len(m.formFields) > 0 {
					m.openFieldForm(m.fieldCursor)
				}
			case "d":
				if len(m.formFields) > 0 {
					m.formFields = append(m.formFields[:m.fieldCursor], m.formFields[m.fieldCursor+1:]...)
					if m.fieldCursor >= len(m.formFields) && m.fieldCursor > 0 {
						m.fieldCursor--
					}
				}
			}
			return m, nil
		}
	}

	if m.taskFocus == 2 {
		return m, nil
	}

	var cmd tea.Cmd
	m.taskInputs[m.taskFocus], cmd = m.taskInputs[m.taskFocus].Update(msg)
	return m, cmd
}

func (m *model) openTaskForm(item task.Task) {
	m.screen = screenTask
	m.editingFile = item.File
	m.taskInputs[0].SetValue(item.Name)
	m.taskInputs[1].SetValue(item.Command)
	m.formFields = cloneFields(item.Fields)
	m.taskFocus = 0
	m.fieldCursor = 0
	m.status = ""
	m.focusTaskControl()
}

func (m *model) closeTaskForm() {
	m.screen = screenList
	m.editingFile = ""
	m.formFields = nil
	for i := range m.taskInputs {
		m.taskInputs[i].Blur()
		m.taskInputs[i].SetValue("")
	}
}

func (m *model) focusTaskControl() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.taskInputs {
		if i == m.taskFocus {
			cmd = m.taskInputs[i].Focus()
			continue
		}
		m.taskInputs[i].Blur()
	}
	return cmd
}

func (m model) saveTaskForm() (tea.Model, tea.Cmd) {
	item := task.Task{
		Name:    m.taskInputs[0].Value(),
		Command: m.taskInputs[1].Value(),
		Fields:  cloneFields(m.formFields),
	}
	saved, err := m.store.Save(item, m.editingFile)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	if err := m.reload(); err != nil {
		m.setError(err)
		return m, nil
	}
	m.closeTaskForm()
	for i := range m.tasks {
		if m.tasks[i].File == saved.File {
			m.cursor = i
			break
		}
	}
	m.setStatus(fmt.Sprintf("Saved %s", saved.Name))
	return m, nil
}

func (m model) updateFieldForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.screen = screenTask
			m.taskFocus = 2
			return m, m.focusTaskControl()
		case "ctrl+s", "enter":
			return m.saveFieldForm()
		case "tab", "down":
			m.moveFieldFocus(1)
			return m, m.focusFieldControl()
		case "shift+tab", "up":
			m.moveFieldFocus(-1)
			return m, m.focusFieldControl()
		case "left", "h":
			if m.fieldFocus == 2 {
				m.fieldTypeCursor = (m.fieldTypeCursor + len(fieldTypes) - 1) % len(fieldTypes)
				return m, nil
			}
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 5 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		case "right", "l":
			if m.fieldFocus == 2 {
				m.fieldTypeCursor = (m.fieldTypeCursor + 1) % len(fieldTypes)
				return m, nil
			}
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 5 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		case " ":
			if m.fieldFocus == 4 {
				m.fieldRaw = !m.fieldRaw
				return m, nil
			}
			if m.fieldFocus == 5 {
				m.fieldOptional = !m.fieldOptional
				return m, nil
			}
		}
	}

	if m.fieldFocus == 2 || m.fieldFocus == 4 || m.fieldFocus == 5 {
		return m, nil
	}
	inputIndex := m.fieldFocus
	if inputIndex == 3 {
		inputIndex = 2
	}
	var cmd tea.Cmd
	m.fieldInputs[inputIndex], cmd = m.fieldInputs[inputIndex].Update(msg)
	return m, cmd
}

func (m *model) openFieldForm(index int) {
	m.screen = screenField
	m.editingField = index
	m.fieldFocus = 0
	m.fieldTypeCursor = 0
	m.fieldRaw = false
	m.fieldOptional = false
	m.status = ""
	for i := range m.fieldInputs {
		m.fieldInputs[i].SetValue("")
	}

	if index >= 0 {
		field := m.formFields[index]
		m.fieldInputs[0].SetValue(field.Key)
		m.fieldInputs[1].SetValue(field.Label)
		m.fieldInputs[2].SetValue(strings.Join(field.Options, ", "))
		m.fieldRaw = field.Raw
		m.fieldOptional = field.Optional
		for i, fieldType := range fieldTypes {
			if field.Type == fieldType {
				m.fieldTypeCursor = i
				break
			}
		}
	}
	m.focusFieldControl()
}

func (m *model) focusFieldControl() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.fieldInputs {
		focus := i == m.fieldFocus || i == 2 && m.fieldFocus == 3
		if focus {
			cmd = m.fieldInputs[i].Focus()
			continue
		}
		m.fieldInputs[i].Blur()
	}
	return cmd
}

func (m *model) moveFieldFocus(direction int) {
	for {
		m.fieldFocus = (m.fieldFocus + direction + 6) % 6
		if m.fieldFocus != 3 || fieldTypes[m.fieldTypeCursor] == task.FieldChoice {
			return
		}
	}
}

func (m model) saveFieldForm() (tea.Model, tea.Cmd) {
	field := task.Field{
		Key:      strings.TrimSpace(m.fieldInputs[0].Value()),
		Label:    strings.TrimSpace(m.fieldInputs[1].Value()),
		Type:     fieldTypes[m.fieldTypeCursor],
		Raw:      m.fieldRaw,
		Optional: m.fieldOptional,
	}
	if field.Type == task.FieldChoice {
		for option := range strings.SplitSeq(m.fieldInputs[2].Value(), ",") {
			field.Options = append(field.Options, strings.TrimSpace(option))
		}
	}

	fields := cloneFields(m.formFields)
	if m.editingField < 0 {
		fields = append(fields, field)
	} else {
		fields[m.editingField] = field
	}
	candidate := task.Task{Name: "validate", Command: "true", Fields: fields}
	if err := candidate.Validate(); err != nil {
		m.setError(err)
		return m, nil
	}

	m.formFields = fields
	if m.editingField < 0 {
		m.fieldCursor = len(fields) - 1
	}
	m.screen = screenTask
	m.taskFocus = 2
	m.status = ""
	return m, m.focusTaskControl()
}

func (m model) startRun(item task.Task) (tea.Model, tea.Cmd) {
	m.runTask = item
	m.runIndex = 0
	m.runValues = make(map[string]string, len(item.Fields))
	m.status = ""
	if len(item.Fields) == 0 {
		return m.executeRun()
	}
	m.screen = screenRun
	return m, m.prepareRunField()
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
		m.runInput = newInput(field.Label, 1000)
		m.runInput.SetValue(m.runValues[field.Key])
		m.runInput.Width = m.contentWidth()
		return m.runInput.Focus()
	case task.FieldChoice:
		for i, option := range field.Options {
			if option == m.runValues[field.Key] {
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
		m.filePicker.Styles.Cursor = cursorStyle
		m.filePicker.Styles.Selected = selectedStyle
		m.filePicker.SetHeight(m.filePickerHeight())
		return m.filePicker.Init()
	}
	return nil
}

func (m model) updateRunForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.screen = screenList
			m.setStatus(fmt.Sprintf("Cancelled %s", m.runTask.Name))
			return m, nil
		case "shift+tab", "ctrl+left":
			return m.previousRun()
		case "up":
			if m.runTask.Fields[m.runIndex].Type == task.FieldText {
				return m.previousRun()
			}
		}
	}

	field := m.runTask.Fields[m.runIndex]
	switch field.Type {
	case task.FieldText:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
			value := strings.TrimSpace(m.runInput.Value())
			if value == "" && !field.Optional {
				m.setError(fmt.Errorf("%s is required", field.Label))
				return m, nil
			}
			m.runValues[field.Key] = value
			return m.advanceRun()
		}
		var cmd tea.Cmd
		m.runInput, cmd = m.runInput.Update(msg)
		return m, cmd
	case task.FieldChoice:
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
				value = field.Options[m.choiceCursor]
			}
			m.runValues[field.Key] = value
			return m.advanceRun()
		}
		return m, nil
	case task.FieldFile:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "s" && field.Optional {
			m.runValues[field.Key] = ""
			return m.advanceRun()
		}
		var cmd tea.Cmd
		m.filePicker, cmd = m.filePicker.Update(msg)
		if selected, path := m.filePicker.DidSelectFile(msg); selected {
			m.runValues[field.Key] = path
			return m.advanceRun()
		}
		if disabled, path := m.filePicker.DidSelectDisabledFile(msg); disabled {
			m.setError(fmt.Errorf("cannot select %s", path))
		}
		return m, cmd
	case task.FieldConfirm:
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
				m.screen = screenList
				m.setStatus(fmt.Sprintf("Cancelled %s", m.runTask.Name))
				return m, nil
			}
			m.runValues[field.Key] = strconv.FormatBool(m.confirmationYes)
			return m.advanceRun()
		}
		return m, nil
	default:
		m.screen = screenList
		m.setError(fmt.Errorf("unknown field type %q", field.Type))
		return m, nil
	}
}

func (m model) advanceRun() (tea.Model, tea.Cmd) {
	m.runIndex++
	m.status = ""
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
			value = field.Options[m.choiceCursor]
		}
		m.runValues[field.Key] = value
	case task.FieldConfirm:
		m.runValues[field.Key] = strconv.FormatBool(m.confirmationYes)
	}

	m.runIndex--
	m.status = ""
	return m, m.prepareRunField()
}

func (m model) executeRun() (tea.Model, tea.Cmd) {
	command, err := m.runTask.Render(m.runValues)
	if err != nil {
		m.screen = screenList
		m.setError(err)
		return m, nil
	}
	m.screen = screenList
	m.setStatus(fmt.Sprintf("Running %s...", m.runTask.Name))
	return m, runCommand(m.store.Workspace(), m.runTask.Name, command)
}

func cloneFields(fields []task.Field) []task.Field {
	cloned := make([]task.Field, len(fields))
	for i, field := range fields {
		cloned[i] = field
		cloned[i].Options = append([]string(nil), field.Options...)
	}
	return cloned
}

func (m *model) reload() error {
	items, err := m.store.Load()
	if err != nil {
		return err
	}
	m.tasks = items
	if len(m.tasks) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.tasks) {
		m.cursor = len(m.tasks) - 1
	}
	return nil
}

func (m *model) setError(err error) {
	m.status = err.Error()
	m.statusError = true
}

func (m *model) setStatus(status string) {
	m.status = status
	m.statusError = false
}

func runCommand(workspace, name, command string) tea.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-c", command)
	cmd.Dir = workspace
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return commandDoneMsg{name: name, err: err}
	})
}

func (m model) View() string {
	switch m.screen {
	case screenTask:
		return m.taskFormView()
	case screenField:
		return m.fieldFormView()
	case screenRun:
		return m.runFormView()
	default:
		return m.listView()
	}
}

func (m model) listView() string {
	var body strings.Builder
	body.WriteString(accentStyle.Render("TASKRUNNER"))
	body.WriteString("\n")
	body.WriteString(mutedStyle.Render(m.store.Workspace()))
	body.WriteString(m.gap())

	if len(m.tasks) == 0 {
		body.WriteString(mutedStyle.Render("No tasks yet. Press n to create one."))
	} else {
		start, end := visibleRange(len(m.tasks), m.cursor, max(1, m.contentHeight()-10))
		if start > 0 {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↑ %d more", start)))
			body.WriteByte('\n')
		}
		for i := start; i < end; i++ {
			item := m.tasks[i]
			name := "  " + item.Name
			if i == m.cursor {
				name = selectedStyle.Render("› " + item.Name)
			}
			body.WriteString(name)
			body.WriteByte('\n')
			if i == m.cursor {
				body.WriteString("  ")
				body.WriteString(mutedStyle.Render(item.Command))
				body.WriteByte('\n')
				body.WriteString("  ")
				body.WriteString(mutedStyle.Render(fmt.Sprintf("%d fields", len(item.Fields))))
				body.WriteByte('\n')
			}
		}
		if end < len(m.tasks) {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↓ %d more", len(m.tasks)-end)))
			body.WriteByte('\n')
		}
	}

	body.WriteString(m.gap())
	body.WriteString(mutedStyle.Render("enter run • n new • e edit • d delete • q quit"))
	body.WriteString(m.statusView())
	return m.renderPanel(body.String())
}

func visibleRange(total, current, limit int) (int, int) {
	if total <= limit {
		return 0, total
	}
	start := current - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > total {
		start = total - limit
	}
	return start, start + limit
}

func (m model) taskFormView() string {
	title := "NEW TASK"
	if m.editingFile != "" {
		title = "EDIT TASK"
	}

	if m.contentHeight() < 9 {
		return m.compactTaskFormView(title)
	}

	var body strings.Builder
	body.WriteString(accentStyle.Render(title))
	body.WriteString(m.gap())
	body.WriteString(taskStepTitle("Name", m.taskFocus == 0))
	body.WriteByte('\n')
	body.WriteString(m.taskInputs[0].View())
	body.WriteString(m.gap())
	body.WriteString(taskStepTitle("Command template", m.taskFocus == 1))
	body.WriteByte('\n')
	body.WriteString(m.taskInputs[1].View())
	body.WriteString("\n")
	body.WriteString(mutedStyle.Render("Use {{field_key}} where a runtime value belongs."))
	body.WriteString(m.gap())
	body.WriteString(taskStepTitle("Fields", m.taskFocus == 2))
	body.WriteByte('\n')
	if len(m.formFields) == 0 {
		empty := mutedStyle.Render("No fields. Press a or enter to add one.")
		if m.taskFocus == 2 {
			empty = selectedStyle.Render("› No fields. Press a or enter to add one.")
		}
		body.WriteString(empty)
	} else {
		start, end := visibleRange(len(m.formFields), m.fieldCursor, max(1, m.contentHeight()-12))
		if start > 0 {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↑ %d more", start)))
			body.WriteByte('\n')
		}
		for i := start; i < end; i++ {
			field := m.formFields[i]
			mode := "argument"
			if field.Raw {
				mode = "RAW"
			}
			requirement := "required"
			if field.Optional {
				requirement = "optional"
			}
			line := fmt.Sprintf("  %s (%s, %s, %s → {{%s}})", field.Label, field.Type, requirement, mode, field.Key)
			if m.taskFocus == 2 && i == m.fieldCursor {
				line = selectedStyle.Render("› " + strings.TrimSpace(line))
			}
			body.WriteString(line)
			body.WriteByte('\n')
		}
		if end < len(m.formFields) {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↓ %d more", len(m.formFields)-end)))
			body.WriteByte('\n')
		}
	}
	body.WriteString(m.gap())
	body.WriteString(mutedStyle.Render("↑/↓ section • tab section • a add • e edit • d delete • ctrl+s save • esc cancel"))
	body.WriteString(m.statusView())
	return m.renderPanel(body.String())
}

func (m model) compactTaskFormView(title string) string {
	var body strings.Builder
	body.WriteString(accentStyle.Render(title))
	body.WriteString(m.gap())

	switch m.taskFocus {
	case 0:
		body.WriteString(taskStepTitle("Name", true))
		body.WriteByte('\n')
		body.WriteString(m.taskInputs[0].View())
	case 1:
		body.WriteString(taskStepTitle("Command template", true))
		body.WriteByte('\n')
		body.WriteString(m.taskInputs[1].View())
	case 2:
		body.WriteString(taskStepTitle("Fields", true))
		body.WriteByte('\n')
		if len(m.formFields) == 0 {
			body.WriteString(selectedStyle.Render("› No fields. Press a or enter to add one."))
			break
		}
		field := m.formFields[m.fieldCursor]
		mode := "argument"
		if field.Raw {
			mode = "RAW"
		}
		requirement := "required"
		if field.Optional {
			requirement = "optional"
		}
		body.WriteString(selectedStyle.Render(fmt.Sprintf(
			"› %s (%s, %s, %s → {{%s}})",
			field.Label,
			field.Type,
			requirement,
			mode,
			field.Key,
		)))
	}

	body.WriteString(m.gap())
	body.WriteString(mutedStyle.Render("↑/↓ section • tab section • ctrl+s save • esc cancel"))
	body.WriteString(m.statusView())
	return m.renderPanel(body.String())
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

func (m model) fieldFormView() string {
	title := "ADD FIELD"
	if m.editingField >= 0 {
		title = "EDIT FIELD"
	}

	var body strings.Builder
	body.WriteString(accentStyle.Render(title))
	body.WriteString(m.gap())
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
	body.WriteString(m.gap())
	body.WriteString(mutedStyle.Render("↑/↓ step • ←/→ change • space toggle • enter save • esc cancel"))
	body.WriteString(m.statusView())
	return m.renderPanel(body.String())
}

func (m model) runFormView() string {
	field := m.runTask.Fields[m.runIndex]
	var body strings.Builder
	body.WriteString(accentStyle.Render(m.runTask.Name))
	body.WriteString("\n")
	body.WriteString(mutedStyle.Render(fmt.Sprintf("Field %d of %d", m.runIndex+1, len(m.runTask.Fields))))
	body.WriteString(m.gap())
	label := field.Label
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
		body.WriteString(m.gap())
		help := "enter continue • ↑/shift+tab back • esc cancel"
		if field.Optional {
			help = "enter skip/continue • ↑/shift+tab back • esc cancel"
		}
		body.WriteString(mutedStyle.Render(help))
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
			option := "  " + field.Options[i]
			if i == m.choiceCursor {
				option = selectedStyle.Render("› " + field.Options[i])
			}
			body.WriteString(option)
			body.WriteByte('\n')
		}
		if end < len(field.Options) {
			body.WriteString(mutedStyle.Render(fmt.Sprintf("↓ %d more", len(field.Options)-end)))
			body.WriteByte('\n')
		}
		body.WriteString(m.gap())
		body.WriteString(mutedStyle.Render("↑/↓ choose • shift+tab back • enter continue • esc cancel"))
	case task.FieldFile:
		body.WriteString(m.filePicker.View())
		body.WriteString(m.gap())
		help := "↑/↓ browse • shift+tab back • enter open/select • esc cancel"
		if field.Optional {
			help = "↑/↓ browse • s/click skip • enter select • shift+tab back • esc cancel"
		}
		body.WriteString(mutedStyle.Render(help))
	case task.FieldConfirm:
		selected := 0
		if m.confirmationYes {
			selected = 1
		}
		body.WriteString(m.pickerRow([]string{"No", "Yes"}, selected))
		body.WriteString(m.gap())
		help := "←/→ choose • shift+tab back • enter confirm • esc cancel"
		if field.Optional {
			help = "←/→ choose • enter continue • shift+tab back • esc cancel"
		}
		body.WriteString(mutedStyle.Render(help))
	}
	body.WriteString(m.statusView())
	return m.renderPanel(body.String())
}

func (m model) statusView() string {
	if m.status == "" {
		return ""
	}
	style := okStyle
	if m.statusError {
		style = errorStyle
	}
	return m.gap() + style.Render(m.status)
}

func (m model) renderPanel(content string) string {
	width := m.contentWidth()
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}

	height := m.contentHeight()
	if len(lines) > height {
		switch height {
		case 1:
			lines = lines[:1]
		case 2:
			lines = []string{lines[0], lines[len(lines)-1]}
		default:
			lines = append(lines[:height-2], mutedStyle.Render("…"), lines[len(lines)-1])
		}
	}
	return panelStyle.Width(width + panelStyle.GetHorizontalPadding()).Render(strings.Join(lines, "\n"))
}
