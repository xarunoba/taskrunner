package tui

import (
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
	"github.com/xarunoba/taskrunner/internal/theme"
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

// styles owns every Lip Gloss style the TUI renders, derived from the
// selected theme palette. Styles live on the model so a theme switch
// rebuilds them without touching package state.
type styles struct {
	accent         lipgloss.Style
	border         lipgloss.Style
	muted          lipgloss.Style
	cursor         lipgloss.Style
	selected       lipgloss.Style
	activeTab      lipgloss.Style
	inactiveTab    lipgloss.Style
	step           lipgloss.Style
	error          lipgloss.Style
	panel          lipgloss.Style
	statusBar      lipgloss.Style
	statusBarError lipgloss.Style
	helpChip       lipgloss.Style
	jobQueued      lipgloss.Style
	jobSucceeded   lipgloss.Style
	jobFailed      lipgloss.Style
	jobCanceled    lipgloss.Style
	helpModal      lipgloss.Style
}

func newStyles(p theme.Palette) styles {
	selection := lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.Selection.Foreground)).
		Background(lipgloss.Color(p.Selection.Background)).
		Bold(true).
		Padding(0, 1)
	pair := func(pair theme.ColorPair) (lipgloss.Color, lipgloss.Color) {
		return lipgloss.Color(pair.Foreground), lipgloss.Color(pair.Background)
	}
	inactiveForeground, inactiveBackground := pair(p.InactiveTab)
	statusForeground, statusBackground := pair(p.Status)
	badgeForeground, badgeBackground := pair(p.ErrorBadge)
	queuedForeground, queuedBackground := pair(p.Queued)
	succeededForeground, succeededBackground := pair(p.Succeeded)
	canceledForeground, canceledBackground := pair(p.Canceled)

	return styles{
		accent:         lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)).Bold(true),
		border:         lipgloss.NewStyle().Foreground(lipgloss.Color(p.Accent)),
		muted:          lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)),
		cursor:         lipgloss.NewStyle().Foreground(lipgloss.Color(p.Cursor)).Bold(true),
		selected:       selection,
		activeTab:      selection,
		inactiveTab:    lipgloss.NewStyle().Foreground(inactiveForeground).Background(inactiveBackground).Padding(0, 1),
		step:           selection,
		error:          lipgloss.NewStyle().Foreground(lipgloss.Color(p.Error)),
		panel:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.Accent)).Padding(1, 2),
		statusBar:      lipgloss.NewStyle().Foreground(statusForeground).Background(statusBackground),
		statusBarError: lipgloss.NewStyle().Foreground(badgeForeground).Background(badgeBackground).Bold(true),
		helpChip:       selection,
		jobQueued:      lipgloss.NewStyle().Foreground(queuedForeground).Background(queuedBackground).Bold(true).Padding(0, 1),
		jobSucceeded:   lipgloss.NewStyle().Foreground(succeededForeground).Background(succeededBackground).Bold(true).Padding(0, 1),
		jobFailed:      lipgloss.NewStyle().Foreground(badgeForeground).Background(badgeBackground).Bold(true).Padding(0, 1),
		jobCanceled:    lipgloss.NewStyle().Foreground(canceledForeground).Background(canceledBackground).Bold(true).Padding(0, 1),
		helpModal:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.Accent)).Padding(1, 2),
	}
}

var fieldTypes = []task.FieldType{
	task.FieldText,
	task.FieldChoice,
	task.FieldFile,
	task.FieldConfirm,
	task.FieldRefer,
}

var fieldTypeLabels = []string{
	"text",
	"choice",
	"file",
	"confirm",
	"refer",
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

type jobAction string

const (
	jobActionCancel jobAction = "cancel"
	jobActionRerun  jobAction = "rerun"
	jobActionDelete jobAction = "delete"
)

type jobActionMsg struct {
	action jobAction
	job    daemon.Job
	err    error
}

type model struct {
	store        *task.Store
	daemon       *daemon.Client
	themes       *theme.Store
	theme        theme.Theme
	styles       styles
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

	settingsOpen    bool
	settingsLoading bool
	settingsSaving  bool
	settingsCursor  int
	settings        []settingChoice

	taskNameInput    textinput.Model
	taskCommandInput textarea.Model
	taskViewport     viewport.Model
	taskFocus        int
	formFields       []task.Field
	formJobPolicy    task.JobPolicy
	fieldCursor      int
	editingFile      string
	standaloneForm   bool

	fieldInputs     [5]textinput.Model
	fieldFocus      int
	fieldTypeCursor int
	fieldFromCursor int
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
	fileCount       int
	fileNames       []string

	result         daemon.Job
	resultViewport viewport.Model
}

func newModel(store *task.Store, tasks []task.Task, themes *theme.Store, selected theme.Theme) model {
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
	prefix := newInput("--tag ", 200)
	suffix := newInput(" 2>/dev/null", 200)
	resultViewport := viewport.New(1, 1)
	taskViewport := viewport.New(1, 1)
	helpViewport := viewport.New(1, 1)

	return model{
		store:            store,
		daemon:           daemon.NewClient(store.Workspace()),
		themes:           themes,
		theme:            selected,
		styles:           newStyles(selected.Palette),
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
		fieldInputs:      [5]textinput.Model{key, label, options, prefix, suffix},
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
	m.taskNameInput.Width = max(1, inputWidth-2)
	m.resizeTaskForm()
	for i := range m.fieldInputs {
		m.fieldInputs[i].Width = max(1, inputWidth-2)
	}
	m.runInput.Width = inputWidth
	m.resizeFilePicker()
	m.resizeResultViewport()
}

func (m model) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	return max(1, width-m.styles.panel.GetHorizontalFrameSize())
}

func (m model) contentHeight() int {
	height := m.height
	if height <= 0 {
		height = 40
	}
	return max(1, height-m.styles.panel.GetVerticalFrameSize()-m.footerLayout().rows+1)
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
		return m.taskFocus == 0 || m.taskFocus == 2
	case screenField:
		_, input := m.fieldInputForFocus(m.fieldFocus)
		return input
	case screenRun:
		return len(m.runTask.Fields) > 0 && m.runTask.Fields[m.runIndex].Type == task.FieldText
	default:
		return false
	}
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
