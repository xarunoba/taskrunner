# Taskrunner

Taskrunner runs workspace-specific shell commands from a terminal UI or direct CLI commands. Each task is a readable JSON file under `.taskrunner/tasks/`.

The TUI uses [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Requirements

- Go 1.27 or newer

Taskrunner executes commands through `$SHELL -c`. It uses `/bin/sh` when `$SHELL` is empty.

## Install

Install from the current checkout:

```sh
go install .
```

Or build a binary in the repository:

```sh
go build -o taskrunner .
```

Run Taskrunner from the workspace whose tasks you want to use:

```sh
taskrunner
```

The current working directory is the workspace.

## TUI

Run `taskrunner` without arguments to open the task list.

### Task list

| Input | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select a task |
| `Enter` | Run the selected task |
| `n` | Create a task |
| `e` | Edit the selected task |
| `d` | Delete the selected task |
| `q` | Quit |
| Mouse click | Select a task or control |
| Mouse wheel | Move through the active control |

### Task editor

| Input | Action |
| --- | --- |
| `↑` / `↓` | Move between the name, command, and field list |
| `Tab` / `Shift+Tab` | Move forward or backward between sections |
| `a` | Add a field |
| `e` or `Enter` | Edit the selected field |
| `d` | Delete the selected field |
| `[` / `]` | Move the selected field earlier or later |
| `F2` or `Ctrl+S` | Save the task |
| `Esc` | Cancel |

Fields run in their displayed order. A field can be required or optional and can use safe argument interpolation or raw shell interpolation.

Labels and choice options can reference values collected by earlier fields:

```text
Deploy {{version}} to {{environment}}?
```

Taskrunner replaces placeholders with collected values. A placeholder without a collected value remains unchanged.

### Running a task

| Input | Action |
| --- | --- |
| `Enter` | Accept the current value |
| `↑` / `↓` | Browse previous values for the active text field |
| `Shift+Tab` or `Ctrl+←` | Return to the previous field |
| `s` | Skip an optional file field |
| `Esc` | Cancel the run |

Moving backward preserves values already entered. Optional text fields accept an empty value, optional choices include a **Skip** entry, and optional confirmations use `false` when **No** is selected.

Taskrunner records non-empty text values when a task starts. Runtime history is separate from task creation and editing. History is stored per task and field under `.taskrunner/history/`; each field keeps its 100 newest unique values.

## CLI

```text
taskrunner
taskrunner create
taskrunner edit <task>
taskrunner run <task> [--set <key>=<value>]...
```

Show command help:

```sh
taskrunner --help
```

Open the task editor directly:

```sh
taskrunner create
taskrunner edit "Deploy"
```

Run a task without opening the TUI:

```sh
taskrunner run "Deploy" \
  --set environment=staging \
  --set version=1.4.0 \
  --set confirmed=true
```

Task names match case-insensitively by display name, JSON filename, or filename without `.json`.

Direct runs require every required field:

- Optional text, choice, and file fields default to an empty value.
- Optional confirmations default to `false`.
- Required confirmations must be set to `true`.
- Choice values must match a configured option.
- Unknown field keys are rejected.

Quote `key=value` when the value contains whitespace or shell characters:

```sh
taskrunner run "Release" --set 'note=release candidate'
```

## Task files

Taskrunner stores one JSON file per task:

```text
<workspace>/.taskrunner/tasks/<task-name>.json
```

Example:

```json
{
  "name": "Deploy",
  "command": "deploy --environment {{environment}} --version {{version}}",
  "fields": [
    {
      "key": "environment",
      "label": "Environment",
      "type": "choice",
      "options": [
        "staging",
        "production"
      ]
    },
    {
      "key": "version",
      "label": "Version",
      "type": "text"
    },
    {
      "key": "confirmed",
      "label": "Deploy {{version}} to {{environment}}?",
      "type": "confirm"
    }
  ]
}
```

Fields are evaluated in JSON order. Tasks without fields run immediately.

### Field properties

| Property | Required | Description |
| --- | --- | --- |
| `key` | Yes | Unique placeholder key. Starts with a letter and contains only letters, numbers, or underscores. |
| `label` | Yes | Prompt displayed at runtime. |
| `type` | Yes | `text`, `choice`, `file`, or `confirm`. |
| `options` | For `choice` | Allowed choice values. |
| `optional` | No | Allows the field to be skipped. Defaults to `false`. |
| `raw` | No | Inserts the value as shell syntax instead of quoting it. Defaults to `false`. |

## Shell interpolation

Taskrunner shell-quotes field values before replacing command placeholders. Spaces, quotes, command substitutions, redirects, pipes, and other shell syntax remain part of one argument.

A raw field bypasses quoting:

```json
{
  "key": "flags",
  "label": "Trusted flags",
  "type": "text",
  "raw": true
}
```

Raw values execute as shell syntax. Use raw mode only with trusted task files and input. The TUI marks raw fields and displays a warning before execution.

## Workspace data

| Path | Contents |
| --- | --- |
| `.taskrunner/tasks/` | Task definitions |
| `.taskrunner/history/` | Runtime text-field history |

Taskrunner generates task filenames and rejects paths outside `.taskrunner/tasks/`. It writes task and history files through temporary files and atomic renames. History files use owner-only permissions.

To share a task, copy its JSON file from `.taskrunner/tasks/` into the same directory in another workspace. Taskrunner validates every task when loading it and reports the filename of malformed input.

## Development

```sh
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

Run TUI changes in a PTY. Check narrow and short terminals as well as wide and tall terminals.

See [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a change.

## License

Taskrunner is available under the [MIT License](LICENSE). Dependencies retain their own licenses; source and binary distributions must include [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
