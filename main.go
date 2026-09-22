package main

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarunoba/taskrunner/internal/task"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	options, err := parseCLI(args)
	if err != nil {
		return err
	}
	if options.mode == modeHelp {
		_, err := fmt.Fprint(stdout, helpText)
		return err
	}

	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	store := task.NewStore(workspace)
	items, err := store.Load()
	if err != nil {
		return err
	}

	initial := newModel(store, items)
	switch options.mode {
	case modeCreate:
		initial.openTaskForm(task.Task{})
	case modeEdit:
		item, err := findTask(items, options.task)
		if err != nil {
			return err
		}
		initial.openTaskForm(item)
	case modeRun:
		item, err := findTask(items, options.task)
		if err != nil {
			return err
		}
		values, err := prepareTaskValues(item, options.values)
		if err != nil {
			return err
		}
		return executeTaskCLI(workspace, item, values, stdin, stdout, stderr)
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
