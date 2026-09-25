package task

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
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

type renderField struct {
	Field
	value   string
	present bool
	flag    bool
}

func (t Task) Render(values map[string]string) (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}
	fields := make([]renderField, len(t.Fields))
	byKey := make(map[string]int, len(fields))
	for i, field := range t.Fields {
		fields[i].Field = field
		if field.Type == FieldRefer {
			source := fields[byKey[field.From]]
			fields[i].value, fields[i].present, fields[i].flag = source.value, source.present, source.flag
		} else {
			value, ok := values[field.Key]
			if !ok {
				return "", fmt.Errorf("field %q has no value", field.Key)
			}
			fields[i].value = value
			fields[i].present = field.present(value)
			fields[i].flag = field.Type == FieldConfirm
		}
		byKey[field.Key] = i
	}

	// This unquoted expansion is never executed. It lets us choose marker and
	// heredoc delimiter names absent from the complete runtime text, including
	// values joined to their prefixes and suffixes.
	expand := func(marker string) string {
		return valuePlaceholderPattern.ReplaceAllStringFunc(t.Command, func(placeholder string) string {
			i, ok := byKey[placeholder[2:len(placeholder)-2]]
			if !ok {
				return placeholder
			}
			field := fields[i]
			if !field.present {
				return ""
			}
			value := field.value
			switch {
			case field.flag:
				value = ""
			case marker != "" && !field.Raw:
				value = marker + strconv.Itoa(i) + "__"
			}
			return field.Prefix + value + field.Suffix
		})
	}
	literal := expand("")
	marker := "__taskrunner_value_"
	for strings.Contains(literal, marker) || strings.Contains(t.Command, marker) {
		marker += "_"
	}
	command := expand(marker)
	if command == literal {
		return command, nil
	}
	parser := syntax.NewParser(syntax.KeepComments(true))
	tree, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return "", fmt.Errorf("parse argument template: %w", err)
	}

	// Normalize backticks to $() and choose fresh heredoc delimiters before
	// inserting values. Otherwise even a quoted heredoc can be terminated by
	// a runtime value containing its delimiter on a line of its own.
	normalize := false
	delimiter := "__taskrunner_eof_"
	for strings.Contains(literal, delimiter) {
		delimiter += "_"
	}
	delimiterCount := 0
	syntax.Walk(tree, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.CmdSubst:
			normalize = normalize || node.Backquotes
		case *syntax.Redirect:
			if node.Hdoc == nil {
				break
			}
			normalize = true
			name := delimiter + strconv.Itoa(delimiterCount) + "__"
			delimiterCount++
			var part syntax.WordPart = &syntax.Lit{Value: name}
			if heredocIsLiteral(command, node) {
				part = &syntax.SglQuoted{Value: name}
			}
			node.Word = &syntax.Word{Parts: []syntax.WordPart{part}}
		}
		return true
	})
	if normalize {
		var normalized strings.Builder
		if err := syntax.NewPrinter().Print(&normalized, tree); err != nil {
			return "", fmt.Errorf("render argument template: %w", err)
		}
		command = normalized.String()
		tree, err = parser.Parse(strings.NewReader(command), "")
		if err != nil {
			return "", fmt.Errorf("parse normalized argument template: %w", err)
		}
	}
	return substituteShellArguments(command, marker, fields, tree)
}

type argumentContext uint8

const (
	argumentUnquoted argumentContext = iota
	argumentSingle
	argumentDouble
	argumentANSI
	argumentHeredoc
	argumentLiteral
	argumentComment
)

type argumentReplacement struct {
	start, end int
	field      int
	context    argumentContext
	allowed    bool
}

func heredocIsLiteral(command string, redirect *syntax.Redirect) bool {
	word := command[redirect.Word.Pos().Offset():redirect.Word.End().Offset()]
	return strings.ContainsAny(word, "'\"\\")
}

func substituteShellArguments(command, marker string, fields []renderField, tree *syntax.File) (string, error) {
	var replacements []argumentReplacement
	for offset := 0; offset < len(command); {
		index := strings.Index(command[offset:], marker)
		if index < 0 {
			break
		}
		start := offset + index
		numberStart := start + len(marker)
		numberLength := strings.Index(command[numberStart:], "__")
		if numberLength < 0 {
			return "", errors.New("unterminated argument marker")
		}
		numberEnd := numberStart + numberLength
		field, err := strconv.Atoi(command[numberStart:numberEnd])
		if err != nil || field < 0 || field >= len(fields) {
			return "", errors.New("invalid argument marker")
		}
		offset = numberEnd + 2
		replacements = append(replacements, argumentReplacement{start: start, end: offset, field: field})
	}
	type state struct {
		context argumentContext
		word    bool
		expr    bool
		index   syntax.Node
	}
	var stack []state
	heredocs := make(map[*syntax.Word]argumentContext)
	syntax.Walk(tree, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		var current state
		if len(stack) > 0 {
			current = stack[len(stack)-1]
		}
		if node == current.index {
			current.expr = true
		}
		leaf := false
		switch node := node.(type) {
		case *syntax.Assign:
			current.index = node.Index
		case *syntax.ArrayElem:
			current.index = node.Index
		case *syntax.Redirect:
			if node.Hdoc != nil {
				context := argumentHeredoc
				if heredocIsLiteral(command, node) {
					context = argumentLiteral
				}
				heredocs[node.Hdoc] = context
			}
		case *syntax.Word:
			current.word = true
			if context, ok := heredocs[node]; ok {
				current.context = context
			}
		case *syntax.CmdSubst, *syntax.ProcSubst:
			current = state{}
		case *syntax.ParamExp, *syntax.ArithmExp, *syntax.ArithmCmd,
			*syntax.BinaryArithm, *syntax.UnaryArithm, *syntax.ParenArithm,
			*syntax.CStyleLoop, *syntax.TestClause:
			current.expr = true
		case *syntax.DblQuoted:
			current.context = argumentDouble
		case *syntax.SglQuoted:
			current.context = argumentSingle
			if node.Dollar {
				current.context = argumentANSI
			}
			leaf = true
		case *syntax.Lit:
			leaf = true
		case *syntax.Comment:
			current = state{context: argumentComment, word: true}
			leaf = true
		}
		stack = append(stack, current)
		if leaf {
			start, end := int(node.Pos().Offset()), int(node.End().Offset())
			index := sort.Search(len(replacements), func(i int) bool { return replacements[i].start >= start })
			for ; index < len(replacements) && replacements[index].end <= end; index++ {
				replacements[index].context = current.context
				replacements[index].allowed = current.word && !current.expr
			}
		}
		return true
	})

	var out strings.Builder
	offset := 0
	for _, replacement := range replacements {
		field := fields[replacement.field]
		if !replacement.allowed {
			return "", fmt.Errorf("field %q must be a shell argument, not a shell expression or identifier; use raw mode only for trusted syntax", field.Key)
		}
		out.WriteString(command[offset:replacement.start])
		if replacement.context != argumentSingle && replacement.context != argumentLiteral && replacement.context != argumentComment {
			backslashes := 0
			for i := replacement.start - 1; i >= 0 && command[i] == '\\'; i-- {
				backslashes++
			}
			if backslashes%2 != 0 {
				out.WriteByte('\\')
			}
		}
		out.WriteString(quoteArgument(field.value, replacement.context))
		offset = replacement.end
	}
	out.WriteString(command[offset:])
	return out.String(), nil
}

// present reports whether the field contributes a fragment for this value.
func (f Field) present(value string) bool {
	if f.Type == FieldConfirm {
		return value == strconv.FormatBool(true)
	}
	return value != ""
}

func quoteArgument(value string, context argumentContext) string {
	switch context {
	case argumentSingle:
		return strings.ReplaceAll(value, "'", `'\''`)
	case argumentDouble:
		return doubleQuoteEscaper.Replace(value)
	case argumentANSI:
		return ansiQuoteEscaper.Replace(value)
	case argumentHeredoc:
		return heredocEscaper.Replace(value)
	case argumentLiteral:
		return value
	case argumentComment:
		return strings.ReplaceAll(value, "\n", "\n# ")
	default:
		return shellQuote(value)
	}
}

var doubleQuoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
var ansiQuoteEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)
var heredocEscaper = strings.NewReplacer(`\`, `\\`, "`", "\\`", `$`, `\$`)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
