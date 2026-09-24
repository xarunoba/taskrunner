package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

func TestRunChoiceOverflowAndMouse(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{40, 10}, {80, 24}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
			m.resize(size[0], size[1])
			options := make([]string, 40)
			for i := range options {
				options[i] = fmt.Sprintf("Option %02d", i)
			}
			started, _ := m.startRun(task.Task{Name: "Choose", Command: "true", Fields: []task.Field{{Key: "value", Label: "Value", Type: task.FieldChoice, Options: options, Optional: true, Raw: true}}})
			m = started.(model)
			for range 40 {
				updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyDown})
				m = updated.(model)
			}
			view := m.View()
			assertFillsTerminal(t, view, size[0], size[1])
			if !strings.Contains(ansi.Strip(view), "› Option 39") || !strings.Contains(ansi.Strip(view), "█") {
				t.Fatalf("last choice or overflow indicator hidden by raw/optional header:\n%s", ansi.Strip(view))
			}
			updated, _ := m.updateMouse(mouseClickOn(t, view, "Skip"))
			m = updated.(model)
			if m.choiceCursor != -1 || !strings.Contains(ansi.Strip(m.View()), "› Skip") {
				t.Fatal("Skip click failed after scrolling")
			}
			updated, _ = m.updateRunForm(tea.KeyMsg{Type: tea.KeyDown})
			m = updated.(model)
			if m.choiceCursor != 0 || !strings.Contains(ansi.Strip(m.View()), "› Option 00") {
				t.Fatal("first option hidden after moving down from Skip")
			}
			m.runTask.Fields[0].Options = options[:2]
			m.resize(80, 24)
			if strings.Contains(ansi.Strip(m.View()), "█") {
				t.Fatal("choice indicator remains when options fit")
			}
		})
	}
}

func TestFilePickerOverflowPositionResizeAndMouse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := range 20 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "f00.txt"), filepath.Join(dir, "link → file")); err != nil {
		t.Fatal(err)
	}
	m := newModel(task.NewStore(dir), nil, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(40, 10)
	started, cmd := m.startRun(task.Task{Name: "File", Command: "true", Fields: []task.Field{{Key: "file", Label: "File", Type: task.FieldFile}}})
	m = started.(model)
	for _, load := range cmd().(tea.BatchMsg) {
		updated, _ := m.Update(load())
		m = updated.(model)
	}
	for range m.filePicker.Height - 1 {
		updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(model)
	}
	if got := m.runFilePickerPercent(m.filePicker.View()); got != 0 {
		t.Fatalf("thumb moved within the first visible window: %v", got)
	}
	for range m.fileCount {
		updated, _ := m.updateRunForm(tea.KeyMsg{Type: tea.KeyPgDown})
		m = updated.(model)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "link → file") {
		t.Fatalf("Page Down hid the selected final entry:\n%s", view)
	}
	if got := m.runFilePickerPercent(m.filePicker.View()); got != 1 {
		t.Fatalf("bottom window progress = %v, want 1", got)
	}
	for _, size := range [][2]int{{140, 50}, {40, 10}} {
		m.resize(size[0], size[1])
		view := m.View()
		assertFillsTerminal(t, view, size[0], size[1])
		if !strings.Contains(ansi.Strip(view), "link → file") {
			t.Fatalf("selected symlink hidden after resize to %dx%d:\n%s", size[0], size[1], ansi.Strip(view))
		}
		if got := strings.Contains(ansi.Strip(view), "█"); got != (size[1] == 10) {
			t.Fatalf("resize indicator present=%t at %dx%d", got, size[0], size[1])
		}
	}
	updated, _ := m.updateMouse(mouseClickOn(t, m.View(), "f19.txt"))
	m = updated.(model)
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	m.filePicker, _ = m.filePicker.Update(enter)
	selected, path := m.filePicker.DidSelectFile(enter)
	if !selected || path != filepath.Join(dir, "f19.txt") {
		t.Fatalf("mouse selected %q, want f19.txt", path)
	}
}
