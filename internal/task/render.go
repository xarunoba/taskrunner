package task

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var fieldKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var valuePlaceholderPattern = regexp.MustCompile(`\{\{[A-Za-z][A-Za-z0-9_]*\}\}`)

func (t Task) Validate() error {
	if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Command) == "" {
		return errors.New("name and command are required")
	}

	keys := make(map[string]struct{}, len(t.Fields))
	for i, field := range t.Fields {
		if !fieldKeyPattern.MatchString(field.Key) {
			return fmt.Errorf("field %d key must start with a letter and contain only letters, numbers, or underscores", i+1)
		}
		if strings.TrimSpace(field.Label) == "" {
			return fmt.Errorf("field %q requires a label", field.Key)
		}
		if _, exists := keys[field.Key]; exists {
			return fmt.Errorf("field key %q is duplicated", field.Key)
		}
		keys[field.Key] = struct{}{}

		switch field.Type {
		case FieldText, FieldFile, FieldConfirm:
		case FieldChoice:
			if len(field.Options) == 0 {
				return fmt.Errorf("choice field %q requires at least one option", field.Key)
			}
			for _, option := range field.Options {
				if strings.TrimSpace(option) == "" {
					return fmt.Errorf("choice field %q contains an empty option", field.Key)
				}
			}
		default:
			return fmt.Errorf("field %q has unknown type %q", field.Key, field.Type)
		}
	}
	return nil
}

// ResolveKnownValues replaces field placeholders that already have runtime values.
// Unknown placeholders remain literal so labels and options can contain braces.
func ResolveKnownValues(text string, values map[string]string) string {
	return valuePlaceholderPattern.ReplaceAllStringFunc(text, func(placeholder string) string {
		key := placeholder[2 : len(placeholder)-2]
		value, ok := values[key]
		if !ok {
			return placeholder
		}
		return value
	})
}

func (t Task) Render(values map[string]string) (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}

	command := t.Command
	for _, field := range t.Fields {
		value, ok := values[field.Key]
		if !ok {
			return "", fmt.Errorf("field %q has no value", field.Key)
		}
		replacement := value
		if !field.Raw {
			replacement = shellQuote(value)
		}
		command = strings.ReplaceAll(command, "{{"+field.Key+"}}", replacement)
	}
	return command, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
