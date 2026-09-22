package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xarunoba/taskrunner/internal/task"
)

type cliMode uint8

const (
	modeTUI cliMode = iota
	modeCreate
	modeEdit
	modeRun
	modeHelp
)

const helpText = `Usage:
  taskrunner
  taskrunner create
  taskrunner edit <task>
  taskrunner run <task> [--set <key>=<value>]...

Commands:
  create        Open the task creation form directly.
  edit          Open an existing task in the editor.
  run           Run a task without opening the TUI.

Run options:
  --set k=v     Pre-seed a task field. Repeat for multiple fields.

Task names are matched case-insensitively by display name or JSON filename.
Required fields must be supplied to "run". Omitted optional fields use empty values.
`

type cliOptions struct {
	mode   cliMode
	task   string
	values map[string]string
}

func parseCLI(args []string) (cliOptions, error) {
	if len(args) == 0 {
		return cliOptions{mode: modeTUI}, nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return cliOptions{}, errors.New("help does not accept arguments")
		}
		return cliOptions{mode: modeHelp}, nil
	case "create":
		if len(args) != 1 {
			return cliOptions{}, errors.New("usage: taskrunner create")
		}
		return cliOptions{mode: modeCreate}, nil
	case "edit":
		if len(args) != 2 {
			return cliOptions{}, errors.New("usage: taskrunner edit <task>")
		}
		return cliOptions{mode: modeEdit, task: args[1]}, nil
	case "run":
		return parseRunCLI(args[1:])
	default:
		return cliOptions{}, fmt.Errorf("unknown command %q\n\n%s", args[0], helpText)
	}
}

func parseRunCLI(args []string) (cliOptions, error) {
	if len(args) == 0 {
		return cliOptions{}, errors.New("usage: taskrunner run <task> [--set <key>=<value>]...")
	}

	options := cliOptions{
		mode:   modeRun,
		task:   args[0],
		values: make(map[string]string),
	}
	for i := 1; i < len(args); i++ {
		argument := args[i]
		var assignment string
		switch {
		case argument == "--set":
			i++
			if i == len(args) {
				return cliOptions{}, errors.New("--set requires <key>=<value>")
			}
			assignment = args[i]
		case strings.HasPrefix(argument, "--set="):
			assignment = strings.TrimPrefix(argument, "--set=")
		default:
			return cliOptions{}, fmt.Errorf("unknown run option %q", argument)
		}

		key, value, ok := strings.Cut(assignment, "=")
		if !ok || key == "" {
			return cliOptions{}, fmt.Errorf("invalid field assignment %q; expected <key>=<value>", assignment)
		}
		options.values[key] = value
	}
	return options, nil
}

func findTask(items []task.Task, name string) (task.Task, error) {
	for _, item := range items {
		fileName := strings.TrimSuffix(item.File, filepath.Ext(item.File))
		if strings.EqualFold(item.Name, name) || strings.EqualFold(fileName, name) || strings.EqualFold(item.File, name) {
			return item, nil
		}
	}
	return task.Task{}, fmt.Errorf("task %q not found", name)
}

func prepareTaskValues(item task.Task, supplied map[string]string) (map[string]string, error) {
	fields := make(map[string]task.Field, len(item.Fields))
	for _, field := range item.Fields {
		fields[field.Key] = field
	}
	for key := range supplied {
		if _, ok := fields[key]; !ok {
			return nil, fmt.Errorf("task %q has no field %q", item.Name, key)
		}
	}

	values := make(map[string]string, len(item.Fields))
	for _, field := range item.Fields {
		value, supplied := supplied[field.Key]
		if !supplied {
			if !field.Optional {
				return nil, fmt.Errorf("missing required field %q; use --set %s=<value>", field.Key, field.Key)
			}
			value := ""
			if field.Type == task.FieldConfirm {
				value = strconv.FormatBool(false)
			}
			values[field.Key] = value
			continue
		}
		if value == "" && !field.Optional {
			return nil, fmt.Errorf("field %q is required", field.Key)
		}

		switch field.Type {
		case task.FieldChoice:
			if value != "" && !contains(field.Options, value) {
				return nil, fmt.Errorf("field %q must be one of: %s", field.Key, strings.Join(field.Options, ", "))
			}
		case task.FieldConfirm:
			confirmed, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("field %q must be true or false", field.Key)
			}
			if !field.Optional && !confirmed {
				return nil, fmt.Errorf("field %q must be confirmed", field.Key)
			}
			value = strconv.FormatBool(confirmed)
		}
		values[field.Key] = value
	}
	return values, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func executeTaskCLI(workspace string, item task.Task, values map[string]string, stdin io.Reader, stdout, stderr io.Writer) error {
	command, err := item.Render(values)
	if err != nil {
		return err
	}
	shell := shellPath()
	cmd := exec.Command(shell, "-c", command)
	cmd.Dir = workspace
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run task %q: %w", item.Name, err)
	}
	return nil
}

func shellPath() string {
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	return "/bin/sh"
}
