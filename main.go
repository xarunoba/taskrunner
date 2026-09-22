package main

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 2 && args[0] == "__daemon" {
		return daemon.Serve(args[1], daemon.DefaultIdleTimeout)
	}

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

	switch options.mode {
	case modeJobs:
		return executeJobsCLI(workspace, options.all, stdout)
	case modeJobLogs, modeJobCancel, modeJobRerun, modeJobRemove:
		return executeJobCLI(workspace, options.mode, options.job, stdout)
	}
	store := task.NewStore(workspace)
	items, err := store.Load()
	if err != nil {
		return err
	}

	initial := newModel(store, items)
	switch options.mode {
	case modeCreate:
		initial.standaloneForm = true
		initial.openTaskForm(task.Task{})
	case modeEdit:
		item, err := findTask(items, options.task)
		if err != nil {
			return err
		}
		initial.standaloneForm = true
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
