package task

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var fieldKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var valuePlaceholderPattern = regexp.MustCompile(`\{\{[A-Za-z][A-Za-z0-9_]*\}\}`)

func (t Task) Validate() error {
	if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Command) == "" {
		return errors.New("name and command are required")
	}

	switch t.JobPolicy {
	case JobSequential, JobParallel, JobCancelPrevious:
	default:
		return fmt.Errorf("unknown job policy %q", t.JobPolicy)
	}

	declared := make(map[string]struct{}, len(t.Fields))
	for i, field := range t.Fields {
		if !fieldKeyPattern.MatchString(field.Key) {
			return fmt.Errorf("field %d key must start with a letter and contain only letters, numbers, or underscores", i+1)
		}
		if strings.TrimSpace(field.Label) == "" {
			return fmt.Errorf("field %q requires a label", field.Key)
		}
		if _, exists := declared[field.Key]; exists {
			return fmt.Errorf("field key %q is duplicated", field.Key)
		}

		switch field.Type {
		case FieldText, FieldFile, FieldConfirm:
			if field.From != "" {
				return fmt.Errorf("field %q with type %q cannot declare from", field.Key, field.Type)
			}
		case FieldChoice:
			if field.From != "" {
				return fmt.Errorf("field %q with type %q cannot declare from", field.Key, field.Type)
			}
			if len(field.Options) == 0 {
				return fmt.Errorf("choice field %q requires at least one option", field.Key)
			}
			for _, option := range field.Options {
				if strings.TrimSpace(option) == "" {
					return fmt.Errorf("choice field %q contains an empty option", field.Key)
				}
			}
		case FieldRefer:
			if !fieldKeyPattern.MatchString(field.From) {
				return fmt.Errorf("refer field %q requires from to name an earlier field key", field.Key)
			}
			if _, ok := declared[field.From]; !ok {
				return fmt.Errorf("refer field %q names unknown or later field %q", field.Key, field.From)
			}
			if field.Optional {
				return fmt.Errorf("refer field %q is never prompted; optional does not apply", field.Key)
			}
		default:
			return fmt.Errorf("field %q has unknown type %q", field.Key, field.Type)
		}
		declared[field.Key] = struct{}{}
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
	present := make(map[string]bool, len(t.Fields))
	declared := make(map[string]Field, len(t.Fields))
	for _, field := range t.Fields {
		declared[field.Key] = field
		if field.Type == FieldRefer {
			if _, ok := values[field.From]; !ok {
				return "", fmt.Errorf("field %q has no value", field.From)
			}
			fragment := ""
			if present[field.From] {
				source := declared[field.From]
				for source.Type == FieldRefer {
					source = declared[source.From]
				}
				fragment = field.referFragment(source, values[field.From])
			}
			command = strings.ReplaceAll(command, "{{"+field.Key+"}}", fragment)
			present[field.Key] = present[field.From]
			continue
		}
		value, ok := values[field.Key]
		if !ok {
			return "", fmt.Errorf("field %q has no value", field.Key)
		}
		command = strings.ReplaceAll(command, "{{"+field.Key+"}}", field.fragment(value))
		present[field.Key] = field.present(value)
	}
	return command, nil
}

// present reports whether the field contributes a fragment for this value.
func (f Field) present(value string) bool {
	if f.Type == FieldConfirm {
		return value == strconv.FormatBool(true)
	}
	return value != ""
}

// fragment builds the text that replaces the field's placeholder. Fields
// without a present value contribute nothing, so optional fields and
// declined confirmations can remove command fragments entirely. Fields
// never referenced in the command inject nothing and only gate execution.
func (f Field) fragment(value string) string {
	if !f.present(value) {
		return ""
	}
	if f.Type == FieldConfirm {
		return f.Prefix + f.Suffix
	}
	replacement := value
	if !f.Raw {
		replacement = shellQuote(value)
	}
	return f.Prefix + replacement + f.Suffix
}

// referFragment mirrors the referenced field's semantics with this field's
// own prefix and suffix. A confirm source contributes no value, so refer
// fields can attach further flags to a confirmation.
func (f Field) referFragment(source Field, sourceValue string) string {
	if source.Type == FieldConfirm {
		return f.Prefix + f.Suffix
	}
	replacement := sourceValue
	if !f.Raw {
		replacement = shellQuote(replacement)
	}
	return f.Prefix + replacement + f.Suffix
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
