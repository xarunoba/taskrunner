package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/xarunoba/taskrunner/internal/daemon"
)

func executeJobsCLI(workspace string, options jobListOptions, stdout io.Writer) error {
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
	for i := len(jobs) - 1; i >= 0; i-- {
		job := jobs[i]
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
	return executeJobLogsCLI(client, job, options, stdout)
}

func executeJobByReference(workspace, reference string, action jobAction, jsonOutput bool, stdout io.Writer) error {
	client, job, err := loadJobByReference(workspace, reference)
	if err != nil {
		return err
	}

	switch action {
	case jobWait:
		return executeJobWaitCLI(client, job, stdout)
	case jobInspect:
		return executeJobInspectCLI(job, jsonOutput, stdout)
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

func executeJobLogsCLI(client *daemon.Client, job daemon.Job, options jobLogsOptions, stdout io.Writer) error {
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

func executeJobWaitCLI(client *daemon.Client, job daemon.Job, stdout io.Writer) error {
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

func executeJobInspectCLI(job daemon.Job, jsonOutput bool, stdout io.Writer) error {
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

func executeJobPruneCLI(client *daemon.Client, options jobPruneOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	jobs, err := client.Jobs()
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}
	cutoff := time.Time{}
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
