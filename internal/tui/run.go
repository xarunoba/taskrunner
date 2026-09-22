package tui

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarunoba/taskrunner/internal/task"
)

// Run opens the workspace task list.
func Run(store *task.Store, items []task.Task, stdin io.Reader, stdout io.Writer) error {
	return start(store, items, nil, stdin, stdout)
}

// Create opens a standalone task creation form.
func Create(store *task.Store, items []task.Task, stdin io.Reader, stdout io.Writer) error {
	item := task.Task{}
	return start(store, items, &item, stdin, stdout)
}

// Edit opens a task in a standalone editing form.
func Edit(store *task.Store, items []task.Task, item task.Task, stdin io.Reader, stdout io.Writer) error {
	return start(store, items, &item, stdin, stdout)
}

func start(store *task.Store, items []task.Task, item *task.Task, stdin io.Reader, stdout io.Writer) error {
	initial := newModel(store, items)
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
