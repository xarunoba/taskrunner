package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

func TestParseRunCLIWithPreseededFields(t *testing.T) {
	t.Parallel()

	options, err := parseCLI([]string{
		"run",
		"Deploy",
		"--set",
		"environment=staging",
		"--set=note=release candidate",
	})
	if err != nil {
		t.Fatalf("parseCLI() error = %v", err)
	}
	if options.mode != modeRun || options.task != "Deploy" {
		t.Fatalf("parseCLI() mode=%d task=%q", options.mode, options.task)
	}
	if got := options.values["environment"]; got != "staging" {
		t.Fatalf("environment = %q, want staging", got)
	}
	if got := options.values["note"]; got != "release candidate" {
		t.Fatalf("note = %q, want release candidate", got)
	}
}

func TestParseJobCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		mode cliMode
		job  string
		all  bool
	}{
		{args: []string{"jobs"}, mode: modeJobs},
		{args: []string{"jobs", "--all"}, mode: modeJobs, all: true},
		{args: []string{"job", "logs", "abc123"}, mode: modeJobLogs, job: "abc123"},
		{args: []string{"job", "cancel", "abc123"}, mode: modeJobCancel, job: "abc123"},
		{args: []string{"job", "rerun", "abc123"}, mode: modeJobRerun, job: "abc123"},
		{args: []string{"job", "rm", "abc123"}, mode: modeJobRemove, job: "abc123"},
	}
	for _, test := range tests {
		options, err := parseCLI(test.args)
		if err != nil {
			t.Fatalf("parseCLI(%q) error = %v", test.args, err)
		}
		if options.mode != test.mode || options.job != test.job || options.all != test.all {
			t.Fatalf(
				"parseCLI(%q) = mode %d, job %q, all %t; want mode %d, job %q, all %t",
				test.args,
				options.mode,
				options.job,
				options.all,
				test.mode,
				test.job,
				test.all,
			)
		}
	}
}

func TestPrepareTaskValuesAllowsOmittedOptionalFields(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Deploy",
		Command: "deploy {{environment}} {{note}} {{confirmed}}",
		Fields: []task.Field{
			{Key: "environment", Label: "Environment", Type: task.FieldChoice, Options: []string{"staging", "production"}},
			{Key: "note", Label: "Note", Type: task.FieldText, Optional: true},
			{Key: "confirmed", Label: "Confirmed", Type: task.FieldConfirm, Optional: true},
		},
	}
	values, err := prepareTaskValues(item, map[string]string{"environment": "staging"})
	if err != nil {
		t.Fatalf("prepareTaskValues() error = %v", err)
	}
	if values["note"] != "" {
		t.Fatalf("optional note = %q, want empty", values["note"])
	}
	if values["confirmed"] != "false" {
		t.Fatalf("optional confirmation = %q, want false", values["confirmed"])
	}
}

func TestPrepareTaskValuesResolvesEarlierValuesInChoiceOptions(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Deploy",
		Command: "deploy {{target}}",
		Fields: []task.Field{
			{Key: "environment", Label: "Environment", Type: task.FieldText},
			{Key: "target", Label: "Target", Type: task.FieldChoice, Options: []string{"{{environment}} primary"}},
		},
	}
	values, err := prepareTaskValues(item, map[string]string{
		"environment": "staging",
		"target":      "staging primary",
	})
	if err != nil {
		t.Fatalf("prepareTaskValues() error = %v", err)
	}
	if got := values["target"]; got != "staging primary" {
		t.Fatalf("target = %q, want %q", got, "staging primary")
	}
}

func TestPrepareTaskValuesRejectsMissingRequiredField(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name:    "Deploy",
		Command: "deploy {{environment}}",
		Fields: []task.Field{
			{Key: "environment", Label: "Environment", Type: task.FieldText},
		},
	}
	_, err := prepareTaskValues(item, nil)
	if err == nil || !strings.Contains(err.Error(), `missing required field "environment"`) {
		t.Fatalf("prepareTaskValues() error = %v", err)
	}
}

func TestExecuteTaskCLIUsesPreparedValues(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- daemon.Serve(workspace, 100*time.Millisecond)
	}()
	socket := filepath.Join(workspace, ".taskrunner", "daemon.sock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon socket was not created")
		}
		time.Sleep(10 * time.Millisecond)
	}

	item := task.Task{
		Name:    "Print",
		File:    "print.json",
		Command: `printf '%s|%s|%s' {{message}} {{note}} {{confirmed}}`,
		Fields: []task.Field{
			{Key: "message", Label: "Message", Type: task.FieldText},
			{Key: "note", Label: "Note", Type: task.FieldText, Optional: true},
			{Key: "confirmed", Label: "Confirmed", Type: task.FieldConfirm, Optional: true},
		},
	}
	values, err := prepareTaskValues(item, map[string]string{"message": "hello world"})
	if err != nil {
		t.Fatalf("prepareTaskValues() error = %v", err)
	}

	var output bytes.Buffer
	if err := executeTaskCLI(workspace, item, values, strings.NewReader(""), &output, &output); err != nil {
		t.Fatalf("executeTaskCLI() error = %v", err)
	}
	if got, want := output.String(), "hello world||false"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
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

func TestJobCLIListsActiveJobsAndManagesHistory(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	serverErr := startCLITestDaemon(t, workspace)
	client := daemon.NewClient(workspace)

	completed, err := client.Start("done.json", "Done", "printf complete", task.JobSequential)
	if err != nil {
		t.Fatalf("start completed job: %v", err)
	}
	completed = waitForCLIJob(t, client, completed.ID)
	active, err := client.Start("active.json", "Active", "sleep 5", task.JobSequential)
	if err != nil {
		t.Fatalf("start active job: %v", err)
	}

	var output bytes.Buffer
	if err := executeJobsCLI(workspace, false, &output); err != nil {
		t.Fatalf("executeJobsCLI(active) error = %v", err)
	}
	if !strings.Contains(output.String(), shortJobID(active.ID)) {
		t.Fatalf("active jobs output does not contain %q:\n%s", shortJobID(active.ID), output.String())
	}
	if strings.Contains(output.String(), shortJobID(completed.ID)) {
		t.Fatalf("active jobs output contains completed job %q:\n%s", shortJobID(completed.ID), output.String())
	}

	output.Reset()
	if err := executeJobsCLI(workspace, true, &output); err != nil {
		t.Fatalf("executeJobsCLI(all) error = %v", err)
	}
	for _, job := range []daemon.Job{completed, active} {
		if !strings.Contains(output.String(), shortJobID(job.ID)) {
			t.Fatalf("all jobs output does not contain %q:\n%s", shortJobID(job.ID), output.String())
		}
	}

	output.Reset()
	if err := executeJobCLI(workspace, modeJobLogs, shortJobID(completed.ID), &output); err != nil {
		t.Fatalf("executeJobCLI(logs) error = %v", err)
	}
	if got, want := output.String(), "complete"; got != want {
		t.Fatalf("job logs = %q, want %q", got, want)
	}
	if err := executeJobCLI(workspace, modeJobRemove, shortJobID(completed.ID), &output); err != nil {
		t.Fatalf("executeJobCLI(rm) error = %v", err)
	}
	if _, err := client.Job(completed.ID, 0); err == nil {
		t.Fatal("removed job remains available")
	}
	if err := executeJobCLI(workspace, modeJobCancel, shortJobID(active.ID), &output); err != nil {
		t.Fatalf("executeJobCLI(cancel) error = %v", err)
	}
	waitForCLIJob(t, client, active.ID)

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("daemon.Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop after becoming idle")
	}
}

func startCLITestDaemon(t *testing.T, workspace string) <-chan error {
	t.Helper()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- daemon.Serve(workspace, 100*time.Millisecond)
	}()
	socket := filepath.Join(workspace, ".taskrunner", "daemon.sock")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			return serverErr
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon socket was not created")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForCLIJob(t *testing.T, client *daemon.Client, id string) daemon.Job {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		job, err := client.Job(id, 0)
		if err != nil {
			t.Fatalf("get job %q: %v", id, err)
		}
		if job.Done() {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %q did not finish", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
