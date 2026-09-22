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
