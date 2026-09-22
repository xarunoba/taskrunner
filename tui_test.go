package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
			assertFitsTerminal(t, m.View(), size.width, size.height)

			m.openTaskForm(task.Task{})
			m.taskFocus = 2
			assertFitsTerminal(t, m.View(), size.width, size.height)
			if !strings.Contains(m.View(), "› No fields") {
				t.Fatal("empty focused fields step has no caret")
			}

			m.formFields = []task.Field{
				{Key: "value", Label: "A long field label", Type: task.FieldChoice, Options: []string{"one", "two"}, Raw: true},
			}
			m.openFieldForm(0)
			m.fieldFocus = 4
			assertFitsTerminal(t, m.View(), size.width, size.height)
		})
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

	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(model)
	if m.taskFocus != 2 {
		t.Fatalf("down from command focus = %d, want 2", m.taskFocus)
	}

	updated, _ = m.updateTaskForm(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(model)
	if m.taskFocus != 1 {
		t.Fatalf("up from fields focus = %d, want 1", m.taskFocus)
	}

	updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Fields"))
	m = updated.(model)
	if m.taskFocus != 2 {
		t.Fatalf("click fields focus = %d, want 2", m.taskFocus)
	}

	updated, _ = m.updateMouse(tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonWheelUp}))
	m = updated.(model)
	if m.taskFocus != 1 {
		t.Fatalf("wheel up from fields focus = %d, want 1", m.taskFocus)
	}
}

func TestTaskFormF2SavesAndReordersFields(t *testing.T) {
	t.Parallel()

	store := task.NewStore(t.TempDir())
	m := newModel(store, nil)
	m.resize(80, 24)
	m.openTaskForm(task.Task{})
	m.taskInputs[0].SetValue("Ordered task")
	m.taskInputs[1].SetValue("printf '%s %s' {{second}} {{first}}")
	m.formFields = []task.Field{
		{Key: "first", Label: "First", Type: task.FieldText},
		{Key: "second", Label: "Second", Type: task.FieldText},
	}
	m.taskFocus = 2
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
	if len(items) != 1 || items[0].Fields[0].Key != "second" {
		t.Fatalf("saved fields = %#v, want second field first", items)
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
	if !m.fieldOptional || m.fieldFocus != 5 {
		t.Fatalf("optional click produced optional=%t focus=%d, want true and 5", m.fieldOptional, m.fieldFocus)
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

func mouseClickOn(t *testing.T, view, text string) tea.MouseMsg {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(view), "\n") {
		if x := strings.Index(line, text); x >= 0 {
			return tea.MouseMsg(tea.MouseEvent{
				X:      x,
				Y:      y,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonLeft,
			})
		}
	}
	t.Fatalf("view does not contain clickable text %q", text)
	return tea.MouseMsg{}
}

func assertFitsTerminal(t *testing.T, view string, width, height int) {
	t.Helper()
	if got := lipgloss.Width(view); got > width {
		t.Fatalf("view width = %d, terminal width = %d", got, width)
	}
	if got := lipgloss.Height(view); got > height {
		t.Fatalf("view height = %d, terminal height = %d", got, height)
	}
}
