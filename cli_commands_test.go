package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

func TestParseExpandedCLIOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args  []string
		check func(cliOptions) bool
	}{
		{args: []string{"run", "Build", "--detach"}, check: func(o cliOptions) bool { return o.mode == modeRun && o.detach }},
		{args: []string{"run", "Build", "--dry-run", "--quiet"}, check: func(o cliOptions) bool { return o.dryRun && o.quiet }},
		{args: []string{"tasks", "--json"}, check: func(o cliOptions) bool { return o.mode == modeTasks && o.json }},
		{args: []string{"task", "show", "Build", "--json"}, check: func(o cliOptions) bool { return o.mode == modeTaskShow && o.task == "Build" && o.json }},
		{args: []string{"task", "validate"}, check: func(o cliOptions) bool { return o.mode == modeTaskValidate && o.task == "" }},
		{args: []string{"task", "rm", "Build", "--force"}, check: func(o cliOptions) bool { return o.mode == modeTaskRemove && o.force }},
		{args: []string{"jobs", "--status", "failed", "--task", "Build", "--limit", "5", "--json"}, check: func(o cliOptions) bool {
			return o.mode == modeJobs && o.status == daemon.StatusFailed && o.filterTask == "Build" && o.limit == 5 && o.json
		}},
		{args: []string{"job", "logs", "--follow", "--tail", "20", "abc"}, check: func(o cliOptions) bool {
			return o.mode == modeJobLogs && o.follow && o.tailSet && o.tail == 20 && o.job == "abc"
		}},
		{args: []string{"job", "wait", "abc"}, check: func(o cliOptions) bool { return o.mode == modeJobWait && o.job == "abc" }},
		{args: []string{"job", "inspect", "abc", "--json"}, check: func(o cliOptions) bool { return o.mode == modeJobInspect && o.json }},
		{args: []string{"job", "prune", "--status", "succeeded", "--before", "7d", "--force"}, check: func(o cliOptions) bool {
			return o.mode == modeJobPrune && o.status == daemon.StatusSucceeded && o.before == 7*24*time.Hour && o.force
		}},
		{args: []string{"completion", "fish"}, check: func(o cliOptions) bool { return o.mode == modeCompletion && o.shell == "fish" }},
	}
	for _, test := range tests {
		options, err := parseCLI(test.args)
		if err != nil {
			t.Fatalf("parseCLI(%q) error = %v", test.args, err)
		}
		if !test.check(options) {
			t.Fatalf("parseCLI(%q) = %#v", test.args, options)
		}
	}
}

func TestContextualHelpShowsCommandsAndOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args     []string
		contains []string
	}{
		{
			args:     []string{"job"},
			contains: []string{"Commands:", "logs", "wait", "inspect", "prune", "Options:"},
		},
		{
			args:     []string{"job", "logs", "--help"},
			contains: []string{"Usage:", "--follow, -f", "--tail lines"},
		},
		{
			args:     []string{"task"},
			contains: []string{"Commands:", "show", "validate", "rm", "Options:"},
		},
		{
			args:     []string{"jobs", "--help"},
			contains: []string{"--all, -a", "--status value", "--task value", "--json", "--quiet"},
		},
		{
			args:     []string{"run", "Build", "--help"},
			contains: []string{"--set k=v", "--detach", "--dry-run", "--quiet"},
		},
		{
			args:     []string{"help", "job", "prune"},
			contains: []string{"--status value", "--before age", "--force"},
		},
	}
	for _, test := range tests {
		var output bytes.Buffer
		if err := run(test.args, strings.NewReader(""), &output, &output); err != nil {
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
	options := cliOptions{dryRun: true, quiet: true}
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
	filtered := filterJobs(jobs, cliOptions{all: true, filterTask: "Build", limit: 1})
	if len(filtered) != 1 || filtered[0].ID != "new-running" {
		t.Fatalf("filtered jobs = %#v, want newest Build job", filtered)
	}
	failed := filterJobs(jobs, cliOptions{status: daemon.StatusFailed})
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
	options := cliOptions{mode: modeJobPrune, status: daemon.StatusSucceeded, force: true}
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
		if err := executeCompletionCLI(shell, &output); err != nil {
			t.Fatalf("executeCompletionCLI(%q) error = %v", shell, err)
		}
		if !strings.Contains(output.String(), "taskrunner") {
			t.Fatalf("%s completion does not register taskrunner", shell)
		}
	}
}
