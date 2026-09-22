package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

type cliMode uint8

const (
	modeTUI cliMode = iota
	modeCreate
	modeEdit
	modeRun
	modeTasks
	modeTaskShow
	modeTaskValidate
	modeTaskRemove
	modeJobs
	modeJobLogs
	modeJobWait
	modeJobInspect
	modeJobCancel
	modeJobRerun
	modeJobRemove
	modeJobPrune
	modeCompletion
	modeHelp
)

const helpText = `Usage:
  taskrunner
  taskrunner create
  taskrunner edit <task>
  taskrunner run <task> [--set <key>=<value>]... [--detach|--dry-run]
  taskrunner tasks [--json]
  taskrunner task <show|validate|rm> [options]
  taskrunner jobs [options]
  taskrunner job <logs|wait|inspect|cancel|rerun|rm|prune> [options]
  taskrunner completion <bash|zsh|fish>

Commands:
  create          Open the task creation form directly.
  edit            Open an existing task in the editor.
  run             Run a task without opening the TUI.
  tasks           List task definitions.
  task show       Print a task definition.
  task validate   Validate one task or every task.
  task rm         Remove a task definition.
  jobs            List active jobs.
  job logs        Print or follow job output.
  job wait        Wait for a job to finish.
  job inspect     Print job details.
  job cancel      Cancel a queued or running job.
  job rerun       Start a new job with the same command and policy.
  job rm          Remove a completed job and its output.
  job prune       Remove matching completed jobs.
  completion      Generate shell completion source.

Run options:
  --set k=v       Set a task field. Repeat for multiple fields.
  --detach        Print the job ID and return without waiting.
  --dry-run       Print the safely rendered command without creating a job.
  --quiet         With --dry-run, print only the rendered command.

Jobs options:
  --all, -a       Include completed jobs.
  --status value  Filter by job status.
  --task value    Filter by task name or filename.
  --limit count   Limit matching jobs.
  --json          Emit JSON.
  --quiet         Print full job IDs only.

Job log options:
  --follow, -f    Follow output until the job finishes.
  --tail lines    Print only the final number of existing lines.

Removal options:
  --force         Skip confirmation for task rm and job prune.
  --before age    Prune jobs older than an age such as 24h or 7d.

Job references accept a full ID or a unique ID prefix or suffix. Task names
match case-insensitively by display name or JSON filename.
`

type cliOptions struct {
	mode       cliMode
	task       string
	job        string
	values     map[string]string
	status     daemon.Status
	filterTask string
	helpTopic  string
	shell      string
	before     time.Duration
	tailSet    bool
	tail       int
	limit      int
	all        bool
	detach     bool
	dryRun     bool
	json       bool
	quiet      bool
	follow     bool
	force      bool
}

func parseCLI(args []string) (cliOptions, error) {
	if len(args) == 0 {
		return cliOptions{mode: modeTUI}, nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		if args[0] == "help" {
			return parseHelpCLI(args[1:])
		}
		if len(args) != 1 {
			return cliOptions{}, errors.New("usage: taskrunner --help")
		}
		return helpOptions("")
	case "create":
		if len(args) == 2 && isHelpArgument(args[1]) {
			return helpOptions("create")
		}
		if len(args) != 1 {
			return cliOptions{}, errors.New("usage: taskrunner create")
		}
		return cliOptions{mode: modeCreate}, nil
	case "edit":
		if len(args) == 2 && isHelpArgument(args[1]) {
			return helpOptions("edit")
		}
		if len(args) != 2 {
			return cliOptions{}, errors.New("usage: taskrunner edit <task>")
		}
		return cliOptions{mode: modeEdit, task: args[1]}, nil
	case "run":
		return parseRunCLI(args[1:])
	case "tasks":
		return parseTasksCLI(args[1:])
	case "task":
		return parseTaskCLI(args[1:])
	case "jobs":
		return parseJobsCLI(args[1:])
	case "job":
		return parseJobCLI(args[1:])
	case "completion":
		return parseCompletionCLI(args[1:])
	default:
		return cliOptions{}, fmt.Errorf("unknown command %q\n\n%s", args[0], helpText)
	}
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
			options := make([]string, len(field.Options))
			for i, option := range field.Options {
				options[i] = task.ResolveKnownValues(option, values)
			}
			if value != "" && !contains(options, value) {
				return nil, fmt.Errorf("field %q must be one of: %s", field.Key, strings.Join(options, ", "))
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

func executeTaskCLI(workspace string, item task.Task, values map[string]string, options cliOptions, stdout, stderr io.Writer) error {
	command, err := item.Render(values)
	if err != nil {
		return err
	}
	for _, field := range item.Fields {
		if !field.Raw {
			continue
		}
		if _, err := fmt.Fprintf(stderr, "Warning: task %q uses raw shell interpolation for field %q.\n", item.Name, field.Key); err != nil {
			return fmt.Errorf("write raw field warning: %w", err)
		}
	}
	if options.dryRun {
		if options.quiet {
			_, err = fmt.Fprintln(stdout, command)
		} else {
			_, err = fmt.Fprintf(stdout, "Command: %s\n", command)
		}
		if err != nil {
			return fmt.Errorf("write rendered command: %w", err)
		}
		return nil
	}

	client := daemon.NewClient(workspace)
	job, err := client.Start(item.File, item.Name, command, item.JobPolicy)
	if err != nil {
		return fmt.Errorf("start task %q: %w", item.Name, err)
	}
	if options.detach {
		if _, err := fmt.Fprintln(stdout, job.ID); err != nil {
			return fmt.Errorf("write job id: %w", err)
		}
		return nil
	}

	outputOffset := 0
	for {
		current, err := client.Job(job.ID, outputOffset)
		if err != nil {
			return fmt.Errorf("follow task %q: %w", item.Name, err)
		}
		if current.Output != "" {
			if _, err := io.WriteString(stdout, current.Output); err != nil {
				return fmt.Errorf("write task %q output: %w", item.Name, err)
			}
		}
		outputOffset = current.OutputSize
		if current.Done() {
			return jobResultError(current)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func findJob(jobs []daemon.Job, reference string) (daemon.Job, error) {
	for _, job := range jobs {
		if job.ID == reference {
			return job, nil
		}
	}

	var match *daemon.Job
	for i := range jobs {
		if !strings.HasPrefix(jobs[i].ID, reference) && !strings.HasSuffix(jobs[i].ID, reference) {
			continue
		}
		if match != nil {
			return daemon.Job{}, fmt.Errorf("job reference %q is ambiguous", reference)
		}
		match = &jobs[i]
	}
	if match == nil {
		return daemon.Job{}, fmt.Errorf("job %q not found", reference)
	}
	return *match, nil
}
