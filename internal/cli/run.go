package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

type taskRunOptions struct {
	assignments []string
	detach      bool
	dryRun      bool
	quiet       bool
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

func addFieldAssignment(values map[string]string, assignment string) error {
	key, value, ok := strings.Cut(assignment, "=")
	if !ok || key == "" {
		return fmt.Errorf("invalid field assignment %q; expected <key>=<value>", assignment)
	}
	values[key] = value
	return nil
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
		if field.Type == task.FieldRefer {
			if _, direct := supplied[field.Key]; direct {
				return nil, fmt.Errorf("field %q derives from %q; set %q instead", field.Key, field.From, field.From)
			}
			values[field.Key] = values[field.From]
			continue
		}
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
