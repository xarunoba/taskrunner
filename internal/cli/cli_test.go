package cli

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

func TestRunCommandWithPreseededFields(t *testing.T) {
	workspace := t.TempDir()
	store := task.NewStore(workspace)
	if _, err := store.Save(task.Task{
		Name:    "Deploy",
		Command: "deploy {{environment}} {{note}}",
		Fields: []task.Field{
			{Key: "environment", Label: "Environment", Type: task.FieldText},
			{Key: "note", Label: "Note", Type: task.FieldText},
		},
	}, ""); err != nil {
		t.Fatalf("save task: %v", err)
	}
	t.Chdir(workspace)

	var stdout, stderr bytes.Buffer
	err := Execute([]string{
		"run",
		"Deploy",
		"--dry-run",
		"--quiet",
		"--set",
		"environment=staging",
		"--set=note=release candidate",
	},
		strings.NewReader(""),
		&stdout,
		&stderr)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got, want := stdout.String(), "deploy 'staging' 'release candidate'\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
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

func TestPrepareTaskValuesDerivesReferFields(t *testing.T) {
	t.Parallel()

	item := task.Task{
		Name: "refer",
		Fields: []task.Field{
			{Key: "db", Label: "Database", Type: task.FieldText, Optional: true},
			{Key: "r", Label: "Confirm DB", Type: task.FieldRefer, From: "db"},
		},
	}

	values, err := prepareTaskValues(item, map[string]string{"db": "appdb"})
	if err != nil {
		t.Fatalf("prepareTaskValues() error = %v", err)
	}
	if values["r"] != "appdb" {
		t.Fatalf("derived refer value = %q, want %q", values["r"], "appdb")
	}

	values, err = prepareTaskValues(item, nil)
	if err != nil {
		t.Fatalf("prepareTaskValues() error = %v", err)
	}
	if values["r"] != "" {
		t.Fatalf("derived refer value = %q, want empty", values["r"])
	}

	if _, err := prepareTaskValues(item, map[string]string{"r": "other"}); err == nil {
		t.Fatal("prepareTaskValues() with direct refer assignment = nil error, want error")
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
	if err := executeTaskCLI(workspace, item, values, taskRunOptions{}, &output, &output); err != nil {
		t.Fatalf("executeTaskCLI() error = %v", err)
	}
	if got, want := output.String(), "hello world||"; got != want {
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
	if err := executeJobsCLI(workspace, jobListOptions{}, &output); err != nil {
		t.Fatalf("executeJobsCLI(active) error = %v", err)
	}
	if !strings.Contains(output.String(), active.ShortID()) {
		t.Fatalf("active jobs output does not contain %q:\n%s", active.ShortID(), output.String())
	}
	if strings.Contains(output.String(), completed.ShortID()) {
		t.Fatalf("active jobs output contains completed job %q:\n%s", completed.ShortID(), output.String())
	}

	output.Reset()
	if err := executeJobsCLI(workspace, jobListOptions{all: true}, &output); err != nil {
		t.Fatalf("executeJobsCLI(all) error = %v", err)
	}
	for _, job := range []daemon.Job{completed, active} {
		if !strings.Contains(output.String(), job.ShortID()) {
			t.Fatalf("all jobs output does not contain %q:\n%s", job.ShortID(), output.String())
		}
	}

	output.Reset()
	if err := executeJobLogsByReference(workspace, completed.ShortID(), jobLogsOptions{tail: -1}, &output); err != nil {
		t.Fatalf("executeJobLogsByReference() error = %v", err)
	}
	if got, want := output.String(), "complete"; got != want {
		t.Fatalf("job logs = %q, want %q", got, want)
	}
	if err := executeJobByReference(workspace, completed.ShortID(), jobRemove, false, &output); err != nil {
		t.Fatalf("executeJobByReference(remove) error = %v", err)
	}
	if _, err := client.Job(completed.ID, 0); err == nil {
		t.Fatal("removed job remains available")
	}
	if err := executeJobByReference(workspace, active.ShortID(), jobCancel, false, &output); err != nil {
		t.Fatalf("executeJobByReference(cancel) error = %v", err)
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
