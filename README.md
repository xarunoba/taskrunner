# Taskrunner

Taskrunner is a workspace-local shell task runner with a responsive terminal UI and a direct CLI. Tasks are readable JSON files, so they can be reviewed, copied, and shared without a database or service.

The TUI is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Features

- Workspace-local tasks stored under `.taskrunner/tasks/`
- Text, choice, file, and confirmation fields
- Required and optional fields
- Safe shell argument interpolation by default
- Explicit raw interpolation for trusted shell syntax
- Keyboard and mouse navigation
- Responsive layouts for narrow, wide, short, and tall terminals
- Direct CLI commands for creating, editing, and running tasks
- Repeatable `--set key=value` field pre-seeding

## Build

Taskrunner requires Go 1.27 or newer.

```sh
go build -o taskrunner .
```

Run the binary from the workspace whose tasks you want to manage:

```sh
./taskrunner
```

Taskrunner treats its current working directory as the workspace.

## TUI

Start the task list:

```sh
./taskrunner
```

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
| Mouse wheel | Move through the active component |

### Task editor

| Input | Action |
| --- | --- |
| `↑` / `↓` | Move between Name, Command, and Fields |
| `Tab` / `Shift+Tab` | Move forward or backward between sections |
| `a` | Add a field |
| `e` or `Enter` | Edit the selected field |
| `d` | Delete the selected field |
| `Ctrl+S` | Save the task |
| `Esc` | Cancel |

Field configuration supports:

- **Type:** text, choice, file, or confirmation
- **Requirement:** required or optional
- **Interpolation:** safely quoted argument or raw shell syntax

### Runtime form

- `Enter` accepts the current value.
- `Shift+Tab` or `Ctrl+←` returns to the previous field.
- `↑` returns from a text field to the previous field.
- Previously entered values are preserved when moving backward.
- Optional text fields can be skipped with an empty value.
- Optional choices include a **Skip** entry.
- Optional file fields can be skipped with `s`.
- An optional confirmation continues with `false` when **No** is selected.

## CLI

Show command help:

```sh
./taskrunner --help
```

Open task creation directly:

```sh
./taskrunner create
```

Open an existing task in the editor:

```sh
./taskrunner edit "Deploy"
```

Run a task without opening the TUI:

```sh
./taskrunner run "Deploy" \
  --set environment=staging \
  --set version=1.4.0
```

Task names are matched case-insensitively by display name, JSON filename, or filename without `.json`.

For direct CLI execution:

- Every required field must be supplied with `--set`.
- Optional text, choice, and file fields default to an empty value.
- Optional confirmations default to `false`.
- Choice values must match one of the field's configured options.
- Required confirmations must be set to `true`.
- Unknown field keys are rejected.

Quote `key=value` when the value contains shell whitespace or characters interpreted by your shell:

```sh
./taskrunner run "Release" --set 'note=release candidate'
```

## Task files

Each task is stored as a separate JSON file:

```text
<workspace>/.taskrunner/tasks/<task-name>.json
```

Example:

```json
{
  "name": "Deploy",
  "command": "deploy --environment {{environment}} --config {{config}} {{extra}}",
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
      "key": "config",
      "label": "Configuration file",
      "type": "file"
    },
    {
      "key": "extra",
      "label": "Additional trusted arguments",
      "type": "text",
      "optional": true,
      "raw": true
    }
  ]
}
```

Field keys become command placeholders using `{{key}}` syntax. Fields are evaluated in their JSON order.

Tasks with no fields run immediately.

### Field properties

| Property | Required | Meaning |
| --- | --- | --- |
| `key` | Yes | Unique placeholder key. Must begin with a letter and contain only letters, numbers, or underscores. |
| `label` | Yes | Prompt shown to the user. |
| `type` | Yes | `text`, `choice`, `file`, or `confirm`. |
| `options` | Choice fields | Allowed choice values. |
| `optional` | No | Allows the field to be skipped. Defaults to `false`. |
| `raw` | No | Disables shell quoting for this field. Defaults to `false`. |

## Shell interpolation and security

Taskrunner shell-quotes normal field values before replacing placeholders. Spaces, quotes, command substitutions, redirects, pipes, and other shell syntax remain part of one argument.

Raw fields bypass quoting:

```json
{
  "key": "flags",
  "label": "Trusted flags",
  "type": "text",
  "raw": true
}
```

Raw values execute as shell syntax. Use raw mode only for trusted task files and trusted input. The TUI marks raw fields and displays a runtime warning.

Commands run through `$SHELL -c` with the workspace as the command directory. If `$SHELL` is empty, Taskrunner uses `/bin/sh`.

## Sharing tasks

To share tasks, copy the relevant JSON files from `.taskrunner/tasks/`. A recipient can place them in the same directory under another workspace.

Taskrunner validates task files when loading them. A malformed task prevents startup and reports the affected filename.

## Development

Format and verify changes with:

```sh
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

For TUI behavior or layout changes, also run the binary in a PTY and check narrow/short and wide/tall terminal sizes.

## License

Taskrunner is licensed under the [MIT License](LICENSE).

Dependencies remain under their own licenses. Source and binary distributions must include [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES). Contributions, including AI-assisted contributions, are subject to the provenance requirements in [CONTRIBUTING.md](CONTRIBUTING.md).
