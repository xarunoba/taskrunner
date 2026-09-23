package tui

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

// Run opens the workspace task list.
func Run(store *task.Store, items []task.Task, themes *theme.Store, selected theme.Theme, stdin io.Reader, stdout io.Writer) error {
	return start(store, items, nil, themes, selected, stdin, stdout)
}

// Create opens a standalone task creation form.
func Create(store *task.Store, items []task.Task, themes *theme.Store, selected theme.Theme, stdin io.Reader, stdout io.Writer) error {
	return start(store, items, &task.Task{}, themes, selected, stdin, stdout)
}

// Edit opens a task in a standalone editing form.
func Edit(store *task.Store, items []task.Task, item task.Task, themes *theme.Store, selected theme.Theme, stdin io.Reader, stdout io.Writer) error {
	return start(store, items, &item, themes, selected, stdin, stdout)
}

func start(store *task.Store, items []task.Task, item *task.Task, themes *theme.Store, selected theme.Theme, stdin io.Reader, stdout io.Writer) error {
	initial := newModel(store, items, themes, selected)
	if item != nil {
		initial.standaloneForm = true
		initial.openTaskForm(*item)
	}
	program := tea.NewProgram(
		initial,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
	)
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}
	return nil
}
