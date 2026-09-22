package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/xarunoba/taskrunner/internal/task"
)

type taskView struct {
	Name      string         `json:"name"`
	File      string         `json:"file"`
	Command   string         `json:"command"`
	Fields    []task.Field   `json:"fields,omitempty"`
	JobPolicy task.JobPolicy `json:"job_policy,omitempty"`
}

func executeTasksCLI(items []task.Task, jsonOutput bool, stdout io.Writer) error {
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

func executeTaskShowCLI(item task.Task, jsonOutput bool, stdout io.Writer) error {
	return writeJSON(stdout, newTaskView(item), !jsonOutput)
}

func executeTaskValidateCLI(items []task.Task, name string) error {
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

func executeTaskRemoveCLI(store *task.Store, items []task.Task, name string, force bool, stdin io.Reader, stdout, stderr io.Writer) error {
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
