package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/xarunoba/taskrunner/internal/daemon"
)

func executeJobs(workspace string, options jobListOptions, stdout io.Writer) error {
	jobs, err := daemon.NewClient(workspace).Jobs()
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	jobs = filterJobs(jobs, options)

	if options.json {
		return writeJSON(stdout, jobs, false)
	}
	if options.quiet {
		for _, job := range jobs {
			if _, err := fmt.Fprintln(stdout, job.ID); err != nil {
				return fmt.Errorf("write jobs: %w", err)
			}
		}
		return nil
	}

	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "JOB ID\tTASK\tSTATUS\tCREATED\tCOMMAND"); err != nil {
		return fmt.Errorf("write jobs: %w", err)
	}
	for _, job := range jobs {
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\n",
			job.ShortID(),
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

func filterJobs(jobs []daemon.Job, options jobListOptions) []daemon.Job {
	filtered := make([]daemon.Job, 0, len(jobs))
	for _, job := range slices.Backward(jobs) {
		if options.status != "" {
			if job.Status != options.status {
				continue
			}
		} else if !options.all && job.Done() {
			continue
		}
		if options.filterTask != "" && !jobMatchesTask(job, options.filterTask) {
			continue
		}
		filtered = append(filtered, job)
		if options.limit > 0 && len(filtered) == options.limit {
			break
		}
	}
	return filtered
}

func jobMatchesTask(job daemon.Job, value string) bool {
	fileName := strings.TrimSuffix(job.TaskID, filepath.Ext(job.TaskID))
	return strings.EqualFold(job.Name, value) || strings.EqualFold(job.TaskID, value) || strings.EqualFold(fileName, value)
}

func executeJobLogsByReference(workspace, reference string, options jobLogsOptions, stdout io.Writer) error {
	client, job, err := loadJobByReference(workspace, reference)
	if err != nil {
		return err
	}
	return executeJobLogs(client, job, options, stdout)
}

func executeJobByReference(workspace, reference string, action jobAction, jsonOutput bool, stdout io.Writer) error {
	client, job, err := loadJobByReference(workspace, reference)
	if err != nil {
		return err
	}

	switch action {
	case jobWait:
		return executeJobWait(client, job, stdout)
	case jobInspect:
		return executeJobInspect(job, jsonOutput, stdout)
	case jobCancel:
		job, err = client.Cancel(job.ID)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "%s\t%s\tcancel requested\n", job.ID, job.Name)
		}
	case jobRerun:
		job, err = client.Rerun(job.ID)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "%s\t%s\t%s\n", job.ID, job.Name, job.Status)
		}
	case jobRemove:
		job, err = client.Remove(job.ID)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "%s\t%s\tremoved\n", job.ID, job.Name)
		}
	default:
		return errors.New("unknown job action")
	}
	if err != nil {
		return fmt.Errorf("%s job %q: %w", jobActionName(action), reference, err)
	}
	return nil
}

func loadJobByReference(workspace, reference string) (*daemon.Client, daemon.Job, error) {
	client := daemon.NewClient(workspace)
	jobs, err := client.Jobs()
	if err != nil {
		return nil, daemon.Job{}, fmt.Errorf("list jobs: %w", err)
	}
	job, err := findJob(jobs, reference)
	if err != nil {
		return nil, daemon.Job{}, err
	}
	return client, job, nil
}

func executeJobLogs(client *daemon.Client, job daemon.Job, options jobLogsOptions, stdout io.Writer) error {
	current, err := client.Job(job.ID, 0)
	if err != nil {
		return fmt.Errorf("read job %q: %w", job.ID, err)
	}
	output := current.Output
	if options.tailSet {
		output = tailLines(output, options.tail)
	}
	if _, err := io.WriteString(stdout, output); err != nil {
		return fmt.Errorf("write job output: %w", err)
	}
	if !options.follow {
		return nil
	}

	offset := current.OutputSize
	for !current.Done() {
		time.Sleep(50 * time.Millisecond)
		current, err = client.Job(job.ID, offset)
		if err != nil {
			return fmt.Errorf("follow job %q: %w", job.ID, err)
		}
		if current.Output != "" {
			if _, err := io.WriteString(stdout, current.Output); err != nil {
				return fmt.Errorf("write job output: %w", err)
			}
		}
		offset = current.OutputSize
	}
	return jobResultError(current)
}

func tailLines(output string, count int) string {
	if count < 0 || output == "" {
		return output
	}
	if count == 0 {
		return ""
	}
	lines := strings.SplitAfter(output, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if count >= len(lines) {
		return output
	}
	return strings.Join(lines[len(lines)-count:], "")
}

func executeJobWait(client *daemon.Client, job daemon.Job, stdout io.Writer) error {
	current := job
	var err error
	for !current.Done() {
		time.Sleep(50 * time.Millisecond)
		current, err = client.Job(job.ID, current.OutputSize)
		if err != nil {
			return fmt.Errorf("wait for job %q: %w", job.ID, err)
		}
	}
	if _, err := fmt.Fprintf(stdout, "%s\t%s\t%s\n", current.ID, current.Name, current.Status); err != nil {
		return fmt.Errorf("write job status: %w", err)
	}
	return jobResultError(current)
}

func executeJobInspect(job daemon.Job, jsonOutput bool, stdout io.Writer) error {
	if jsonOutput {
		return writeJSON(stdout, job, false)
	}
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	values := [][2]string{
		{"ID", job.ID},
		{"Task", job.Name},
		{"Task file", job.TaskID},
		{"Status", string(job.Status)},
		{"Policy", jobPolicyName(job.Policy)},
		{"Command", job.Command},
		{"Created", formatJobTime(job.CreatedAt)},
		{"Started", formatJobTime(job.StartedAt)},
		{"Ended", formatJobTime(job.EndedAt)},
		{"Output bytes", fmt.Sprintf("%d", job.OutputSize)},
		{"Error", job.Error},
		{"Storage error", job.StorageError},
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(table, "%s:\t%s\n", value[0], value[1]); err != nil {
			return fmt.Errorf("write job details: %w", err)
		}
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("write job details: %w", err)
	}
	return nil
}

func formatJobTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format(time.RFC3339)
}

func executeJobPrune(client *daemon.Client, options jobPruneOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	jobs, err := client.Jobs()
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	var cutoff time.Time
	if options.before > 0 {
		cutoff = time.Now().Add(-options.before)
	}
	matches := make([]daemon.Job, 0, len(jobs))
	for _, job := range jobs {
		if !job.Done() || options.status != "" && job.Status != options.status {
			continue
		}
		if !cutoff.IsZero() && (job.EndedAt.IsZero() || !job.EndedAt.Before(cutoff)) {
			continue
		}
		matches = append(matches, job)
	}
	if len(matches) == 0 {
		_, err := fmt.Fprintln(stdout, "Removed 0 jobs.")
		return err
	}
	confirmed, err := confirmDestructive(
		stdin,
		stderr,
		fmt.Sprintf("Remove %d completed jobs? [y/N] ", len(matches)),
		options.force,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		_, err := fmt.Fprintln(stderr, "Aborted.")
		return err
	}
	for _, job := range matches {
		if _, err := client.Remove(job.ID); err != nil {
			return fmt.Errorf("remove job %q: %w", job.ID, err)
		}
	}
	if _, err := fmt.Fprintf(stdout, "Removed %d jobs.\n", len(matches)); err != nil {
		return fmt.Errorf("write prune result: %w", err)
	}
	return nil
}

func jobResultError(job daemon.Job) error {
	if job.StorageError != "" {
		return fmt.Errorf("save job history: %s", job.StorageError)
	}
	if job.Status == daemon.StatusSucceeded {
		return nil
	}
	if job.Error != "" {
		return fmt.Errorf("job %s: %s", job.Status, job.Error)
	}
	return fmt.Errorf("job %s", job.Status)
}

func jobActionName(action jobAction) string {
	switch action {
	case jobWait:
		return "wait for"
	case jobInspect:
		return "inspect"
	case jobCancel:
		return "cancel"
	case jobRerun:
		return "rerun"
	case jobRemove:
		return "remove"
	default:
		return "manage"
	}
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
			return executeJobs(workspace, options, a.stdout)
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
	cmd.Flags().IntVar(&options.tail, "tail", -1, "Print only the last N lines of output")
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
			return executeJobPrune(daemon.NewClient(workspace), options, a.stdin, a.stdout, a.stderr)
		},
	}
	cmd.Flags().StringVar(&statusValue, "status", "", "Remove only jobs with this completed status")
	cmd.Flags().StringVar(&beforeValue, "before", "", "Remove jobs older than a duration such as 24h or 7d")
	cmd.Flags().BoolVar(&options.force, "force", false, "Remove without interactive confirmation")
	return cmd
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
