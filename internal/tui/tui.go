package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

type screen uint8

const taskCommandMinHeight = 3

const (
	screenList screen = iota
	screenTask
	screenField
	screenRun
	screenJobs
	screenResult
)

var (
	accentColor         = lipgloss.Color("63")
	accentStyle         = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	borderStyle         = lipgloss.NewStyle().Foreground(accentColor)
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cursorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selectedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("57")).Bold(true).Padding(0, 1)
	activeTabStyle      = selectedStyle
	inactiveTabStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("236")).Padding(0, 1)
	stepStyle           = selectedStyle
	errorStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	panelStyle          = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentColor).Padding(1, 2)
	statusBarStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("236"))
	statusBarErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("196")).Bold(true)
	jobQueuedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(accentColor).Bold(true).Padding(0, 1)
	jobSucceededStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("42")).Bold(true).Padding(0, 1)
	jobFailedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("196")).Bold(true).Padding(0, 1)
	jobCanceledStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("241")).Bold(true).Padding(0, 1)
	helpModalStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentColor).Padding(1, 2)
)

var fieldTypes = []task.FieldType{
	task.FieldText,
	task.FieldChoice,
	task.FieldFile,
	task.FieldConfirm,
}

var jobPolicies = []task.JobPolicy{
	task.JobSequential,
	task.JobParallel,
	task.JobCancelPrevious,
}

var jobPolicyLabels = []string{
	"Sequential",
	"Parallel",
	"Cancel previous",
}

type daemonPollMsg struct {
	jobs    []daemon.Job
	details map[string]daemon.Job
	err     error
}

type taskStartedMsg struct {
	job daemon.Job
	err error
}

type jobOpenedMsg struct {
	job daemon.Job
	err error
}

type jobActionMsg struct {
	action string
	job    daemon.Job
	err    error
}

type helpBinding struct {
	key         string
	description string
}

type helpGroup struct {
	title    string
	bindings []helpBinding
}

type model struct {
	store        *task.Store
	daemon       *daemon.Client
	tasks        []task.Task
	jobs         []daemon.Job
	running      map[string]int
	queued       map[string]int
	latest       map[string]daemon.Status
	trackedJobs  map[string]struct{}
	cursor       int
	jobCursor    int
	screen       screen
	taskScreen   screen
	jobScreen    screen
	status       string
	statusError  bool
	width        int
	height       int
	helpOpen     bool
	helpViewport viewport.Model

	taskNameInput    textinput.Model
	taskCommandInput textarea.Model
	taskViewport     viewport.Model
	taskFocus        int
	formFields       []task.Field
	formJobPolicy    task.JobPolicy
	fieldCursor      int
	editingFile      string
	standaloneForm   bool

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
	runHistory      []string
	runHistoryIndex int
	runHistoryDraft string
	choiceCursor    int
	confirmationYes bool
	filePicker      filepicker.Model

	result         daemon.Job
	resultViewport viewport.Model
}

func newModel(store *task.Store, tasks []task.Task) model {
	name := newInput("Build project", 100)
	command := textarea.New()
	command.Placeholder = "go build ./..."
	command.Prompt = ""
	command.ShowLineNumbers = false
	command.CharLimit = 1000
	command.SetHeight(taskCommandMinHeight)
	key := newInput("environment", 50)
	label := newInput("Environment", 100)
	options := newInput("development, staging, production", 1000)
	resultViewport := viewport.New(1, 1)
	taskViewport := viewport.New(1, 1)
	helpViewport := viewport.New(1, 1)

	return model{
		store:            store,
		daemon:           daemon.NewClient(store.Workspace()),
		tasks:            tasks,
		running:          make(map[string]int),
		queued:           make(map[string]int),
		latest:           make(map[string]daemon.Status),
		trackedJobs:      make(map[string]struct{}),
		jobScreen:        screenJobs,
		taskNameInput:    name,
		taskCommandInput: command,
		taskViewport:     taskViewport,
		helpViewport:     helpViewport,
		fieldInputs:      [3]textinput.Model{key, label, options},
		editingField:     -1,
		resultViewport:   resultViewport,
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
	m.taskNameInput.Width = max(1, inputWidth-1)
	m.resizeTaskForm()
	for i := range m.fieldInputs {
		m.fieldInputs[i].Width = inputWidth
	}
	m.runInput.Width = inputWidth
	m.filePicker.SetHeight(m.filePickerHeight())
	m.resizeResultViewport()
}

func (m model) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	return max(1, width-panelStyle.GetHorizontalFrameSize())
}

func (m model) contentHeight() int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	return max(1, height-panelStyle.GetVerticalFrameSize()-1)
}

func (m *model) resizeTaskForm() {
	width := max(1, m.contentWidth()-1)
	m.taskViewport.Width = width
	m.taskViewport.Height = m.contentHeight()
	m.taskCommandInput.SetWidth(width)
	m.taskCommandInput.SetHeight(taskCommandVisualHeight(m.taskCommandInput.Value(), width))
}

func taskCommandVisualHeight(command string, width int) int {
	return max(taskCommandMinHeight, taskCommandLineHeight(command, width))
}

func taskCommandLineHeight(command string, width int) int {
	wrapped := ansi.Wrap(command, max(1, width), " ")
	return strings.Count(wrapped, "\n") + 1
}

func (m model) taskCommandView() string {
	lines := strings.Split(m.taskCommandInput.View(), "\n")
	return strings.Join(lines[:min(m.taskCommandInput.Height(), len(lines))], "\n")
}

func scrollBar(height int, scrollable bool, percent float64) string {
	lines := make([]string, max(1, height))
	if !scrollable {
		return strings.Join(lines, "\n")
	}
	thumb := int(percent*float64(len(lines)-1) + 0.5)
	for i := range lines {
		lines[i] = mutedStyle.Render("│")
	}
	lines[thumb] = accentStyle.Render("█")
	return strings.Join(lines, "\n")
}

func (m model) filePickerHeight() int {
	return max(1, m.contentHeight()-8)
}

func (m *model) resizeResultViewport() {
	atBottom := m.resultViewport.AtBottom()
	m.resultViewport.Width = max(1, m.contentWidth()-1)
	chromeHeight := 1 // Job status and task name.
	if m.result.StorageError != "" {
		chromeHeight++
	}
	if m.gap() == "\n\n" {
		chromeHeight++
	}
	m.resultViewport.Height = max(1, m.contentHeight()-chromeHeight)
	if atBottom {
		m.resultViewport.GotoBottom()
	}
}

func (m model) gap() string {
	if m.height > 0 && m.height < 24 {
		return "\n"
	}
	return "\n\n"
}

func (m model) acceptsTextInput() bool {
	switch m.screen {
	case screenTask:
		return m.taskFocus == 0 || m.taskFocus == 1
	case screenField:
		return m.fieldFocus == 0 || m.fieldFocus == 1 || m.fieldFocus == 3
	case screenRun:
		return len(m.runTask.Fields) > 0 && m.runTask.Fields[m.runIndex].Type == task.FieldText
	default:
		return false
	}
}

func (m model) helpGroups() []helpGroup {
	general := helpGroup{
		title: "General",
		bindings: []helpBinding{
			{key: "tab / f6", description: "switch Tasks or Jobs"},
			{key: "q / ctrl+c", description: "quit"},
			{key: "? / f1 / esc", description: "close keybinds"},
		},
	}
	switch m.screen {
	case screenTask:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "tab / shift+tab", description: "move between sections"},
					{key: "↑ / ↓", description: "move or select"},
					{key: "enter", description: "insert command line or edit field"},
					{key: "← / →", description: "change job policy"},
					{key: "a / e / d", description: "add, edit, or delete field"},
					{key: "[ / ]", description: "reorder selected field"},
					{key: "f2 / ctrl+s", description: "save task"},
					{key: "esc", description: "cancel"},
				},
			},
			general,
		}
	case screenField:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "tab / shift+tab", description: "move between controls"},
					{key: "↑ / ↓", description: "move between controls"},
					{key: "← / → / space", description: "change selected option"},
					{key: "enter / ctrl+s", description: "save field"},
					{key: "esc", description: "cancel"},
				},
			},
			general,
		}
	case screenRun:
		bindings := []helpBinding{
			{key: "enter", description: "accept and continue"},
			{key: "shift+tab / ctrl+←", description: "return to previous field"},
			{key: "esc", description: "cancel run"},
		}
		if len(m.runTask.Fields) > 0 {
			switch m.runTask.Fields[m.runIndex].Type {
			case task.FieldText, task.FieldChoice:
				bindings = append(bindings, helpBinding{key: "↑ / ↓", description: "history or choice"})
			case task.FieldFile:
				bindings = append(bindings, helpBinding{key: "↑ / ↓", description: "browse files"})
			case task.FieldConfirm:
				bindings = append(bindings, helpBinding{key: "← / →", description: "choose No or Yes"})
			}
		}
		return []helpGroup{{bindings: bindings}, general}
	case screenJobs:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "↑ / k", description: "select previous job"},
					{key: "↓ / j", description: "select next job"},
					{key: "enter", description: "open selected log"},
					{key: "c", description: "cancel selected job"},
					{key: "r", description: "rerun selected job"},
					{key: "d", description: "delete selected completed job"},
				},
			},
			general,
		}
	case screenResult:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "↑ / ↓", description: "scroll one line"},
					{key: "pgup / pgdown", description: "scroll one page"},
					{key: "home / g", description: "jump to top"},
					{key: "end / G", description: "jump to bottom"},
					{key: "c / r / d", description: "cancel, rerun, or delete job"},
					{key: "esc", description: "return to Jobs"},
				},
			},
			general,
		}
	default:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "↑ / k", description: "select previous task"},
					{key: "↓ / j", description: "select next task"},
					{key: "enter", description: "run selected task"},
					{key: "n / e / d", description: "new, edit, or delete task"},
				},
			},
			general,
		}
	}
}

func (m model) helpContent() string {
	var body strings.Builder
	for _, group := range m.helpGroups() {
		if body.Len() > 0 {
			body.WriteString("\n\n")
		}
		if group.title != "" {
			body.WriteString(accentStyle.Render(group.title))
			body.WriteByte('\n')
		}
		keyWidth := 0
		for _, binding := range group.bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(binding.key))
		}
		for i, binding := range group.bindings {
			if i > 0 {
				body.WriteByte('\n')
			}
			body.WriteString("  ")
			body.WriteString(mutedStyle.Render(binding.key))
			body.WriteString(strings.Repeat(" ", keyWidth-ansi.StringWidth(binding.key)+2))
			body.WriteString(binding.description)
		}
	}
	return body.String()
}

func (m model) helpScreenName() string {
	switch m.screen {
	case screenTask:
		return "Task editor"
	case screenField:
		return "Field editor"
	case screenRun:
		return "Run task"
	case screenJobs:
		return "Jobs"
	case screenResult:
		return "Job log"
	default:
		return "Tasks"
	}
}

func (m model) helpTitle() string {
	return "Keybinds - " + m.helpScreenName()
}

func (m model) compactHelp() bool {
	return m.width < 60 || m.height < 18
}

func (m *model) syncHelpViewport() {
	content := m.helpContent()
	lineCount := strings.Count(content, "\n") + 1
	if m.compactHelp() {
		m.helpViewport.Width = max(1, m.contentWidth()-1)
		m.helpViewport.Height = m.contentHeight()
	} else {
		modalWidth := min(72, max(1, m.width-8))
		m.helpViewport.Width = max(1, modalWidth-helpModalStyle.GetHorizontalFrameSize())
		maxHeight := max(1, m.height-6-helpModalStyle.GetVerticalFrameSize())
		m.helpViewport.Height = min(lineCount, maxHeight)
	}
	m.helpViewport.SetContent(content)
}

func (m *model) openHelp() {
	m.helpOpen = true
	m.syncHelpViewport()
	m.helpViewport.GotoTop()
}

func (m model) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "?", "f1", "esc":
			m.helpOpen = false
			return m, nil
		case "q":
			return m, tea.Quit
		case "home", "g":
			m.helpViewport.GotoTop()
			return m, nil
		case "end", "G":
			m.helpViewport.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.helpViewport, cmd = m.helpViewport.Update(msg)
	return m, cmd
}

func (m model) footerControls() []string {
	target := "Jobs"
	switchKey := "tab"
	if topLevelTab(m.screen) == screenJobs {
		target = "Tasks"
	}
	if m.screen != screenList && m.screen != screenJobs {
		switchKey = "f6"
	}
	quitKey := "q"
	helpKey := "?"
	if m.acceptsTextInput() {
		quitKey = "ctrl+c"
		helpKey = "f1"
	}
	return []string{
		fmt.Sprintf("%s switch to %s", switchKey, target),
		quitKey + " quit",
		helpKey + " keybinds",
	}
}

func (m model) topLevelFooter() string {
	return strings.Join(m.footerControls(), " • ")
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.pollDaemon())
}

func (m model) pollDaemon() tea.Cmd {
	client := m.daemon
	tracked := make(map[string]struct{}, len(m.trackedJobs))
	for id := range m.trackedJobs {
		tracked[id] = struct{}{}
	}
	resultID := ""
	resultOffset := 0
	if m.screen == screenResult && m.result.ID != "" && !m.result.Done() {
		resultID = m.result.ID
		resultOffset = m.result.OutputSize
	}
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		jobs, err := client.Jobs()
		if err != nil {
			return daemonPollMsg{err: err}
		}
		details := make(map[string]daemon.Job)
		for _, job := range jobs {
			offset := 0
			_, trackedJob := tracked[job.ID]
			switch {
			case trackedJob && job.Done():
			case job.ID == resultID:
				offset = resultOffset
			default:
				continue
			}
			detail, err := client.Job(job.ID, offset)
			if err != nil {
				return daemonPollMsg{err: err}
			}
			details[job.ID] = detail
		}
		return daemonPollMsg{jobs: jobs, details: details}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		if m.helpOpen {
			m.syncHelpViewport()
		}
		return m, nil
	case daemonPollMsg:
		m.applyDaemonPoll(msg)
		return m, m.pollDaemon()
	case taskStartedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.trackedJobs[msg.job.ID] = struct{}{}
		return m, nil
	case jobOpenedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.openResult(msg.job)
		return m, nil
	case jobActionMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.applyJobAction(msg)
		return m, nil
	case tea.MouseMsg:
		if m.helpOpen {
			if tea.MouseEvent(msg).IsWheel() {
				return m.updateHelp(msg)
			}
			return m, nil
		}
		return m.updateMouse(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
		if m.helpOpen {
			return m.updateHelp(msg)
		}
		switch msg.String() {
		case "f1":
			m.openHelp()
			return m, nil
		case "?":
			if !m.acceptsTextInput() {
				m.openHelp()
				return m, nil
			}
		case "q":
			if !m.acceptsTextInput() {
				return m, tea.Quit
			}
		case "f6":
			m.switchTab(otherTopLevelTab(m.screen))
			return m, nil
		}
	}

	switch m.screen {
	case screenTask:
		return m.updateTaskForm(msg)
	case screenField:
		return m.updateFieldForm(msg)
	case screenRun:
		return m.updateRunForm(msg)
	case screenJobs:
		return m.updateJobs(msg)
	case screenResult:
		return m.updateResult(msg)
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
		if m.screen == screenResult {
			return m.updateResult(msg)
		}
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
		case screenJobs:
			return m.updateJobs(key)
		default:
			return m.updateList(key)
		}
	}
	if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if screen, ok := m.topLevelTabAt(event.Y, event.X); ok {
		m.switchTab(screen)
		return m, nil
	}

	line := m.mouseLine(event.Y)
	if m.mouseLineIsLastMatch(event.Y, "keybinds") {
		switch optionAtX(line, event.X-3, m.footerControls()) {
		case 0:
			m.switchTab(otherTopLevelTab(m.screen))
			return m, nil
		case 1:
			return m, tea.Quit
		case 2:
			m.openHelp()
			return m, nil
		}
	}
	switch m.screen {
	case screenTask:
		return m.clickTaskForm(line, event.X)
	case screenField:
		return m.clickFieldForm(line, event.X)
	case screenRun:
		return m.clickRunForm(line, event.X)
	case screenJobs:
		return m.clickJobs(line, event.X)
	case screenResult:
		return m, nil
	default:
		return m.clickTaskList(line, event.X)
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
	line = strings.TrimPrefix(line, "╰─ ")
	if border := strings.LastIndex(line, " ─"); border >= 0 {
		line = line[:border]
	}
	return strings.TrimSpace(line)
}

func (m model) mouseLineIsLastMatch(y int, text string) bool {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], text) {
			return y == i
		}
	}
	return false
}

func (m model) clickTaskList(line string, x int) (tea.Model, tea.Cmd) {
	if strings.Contains(line, "enter run") {
		switch optionAtX(line, x-3, []string{"enter run", "n new", "e edit", "d delete", "q quit"}) {
		case 0:
			return m.updateList(tea.KeyMsg{Type: tea.KeyEnter})
		case 1:
			return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		case 2:
			return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
		case 3:
			return m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		case 4:
			return m, tea.Quit
		}
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

func (m model) clickJobs(line string, x int) (tea.Model, tea.Cmd) {
	if strings.Contains(line, "enter log") {
		switch optionAtX(line, x-3, []string{"enter log", "c cancel", "r rerun", "d delete job", "q quit"}) {
		case 0:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyEnter})
		case 1:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		case 2:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		case 3:
			return m.updateJobs(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		case 4:
			return m, tea.Quit
		}
	}
	for i := range m.jobs {
		job := m.jobs[len(m.jobs)-1-i]
		if !strings.Contains(line, job.ShortID()) {
			continue
		}
		if m.jobCursor == i {
			return m, m.openJob(job.ID)
		}
		m.jobCursor = i
		return m, nil
	}
	return m, nil
}

func (m model) topLevelTabAt(y, x int) (screen, bool) {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if y < 0 || y >= len(lines) {
		return 0, false
	}
	line := lines[y]
	for _, tab := range []struct {
		label  string
		screen screen
	}{
		{label: "Tasks", screen: screenList},
		{label: "Jobs", screen: screenJobs},
	} {
		start := strings.Index(line, tab.label)
		if start >= 0 && x >= start && x < start+len(tab.label) {
			return tab.screen, true
		}
	}
	return 0, false
}

func (m model) clickTaskForm(line string, x int) (tea.Model, tea.Cmd) {
	switch {
	case strings.HasPrefix(line, "Name") || containsNonEmpty(line, m.taskNameInput.Value()):
		m.taskFocus = 0
		m.taskNameInput.SetCursor(max(0, x-3))
		return m, m.focusTaskControl()
	case strings.HasPrefix(line, "Command") || containsValueLine(line, m.taskCommandInput.Value()):
		m.taskFocus = 1
		return m, m.focusTaskControl()
	case strings.HasPrefix(line, "Job policy"):
		m.taskFocus = 2
		m.moveJobPolicy(1)
		return m, m.focusTaskControl()
	case strings.Contains(line, "Sequential") || strings.Contains(line, "Parallel") || strings.Contains(line, "Cancel previous"):
		if selected := optionAtX(line, x-3, jobPolicyLabels); selected >= 0 {
			m.formJobPolicy = jobPolicies[selected]
			m.taskFocus = 2
			return m, m.focusTaskControl()
		}
	case strings.HasPrefix(line, "Fields") || strings.Contains(line, "No fields."):
		m.taskFocus = 3
		return m, m.focusTaskControl()
	}
	for i, field := range m.formFields {
		if strings.Contains(line, field.Label) {
			m.taskFocus = 3
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
			if strings.Contains(line, task.ResolveKnownValues(option, m.runValues)) {
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

func containsValueLine(text, value string) bool {
	for line := range strings.SplitSeq(value, "\n") {
		if containsNonEmpty(text, line) {
			return true
		}
	}
	return false
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
	case "tab", "shift+tab":
		m.switchTab(screenJobs)
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
			if m.standaloneForm {
				return m, tea.Quit
			}
			m.closeTaskForm()
			return m, nil
		case "ctrl+s", "f2":
			return m.saveTaskForm()
		case "tab":
			m.taskFocus = (m.taskFocus + 1) % 4
			return m, m.focusTaskControl()
		case "shift+tab":
			m.taskFocus = (m.taskFocus + 3) % 4
			return m, m.focusTaskControl()
		case "up":
			if m.taskFocus == 1 {
				break
			}
			if m.taskFocus == 3 && m.fieldCursor > 0 {
				m.fieldCursor--
				return m, nil
			}
			if m.taskFocus > 0 {
				m.taskFocus--
				return m, m.focusTaskControl()
			}
		case "down":
			if m.taskFocus == 1 {
				break
			}
			if m.taskFocus == 3 {
				if m.fieldCursor < len(m.formFields)-1 {
					m.fieldCursor++
				}
				return m, nil
			}
			m.taskFocus++
			return m, m.focusTaskControl()
		case "enter":
			if m.taskFocus == 1 {
				break
			}
			if m.taskFocus < 3 {
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
			case "left", "h":
				m.moveJobPolicy(-1)
			case "right", "l", " ":
				m.moveJobPolicy(1)
			}
			return m, nil
		}
		if m.taskFocus == 3 {
			switch key.String() {
			case "k":
				if m.fieldCursor > 0 {
					m.fieldCursor--
				} else {
					m.taskFocus = 2
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
			case "[":
				if m.fieldCursor > 0 {
					m.formFields[m.fieldCursor-1], m.formFields[m.fieldCursor] = m.formFields[m.fieldCursor], m.formFields[m.fieldCursor-1]
					m.fieldCursor--
				}
			case "]":
				if m.fieldCursor < len(m.formFields)-1 {
					m.formFields[m.fieldCursor], m.formFields[m.fieldCursor+1] = m.formFields[m.fieldCursor+1], m.formFields[m.fieldCursor]
					m.fieldCursor++
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

	if m.taskFocus >= 2 {
		return m, nil
	}

	var cmd tea.Cmd
	if m.taskFocus == 0 {
		m.taskNameInput, cmd = m.taskNameInput.Update(msg)
	} else {
		m.taskCommandInput, cmd = m.taskCommandInput.Update(msg)
		m.resizeTaskForm()
	}
	return m, cmd
}

func (m *model) moveJobPolicy(direction int) {
	index := jobPolicyIndex(m.formJobPolicy)
	index = (index + direction + len(jobPolicies)) % len(jobPolicies)
	m.formJobPolicy = jobPolicies[index]
}

func jobPolicyIndex(policy task.JobPolicy) int {
	for i, candidate := range jobPolicies {
		if candidate == policy {
			return i
		}
	}
	return 0
}

func (m *model) openTaskForm(item task.Task) {
	m.showScreen(screenTask)
	m.editingFile = item.File
	m.taskNameInput.SetValue(item.Name)
	m.taskCommandInput.SetValue(item.Command)
	m.resizeTaskForm()
	m.formFields = cloneFields(item.Fields)
	m.formJobPolicy = item.JobPolicy
	m.taskFocus = 0
	m.fieldCursor = 0
	m.focusTaskControl()
}

func (m *model) closeTaskForm() {
	m.showScreen(screenList)
	m.editingFile = ""
	m.formFields = nil
	m.formJobPolicy = task.JobSequential
	m.taskNameInput.Blur()
	m.taskNameInput.SetValue("")
	m.taskCommandInput.Blur()
	m.taskCommandInput.SetValue("")
}

func (m *model) focusTaskControl() tea.Cmd {
	m.taskNameInput.Blur()
	m.taskCommandInput.Blur()
	switch m.taskFocus {
	case 0:
		return m.taskNameInput.Focus()
	case 1:
		return m.taskCommandInput.Focus()
	default:
		return nil
	}
}

func (m model) saveTaskForm() (tea.Model, tea.Cmd) {
	item := task.Task{
		Name:      m.taskNameInput.Value(),
		Command:   m.taskCommandInput.Value(),
		Fields:    cloneFields(m.formFields),
		JobPolicy: m.formJobPolicy,
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
	if m.standaloneForm {
		return m, tea.Quit
	}
	m.closeTaskForm()
	m.setStatus(fmt.Sprintf("Saved %s", saved.Name))
	for i := range m.tasks {
		if m.tasks[i].File == saved.File {
			m.cursor = i
			break
		}
	}
	return m, nil
}

func (m model) updateFieldForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.showScreen(screenTask)
			m.taskFocus = 3
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
	m.showScreen(screenField)
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
	m.showScreen(screenTask)
	m.taskFocus = 3
	m.status = ""
	return m, m.focusTaskControl()
}

func (m model) startRun(item task.Task) (tea.Model, tea.Cmd) {
	m.runTask = item
	m.runIndex = 0
	m.runValues = make(map[string]string, len(item.Fields))
	m.runHistoryIndex = -1
	m.status = ""
	if len(item.Fields) == 0 {
		return m.executeRun()
	}
	m.showScreen(screenRun)
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
				value = task.ResolveKnownValues(field.Options[m.choiceCursor], m.runValues)
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
				m.showScreen(screenList)
				m.setStatus(fmt.Sprintf("Cancelled %s", m.runTask.Name))
				return m, nil
			}
			m.runValues[field.Key] = strconv.FormatBool(m.confirmationYes)
			return m.advanceRun()
		}
		return m, nil
	default:
		m.showScreen(screenList)
		m.setError(fmt.Errorf("unknown field type %q", field.Type))
		return m, nil
	}
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

func (m *model) showScreen(next screen) {
	if m.screen == next {
		return
	}
	m.screen = next
	m.status = ""
	m.statusError = false
}

func (m *model) applyDaemonPoll(msg daemonPollMsg) {
	if msg.err != nil {
		m.setError(fmt.Errorf("poll daemon: %w", msg.err))
		return
	}
	m.setJobs(msg.jobs)

	for _, job := range m.jobs {
		detail, ok := msg.details[job.ID]
		if !ok {
			continue
		}
		if m.result.ID == job.ID {
			m.updateOpenResult(detail)
		}
		if _, tracked := m.trackedJobs[job.ID]; tracked && job.Done() {
			delete(m.trackedJobs, job.ID)
			if m.screen == screenList {
				m.openResult(detail)
			}
		}
	}
}

func (m *model) setJobs(jobs []daemon.Job) {
	m.jobs = jobs
	clear(m.running)
	clear(m.queued)
	clear(m.latest)
	for _, job := range m.jobs {
		m.latest[job.TaskID] = job.Status
		switch job.Status {
		case daemon.StatusRunning:
			m.running[job.TaskID]++
		case daemon.StatusQueued:
			m.queued[job.TaskID]++
		}
	}
	if m.jobCursor >= len(m.jobs) && m.jobCursor > 0 {
		m.jobCursor = len(m.jobs) - 1
	}
}

func (m *model) updateOpenResult(job daemon.Job) {
	atBottom := m.resultViewport.AtBottom()
	if len(job.Output) == job.OutputSize {
		m.result.Output = job.Output
	} else {
		m.result.Output += job.Output
	}
	job.Output = m.result.Output
	m.result = job
	m.setResultContent()
	if atBottom {
		m.resultViewport.GotoBottom()
	}
}

func (m *model) openResult(job daemon.Job) {
	m.result = job
	m.setResultContent()
	m.resultViewport.GotoBottom()
	m.showScreen(screenResult)
}

func (m *model) setResultContent() {
	output := ansi.Strip(m.result.Output)
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	output = strings.TrimRight(output, "\n")
	if output == "" {
		output = "(no output)"
	}

	var content strings.Builder
	if m.result.Command != "" {
		script := ansi.Hardwrap("$ "+m.result.Command, m.resultViewport.Width, true)
		content.WriteString(mutedStyle.Render(script))
		content.WriteString("\n\n")
	}
	content.WriteString(output)
	m.resultViewport.SetContent(content.String())
	m.resizeResultViewport()
}

func (m model) openJob(id string) tea.Cmd {
	client := m.daemon
	return func() tea.Msg {
		job, err := client.Job(id, 0)
		if err != nil {
			err = fmt.Errorf("open job: %w", err)
		}
		return jobOpenedMsg{job: job, err: err}
	}
}

func (m model) jobAction(action, id string) tea.Cmd {
	client := m.daemon
	return func() tea.Msg {
		var (
			job daemon.Job
			err error
		)
		switch action {
		case "cancel":
			job, err = client.Cancel(id)
		case "rerun":
			job, err = client.Rerun(id)
		case "delete":
			job, err = client.Remove(id)
		}
		if err != nil {
			err = fmt.Errorf("%s job: %w", action, err)
		}
		return jobActionMsg{action: action, job: job, err: err}
	}
}

func (m *model) applyJobAction(msg jobActionMsg) {
	switch msg.action {
	case "rerun":
		m.jobs = append(m.jobs, msg.job)
		m.trackedJobs[msg.job.ID] = struct{}{}
		m.jobCursor = 0
		m.showScreen(screenJobs)
		m.setStatus(fmt.Sprintf("Started new job for %s", msg.job.Name))
	case "delete":
		for i := range m.jobs {
			if m.jobs[i].ID != msg.job.ID {
				continue
			}
			last := len(m.jobs) - 1
			copy(m.jobs[i:], m.jobs[i+1:])
			m.jobs[last] = daemon.Job{}
			m.setJobs(m.jobs[:last])
			break
		}
		delete(m.trackedJobs, msg.job.ID)
		if m.result.ID == msg.job.ID {
			m.result = daemon.Job{}
			m.showScreen(screenJobs)
			m.jobScreen = screenJobs
		}
		m.setStatus(fmt.Sprintf("Deleted job %s %s", msg.job.Name, msg.job.ShortID()))
	default:
		for i := range m.jobs {
			if m.jobs[i].ID == msg.job.ID {
				m.jobs[i] = msg.job
				break
			}
		}
		m.setStatus(fmt.Sprintf("Cancel requested for %s %s", msg.job.Name, msg.job.ShortID()))
	}
}

func (m model) updateJobs(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.switchTab(screenList)
	case "up", "k":
		if m.jobCursor > 0 {
			m.jobCursor--
		}
	case "down", "j":
		if m.jobCursor < len(m.jobs)-1 {
			m.jobCursor++
		}
	case "enter":
		job, ok := m.selectedJob()
		if ok {
			return m, m.openJob(job.ID)
		}
	case "c", "r", "d":
		job, ok := m.selectedJob()
		if !ok {
			break
		}
		action := "delete"
		if key.String() == "c" {
			action = "cancel"
		} else if key.String() == "r" {
			action = "rerun"
		}
		return m, m.jobAction(action, job.ID)
	}
	return m, nil
}

func (m model) selectedJob() (daemon.Job, bool) {
	if len(m.jobs) == 0 || m.jobCursor < 0 || m.jobCursor >= len(m.jobs) {
		return daemon.Job{}, false
	}
	return m.jobs[len(m.jobs)-1-m.jobCursor], true
}

func (m model) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.showScreen(screenJobs)
			return m, nil
		case "home", "g":
			m.resultViewport.GotoTop()
			return m, nil
		case "end", "G":
			m.resultViewport.GotoBottom()
			return m, nil
		case "c", "r", "d":
			if m.result.ID == "" {
				return m, nil
			}
			action := "delete"
			if key.String() == "c" {
				action = "cancel"
			} else if key.String() == "r" {
				action = "rerun"
			}
			return m, m.jobAction(action, m.result.ID)
		}
	}
	var cmd tea.Cmd
	m.resultViewport, cmd = m.resultViewport.Update(msg)
	return m, cmd
}

func (m model) View() string {
	base := m.baseView()
	if !m.helpOpen {
		return base
	}
	m.syncHelpViewport()
	help := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.helpViewport.View(),
		scrollBar(
			m.helpViewport.Height,
			!m.helpViewport.AtTop() || !m.helpViewport.AtBottom(),
			m.helpViewport.ScrollPercent(),
		),
	)
	if m.compactHelp() {
		panel := titledPanel(m.renderPanelWithFooter(help), m.helpTitle())
		return panel + "\n" + m.statusBar()
	}
	return overlay(base, titledPanel(helpModalStyle.Render(help), m.helpTitle()), m.width, m.height)
}

func titledPanel(panel, title string) string {
	lines := strings.Split(panel, "\n")
	if len(lines) == 0 {
		return panel
	}
	width := ansi.StringWidth(lines[0])
	titleWidth := max(0, width-5)
	title = ansi.Truncate(accentStyle.Render(title), titleWidth, "…")
	fillWidth := max(0, width-ansi.StringWidth(title)-5)
	lines[0] = borderStyle.Render("╭─ ") +
		title +
		borderStyle.Render(" "+strings.Repeat("─", fillWidth)+"╮")
	return strings.Join(lines, "\n")
}

func (m model) baseView() string {
	switch m.screen {
	case screenTask:
		return m.taskFormView()
	case screenField:
		return m.fieldFormView()
	case screenRun:
		return m.runFormView()
	case screenJobs:
		return m.jobsView()
	case screenResult:
		return m.resultView()
	default:
		return m.listView()
	}
}

func overlay(background, foreground string, width, height int) string {
	backgroundLines := strings.Split(background, "\n")
	foregroundLines := strings.Split(foreground, "\n")
	foregroundWidth := lipgloss.Width(foreground)
	x := max(0, (width-foregroundWidth)/2)
	y := max(0, (height-len(foregroundLines))/2)
	lines := make([]string, height)
	for i := range lines {
		if i < len(backgroundLines) {
			lines[i] = ansi.Truncate(backgroundLines[i], width, "")
		}
		lines[i] += strings.Repeat(" ", max(0, width-ansi.StringWidth(lines[i])))
	}
	for i, foregroundLine := range foregroundLines {
		row := y + i
		if row >= len(lines) {
			break
		}
		left := ansi.Cut(lines[row], 0, x)
		left += strings.Repeat(" ", max(0, x-ansi.StringWidth(left)))
		rightStart := min(width, x+foregroundWidth)
		right := ansi.Cut(lines[row], rightStart, width)
		line := left + foregroundLine + right
		lines[row] = ansi.Truncate(line, width, "")
		lines[row] += strings.Repeat(" ", max(0, width-ansi.StringWidth(lines[row])))
	}
	return strings.Join(lines, "\n")
}

func topLevelTab(current screen) screen {
	switch current {
	case screenJobs, screenResult:
		return screenJobs
	default:
		return screenList
	}
}

func otherTopLevelTab(current screen) screen {
	if topLevelTab(current) == screenList {
		return screenJobs
	}
	return screenList
}

func (m *model) switchTab(target screen) {
	currentTab := topLevelTab(m.screen)
	targetTab := topLevelTab(target)
	if currentTab == targetTab {
		return
	}

	if currentTab == screenList {
		m.taskScreen = m.screen
	} else {
		m.jobScreen = m.screen
	}
	if targetTab == screenList {
		m.showScreen(m.taskScreen)
	} else {
		m.showScreen(m.jobScreen)
	}
}

func (m model) workspaceHeader(active screen) string {
	tasksStyle := inactiveTabStyle
	jobsStyle := inactiveTabStyle
	if topLevelTab(active) == screenList {
		tasksStyle = activeTabStyle
	} else {
		jobsStyle = activeTabStyle
	}

	tabs := tasksStyle.Render("Tasks") + " " + jobsStyle.Render("Jobs")
	width := m.contentWidth() + panelStyle.GetHorizontalFrameSize()
	titleWidth := max(0, width-5)
	title := ansi.Truncate(tabs, titleWidth, "")
	if pathWidth := titleWidth - ansi.StringWidth(tabs) - 2; pathWidth > 0 {
		path := ansi.Truncate(m.store.Workspace(), pathWidth, "…")
		title += "  " + mutedStyle.Render(path)
	}
	fillWidth := max(0, width-ansi.StringWidth(title)-5)
	return borderStyle.Render("╭─ ") +
		title +
		borderStyle.Render(" "+strings.Repeat("─", fillWidth)+"╮")
}

func (m model) renderWorkspacePanel(content string, active screen) string {
	panel := m.renderPanelWithFooter(content)
	_, rest, ok := strings.Cut(panel, "\n")
	view := m.workspaceHeader(active)
	if ok {
		view += "\n" + rest
	}
	return view + "\n" + m.statusBar()
}

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

func jobPolicySummary(policy task.JobPolicy) string {
	switch policy {
	case task.JobParallel:
		return "parallel jobs"
	case task.JobCancelPrevious:
		return "cancel previous job"
	default:
		return "sequential jobs"
	}
}

func (m model) taskActivity(taskID string) string {
	running := m.running[taskID]
	queued := m.queued[taskID]
	switch {
	case running > 0 && queued > 0:
		return fmt.Sprintf("  [running %d, queued %d]", running, queued)
	case running > 0:
		return fmt.Sprintf("  [running %d]", running)
	case queued > 0:
		return fmt.Sprintf("  [queued %d]", queued)
	case m.latest[taskID] == daemon.StatusSucceeded:
		return "  [succeeded]"
	case m.latest[taskID] == daemon.StatusFailed:
		return "  [failed]"
	case m.latest[taskID] == daemon.StatusCanceled:
		return "  [canceled]"
	default:
		return ""
	}
}

func (m model) jobsView() string {
	var body strings.Builder
	if len(m.jobs) == 0 {
		body.WriteString(mutedStyle.Render("No jobs yet."))
	} else {
		start, end := visibleRange(len(m.jobs), m.jobCursor, max(1, m.contentHeight()-4))
		for i := start; i < end; i++ {
			job := m.jobs[len(m.jobs)-1-i]
			line := fmt.Sprintf("%-9s %s  %s", strings.ToUpper(string(job.Status)), job.Name, job.ShortID())
			if i == m.jobCursor {
				line = selectedStyle.Render("› " + line)
			} else {
				line = "  " + line
			}
			body.WriteString(line)
			body.WriteByte('\n')
			if i == m.jobCursor {
				body.WriteString("  ")
				body.WriteString(mutedStyle.Render(fmt.Sprintf(
					"%s • %d bytes output • %s",
					job.CreatedAt.Local().Format("2006-01-02 15:04:05"),
					job.OutputSize,
					job.Command,
				)))
				body.WriteByte('\n')
			}
		}
	}
	return m.renderWorkspacePanel(body.String(), screenJobs)
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
	line := fmt.Sprintf("%d. %s (%s, %s, %s → {{%s}})", index+1, field.Label, field.Type, requirement, mode, field.Key)
	if m.taskFocus == 3 && index == m.fieldCursor {
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
	title := "NEW TASK"
	if m.editingFile != "" {
		title = "EDIT TASK"
	}

	var body strings.Builder
	line := 0
	write := func(value string) {
		body.WriteString(value)
		line += strings.Count(value, "\n")
	}
	focusLine := 0

	write(accentStyle.Render(title))
	write(m.gap())

	if m.taskFocus == 0 {
		focusLine = line
	}
	write(taskStepTitle("Name", m.taskFocus == 0))
	write("\n")
	write(m.taskNameInput.View())
	write(m.gap())

	write(taskStepTitle("Command template", m.taskFocus == 1))
	write("\n")
	if m.taskFocus == 1 {
		focusLine = line + m.taskCommandCursorLine()
	}
	write(m.taskCommandView())
	write("\n")
	write(mutedStyle.Render("Use {{field_key}} where a runtime value belongs."))
	write(m.gap())

	if m.taskFocus == 2 {
		focusLine = line
	}
	write(taskStepTitle("Job policy", m.taskFocus == 2))
	write("\n")
	write(m.pickerRow(jobPolicyLabels, jobPolicyIndex(m.formJobPolicy)))
	write(m.gap())

	write(taskStepTitle("Fields", m.taskFocus == 3))
	write("\n")
	if len(m.formFields) == 0 {
		if m.taskFocus == 3 {
			focusLine = line
			write(selectedStyle.Render("› No fields. Press a or enter to add one."))
		} else {
			write(mutedStyle.Render("No fields. Press a or enter to add one."))
		}
		return body.String(), focusLine
	}
	for i := range m.formFields {
		if m.taskFocus == 3 && i == m.fieldCursor {
			focusLine = line
		}
		write(m.taskFieldLine(i))
		if i < len(m.formFields)-1 {
			write("\n")
		}
	}
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
	return m.renderWorkspacePanel(body.String(), screenField)
}

func (m model) runFormView() string {
	field := m.runTask.Fields[m.runIndex]
	var body strings.Builder
	body.WriteString(accentStyle.Render(m.runTask.Name))
	body.WriteString("\n")
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

func (m model) resultView() string {
	var body strings.Builder
	status := strings.ToUpper(string(m.result.Status))
	style := jobQueuedStyle
	switch m.result.Status {
	case daemon.StatusFailed:
		style = jobFailedStyle
	case daemon.StatusSucceeded:
		style = jobSucceededStyle
	case daemon.StatusCanceled:
		style = jobCanceledStyle
	}
	body.WriteString(style.Render(status))
	body.WriteString("  ")
	body.WriteString(m.result.Name)
	if m.result.Error != "" {
		body.WriteString(": ")
		body.WriteString(m.result.Error)
	}
	if m.result.StorageError != "" {
		body.WriteByte('\n')
		body.WriteString(errorStyle.Render("Log persistence: " + m.result.StorageError))
	}
	body.WriteString(m.gap())
	body.WriteString(lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.resultViewport.View(),
		scrollBar(
			m.resultViewport.Height,
			!m.resultViewport.AtTop() || !m.resultViewport.AtBottom(),
			m.resultViewport.ScrollPercent(),
		),
	))
	return m.renderWorkspacePanel(body.String(), screenResult)
}

func (m model) statusBar() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	message := ansi.Truncate(m.status, max(1, width-2), "…")
	line := " " + message
	line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
	line = ansi.Truncate(line, width, "")
	style := statusBarStyle
	if m.statusError {
		style = statusBarErrorStyle
	}
	return style.Render(line)
}

func (m model) navigationFooter() string {
	width := m.contentWidth() + panelStyle.GetHorizontalFrameSize()
	titleWidth := max(0, width-5)
	title := mutedStyle.Render(ansi.Truncate(m.topLevelFooter(), titleWidth, "…"))
	fillWidth := max(0, width-ansi.StringWidth(title)-5)
	return borderStyle.Render("╰─ ") +
		title +
		borderStyle.Render(" "+strings.Repeat("─", fillWidth)+"╯")
}

func (m model) renderPanelWithFooter(content string) string {
	lines := strings.Split(m.renderPanel(content), "\n")
	lines[len(lines)-1] = m.navigationFooter()
	return strings.Join(lines, "\n")
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
	return panelStyle.
		Width(width + panelStyle.GetHorizontalPadding()).
		Height(height + panelStyle.GetVerticalPadding()).
		Render(strings.Join(lines, "\n"))
}
