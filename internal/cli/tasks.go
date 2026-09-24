package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/xarunoba/taskrunner/internal/task"
)

type taskView struct {
	Name      string         `json:"name"`
	File      string         `json:"file"`
	Command   string         `json:"command"`
	Fields    []task.Field   `json:"fields,omitempty"`
	JobPolicy task.JobPolicy `json:"job_policy,omitempty"`
}

func executeTasks(items []task.Task, jsonOutput bool, stdout io.Writer) error {
	if jsonOutput {
		views := make([]taskView, len(items))
		for i, item := range items {
			views[i] = newTaskView(item)
		}
		return writeJSON(stdout, views, false)
	}

	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "TASK\tFILE\tFIELDS\tPOLICY\tCOMMAND"); err != nil {
		return fmt.Errorf("write tasks: %w", err)
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%d\t%s\t%s\n",
			item.Name,
			item.File,
			len(item.Fields),
			jobPolicyName(item.JobPolicy),
			item.Command,
		); err != nil {
			return fmt.Errorf("write tasks: %w", err)
		}
	}
	if err := table.Flush(); err != nil {
		return fmt.Errorf("write tasks: %w", err)
	}
	return nil
}

func executeTaskShow(item task.Task, jsonOutput bool, stdout io.Writer) error {
	return writeJSON(stdout, newTaskView(item), !jsonOutput)
}

func executeTaskValidate(items []task.Task, name string) error {
	if name != "" {
		item, err := findTask(items, name)
		if err != nil {
			return err
		}
		return item.Validate()
	}
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("validate task %q: %w", item.Name, err)
		}
	}
	return nil
}

func executeTaskRemove(store *task.Store, items []task.Task, name string, force bool, stdin io.Reader, stdout, stderr io.Writer) error {
	item, err := findTask(items, name)
	if err != nil {
		return err
	}
	confirmed, err := confirmDestructive(
		stdin,
		stderr,
		fmt.Sprintf("Remove task %q? [y/N] ", item.Name),
		force,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		_, err := fmt.Fprintln(stderr, "Aborted.")
		return err
	}
	if err := store.Delete(item); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "%s\t%s\tremoved\n", item.File, item.Name); err != nil {
		return fmt.Errorf("write task removal: %w", err)
	}
	return nil
}

func newTaskView(item task.Task) taskView {
	return taskView{
		Name:      item.Name,
		File:      item.File,
		Command:   item.Command,
		Fields:    item.Fields,
		JobPolicy: item.JobPolicy,
	}
}

func jobPolicyName(policy task.JobPolicy) string {
	if policy == task.JobSequential {
		return "sequential"
	}
	return string(policy)
}

func writeJSON(w io.Writer, value any, indent bool) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if indent {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func confirmDestructive(stdin io.Reader, stderr io.Writer, prompt string, force bool) (bool, error) {
	if force {
		return true, nil
	}
	file, ok := stdin.(*os.File)
	if !ok {
		return false, errors.New("confirmation requires a terminal; use --force")
	}
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect input: %w", err)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return false, errors.New("confirmation requires a terminal; use --force")
	}
	if _, err := fmt.Fprint(stderr, prompt); err != nil {
		return false, fmt.Errorf("write confirmation prompt: %w", err)
	}
	answer, err := bufio.NewReader(file).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
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
			return executeTasks(items, jsonOutput, a.stdout)
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
			return executeTaskShow(item, jsonOutput, a.stdout)
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
			return executeTaskValidate(items, name)
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
			return executeTaskRemove(store, items, args[0], force, a.stdin, a.stdout, a.stderr)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Remove without interactive confirmation")
	return cmd
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
