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

Run `taskrunner` without arguments to open the **Tasks** tab.

### Tasks

| Input | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select a task |
| `Enter` | Run the selected task |
| `Tab` / `Shift+Tab` | Switch between the **Tasks** and **Jobs** tabs |
| `n` | Create a task |
| `e` | Edit the selected task |
| `d` | Delete the selected task |
| `q` | Quit |
| Mouse click | Select a task or control |
| Mouse wheel | Move through the active control |

### Task editor

| Input | Action |
| --- | --- |
| `↑` / `↓` | Move between sections; move between lines in the command editor |
| `Enter` | Insert a new line in the command editor; advance or edit elsewhere |
| `Tab` / `Shift+Tab` | Move forward or backward between sections |
| `←` / `→` or `h` / `l` | Select a job policy while that section is active |
| `a` | Add a field |
| `e` or `Enter` | Edit the selected field |
| `d` | Delete the selected field |
| `[` / `]` | Move the selected field earlier or later |
| `F2` or `Ctrl+S` | Save the task |
| `Esc` | Cancel |

The task name label stays above its input. The complete task form uses one scrolling viewport with a position bar, and automatically keeps the active input or field visible. Job logs use the same position bar.

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

### Jobs and logs

Taskrunner starts a workspace daemon on demand. The daemon owns TUI and CLI jobs, so a job continues after the TUI closes. It exits after 10 seconds with no clients and no queued or running jobs.

Different tasks can run concurrently. Each task has one of three job policies:

- **Sequential** queues a new job until older jobs for the same task finish. This is the default.
- **Parallel** starts every job immediately.
- **Cancel previous** cancels queued and running older jobs for the same task, then starts the new job.

The **Tasks** tab marks running and queued jobs. When a task is idle, it shows the latest job as **succeeded**, **failed**, or **canceled**. The **Jobs** tab lists every job with its status, command, timestamps, and combined standard output and standard error log.

| Input | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select a job |
| `Enter` | Open the selected job log |
| `Tab` / `Shift+Tab` | Switch between the **Tasks** and **Jobs** tabs |
| `c` | Cancel the selected queued or running job |
| `r` | Rerun the selected job as a new job |
| `d` | Delete the selected completed job and its output |
| `q` | Quit |
| Mouse click | Select a job or control; click a selected job again to open its log |

Running logs update live. The log viewport uses all available panel height. Completed logs remain available after the daemon exits and restarts.

| Input | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Scroll one line |
| `Page Up` / `Page Down` | Scroll one page |
| `Home` or `g` | Jump to the top |
| `End` or `G` | Jump to the bottom |
| `c` | Cancel this queued or running job |
| `r` | Rerun this job as a new job |
| `d` | Delete this completed job and its output |
| `Esc` | Return to the **Jobs** tab |
| Mouse wheel | Scroll the log |

## CLI

The CLI uses Cobra for command parsing, contextual help, argument validation, and shell completion.

```text
taskrunner
taskrunner create
taskrunner edit <task>
taskrunner run <task> [--set <key>=<value>]... [--detach|--dry-run]
taskrunner tasks [--json]
taskrunner task show <task> [--json]
taskrunner task validate [task]
taskrunner task rm <task> [--force]
taskrunner jobs [--all] [--status <status>] [--task <task>] [--limit <count>] [--json|--quiet]
taskrunner job logs <job> [--follow] [--tail <lines>]
taskrunner job wait <job>
taskrunner job inspect <job> [--json]
taskrunner job cancel <job>
taskrunner job rerun <job>
taskrunner job rm <job>
taskrunner job prune [--status <status>] [--before <age>] [--force]
taskrunner completion <bash|zsh|fish>
```

Show command help:

```sh
taskrunner --help
```

Command categories and subcommands provide contextual help:

```sh
taskrunner task
taskrunner job
taskrunner jobs --help
taskrunner job logs --help
taskrunner help job prune
```

Category help lists its commands. Command help lists that command's options.

Open the task editor directly:

```sh
taskrunner create
taskrunner edit "Deploy"
```

Saving or canceling a directly opened create or edit form returns to the shell instead of opening the main TUI.

### Run tasks

Run a task and stream its combined standard output and standard error:

```sh
taskrunner run "Deploy" \
  --set environment=staging \
  --set version=1.4.0 \
  --set confirmed=true
```

Use `--detach` to print the new job ID and return immediately:

```sh
job_id=$(taskrunner run "Deploy" --detach --set environment=staging)
taskrunner job wait "$job_id"
```

Use `--dry-run` to validate the values and print the safely rendered command without creating a job. `--quiet` removes the `Command:` label for scripts:

```sh
taskrunner run "Deploy" --dry-run --quiet --set environment=staging
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

### Manage tasks

`taskrunner tasks` prints task names, files, field counts, policies, and command templates. `--json` emits a JSON array.

`taskrunner task show` prints the persisted definition. Its default output is indented JSON; `--json` emits compact JSON. `taskrunner task validate` validates one task or every task and produces no output on success.

`taskrunner task rm` removes only the task definition. It does not remove job history. Interactive removal requires confirmation; scripts must pass `--force`.

### Manage jobs

`taskrunner jobs` lists queued and running jobs, newest first. Options:

- `--all` or `-a` includes completed jobs.
- `--status` selects `queued`, `running`, `succeeded`, `failed`, or `canceled`.
- `--task` matches a task display name or JSON filename.
- `--limit` limits the number of matching jobs.
- `--json` emits a JSON array.
- `--quiet` prints one full job ID per line.

Job arguments accept the full ID or a unique ID prefix or suffix. The short ID printed by the table is a supported suffix.

`taskrunner job logs` prints combined standard output and standard error. `--tail` selects the final number of lines. `--follow` or `-f` follows new output and exits with the job result.

`taskrunner job wait` waits without printing logs and exits with the job result. `taskrunner job inspect` prints the full ID, task, status, policy, command, timestamps, output size, and errors. Use `--json` for machine-readable output.

`taskrunner job cancel` accepts queued or running jobs. `taskrunner job rerun` starts a new job with the original command and policy. `taskrunner job rm` removes a completed job and its output; active jobs must be canceled first.

`taskrunner job prune` removes matching completed jobs. `--status` limits removal to one completed status. `--before` accepts durations such as `24h` or `7d`. Interactive pruning requires confirmation; scripts must pass `--force`.

### Shell completion

Generate a Cobra completion script for Bash, Zsh, or Fish. Each script includes the current commands and flags:

```sh
taskrunner completion bash
taskrunner completion zsh
taskrunner completion fish
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
  "job_policy": "parallel",
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

Fields are evaluated in JSON order. Tasks without fields run immediately. `job_policy` is optional and accepts `"parallel"` or `"cancel_previous"`; omission means sequential.

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
| `.taskrunner/runs/` | One persisted JSON record and log per job |
| `.taskrunner/daemon.sock` | Active daemon IPC socket |
| `.taskrunner/daemon.lock` | Workspace daemon lock |

Taskrunner generates task filenames and rejects paths outside `.taskrunner/tasks/`. It writes task, value-history, and job files through temporary files and atomic renames. History and job files use owner-only permissions.

To share a task, copy its JSON file from `.taskrunner/tasks/` into the same directory in another workspace. Taskrunner validates every task when loading it and reports the filename of malformed input.

## Development

```sh
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

The executable entry point stays in `main.go`. `internal/cli` owns the Cobra command tree and non-TUI command execution. `internal/tui` owns Bubble Tea interaction and rendering. `internal/task` owns task data and persistence, while `internal/daemon` owns background job execution and history.

Run TUI changes in a PTY. Check narrow and short terminals as well as wide and tall terminals.

See [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a change.

## License

Taskrunner is available under the [MIT License](LICENSE). Dependencies retain their own licenses; source and binary distributions must include [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
