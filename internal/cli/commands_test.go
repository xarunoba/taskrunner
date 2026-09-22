package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

func TestContextualHelpShowsCommandsAndOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args     []string
		contains []string
	}{
		{
			args:     []string{"job"},
			contains: []string{"Available Commands:", "logs", "wait", "inspect", "prune"},
		},
		{
			args:     []string{"job", "logs", "--help"},
			contains: []string{"Usage:", "--follow", "--tail"},
		},
		{
			args:     []string{"task"},
			contains: []string{"Available Commands:", "show", "validate", "rm"},
		},
		{
			args:     []string{"jobs", "--help"},
			contains: []string{"--all", "--status", "--task", "--json", "--quiet"},
		},
		{
			args:     []string{"run", "Build", "--help"},
			contains: []string{"--set", "--detach", "--dry-run", "--quiet"},
		},
		{
			args:     []string{"help", "job", "prune"},
			contains: []string{"--status", "--before", "--force"},
		},
	}
	for _, test := range tests {
		var output bytes.Buffer
		if err := Execute(test.args, strings.NewReader(""), &output, &output); err != nil {
			t.Fatalf("run(%q) error = %v", test.args, err)
		}
		for _, expected := range test.contains {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("run(%q) output does not contain %q:\n%s", test.args, expected, output.String())
			}
		}
	}
}

func TestTaskCommandsExposeAndRemovePersistedDefinitions(t *testing.T) {
	t.Parallel()

	store := task.NewStore(t.TempDir())
	saved, err := store.Save(task.Task{Name: "Build", Command: "go build ./..."}, "")
	if err != nil {
		t.Fatalf("save task: %v", err)
	}
	items, err := store.Load()
	if err != nil {
		t.Fatalf("load tasks: %v", err)
	}

	var output bytes.Buffer
	if err := executeTasksCLI(items, true, &output); err != nil {
		t.Fatalf("executeTasksCLI() error = %v", err)
	}
	var listed []taskView
	if err := json.Unmarshal(output.Bytes(), &listed); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	if len(listed) != 1 || listed[0].File != saved.File || listed[0].Command != saved.Command {
		t.Fatalf("listed tasks = %#v, want saved task", listed)
	}

	if err := executeTaskRemoveCLI(store, items, "Build", false, strings.NewReader("yes\n"), &output, &output); err == nil || !strings.Contains(err.Error(), "use --force") {
		t.Fatalf("nonterminal task removal error = %v", err)
	}
	if err := executeTaskRemoveCLI(store, items, "Build", true, strings.NewReader(""), &output, &output); err != nil {
		t.Fatalf("forced task removal: %v", err)
	}
	items, err = store.Load()
	if err != nil {
		t.Fatalf("reload tasks: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("tasks after removal = %d, want 0", len(items))
	}
}

func TestDryRunQuotesValuesAndWarnsForRawFields(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Deploy",
		Command: "deploy {{version}} {{flags}}",
		Fields: []task.Field{
			{Key: "version", Label: "Version", Type: task.FieldText},
			{Key: "flags", Label: "Flags", Type: task.FieldText, Raw: true},
		},
	}
	values := map[string]string{"version": "release candidate", "flags": "--force"}
	var stdout, stderr bytes.Buffer
	options := taskRunOptions{dryRun: true, quiet: true}
	if err := executeTaskCLI(t.TempDir(), item, values, options, &stdout, &stderr); err != nil {
		t.Fatalf("executeTaskCLI(dry-run) error = %v", err)
	}
	if got, want := stdout.String(), "deploy 'release candidate' --force\n"; got != want {
		t.Fatalf("dry-run output = %q, want %q", got, want)
	}
	if !strings.Contains(stderr.String(), `raw shell interpolation for field "flags"`) {
		t.Fatalf("dry-run warning = %q", stderr.String())
	}
}

func TestJobFiltersAndTailLines(t *testing.T) {
	t.Parallel()

	jobs := []daemon.Job{
		{ID: "old-success", Name: "Build", TaskID: "build.json", Status: daemon.StatusSucceeded},
		{ID: "new-running", Name: "Build", TaskID: "build.json", Status: daemon.StatusRunning},
		{ID: "newest-failed", Name: "Test", TaskID: "test.json", Status: daemon.StatusFailed},
	}
	filtered := filterJobs(jobs, jobListOptions{all: true, filterTask: "Build", limit: 1})
	if len(filtered) != 1 || filtered[0].ID != "new-running" {
		t.Fatalf("filtered jobs = %#v, want newest Build job", filtered)
	}
	failed := filterJobs(jobs, jobListOptions{status: daemon.StatusFailed})
	if len(failed) != 1 || failed[0].ID != "newest-failed" {
		t.Fatalf("failed jobs = %#v", failed)
	}
	if got, want := tailLines("one\ntwo\nthree\n", 2), "two\nthree\n"; got != want {
		t.Fatalf("tailLines() = %q, want %q", got, want)
	}
	if got := tailLines("one\ntwo", 0); got != "" {
		t.Fatalf("tailLines(..., 0) = %q, want empty", got)
	}
}

func TestJobPruneRemovesOnlyMatchingCompletedJobs(t *testing.T) {
	workspace := t.TempDir()
	serverErr := startCLITestDaemon(t, workspace)
	client := daemon.NewClient(workspace)

	succeeded, err := client.Start("pass.json", "Pass", "true", task.JobSequential)
	if err != nil {
		t.Fatalf("start successful job: %v", err)
	}
	failed, err := client.Start("fail.json", "Fail", "false", task.JobSequential)
	if err != nil {
		t.Fatalf("start failed job: %v", err)
	}
	waitForCLIJob(t, client, succeeded.ID)
	waitForCLIJob(t, client, failed.ID)

	var output bytes.Buffer
	options := jobPruneOptions{status: daemon.StatusSucceeded, force: true}
	if err := executeJobPruneCLI(client, options, strings.NewReader(""), &output, &output); err != nil {
		t.Fatalf("executeJobPruneCLI() error = %v", err)
	}
	jobs, err := client.Jobs()
	if err != nil {
		t.Fatalf("list jobs after prune: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != failed.ID {
		t.Fatalf("jobs after prune = %#v, want failed job", jobs)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("daemon.Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop after becoming idle")
	}
}

func TestCompletionScriptsRegisterTaskrunner(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "zsh", "fish"} {
		var output bytes.Buffer
		if err := Execute([]string{"completion", shell}, strings.NewReader(""), &output, &output); err != nil {
			t.Fatalf("run(completion %q) error = %v", shell, err)
		}
		if !strings.Contains(output.String(), "taskrunner") {
			t.Fatalf("%s completion does not register taskrunner", shell)
		}
	}
}
