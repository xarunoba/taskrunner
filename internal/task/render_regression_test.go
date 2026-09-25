package task

import (
	"os/exec"
	"testing"
)

func TestRenderShellSyntaxBoundaries(t *testing.T) {
	t.Parallel()
	value := "; printf INJECTED; # $(printf INJECTED) `printf INJECTED` ' \" \\"
	tests := []struct {
		name, command, prefix, suffix, value, want string
	}{
		{name: "escaped placeholder", command: `printf "%s" \{{value}}`, value: value, want: `\` + value},
		{name: "escaped prefix", command: `printf "%s" {{value}}`, prefix: `\`, value: value, want: `\` + value},
		{name: "quoted affixes", command: `printf "%s" {{value}}`, prefix: `"`, suffix: `"`, value: value, want: value},
		{name: "nested substitution", command: `printf "%s" "$(printf "%s" {{value}})"`, value: value, want: value},
		{name: "backticks", command: "printf \"%s\" \"`printf \"%s\" {{value}}`\"", value: value, want: value},
		{name: "comment quote", command: "# don't\nprintf '%s' {{value}}", value: value, want: value},
		{name: "comment newlines", command: "# {{value}}\nprintf safe", value: "first\nprintf INJECTED\n#", want: "safe"},
		{name: "expanding heredoc", command: "cat <<EOF\ndon't\n{{value}}\nEOF", value: value, want: "don't\n" + value + "\n"},
		{name: "quoted heredoc delimiter", command: "cat <<'EOF'\n{{value}}\nEOF", value: "EOF\nprintf INJECTED\n#", want: "EOF\nprintf INJECTED\n#\n"},
		{name: "expanding heredoc delimiter", command: "cat <<EOF\n{{value}}\nEOF", value: "EOF\nprintf INJECTED\n#", want: "EOF\nprintf INJECTED\n#\n"},
		{name: "escaped heredoc delimiter", command: "cat <<\\EOF\n{{value}}\nEOF", value: value, want: value + "\n"},
		{name: "tab stripped heredoc", command: "cat <<-EOF\n\t{{value}}\n\tEOF", value: value, want: value + "\n"},
		{name: "nested heredocs", command: "cat <<OUTER\n$(cat <<INNER\n{{value}}\nINNER\n)\nOUTER", value: value, want: value + "\n"},
		{name: "ANSI quotes", command: `printf "%s" $'{{value}}'`, value: value, want: value},
		{name: "marker collision", command: `printf "%s" {{value}}`, value: "__taskrunner_value_0__", want: "__taskrunner_value_0__"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := Task{
				Name: "Quote boundary", Command: tt.command,
				Fields: []Field{{Key: "value", Label: "Value", Type: FieldText, Prefix: tt.prefix, Suffix: tt.suffix}},
			}
			command, err := item.Render(map[string]string{"value": tt.value})
			if err != nil {
				t.Fatal(err)
			}
			output, err := exec.Command("/bin/bash", "-c", command).CombinedOutput()
			if err != nil {
				t.Fatalf("execute rendered command: %v, output %q", err, output)
			}
			if string(output) != tt.want {
				t.Fatalf("output = %q, want %q", output, tt.want)
			}
		})
	}
}

func TestRenderRejectsArgumentValuesInShellExpressions(t *testing.T) {
	t.Parallel()
	for _, command := range []string{
		`printf %s $(({{value}}))`,
		`printf %s "${unset:-{{value}}}"`,
		`{{value}}=example`,
		`a[{{value}}]=example`,
		`a=([{{value}}]=example)`,
		`[[ 1 -eq {{value}} ]]`,
	} {
		item := Task{Name: "Expression", Command: command, Fields: []Field{{Key: "value", Label: "Value", Type: FieldText}}}
		if _, err := item.Render(map[string]string{"value": "a[$(printf INJECTED)]"}); err == nil {
			t.Errorf("accepted an argument placeholder in %q", command)
		}
	}
}
