package task

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestTaskRenderPreservesShellArgument(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "quote input",
		Command: `printf '%s' {{value}}`,
		Fields: []Field{
			{Key: "value", Label: "Value", Type: FieldText},
		},
	}
	command, err := item.Render(map[string]string{"value": `it's safe; printf injected`})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), `it's safe; printf injected`; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestTaskRenderAllowsExplicitRawShellSyntax(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "raw escapes",
		Command: `printf '%b' {{value}}`,
		Fields: []Field{
			{Key: "value", Label: "Value", Type: FieldText, Raw: true},
		},
	}
	command, err := item.Render(map[string]string{"value": `"first\nsecond"`})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), "first\nsecond"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestRequiredFieldKeepsExistingJSONShape(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Field{
		Key:   "value",
		Label: "Value",
		Type:  FieldText,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "optional") {
		t.Fatalf("required field JSON unexpectedly contains optional flag: %s", data)
	}
}
