package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

func TestFieldOverflowFollowsKeyboardAndMouse(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{24, 10}, {40, 10}, {140, 50}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
			m.resize(size[0], size[1])
			m.openFieldForm(-1)
			m.fieldInputs[0].SetValue("field_key")
			m.fieldInputs[1].SetValue("Field label")
			m.fieldInputs[3].SetValue("--prefix")
			m.fieldInputs[4].SetValue("--suffix")
			for _, want := range []string{"field_key", "Field label", "› text", "› Argument", "--prefix", "--suffix", "› Required"} {
				view := m.View()
				assertFillsTerminal(t, view, size[0], size[1])
				if !strings.Contains(ansi.Strip(view), want) {
					t.Fatalf("focused control %q hidden:\n%s", want, ansi.Strip(view))
				}
				if got := strings.Contains(ansi.Strip(view), "█"); got != (size[1] == 10) {
					t.Fatalf("overflow indicator present=%t at %dx%d", got, size[0], size[1])
				}
				if want != "› Required" {
					updated, _ := m.updateFieldForm(tea.KeyMsg{Type: tea.KeyTab})
					m = updated.(model)
				}
			}
			updated, _ := m.updateFieldForm(tea.KeyMsg{Type: tea.KeyRight})
			m = updated.(model)
			updated, _ = m.updateMouse(mouseClickOn(t, m.View(), "Required"))
			m = updated.(model)
			if m.fieldOptional || m.fieldFocus != 7 {
				t.Fatal("click on scrolled Requirement control did not select Required")
			}
			for range 6 {
				updated, _ = m.updateFieldForm(tea.KeyMsg{Type: tea.KeyShiftTab})
				m = updated.(model)
			}
			if view := ansi.Strip(m.View()); !strings.Contains(view, "field_key") || m.fieldInputs[4].Value() != "--suffix" {
				t.Fatalf("backward navigation hid Key or lost Suffix:\n%s", view)
			}
		})
	}
}

func TestWrappedPickerSelectionRemainsVisible(t *testing.T) {
	t.Parallel()
	m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(24, 10)
	m.formFields = []task.Field{
		{Key: "alpha", Label: "Alpha", Type: task.FieldText},
		{Key: "beta", Label: "Beta", Type: task.FieldText},
		{Key: "gamma", Label: "Gamma", Type: task.FieldText},
	}
	m.openFieldForm(-1)
	m.fieldTypeCursor = 4
	m.fieldFromCursor = 2
	m.fieldFocus = 3
	if view := ansi.Strip(m.View()); !strings.Contains(view, "› gamma") {
		t.Fatalf("wrapped reference selection hidden:\n%s", view)
	}
	m.openTaskForm(task.Task{})
	m.taskFocus = 3
	m.formJobPolicy = task.JobCancelPrevious
	if view := ansi.Strip(m.View()); !strings.Contains(view, "› Cancel") {
		t.Fatalf("wrapped job policy selection hidden:\n%s", view)
	}
}
