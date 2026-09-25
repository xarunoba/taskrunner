package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

func TestOptionalFileSkipReachableByKeyboardAfterBrowsing(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Pick",
		Command: "printf {{file}}",
		Fields: []task.Field{
			{Key: "file", Label: "Choose file", Type: task.FieldFile, Optional: true},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item}, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(80, 24)
	updated, _ := m.startRun(item)
	m = updated.(model)
	if m.choiceCursor != -1 {
		t.Fatalf("initial choiceCursor = %d, want -1 on Skip", m.choiceCursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.choiceCursor != 0 {
		t.Fatalf("choiceCursor after down = %d, want 0 in the picker", m.choiceCursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)
	if m.choiceCursor != -1 {
		t.Fatalf("choiceCursor after up at the picker top = %d, want -1 back on Skip", m.choiceCursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.runValues["file"] != "" {
		t.Fatalf("file value after skip = %q, want empty", m.runValues["file"])
	}
	if m.screen != screenList {
		t.Fatalf("screen after skipping the only field = %d, want list", m.screen)
	}
}

func TestOptionalFileKeysStayInertWhileSkipSelected(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Pick",
		Command: "printf {{file}}",
		Fields: []task.Field{
			{Key: "file", Label: "Choose file", Type: task.FieldFile, Optional: true},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item}, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(80, 24)
	updated, _ := m.startRun(item)
	m = updated.(model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = updated.(model)
	if m.choiceCursor != -1 {
		t.Fatalf("choiceCursor after g on Skip = %d, want -1", m.choiceCursor)
	}
}

func TestTaskFormFieldClickBeatsNameMatch(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(80, 24)
	m.openTaskForm(task.Task{
		Name:    "Build",
		Command: "true",
		Fields: []task.Field{
			{Key: "target", Label: "Build target", Type: task.FieldText},
		},
	})

	view := strings.Split(ansi.Strip(m.View()), "\n")
	row := -1
	for y, line := range view {
		if strings.Contains(line, "Build target") {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("field row not visible:\n%s", m.View())
	}

	updated, _ := m.updateMouse(mouseClickAt(5, row))
	m = updated.(model)
	if m.taskFocus != 1 || m.fieldCursor != 0 {
		t.Fatalf("field row click produced focus=%d cursor=%d, want fields focus and field 0", m.taskFocus, m.fieldCursor)
	}
}

func TestLateJobOpenDoesNotHijackTaskDraft(t *testing.T) {
	t.Parallel()

	m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(80, 24)
	m.openTaskForm(task.Task{Name: "Draft", Command: "true"})
	m.taskNameInput.SetValue("Renamed draft")

	updated, _ := m.Update(jobOpenedMsg{job: daemon.Job{ID: "late-1", Name: "Other", Status: daemon.StatusSucceeded}})
	m = updated.(model)
	if m.screen != screenTask {
		t.Fatalf("late job open switched screen to %d, want task form", m.screen)
	}
	if got := m.taskNameInput.Value(); got != "Renamed draft" {
		t.Fatalf("draft name after late job open = %q, want %q", got, "Renamed draft")
	}
}

func TestResultCommandRewrapsAfterResize(t *testing.T) {
	t.Parallel()

	command := "printf " + strings.Repeat("a", 100)
	m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(120, 30)
	updated, _ := m.Update(jobOpenedMsg{job: daemon.Job{
		ID:      "run-12345678",
		Name:    "Long",
		Command: command,
		Status:  daemon.StatusSucceeded,
		Output:  "done",
	}})
	m = updated.(model)
	if m.screen != screenResult {
		t.Fatalf("screen = %d, want result", m.screen)
	}
	if wide := ansi.Strip(m.View()); !strings.Contains(wide, command) {
		t.Fatalf("wide result view lost the command:\n%s", wide)
	}

	m.resize(40, 30)
	narrow := ansi.Strip(m.View())
	if strings.Contains(narrow, command) {
		t.Fatalf("narrow result view kept the wide-wrapped command on one line:\n%s", narrow)
	}
	if !strings.Contains(narrow, strings.Repeat("a", 20)) {
		t.Fatalf("narrow result view lost the wrapped command entirely:\n%s", narrow)
	}
}

func TestOptionalFileSkipVisibleOnShortTerminal(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Pick",
		Command: "printf {{file}}",
		Fields: []task.Field{
			{Key: "file", Label: "Choose file", Type: task.FieldFile, Optional: true},
		},
	}
	m := newModel(task.NewStore(t.TempDir()), []task.Task{item}, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(24, 10)
	updated, _ := m.startRun(item)
	m = updated.(model)

	header := m.runFormHeader(item.Fields[0])
	headerRows := strings.Count(header, "\n")
	if headerRows > m.contentHeight()-1 {
		t.Fatalf("run header takes %d of %d content rows, leaving no list row", headerRows, m.contentHeight())
	}
	skipRow := m.styles.panel.GetBorderTopSize() + m.styles.panel.GetPaddingTop() + headerRows - 1
	if skipRow >= m.height {
		t.Fatalf("Skip row %d rendered past the %d-row terminal", skipRow, m.height)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Skip") {
		t.Fatalf("Skip row clipped on a 24x10 terminal:\n%s", m.View())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.runValues["file"] != "" || m.screen != screenList {
		t.Fatalf("enter on Skip at 24x10 gave file=%q screen=%d, want empty skip to list", m.runValues["file"], m.screen)
	}
}
