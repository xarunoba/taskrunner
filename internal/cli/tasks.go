package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// reportLoadWarnings prints per-file task load problems to stderr so JSON
// and table stdout stays machine-readable.
func reportLoadWarnings(store *task.Store, stderr io.Writer) error {
	for _, warning := range store.LoadWarnings() {
		if _, err := fmt.Fprintln(stderr, "warning:", warning); err != nil {
			return fmt.Errorf("write load warning: %w", err)
		}
	}
	return nil
}

// findInvalidTask returns the skipped task file with exactly that filename,
// if any. Only the exact byte filename matches: skipped files cannot be
// resolved through name or stem fallbacks.
func findInvalidTask(store *task.Store, name string) (task.Task, bool) {
	for _, item := range store.InvalidTasks() {
		if item.File == name {
			return item, true
		}
	}
	return task.Task{}, false
}

func findValidTask(store *task.Store, items []task.Task, name string) (task.Task, error) {
	for i, item := range store.InvalidTasks() {
		if item.File == name {
			return task.Task{}, store.LoadWarnings()[i]
		}
	}
	return findTask(items, name)
}

func executeTaskValidate(store *task.Store, items []task.Task, name string) error {
	if name != "" {
		item, err := findValidTask(store, items, name)
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
	if warnings := store.LoadWarnings(); len(warnings) > 0 {
		return fmt.Errorf("validate tasks: %w", warnings[0])
	}
	return nil
}

func executeTaskRemove(store *task.Store, items []task.Task, name string, force bool, stdin io.Reader, stdout, stderr io.Writer) error {
	item, invalid := findInvalidTask(store, name)
	if !invalid {
		var err error
		item, err = findTask(items, name)
		if err != nil {
			return err
		}
	}
	label := item.Name
	if label == "" {
		label = item.File
	}
	confirmed, err := confirmDestructive(
		stdin,
		stderr,
		fmt.Sprintf("Remove task %q? [y/N] ", label),
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
			_, store, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			if err := reportLoadWarnings(store, a.stderr); err != nil {
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
			_, store, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			if err := reportLoadWarnings(store, a.stderr); err != nil {
				return err
			}
			item, err := findValidTask(store, items, args[0])
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
			_, store, items, err := loadTaskStore()
			if err != nil {
				return err
			}
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			if err := reportLoadWarnings(store, a.stderr); err != nil {
				return err
			}
			return executeTaskValidate(store, items, name)
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
			if err := reportLoadWarnings(store, a.stderr); err != nil {
				return err
			}
			return executeTaskRemove(store, items, args[0], force, a.stdin, a.stdout, a.stderr)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Remove without interactive confirmation")
	return cmd
}

// findTask prefers an exact filename, then case-insensitive filename, task
// name, and file-stem matches. Within each fallback priority an ambiguous
// match is rejected with the exact candidate filenames; exact byte
// filenames always win.
func findTask(items []task.Task, name string) (task.Task, error) {
	for i := range items {
		if items[i].File == name {
			return items[i], nil
		}
	}
	for priority := range 3 {
		match := -1
		var files []string
		for i := range items {
			key := items[i].File
			switch priority {
			case 1:
				key = items[i].Name
			case 2:
				key = strings.TrimSuffix(items[i].File, filepath.Ext(items[i].File))
			}
			if !strings.EqualFold(key, name) {
				continue
			}
			if match < 0 {
				match = i
				continue
			}
			if files == nil {
				files = []string{items[match].File}
			}
			files = append(files, items[i].File)
		}
		if len(files) > 0 {
			sort.Strings(files)
			return task.Task{}, fmt.Errorf(
				"task %q is ambiguous, matches %s; use the exact filename",
				name, strings.Join(files, ", "),
			)
		}
		if match >= 0 {
			return items[match], nil
		}
	}
	return task.Task{}, fmt.Errorf("task %q not found", name)
}
