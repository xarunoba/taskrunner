package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

func TestViewsFitTerminal(t *testing.T) {
	t.Parallel()

	tasks := make([]task.Task, 20)
	for i := range tasks {
		tasks[i] = task.Task{
			Name:    fmt.Sprintf("Task %02d with a long descriptive name", i),
			Command: "printf '%s' a-command-that-is-longer-than-a-narrow-terminal",
		}
	}

	sizes := []struct {
		width  int
		height int
	}{
		{width: 24, height: 10},
		{width: 32, height: 14},
		{width: 80, height: 24},
		{width: 140, height: 50},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			m := newModel(task.NewStore(t.TempDir()), tasks)
			m.cursor = 10
			m.resize(size.width, size.height)
			assertFillsTerminal(t, m.View(), size.width, size.height)

			m.openTaskForm(task.Task{})
			m.taskFocus = 1
			assertFillsTerminal(t, m.View(), size.width, size.height)
			if view := ansi.Strip(m.View()); !strings.Contains(view, "› No fields") {
				t.Fatalf("task form viewport hides focused fields:\n%s", view)
			}

			m.formFields = []task.Field{
				{Key: "value", Label: "A long field label", Type: task.FieldChoice, Options: []string{"one", "two"}, Raw: true},
			}
			m.openFieldForm(0)
			m.fieldFocus = 4
			assertFillsTerminal(t, m.View(), size.width, size.height)
		})
	}
}

func TestTaskFormViewportFollowsFocusedControls(t *testing.T) {
	t.Parallel()

	command := "one\ntwo\nthree\nfour"
	fields := []task.Field{
		{Key: "one", Label: "One", Type: task.FieldText},
		{Key: "two", Label: "Two", Type: task.FieldText},
		{Key: "three", Label: "Three", Type: task.FieldText},
		{Key: "four", Label: "Four", Type: task.FieldText},
	}
	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(24, 10)
	m.openTaskForm(task.Task{Name: "Compact", Command: command, Fields: fields})

	view := ansi.Strip(m.View())
	viewLines := strings.Split(view, "\n")
	nameLine, inputLine := -1, -1
	for i, line := range viewLines {
		if strings.Contains(line, "Name") {
			nameLine = i
		}
		if strings.Contains(line, "Compact") {
			inputLine = i
		}
	}
	if nameLine < 0 || inputLine != nameLine+1 {
		t.Fatalf("task name label is not directly above its input:\n%s", view)
	}
	if !strings.Contains(view, "█") {
		t.Fatalf("scrollable task form has no position bar:\n%s", view)
	}

	m.taskFocus = 2
	m.focusTaskControl()
	updated, _ := m.updateTaskForm(tea.KeyMsg{Type: tea.KeyCtrlEnd})
	m = updated.(model)
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "four") {
		t.Fatalf("task form viewport does not follow the command cursor:\n%s", view)
	}
	if got := m.taskCommandInput.Value(); got != command {
		t.Fatalf("task form viewport changed the command:\n%q", got)
	}

	m.taskFocus = 1
	m.fieldCursor = len(fields) - 1
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "4. Four") {
		t.Fatalf("task form viewport does not follow the selected field:\n%s", view)
	}
}

func TestScrollBarShowsViewportPosition(t *testing.T) {
	t.Parallel()

	if got, want := ansi.Strip(scrollBar(3, true, 0)), "█\n│\n│"; got != want {
		t.Fatalf("top scroll bar = %q, want %q", got, want)
	}
	if got, want := ansi.Strip(scrollBar(3, true, 1)), "│\n│\n█"; got != want {
		t.Fatalf("bottom scroll bar = %q, want %q", got, want)
	}
}

func TestTaskLogOpensAtBottomAndOutputClickDoesNotClose(t *testing.T) {
	t.Parallel()

	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %02d", i)
	}
	lines[len(lines)-1] = "esc back"
	job := daemon.Job{
		ID:         "run-12345678",
		Name:       "Capture",
		Command:    "printf output",
		Status:     daemon.StatusSucceeded,
		Output:     strings.Join(lines, "\n"),
		OutputSize: len(strings.Join(lines, "\n")),
	}

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(40, 10)
	updated, _ := m.Update(jobOpenedMsg{job: job})
	m = updated.(model)
	if m.screen != screenResult {
		t.Fatalf("result state screen=%d, want result", m.screen)
	}
	if !m.resultViewport.AtBottom() {
		t.Fatal("result viewport did not open at the bottom")
	}
	if got, want := m.resultViewport.Height, m.contentHeight()-1; got != want {
		t.Fatalf("compact log viewport height = %d, want maximum %d", got, want)
	}
	assertFillsTerminal(t, m.View(), 40, 10)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "esc back") {
		t.Fatalf("result view does not show final output line:\n%s", view)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "█") {
		t.Fatalf("scrollable job log has no position bar:\n%s", view)
	}

	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "esc back"))
	m = updated.(model)
	if m.screen != screenResult {
		t.Fatalf("screen after clicking log output = %d, want result", m.screen)
	}
	updated, _ = m.updateResult(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)
	if m.resultViewport.AtBottom() {
		t.Fatal("result viewport did not scroll up")
	}
	updated, _ = m.updateResult(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.screen != screenJobs {
		t.Fatalf("screen after leaving log = %d, want runs", m.screen)
	}
}

func TestFailedTaskLogShowsFailureSummary(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	updated, _ := m.Update(jobOpenedMsg{job: daemon.Job{
		ID:      "run-failed",
		Name:    "Failing task",
		Command: "exit 7",
		Status:  daemon.StatusFailed,
		Output:  "failure details\n",
		Error:   "exit status 7",
	}})
	m = updated.(model)
	styledView := m.View()
	view := ansi.Strip(styledView)
	if !strings.Contains(view, "FAILED") || !strings.Contains(view, "Failing task: exit status 7") {
		t.Fatalf("result view does not contain failure summary:\n%s", view)
	}
	if !strings.Contains(styledView, jobFailedStyle.Render("FAILED")) {
		t.Fatalf("failed status does not use its background badge:\n%s", styledView)
	}
	if strings.Contains(view, "JOB LOG") {
		t.Fatalf("result view still contains the redundant JOB LOG title:\n%s", view)
	}
	if !strings.Contains(view, "failure details") {
		t.Fatalf("result view does not contain command output:\n%s", view)
	}
}

func TestTaskLogRightAlignsElapsedTime(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)
	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(60, 16)
	m.openResult(daemon.Job{
		ID:        "run-timed",
		Name:      "Timed task",
		Status:    daemon.StatusSucceeded,
		StartedAt: started,
		EndedAt:   started.Add(1500 * time.Millisecond),
	})

	var header string
	for line := range strings.SplitSeq(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "Timed task") {
			header = line
			break
		}
	}
	if header == "" {
		t.Fatalf("result view does not contain task title:\n%s", ansi.Strip(m.View()))
	}
	if !strings.Contains(header, "Timed task") || !strings.Contains(header, "1.5s") {
		t.Fatalf("result header does not contain title and elapsed time: %q", header)
	}
	if !strings.HasSuffix(header, "1.5s  │") {
		t.Fatalf("elapsed time is not aligned to the content's right edge: %q", header)
	}
}

func TestTaskLogWrapsTheCompleteCommand(t *testing.T) {
	t.Parallel()

	command := "printf '%s' alpha-bravo-charlie-delta-echo\nprintf finished"
	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(32, 16)
	m.openResult(daemon.Job{
		ID:      "run-wrapped",
		Name:    "Wrapped command",
		Command: command,
		Status:  daemon.StatusSucceeded,
	})

	log := ansi.Strip(m.resultViewport.View())
	compact := strings.NewReplacer(" ", "", "\n", "", "\r", "").Replace
	if !strings.Contains(compact(log), compact("$ "+command)) {
		t.Fatalf("wrapped log does not contain the complete command:\n%s", log)
	}
	if strings.Contains(log, "alpha-bravo-charlie-delta-echo") {
		t.Fatalf("long command line did not wrap inside the log:\n%s", log)
	}

	longName := "task-name-that-is-too-long-for-the-list-panel"
	list := newModel(task.NewStore(t.TempDir()), []task.Task{{Name: longName, Command: command}})
	list.resize(32, 16)
	listView := ansi.Strip(list.View())
	if strings.Contains(listView, longName) || !strings.Contains(listView, "…") {
		t.Fatalf("non-log list content did not truncate:\n%s", listView)
	}
}

func TestWorkspaceHeaderUsesTopBorder(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	m := newModel(task.NewStore(workspace), nil)
	m.resize(80, 24)

	m.setStatus("Saved")
	styledView := m.View()
	styledHeader, _, _ := strings.Cut(styledView, "\n")
	if !strings.Contains(styledHeader, activeTabStyle.Render("Tasks")) ||
		!strings.Contains(styledHeader, inactiveTabStyle.Render("Jobs")) {
		t.Fatalf("top border does not render active and inactive tab backgrounds:\n%s", styledHeader)
	}

	view := ansi.Strip(styledView)
	lines := strings.Split(view, "\n")
	header := lines[0]
	tasks := strings.Index(header, "Tasks")
	jobs := strings.Index(header, "Jobs")
	path := strings.Index(header, workspace)
	if tasks < 0 || jobs <= tasks || path <= jobs {
		t.Fatalf("top border does not contain ordered tabs and workspace:\n%s", header)
	}
	footer := lines[len(lines)-1]
	if !strings.HasPrefix(footer, "╰") || !strings.HasSuffix(footer, "╯") {
		t.Fatalf("footer does not show the bottom border corners:\n%s", footer)
	}
	status := footer
	if !strings.Contains(status, "Saved") || !strings.Contains(status, "? keybinds") {
		t.Fatalf("footer does not contain the status message and keybinds chip:\n%s", view)
	}
	styledFooter := strings.Split(styledView, "\n")[len(lines)-1]
	if !strings.Contains(styledFooter, statusBarStyle.Render(" Saved")) {
		t.Fatalf("status message does not use the status bar background:\n%s", styledFooter)
	}
	if !strings.Contains(styledFooter, helpChipStyle.Render("? keybinds")) {
		t.Fatalf("keybinds chip does not use its own background:\n%s", styledFooter)
	}
	if strings.Contains(view, "tab switch") || strings.Contains(view, "q quit") {
		t.Fatalf("footer still contains navigation controls:\n%s", view)
	}
	if strings.Contains(view, "TASKRUNNER") {
		t.Fatalf("view still contains the taskrunner title:\n%s", view)
	}
}
func TestFooterShowsOnlyTopLevelControls(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	footer := ansi.Strip(strings.Split(m.View(), "\n")[m.height-1])
	if !strings.Contains(footer, "? keybinds") {
		t.Fatalf("Tasks footer does not contain %q:\n%s", "? keybinds", footer)
	}
	for _, text := range []string{"enter run", "n new", "e edit", "d delete", "tab switch", "q quit"} {
		if strings.Contains(footer, text) {
			t.Fatalf("Tasks footer still contains control %q:\n%s", text, footer)
		}
	}

	m.openTaskForm(task.Task{})
	footer = ansi.Strip(strings.Split(m.View(), "\n")[m.height-1])
	if !strings.Contains(footer, "? keybinds") {
		t.Fatalf("text-input footer does not contain %q:\n%s", "? keybinds", footer)
	}
}

func TestFooterControlsSupportMouseInput(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)

	updated, _ := m.updateMouse(mouseClickOn(t, m.View(), "? keybinds"))
	m = updated.(model)
	if !m.helpOpen {
		t.Fatal("clicking keybind footer control did not open help")
	}
	m.helpOpen = false

	updated, _ = m.updateMouse(mouseClickAt(m.width-1, m.height-1))
	m = updated.(model)
	if m.helpOpen {
		t.Fatal("clicking the bottom border corner opened help")
	}
}

func TestAdaptiveKeybindHelpPreservesContextAndCloses(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), []task.Task{{Name: "Visible task", Command: "true"}})
	m.resize(80, 24)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(model)
	if !m.helpOpen {
		t.Fatal("? did not open keybind help")
	}
	view := ansi.Strip(m.View())
	assertFillsTerminal(t, m.View(), 80, 24)
	for _, text := range []string{"╭─ Keybinds - Tasks ", "Visible task", "enter", "run selected task", "General"} {
		if !strings.Contains(view, text) {
			t.Fatalf("overlay help does not contain %q:\n%s", text, view)
		}
	}
	groups := m.helpGroups()
	if len(groups) != 2 || groups[0].title != "" || groups[1].title != "General" {
		t.Fatalf("help section titles = %#v, want only General", groups)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.helpOpen || m.screen != screenList {
		t.Fatalf("closing help produced open=%t screen=%d", m.helpOpen, m.screen)
	}

	m.resize(40, 10)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(model)
	assertFillsTerminal(t, m.View(), 40, 10)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "╭─ Keybinds - Tasks ") {
		t.Fatalf("compact help fallback is missing its border title:\n%s", view)
	}
}

func TestKeybindHelpReopensAtTop(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(40, 10)
	m.openTaskForm(task.Task{})
	m.openHelp()
	m.helpViewport.GotoBottom()
	if m.helpViewport.AtTop() {
		t.Fatal("test setup did not produce scrollable help")
	}

	m.helpOpen = false
	m.openHelp()
	if !m.helpViewport.AtTop() || m.helpViewport.YOffset != 0 {
		t.Fatalf("reopened help offset = %d, want top", m.helpViewport.YOffset)
	}
}

func TestQuestionMarkRemainsEditableAndF1OpensHelp(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{})
	m.taskFocus = 2
	m.focusTaskControl()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(model)
	if m.helpOpen || m.taskCommandInput.Value() != "?" {
		t.Fatalf("question mark in command produced help=%t value=%q", m.helpOpen, m.taskCommandInput.Value())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = updated.(model)
	if !m.helpOpen {
		t.Fatal("F1 did not open help while editing command text")
	}
}

func TestTabsRestoreScreenAndValues(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{})
	m.taskNameInput.SetValue("draft task")
	m.setStatus("editing status")

	updated, _ := m.updateMouse(mouseClickOn(t, m.View(), "Jobs"))
	m = updated.(model)
	if m.screen != screenJobs {
		t.Fatalf("screen after switching to Jobs = %d, want jobs", m.screen)
	}
	if m.status != "" {
		t.Fatalf("status after switching screens = %q, want empty", m.status)
	}

	job := daemon.Job{ID: "job-preserved", Name: "Preserved", Status: daemon.StatusRunning, Output: "output"}
	m.openResult(job)
	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Tasks"))
	m = updated.(model)
	if m.screen != screenTask || m.taskNameInput.Value() != "draft task" {
		t.Fatalf("restored task state = screen %d, value %q", m.screen, m.taskNameInput.Value())
	}

	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Jobs"))
	m = updated.(model)
	if m.screen != screenResult || m.result.ID != job.ID {
		t.Fatalf("restored job state = screen %d, job %q", m.screen, m.result.ID)
	}
}

func TestJobsTabListsEveryJobNewestFirst(t *testing.T) {
	t.Parallel()

	jobs := []daemon.Job{
		{ID: "job-older-12345678", TaskID: "build.json", Name: "Build", Command: "first", Status: daemon.StatusSucceeded},
		{ID: "job-newer-87654321", TaskID: "build.json", Name: "Build", Command: "second", Status: daemon.StatusRunning},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{{
		Name: "Build", Command: "true", File: "build.json",
	}})
	m.resize(80, 24)
	m.applyDaemonPoll(daemonPollMsg{jobs: jobs, details: make(map[string]daemon.Job)})

	updated, _ := m.updateList(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	view := ansi.Strip(m.View())
	for _, id := range []string{"12345678", "87654321"} {
		if !strings.Contains(view, id) {
			t.Fatalf("jobs tab does not contain job %q:\n%s", id, view)
		}
	}
	selected, ok := m.selectedJob()
	if !ok || selected.ID != jobs[1].ID {
		t.Fatalf("selected job = %#v, want newest job %q", selected, jobs[1].ID)
	}
	if list := ansi.Strip(m.listView()); !strings.Contains(list, "[running 1]") {
		t.Fatalf("task list does not show running job:\n%s", list)
	}

	updated, _ = m.updateJobs(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.screen != screenList {
		t.Fatalf("screen after tab from jobs = %d, want tasks", m.screen)
	}
	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Jobs"))
	m = updated.(model)
	if m.screen != screenJobs {
		t.Fatalf("screen after clicking Jobs tab = %d, want jobs", m.screen)
	}
}

func TestDeleteJobActionRemovesJobAndClosesOpenLog(t *testing.T) {
	t.Parallel()

	remaining := daemon.Job{
		ID: "job-older-12345678", TaskID: "build.json", Name: "Build", Status: daemon.StatusSucceeded,
	}
	deleted := daemon.Job{
		ID: "job-newer-87654321", TaskID: "build.json", Name: "Build", Status: daemon.StatusFailed,
	}
	m := newModel(task.NewStore(t.TempDir()), nil)
	m.applyDaemonPoll(daemonPollMsg{
		jobs:    []daemon.Job{remaining, deleted},
		details: make(map[string]daemon.Job),
	})
	m.openResult(deleted)
	m.jobScreen = screenResult
	m.trackedJobs[deleted.ID] = struct{}{}

	m.applyJobAction(jobActionMsg{action: "delete", job: deleted})

	if len(m.jobs) != 1 || m.jobs[0].ID != remaining.ID {
		t.Fatalf("jobs after deletion = %#v, want only %q", m.jobs, remaining.ID)
	}
	if m.screen != screenJobs || m.jobScreen != screenJobs || m.result.ID != "" {
		t.Fatalf("open log after deletion = screen %d, saved screen %d, result %q", m.screen, m.jobScreen, m.result.ID)
	}
	if _, ok := m.trackedJobs[deleted.ID]; ok {
		t.Fatalf("deleted job %q remains tracked", deleted.ID)
	}
	if m.latest["build.json"] != daemon.StatusSucceeded {
		t.Fatalf("latest task status = %q, want succeeded", m.latest["build.json"])
	}
	if got, want := m.status, "Deleted job Build 87654321"; got != want {
		t.Fatalf("deletion status = %q, want %q", got, want)
	}
}

func TestTaskListShowsPerTaskStatusWithoutTransientRunMessage(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), []task.Task{
		{Name: "Build", Command: "true", File: "build.json"},
		{Name: "Deploy", Command: "false", File: "deploy.json"},
		{Name: "Cleanup", Command: "false", File: "cleanup.json"},
	})
	m.resize(80, 24)
	updated, _ := m.Update(taskStartedMsg{job: daemon.Job{
		ID: "build-running", TaskID: "build.json", Name: "Build", Status: daemon.StatusRunning,
	}})
	m = updated.(model)
	m.applyDaemonPoll(daemonPollMsg{
		jobs: []daemon.Job{
			{ID: "build-done", TaskID: "build.json", Name: "Build", Status: daemon.StatusSucceeded},
			{ID: "deploy-done", TaskID: "deploy.json", Name: "Deploy", Status: daemon.StatusFailed},
			{ID: "cleanup-done", TaskID: "cleanup.json", Name: "Cleanup", Status: daemon.StatusCanceled},
		},
		details: make(map[string]daemon.Job),
	})

	view := ansi.Strip(m.listView())
	for _, status := range []string{"Build  [succeeded]", "Deploy  [failed]", "Cleanup  [canceled]"} {
		if !strings.Contains(view, status) {
			t.Fatalf("task list does not contain %q:\n%s", status, view)
		}
	}
	if strings.Contains(view, "Build running") {
		t.Fatalf("task list contains redundant transient run message:\n%s", view)
	}
}

func TestOpenTaskLogAppendsLiveOutput(t *testing.T) {
	t.Parallel()

	job := daemon.Job{
		ID: "run-live", TaskID: "build.json", Name: "Build",
		Status: daemon.StatusRunning, Output: "first\n", OutputSize: 6,
	}
	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.openResult(job)
	m.applyDaemonPoll(daemonPollMsg{
		jobs: []daemon.Job{{
			ID: "run-live", TaskID: "build.json", Name: "Build",
			Status: daemon.StatusSucceeded, OutputSize: 13,
		}},
		details: map[string]daemon.Job{
			"run-live": {
				ID: "run-live", TaskID: "build.json", Name: "Build",
				Status: daemon.StatusSucceeded, Output: "second\n", OutputSize: 13,
			},
		},
	})
	if got, want := m.result.Output, "first\nsecond\n"; got != want {
		t.Fatalf("live log output = %q, want %q", got, want)
	}
	if m.result.Status != daemon.StatusSucceeded {
		t.Fatalf("live log status = %q, want succeeded", m.result.Status)
	}
}

func TestTaskFormHighlightsSelectedField(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{
		Name: "Demo",
		Fields: []task.Field{
			{Key: "one", Label: "One", Type: task.FieldText},
			{Key: "two", Label: "Two", Type: task.FieldText},
		},
	})

	m.taskFocus = 1
	m.fieldCursor = 0
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "› 1. One") || strings.Contains(view, "› 2. Two") {
		t.Fatalf("fields focused, cursor 0, wrong selection marker:\n%s", view)
	}

	m.fieldCursor = 1
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "› 2. Two") || strings.Contains(view, "› 1. One") {
		t.Fatalf("fields focused, cursor 1, wrong selection marker:\n%s", view)
	}

	m.taskFocus = 3
	view = ansi.Strip(m.View())
	if strings.Contains(view, "› 1. One") || strings.Contains(view, "› 2. Two") {
		t.Fatalf("field lines highlighted while job policy focused:\n%s", view)
	}
	if !strings.Contains(view, "› Sequential") {
		t.Fatalf("job policy row lost its selection marker:\n%s", view)
	}
}

func TestTaskFormArrowAndMouseNavigation(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{})

	updated, _ := m.updateTaskForm(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.taskFocus != 1 {
		t.Fatalf("down from name focus = %d, want 1", m.taskFocus)
	}

	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.taskFocus != 2 {
		t.Fatalf("tab from fields focus = %d, want 2", m.taskFocus)
	}

	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.taskFocus != 3 {
		t.Fatalf("tab from command focus = %d, want 3", m.taskFocus)
	}

	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)
	if m.taskFocus != 2 {
		t.Fatalf("up from job policy focus = %d, want 2", m.taskFocus)
	}

	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Cancel previous"))
	m = updated.(model)
	if m.formJobPolicy != task.JobCancelPrevious || m.taskFocus != 3 {
		t.Fatalf("job policy click produced policy=%q focus=%d", m.formJobPolicy, m.taskFocus)
	}

	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Fields"))
	m = updated.(model)
	if m.taskFocus != 1 {
		t.Fatalf("click fields focus = %d, want 1", m.taskFocus)
	}

	updated, _ = m.updateMouse(tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonWheelUp}))
	m = updated.(model)
	if m.taskFocus != 0 {
		t.Fatalf("wheel up from fields focus = %d, want 0", m.taskFocus)
	}
}

func TestTaskCommandEditorStartsMultiline(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{})

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	labelLine, hintLine := -1, -1
	for i, line := range lines {
		switch {
		case strings.Contains(line, "Command template"):
			labelLine = i
		case strings.Contains(line, "Use {{field_key}}"):
			hintLine = i
		}
	}
	if labelLine < 0 || hintLine != labelLine+4 {
		t.Fatalf("empty command editor does not reserve three rows:\n%s", ansi.Strip(m.View()))
	}
}

func TestTaskCommandEditorAcceptsAndSavesMultipleLines(t *testing.T) {
	t.Parallel()

	store := task.NewStore(t.TempDir())
	m := newModel(store, nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{})
	m.taskNameInput.SetValue("Multiline")

	updated, _ := m.updateTaskForm(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("printf first")},
		{Type: tea.KeyEnter},
		{Type: tea.KeyRunes, Runes: []rune("printf second")},
	} {
		updated, _ = m.updateTaskForm(msg)
		m = updated.(model)
	}
	if got, want := m.taskCommandInput.Value(), "printf first\nprintf second"; got != want {
		t.Fatalf("multiline command = %q, want %q", got, want)
	}
	if m.taskFocus != 2 {
		t.Fatalf("focus after command newline = %d, want command editor", m.taskFocus)
	}

	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	items, err := store.Load()
	if err != nil {
		t.Fatalf("load saved task: %v", err)
	}
	if len(items) != 1 || items[0].Command != "printf first\nprintf second" {
		t.Fatalf("saved tasks = %#v, want multiline command", items)
	}
}

func TestTaskFormF2SavesAndReordersFields(t *testing.T) {
	t.Parallel()

	store := task.NewStore(t.TempDir())
	m := newModel(store, nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{})
	m.taskNameInput.SetValue("Ordered task")
	m.taskCommandInput.SetValue("printf '%s %s' {{second}} {{first}}")
	m.formFields = []task.Field{
		{Key: "first", Label: "First", Type: task.FieldText},
		{Key: "second", Label: "Second", Type: task.FieldText},
	}
	m.formJobPolicy = task.JobParallel
	m.taskFocus = 1
	m.fieldCursor = 1

	updated, _ := m.updateTaskForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m = updated.(model)
	if m.fieldCursor != 0 || m.formFields[0].Key != "second" {
		t.Fatalf("move earlier produced cursor=%d fields=%q,%q", m.fieldCursor, m.formFields[0].Key, m.formFields[1].Key)
	}

	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyF2})
	m = updated.(model)
	if m.screen != screenList {
		t.Fatalf("screen after F2 = %d, want list", m.screen)
	}
	items, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load() error = %v", err)
	}
	if len(items) != 1 || items[0].Fields[0].Key != "second" || items[0].JobPolicy != task.JobParallel {
		t.Fatalf("saved task = %#v, want second field first and parallel jobs", items)
	}
}

func TestRuntimeResolvesEarlierValuesInConfirmationLabel(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Deploy",
		Command: "deploy {{version}}",
		Fields: []task.Field{
			{Key: "version", Label: "Version", Type: task.FieldText},
			{Key: "confirmed", Label: "Deploy {{version}} to {{environment}}?", Type: task.FieldConfirm},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item})
	m.resize(80, 24)
	started, _ := m.startRun(item)
	m = started.(model)
	m.runInput.SetValue("1.4.0")

	updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Deploy 1.4.0 to {{environment}}?") {
		t.Fatalf("confirmation view does not contain resolved label:\n%s", view)
	}
}

func TestRuntimeCanReturnToEarlierInput(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "two inputs",
		Command: "printf '%s %s' {{first}} {{second}}",
		Fields: []task.Field{
			{Key: "first", Label: "First", Type: task.FieldText},
			{Key: "second", Label: "Second", Type: task.FieldText},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item})
	m.resize(80, 24)
	started, _ := m.startRun(item)
	m = started.(model)
	m.runInput.SetValue("first value")

	updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.runIndex != 1 {
		t.Fatalf("run index after enter = %d, want 1", m.runIndex)
	}
	m.runInput.SetValue("second value")

	updated, _ = m.updateRunForm(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(model)
	if m.runIndex != 0 {
		t.Fatalf("run index after back = %d, want 0", m.runIndex)
	}
	if got := m.runInput.Value(); got != "first value" {
		t.Fatalf("restored first input = %q, want %q", got, "first value")
	}
	if got := m.runValues["second"]; got != "second value" {
		t.Fatalf("preserved second input = %q, want %q", got, "second value")
	}
}

func TestFieldFormSavesReferField(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.formFields = []task.Field{
		{Key: "db", Label: "Database", Type: task.FieldText, Optional: true},
	}
	m.openFieldForm(-1)
	m.fieldInputs[0].SetValue("r")
	m.fieldInputs[1].SetValue("Confirm DB")

	for range 2 { // focus: Key -> Label -> Type
		updated, _ := m.updateFieldForm(tea.KeyMsg{Type: tea.KeyTab})
		m = updated.(model)
	}
	for range 4 { // select refer in the type picker
		updated, _ := m.updateFieldForm(tea.KeyMsg{Type: tea.KeyRight})
		m = updated.(model)
	}
	if got := fieldTypes[m.fieldTypeCursor]; got != task.FieldRefer {
		t.Fatalf("type = %q, want refer", got)
	}

	updated, _ := m.updateFieldForm(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(model)
	if m.fieldFocus != 3 {
		t.Fatalf("focus = %d, want 3 for the From picker", m.fieldFocus)
	}
	updated, _ = m.updateFieldForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(model)
	if got := m.fieldInputs[2].Value(); got != "" {
		t.Fatalf("options input received %q while From picker focused, want no input", got)
	}

	updated, _ = m.updateFieldForm(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	if len(m.formFields) != 2 {
		t.Fatalf("field count = %d, want 2", len(m.formFields))
	}
	saved := m.formFields[1]
	if saved.Type != task.FieldRefer || saved.From != "db" || saved.Optional {
		t.Fatalf("saved field = %+v, want refer from db without optional", saved)
	}
}

func TestRunSkipsReferFields(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "refer run",
		Command: "true {{db}} {{r}}",
		Fields: []task.Field{
			{Key: "db", Label: "Database", Type: task.FieldText, Optional: true, Prefix: "--db "},
			{Key: "r", Label: "Confirm DB", Type: task.FieldRefer, From: "db", Prefix: "--confirm "},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item})
	m.resize(80, 24)
	started, _ := m.startRun(item)
	m = started.(model)
	if key := m.runTask.Fields[m.runIndex].Key; key != "db" {
		t.Fatalf("first prompted field = %q, want db", key)
	}

	m.runInput.SetValue("appdb")
	updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if got := m.runValues["r"]; got != "appdb" {
		t.Fatalf("derived refer value = %q, want appdb", got)
	}
	if m.screen != screenList {
		t.Fatalf("screen = %d after final field, want list (refer field must not prompt)", m.screen)
	}
}

func TestFieldFormRoundTripsPrefixAndSuffix(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.formFields = []task.Field{
		{Key: "tag", Label: "Tag", Type: task.FieldText, Prefix: "--tag ", Suffix: "!"},
	}

	m.openFieldForm(0)
	if got := m.fieldInputs[3].Value(); got != "--tag " {
		t.Fatalf("prefix input = %q, want %q", got, "--tag ")
	}
	if got := m.fieldInputs[4].Value(); got != "!" {
		t.Fatalf("suffix input = %q, want %q", got, "!")
	}

	m.fieldInputs[4].SetValue("")
	updated, _ := m.saveFieldForm()
	m = updated.(model)
	if len(m.formFields) != 1 {
		t.Fatalf("field count = %d, want 1", len(m.formFields))
	}
	saved := m.formFields[0]
	if saved.Prefix != "--tag " || saved.Suffix != "" {
		t.Fatalf("saved prefix = %q, suffix = %q, want %q and empty", saved.Prefix, saved.Suffix, "--tag ")
	}

	m.openFieldForm(-1)
	for i := range m.fieldInputs {
		if got := m.fieldInputs[i].Value(); got != "" {
			t.Fatalf("input %d = %q on new field form, want empty", i, got)
		}
	}
}

func TestRuntimeTextHistoryUsesArrowKeys(t *testing.T) {
	t.Parallel()

	store := task.NewStore(t.TempDir())
	item := task.Task{
		Name:    "Release",
		Command: "true",
		File:    "release.json",
		Fields: []task.Field{
			{Key: "version", Label: "Version", Type: task.FieldText},
		},
	}
	for _, version := range []string{"1.0.0", "1.1.0"} {
		if err := store.RecordValueHistory(item, map[string]string{"version": version}); err != nil {
			t.Fatalf("RecordValueHistory() error = %v", err)
		}
	}

	m := newModel(store, []task.Task{item})
	m.resize(80, 24)
	started, _ := m.startRun(item)
	m = started.(model)
	m.runInput.SetValue("draft")

	for _, want := range []string{"1.1.0", "1.0.0", "1.0.0"} {
		updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyUp})
		m = updated.(model)
		if got := m.runInput.Value(); got != want {
			t.Fatalf("up history value = %q, want %q", got, want)
		}
	}
	for _, want := range []string{"1.1.0", "draft", "draft"} {
		updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
		if got := m.runInput.Value(); got != want {
			t.Fatalf("down history value = %q, want %q", got, want)
		}
	}

	updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	history, err := store.ValueHistory(item.File, "version")
	if err != nil {
		t.Fatalf("ValueHistory() error = %v", err)
	}
	if len(history) == 0 || history[0] != "draft" {
		t.Fatalf("recorded history = %#v, want draft first", history)
	}
}

func TestMouseSelectsFieldControlsAndRuntimeChoices(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil)
	m.resize(80, 24)
	m.formFields = []task.Field{
		{Key: "value", Label: "Value", Type: task.FieldText},
	}
	m.openFieldForm(0)

	updated, _ := m.updateMouse(mouseClickOn(t, m.View(), "Raw"))
	m = updated.(model)
	if !m.fieldRaw || m.fieldFocus != 4 {
		t.Fatalf("raw click produced raw=%t focus=%d, want true and 4", m.fieldRaw, m.fieldFocus)
	}

	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Optional"))
	m = updated.(model)
	if !m.fieldOptional || m.fieldFocus != 7 {
		t.Fatalf("optional click produced optional=%t focus=%d, want true and 7", m.fieldOptional, m.fieldFocus)
	}
	updated, _ = m.saveFieldForm()
	m = updated.(model)
	if !m.formFields[0].Optional {
		t.Fatal("saved field is not optional")
	}

	item := task.Task{
		Name:    "choice",
		Command: "printf '%s' {{choice}}",
		Fields: []task.Field{
			{Key: "choice", Label: "Choice", Type: task.FieldChoice, Options: []string{"one", "two"}},
		},
	}
	started, _ := m.startRun(item)
	m = started.(model)
	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "two"))
	m = updated.(model)
	if m.choiceCursor != 1 {
		t.Fatalf("choice click cursor = %d, want 1", m.choiceCursor)
	}
}

func TestOptionalRuntimeFieldsCanBeSkipped(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "optional fields",
		Command: "printf '%s|%s|%s|%s' {{text}} {{choice}} {{file}} {{confirm}}",
		Fields: []task.Field{
			{Key: "text", Label: "Text", Type: task.FieldText, Optional: true},
			{Key: "choice", Label: "Choice", Type: task.FieldChoice, Options: []string{"one", "two"}, Optional: true},
			{Key: "file", Label: "File", Type: task.FieldFile, Optional: true},
			{Key: "confirm", Label: "Confirm", Type: task.FieldConfirm, Optional: true},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item})
	m.resize(80, 24)
	started, _ := m.startRun(item)
	m = started.(model)

	updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.runIndex != 1 || m.runValues["text"] != "" {
		t.Fatalf("optional text skip produced index=%d value=%q", m.runIndex, m.runValues["text"])
	}
	if m.choiceCursor != -1 {
		t.Fatalf("optional choice cursor = %d, want -1", m.choiceCursor)
	}

	updated, _ = m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.runIndex != 2 || m.runValues["choice"] != "" {
		t.Fatalf("optional choice skip produced index=%d value=%q", m.runIndex, m.runValues["choice"])
	}

	updated, _ = m.updateRunForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(model)
	if m.runIndex != 3 || m.runValues["file"] != "" {
		t.Fatalf("optional file skip produced index=%d value=%q", m.runIndex, m.runValues["file"])
	}

	updated, _ = m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.screen != screenList || m.runValues["confirm"] != "false" {
		t.Fatalf("optional confirmation skip produced screen=%d value=%q", m.screen, m.runValues["confirm"])
	}
}

func TestRequiredTextFieldStillRejectsEmptyValue(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "required field",
		Command: "printf '%s' {{value}}",
		Fields: []task.Field{
			{Key: "value", Label: "Value", Type: task.FieldText},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item})
	started, _ := m.startRun(item)
	m = started.(model)

	updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.runIndex != 0 || !m.statusError {
		t.Fatalf("required empty input produced index=%d error=%t", m.runIndex, m.statusError)
	}
}

func TestStandaloneTaskFormQuitsAfterCancelAndSave(t *testing.T) {
	t.Parallel()

	store := task.NewStore(t.TempDir())
	m := newModel(store, nil)
	m.standaloneForm = true
	m.openTaskForm(task.Task{})

	updated, cmd := m.updateTaskForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if cmd == nil {
		t.Fatal("standalone cancel returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("standalone cancel did not quit")
	}
	if m.screen != screenTask {
		t.Fatalf("standalone cancel screen = %d, want task form", m.screen)
	}

	m.taskNameInput.SetValue("Saved task")
	m.taskCommandInput.SetValue("printf saved")
	m, cmd = m.saveTaskForm()
	if cmd == nil {
		t.Fatal("standalone save returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("standalone save did not quit")
	}
	if m.screen != screenTask {
		t.Fatalf("standalone save screen = %d, want task form", m.screen)
	}
	tasks, err := store.Load()
	if err != nil {
		t.Fatalf("load saved task: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "Saved task" {
		t.Fatalf("saved tasks = %#v, want Saved task", tasks)
	}
}

func mouseClickOn(t *testing.T, view, text string) tea.MouseMsg {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(view), "\n") {
		if before, _, ok := strings.Cut(line, text); ok {
			// Mouse X positions use terminal columns, not rune or byte offsets.
			return mouseClickAt(ansi.StringWidth(before), y)
		}
	}
	t.Fatalf("view does not contain clickable text %q", text)
	return tea.MouseMsg{}
}

func mouseClickAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg(tea.MouseEvent{
		X:      x,
		Y:      y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
}

func assertFillsTerminal(t *testing.T, view string, width, height int) {
	t.Helper()
	if got := lipgloss.Width(view); got != width {
		t.Fatalf("view width = %d, want terminal width %d", got, width)
	}
	if got := lipgloss.Height(view); got != height {
		t.Fatalf("view height = %d, want terminal height %d", got, height)
	}
}
