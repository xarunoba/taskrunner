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

func TestTaskJobPolicyJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Task{
		Name:    "Build",
		Command: "go build ./...",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "job_policy") {
		t.Fatalf("sequential task JSON unexpectedly contains a job policy: %s", data)
	}

	data, err = json.Marshal(Task{
		Name:      "Build",
		Command:   "go build ./...",
		JobPolicy: JobCancelPrevious,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var item Task
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if item.JobPolicy != JobCancelPrevious {
		t.Fatalf("task job policy = %q, want %q", item.JobPolicy, JobCancelPrevious)
	}
}

func TestResolveKnownValuesLeavesUnknownPlaceholdersLiteral(t *testing.T) {
	t.Parallel()

	got := ResolveKnownValues(
		"Deploy {{version}} to {{environment}}?",
		map[string]string{"version": "1.4.0"},
	)
	if want := "Deploy 1.4.0 to {{environment}}?"; got != want {
		t.Fatalf("ResolveKnownValues() = %q, want %q", got, want)
	}
}

func TestTaskRenderDropsEmptyOptionalValues(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "optional drop",
		Command: `printf '%s;' deploy {{tag}}`,
		Fields: []Field{
			{Key: "tag", Label: "Tag", Type: FieldText, Optional: true, Prefix: "--tag "},
		},
	}

	command, err := item.Render(map[string]string{"tag": ""})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), "deploy;"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestTaskRenderAppliesPrefixAndSuffix(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "prefixed fragment",
		Command: `printf '%s;' deploy {{tag}}`,
		Fields: []Field{
			{Key: "tag", Label: "Tag", Type: FieldText, Prefix: "--tag ", Suffix: "!"},
		},
	}

	command, err := item.Render(map[string]string{"tag": "v1"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), "deploy;--tag;v1!;"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestTaskRenderConfirmRendersValuelessFlag(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "confirm flag",
		Command: `printf '%s;' cleanup {{force}}`,
		Fields: []Field{
			{Key: "force", Label: "Force", Type: FieldConfirm, Optional: true, Prefix: "--force"},
		},
	}

	confirmed, err := item.Render(map[string]string{"force": "true"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got, want := confirmed, "printf '%s;' cleanup --force"; got != want {
		t.Fatalf("confirmed command = %q, want %q", got, want)
	}

	declined, err := item.Render(map[string]string{"force": "false"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got, want := declined, "printf '%s;' cleanup "; got != want {
		t.Fatalf("declined command = %q, want %q", got, want)
	}
}

func TestTaskRenderReferMirrorsSourceValueAndPresence(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "refer value",
		Command: `printf '%s;' command {{db}} {{confirmDb}}`,
		Fields: []Field{
			{Key: "db", Label: "Database", Type: FieldText, Optional: true, Prefix: "--db "},
			{Key: "confirmDb", Label: "Confirm DB", Type: FieldRefer, From: "db", Prefix: "--confirm "},
		},
	}

	filled, err := item.Render(map[string]string{"db": "appdb", "confirmDb": "appdb"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", filled).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), "command;--db;appdb;--confirm;appdb;"; got != want {
		t.Fatalf("filled output = %q, want %q", got, want)
	}

	empty, err := item.Render(map[string]string{"db": "", "confirmDb": ""})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err = exec.Command("/bin/sh", "-c", empty).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), "command;"; got != want {
		t.Fatalf("empty output = %q, want %q", got, want)
	}
}

func TestTaskRenderReferOnConfirmIsFlagOnly(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "refer confirm",
		Command: `printf '%s;' cleanup {{force}} {{yes}}`,
		Fields: []Field{
			{Key: "force", Label: "Force", Type: FieldConfirm, Optional: true, Prefix: "--force"},
			{Key: "yes", Label: "Also Yes", Type: FieldRefer, From: "force", Prefix: "--yes"},
		},
	}

	confirmed, err := item.Render(map[string]string{"force": "true", "yes": "true"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got, want := confirmed, "printf '%s;' cleanup --force --yes"; got != want {
		t.Fatalf("confirmed command = %q, want %q", got, want)
	}

	declined, err := item.Render(map[string]string{"force": "false", "yes": "false"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got, want := declined, "printf '%s;' cleanup  "; got != want {
		t.Fatalf("declined command = %q, want %q", got, want)
	}
}

func TestTaskRenderReferChainResolvesFlags(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "refer chain",
		Command: `deploy {{force}} {{a}} {{b}}`,
		Fields: []Field{
			{Key: "force", Label: "Force", Type: FieldConfirm, Optional: true, Prefix: "--force"},
			{Key: "a", Label: "A", Type: FieldRefer, From: "force", Prefix: "--a"},
			{Key: "b", Label: "B", Type: FieldRefer, From: "a", Prefix: "--b"},
		},
	}

	command, err := item.Render(map[string]string{"force": "true", "a": "true", "b": "true"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got, want := command, "deploy --force --a --b"; got != want {
		t.Fatalf("command = %q, want %q (chain must not inject 'true')", got, want)
	}
}

func TestTaskValidateRejectsInvalidReferFields(t *testing.T) {
	t.Parallel()

	cases := map[string]Task{
		"missing from": {
			Name: "t", Command: "true",
			Fields: []Field{{Key: "r", Label: "R", Type: FieldRefer}},
		},
		"unknown or later from": {
			Name: "t", Command: "true",
			Fields: []Field{
				{Key: "r", Label: "R", Type: FieldRefer, From: "db"},
				{Key: "db", Label: "DB", Type: FieldText},
			},
		},
		"optional refer": {
			Name: "t", Command: "true",
			Fields: []Field{
				{Key: "db", Label: "DB", Type: FieldText},
				{Key: "r", Label: "R", Type: FieldRefer, From: "db", Optional: true},
			},
		},
		"from on text field": {
			Name: "t", Command: "true",
			Fields: []Field{
				{Key: "db", Label: "DB", Type: FieldText, From: "db"},
			},
		},
	}
	for name, item := range cases {
		if err := item.Validate(); err == nil {
			t.Fatalf("%s: Validate() = nil, want error", name)
		}
	}
}

func TestTaskValidateAcceptsReferFields(t *testing.T) {
	t.Parallel()

	item := Task{
		Name: "t", Command: "true",
		Fields: []Field{
			{Key: "db", Label: "DB", Type: FieldText, Optional: true},
			{Key: "r", Label: "R", Type: FieldRefer, From: "db", Prefix: "--db "},
		},
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestTaskRenderUnreferencedFieldsOnlyGate(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "gate",
		Command: `printf '%s;' deploy`,
		Fields: []Field{
			{Key: "gate", Label: "Gate", Type: FieldConfirm},
			{Key: "ticket", Label: "Ticket", Type: FieldText},
		},
	}

	command, err := item.Render(map[string]string{"gate": "true", "ticket": "AB-12"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got, want := command, "printf '%s;' deploy"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
}

func TestTaskRenderPrefixAndSuffixWithRawValue(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "raw fragment",
		Command: `printf '%b' {{paths}}`,
		Fields: []Field{
			{Key: "paths", Label: "Paths", Type: FieldText, Raw: true, Prefix: "{", Suffix: "}"},
		},
	}

	command, err := item.Render(map[string]string{"paths": `"a b"`})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), "{a b}"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestRenderDoesNotResubstitutePlaceholderLookalikes(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "resubstitution",
		Command: `printf '%s\n' {{first}}`,
		Fields: []Field{
			{Key: "first", Label: "First", Type: FieldText},
			{Key: "second", Label: "Second", Type: FieldText},
		},
	}
	command, err := item.Render(map[string]string{
		"first":  "{{second}}",
		"second": `; printf INJECTED; #`,
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), "{{second}}\n"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestRenderEscapesValuesInsideDoubleQuotes(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "double quoted",
		Command: `printf '%s\n' "{{value}}"`,
		Fields: []Field{
			{Key: "value", Label: "Value", Type: FieldText},
		},
	}
	value := `$(printf INJECTED)"; rm -rf x #`
	command, err := item.Render(map[string]string{"value": value})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), value+"\n"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestRenderPreservesValuesInsideSingleQuotes(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "single quoted",
		Command: `printf '%s\n' '{{value}}'`,
		Fields: []Field{
			{Key: "value", Label: "Value", Type: FieldText},
		},
	}
	value := `it's "quoted" $(x)`
	command, err := item.Render(map[string]string{"value": value})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), value+"\n"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}

func TestRenderQuotesSameFieldPerContext(t *testing.T) {
	t.Parallel()

	item := Task{
		Name:    "mixed contexts",
		Command: `printf '%s %s\n' {{value}} "{{value}}"`,
		Fields: []Field{
			{Key: "value", Label: "Value", Type: FieldText},
		},
	}
	value := `a'b"c$d`
	command, err := item.Render(map[string]string{"value": value})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("execute rendered command: %v", err)
	}
	if got, want := string(output), value+" "+value+"\n"; got != want {
		t.Fatalf("rendered output = %q, want %q", got, want)
	}
}
