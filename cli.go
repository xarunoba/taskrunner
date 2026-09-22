package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
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
	modeJobs
	modeJobLogs
	modeJobCancel
	modeJobRerun
	modeJobRemove
	modeHelp
)

const helpText = `Usage:
  taskrunner
  taskrunner create
  taskrunner edit <task>
  taskrunner run <task> [--set <key>=<value>]...
  taskrunner jobs [--all]
  taskrunner job <logs|cancel|rerun|rm> <job>

Commands:
  create        Open the task creation form directly.
  edit          Open an existing task in the editor.
  run           Run a task without opening the TUI.
  jobs          List active jobs. Use --all to include completed jobs.
  job logs      Print a job's combined standard output and standard error.
  job cancel    Cancel a queued or running job.
  job rerun     Start a new job with the same command and policy.
  job rm        Remove a completed job and its output.

Run options:
  --set k=v     Pre-seed a task field. Repeat for multiple fields.

Job references accept a full ID or a unique ID prefix or suffix.
Task names are matched case-insensitively by display name or JSON filename.
Required fields must be supplied to "run". Omitted optional fields use empty values.
`

type cliOptions struct {
	mode   cliMode
	task   string
	job    string
	values map[string]string
	all    bool
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
	case "jobs":
		return parseJobsCLI(args[1:])
	case "job":
		return parseJobCLI(args[1:])
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

func parseJobsCLI(args []string) (cliOptions, error) {
	switch {
	case len(args) == 0:
		return cliOptions{mode: modeJobs}, nil
	case len(args) == 1 && (args[0] == "--all" || args[0] == "-a"):
		return cliOptions{mode: modeJobs, all: true}, nil
	default:
		return cliOptions{}, errors.New("usage: taskrunner jobs [--all]")
	}
}

func parseJobCLI(args []string) (cliOptions, error) {
	if len(args) != 2 {
		return cliOptions{}, errors.New("usage: taskrunner job <logs|cancel|rerun|rm> <job>")
	}

	var mode cliMode
	switch args[0] {
	case "logs":
		mode = modeJobLogs
	case "cancel":
		mode = modeJobCancel
	case "rerun":
		mode = modeJobRerun
	case "rm":
		mode = modeJobRemove
	default:
		return cliOptions{}, fmt.Errorf("unknown job command %q", args[0])
	}
	return cliOptions{mode: mode, job: args[1]}, nil
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

func executeTaskCLI(workspace string, item task.Task, values map[string]string, _ io.Reader, stdout, _ io.Writer) error {
	command, err := item.Render(values)
	if err != nil {
		return err
	}
	client := daemon.NewClient(workspace)
	job, err := client.Start(item.File, item.Name, command, item.JobPolicy)
	if err != nil {
		return fmt.Errorf("start task %q: %w", item.Name, err)
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
			if current.StorageError != "" {
				return fmt.Errorf("save job history for task %q: %s", item.Name, current.StorageError)
			}
			if current.Status != daemon.StatusSucceeded {
				return fmt.Errorf("run task %q: %s", item.Name, current.Error)
			}
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func executeJobsCLI(workspace string, all bool, stdout io.Writer) error {
	jobs, err := daemon.NewClient(workspace).Jobs()
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "JOB ID\tTASK\tSTATUS\tCREATED\tCOMMAND"); err != nil {
		return fmt.Errorf("write jobs: %w", err)
	}
	for i := len(jobs) - 1; i >= 0; i-- {
		job := jobs[i]
		if !all && job.Done() {
			continue
		}
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\n",
			shortJobID(job.ID),
			job.Name,
			job.Status,
			job.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			job.Command,
		); err != nil {
			return fmt.Errorf("write jobs: %w", err)
		}
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("write jobs: %w", err)
	}
	return nil
}

func executeJobCLI(workspace string, mode cliMode, reference string, stdout io.Writer) error {
	client := daemon.NewClient(workspace)
	jobs, err := client.Jobs()
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	job, err := findJob(jobs, reference)
	if err != nil {
		return err
	}

	var result string
	switch mode {
	case modeJobLogs:
		job, err = client.Job(job.ID, 0)
		if err == nil {
			_, err = io.WriteString(stdout, job.Output)
		}
	case modeJobCancel:
		job, err = client.Cancel(job.ID)
		result = fmt.Sprintf("%s\t%s\tcancel requested", job.ID, job.Name)
	case modeJobRerun:
		job, err = client.Rerun(job.ID)
		result = fmt.Sprintf("%s\t%s\t%s", job.ID, job.Name, job.Status)
	case modeJobRemove:
		job, err = client.Remove(job.ID)
		result = fmt.Sprintf("%s\t%s\tremoved", job.ID, job.Name)
	}
	if err != nil {
		return fmt.Errorf("%s job %q: %w", jobOperation(mode), reference, err)
	}
	if result != "" {
		if _, err := fmt.Fprintln(stdout, result); err != nil {
			return fmt.Errorf("write job result: %w", err)
		}
	}
	return nil
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

func jobOperation(mode cliMode) string {
	switch mode {
	case modeJobLogs:
		return "read"
	case modeJobCancel:
		return "cancel"
	case modeJobRerun:
		return "rerun"
	case modeJobRemove:
		return "remove"
	default:
		return "manage"
	}
}
