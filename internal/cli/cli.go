package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/tui"
)

type cliApp struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

type taskRunOptions struct {
	assignments []string
	detach      bool
	dryRun      bool
	quiet       bool
}

type jobListOptions struct {
	status     daemon.Status
	filterTask string
	limit      int
	all        bool
	json       bool
	quiet      bool
}

type jobLogsOptions struct {
	tailSet bool
	tail    int
	follow  bool
}

type jobPruneOptions struct {
	status daemon.Status
	before time.Duration
	force  bool
}

type jobAction uint8

const (
	jobWait jobAction = iota + 1
	jobInspect
	jobCancel
	jobRerun
	jobRemove
)

// Execute parses args and runs the selected command.
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	root := newRootCommand(stdin, stdout, stderr)
	root.SetArgs(args)
	return root.Execute()
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	app := &cliApp{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
	root := &cobra.Command{
		Use:           "taskrunner",
		Short:         "Run workspace tasks",
		Long:          "Taskrunner stores and runs tasks for the current workspace. Run without a command to open the TUI.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.runTUI("", false)
		},
	}
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(
		app.newCreateCommand(),
		app.newEditCommand(),
		app.newRunCommand(),
		app.newTasksCommand(),
		app.newTaskCommand(),
		app.newJobsCommand(),
		app.newJobCommand(),
		newCompletionCommand(root),
		newDaemonCommand(),
	)
	return root
}

func (a *cliApp) newCreateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Open the task creation form",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runTUI("", true)
		},
	}
}

func (a *cliApp) newEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <task>",
		Short: "Open a task in the editor",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runTUI(args[0], false)
		},
	}
}

func (a *cliApp) newRunCommand() *cobra.Command {
	var options taskRunOptions
	cmd := &cobra.Command{
		Use:   "run <task>",
		Short: "Run a task without opening the TUI",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if options.detach && options.dryRun {
				return errors.New("--detach and --dry-run cannot be used together")
			}
			if options.quiet && !options.dryRun {
				return errors.New("--quiet requires --dry-run")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			workspace, _, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			item, err := findTask(items, args[0])
			if err != nil {
				return err
			}
			supplied := make(map[string]string, len(options.assignments))
			for _, assignment := range options.assignments {
				if err := addFieldAssignment(supplied, assignment); err != nil {
					return err
				}
			}
			values, err := prepareTaskValues(item, supplied)
			if err != nil {
				return err
			}
			return executeTaskCLI(workspace, item, values, options, a.stdout, a.stderr)
		},
	}
	cmd.Flags().StringArrayVar(&options.assignments, "set", nil, "Set a task field as key=value; repeatable")
	cmd.Flags().BoolVar(&options.detach, "detach", false, "Print the job ID and return without waiting")
	cmd.Flags().BoolVar(&options.dryRun, "dry-run", false, "Print the rendered command without creating a job")
	cmd.Flags().BoolVar(&options.quiet, "quiet", false, "With --dry-run, print only the rendered command")
	return cmd
}

func (a *cliApp) newTasksCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "List task definitions",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, _, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			return executeTasksCLI(items, jsonOutput, a.stdout)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a JSON array")
	return cmd
}

func (a *cliApp) newTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage task definitions",
	}
	cmd.AddCommand(
		a.newTaskShowCommand(),
		a.newTaskValidateCommand(),
		a.newTaskRemoveCommand(),
	)
	return cmd
}

func (a *cliApp) newTaskShowCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <task>",
		Short: "Print a task definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			_, _, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			item, err := findTask(items, args[0])
			if err != nil {
				return err
			}
			return executeTaskShowCLI(item, jsonOutput, a.stdout)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit compact JSON")
	return cmd
}

func (a *cliApp) newTaskValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [task]",
		Short: "Validate one task or every task",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			_, _, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			return executeTaskValidateCLI(items, name)
		},
	}
}

func (a *cliApp) newTaskRemoveCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rm <task>",
		Short: "Remove a task definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			_, store, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			return executeTaskRemoveCLI(store, items, args[0], force, a.stdin, a.stdout, a.stderr)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Remove without interactive confirmation")
	return cmd
}

func (a *cliApp) newJobsCommand() *cobra.Command {
	var (
		options     jobListOptions
		statusValue string
	)
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "List jobs",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if statusValue != "" {
				status, err := parseJobStatus(statusValue)
				if err != nil {
					return err
				}
				options.status = status
			}
			if cmd.Flags().Changed("limit") && options.limit < 1 {
				return errors.New("--limit requires a positive integer")
			}
			if options.json && options.quiet {
				return errors.New("--json and --quiet cannot be used together")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			workspace, err := workspacePath()
			if err != nil {
				return err
			}
			return executeJobsCLI(workspace, options, a.stdout)
		},
	}
	cmd.Flags().BoolVarP(&options.all, "all", "a", false, "Include completed jobs")
	cmd.Flags().StringVar(&statusValue, "status", "", "Filter by job status")
	cmd.Flags().StringVar(&options.filterTask, "task", "", "Filter by task name or filename")
	cmd.Flags().IntVar(&options.limit, "limit", 0, "Limit matching jobs")
	cmd.Flags().BoolVar(&options.json, "json", false, "Emit a JSON array")
	cmd.Flags().BoolVar(&options.quiet, "quiet", false, "Print full job IDs only")
	return cmd
}

func (a *cliApp) newJobCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "Manage jobs",
		Long:  "Manage jobs. Job references accept a full ID or a unique ID prefix or suffix.",
	}
	cmd.AddCommand(
		a.newJobLogsCommand(),
		a.newJobReferenceCommand("wait <job>", "Wait for a job to finish", jobWait),
		a.newJobInspectCommand(),
		a.newJobReferenceCommand("cancel <job>", "Cancel a queued or running job", jobCancel),
		a.newJobReferenceCommand("rerun <job>", "Start a new job with the same command and policy", jobRerun),
		a.newJobReferenceCommand("rm <job>", "Remove a completed job and its output", jobRemove),
		a.newJobPruneCommand(),
	)
	return cmd
}

func (a *cliApp) newJobLogsCommand() *cobra.Command {
	options := jobLogsOptions{tail: -1}
	cmd := &cobra.Command{
		Use:   "logs <job>",
		Short: "Print or follow job output",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			options.tailSet = cmd.Flags().Changed("tail")
			if options.tailSet && options.tail < 0 {
				return errors.New("--tail requires a non-negative integer")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			workspace, err := workspacePath()
			if err != nil {
				return err
			}
			return executeJobLogsByReference(workspace, args[0], options, a.stdout)
		},
	}
	cmd.Flags().BoolVarP(&options.follow, "follow", "f", false, "Follow output until the job finishes")
	cmd.Flags().IntVar(&options.tail, "tail", -1, "Print only the final number of existing lines")
	return cmd
}

func (a *cliApp) newJobInspectCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "inspect <job>",
		Short: "Print job details",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			workspace, err := workspacePath()
			if err != nil {
				return err
			}
			return executeJobByReference(workspace, args[0], jobInspect, jsonOutput, a.stdout)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit JSON")
	return cmd
}

func (a *cliApp) newJobReferenceCommand(use, short string, action jobAction) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			workspace, err := workspacePath()
			if err != nil {
				return err
			}
			return executeJobByReference(workspace, args[0], action, false, a.stdout)
		},
	}
}

func (a *cliApp) newJobPruneCommand() *cobra.Command {
	var (
		options     jobPruneOptions
		statusValue string
		beforeValue string
	)
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove matching completed jobs",
		Args:  cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if statusValue != "" {
				status, err := parseJobStatus(statusValue)
				if err != nil {
					return err
				}
				if status == daemon.StatusQueued || status == daemon.StatusRunning {
					return errors.New("job prune only accepts completed statuses")
				}
				options.status = status
			}
			if beforeValue != "" {
				before, err := parseAge(beforeValue)
				if err != nil {
					return fmt.Errorf("invalid --before value: %w", err)
				}
				options.before = before
			}
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			workspace, err := workspacePath()
			if err != nil {
				return err
			}
			return executeJobPruneCLI(daemon.NewClient(workspace), options, a.stdin, a.stdout, a.stderr)
		},
	}
	cmd.Flags().StringVar(&statusValue, "status", "", "Remove only jobs with this completed status")
	cmd.Flags().StringVar(&beforeValue, "before", "", "Remove jobs older than an age such as 24h or 7d")
	cmd.Flags().BoolVar(&options.force, "force", false, "Remove without interactive confirmation")
	return cmd
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "completion <bash|zsh|fish>",
		Short:                 "Generate shell completion source",
		Args:                  cobra.ExactArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish"},
		ValidArgsFunction:     nil,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}

func newDaemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "__daemon <workspace>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return daemon.Serve(args[0], daemon.DefaultIdleTimeout)
		},
	}
}

func (a *cliApp) runTUI(taskName string, create bool) error {
	_, store, items, err := loadTaskStore()
	if err != nil {
		return err
	}
	if create {
		return tui.Create(store, items, a.stdin, a.stdout)
	}
	if taskName == "" {
		return tui.Run(store, items, a.stdin, a.stdout)
	}
	item, err := findTask(items, taskName)
	if err != nil {
		return err
	}
	return tui.Edit(store, items, item, a.stdin, a.stdout)
}

func workspacePath() (string, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get workspace: %w", err)
	}
	return workspace, nil
}

func loadTaskStore() (string, *task.Store, []task.Task, error) {
	workspace, err := workspacePath()
	if err != nil {
		return "", nil, nil, err
	}
	store := task.NewStore(workspace)
	items, err := store.Load()
	if err != nil {
		return "", nil, nil, err
	}
	return workspace, store, items, nil
}

func addFieldAssignment(values map[string]string, assignment string) error {
	key, value, ok := strings.Cut(assignment, "=")
	if !ok || key == "" {
		return fmt.Errorf("invalid field assignment %q; expected <key>=<value>", assignment)
	}
	values[key] = value
	return nil
}

func parseJobStatus(value string) (daemon.Status, error) {
	status := daemon.Status(value)
	switch status {
	case daemon.StatusQueued, daemon.StatusRunning, daemon.StatusSucceeded, daemon.StatusFailed, daemon.StatusCanceled:
		return status, nil
	default:
		return "", fmt.Errorf("unknown job status %q", value)
	}
}

func parseAge(value string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(value, "d"); ok {
		count, err := strconv.Atoi(days)
		if err != nil || count < 1 {
			return 0, errors.New("day duration must be a positive integer such as 7d")
		}
		return time.Duration(count) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, errors.New("duration must be positive, such as 24h or 7d")
	}
	return duration, nil
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

func executeTaskCLI(workspace string, item task.Task, values map[string]string, options taskRunOptions, stdout, stderr io.Writer) error {
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
