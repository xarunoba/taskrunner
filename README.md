# Taskrunner

Taskrunner runs workspace-specific shell commands from a terminal UI or direct CLI commands. Each task is a readable JSON file under `.taskrunner/tasks/`.

## Requirements

- Go 1.27 or newer

Taskrunner executes commands through `$SHELL -c`. It uses `/bin/sh` when `$SHELL` is empty.

## Install

```sh
go install .
```

Or build a binary in the repository:

```sh
go build -o taskrunner .
```

Run Taskrunner from the workspace whose tasks you want to use. The current working directory is the workspace:

```sh
taskrunner
```

## TUI

Run `taskrunner` without arguments to open the **Tasks** tab. Press `?` for keybinds on the current screen; while editing text, use `F1` for help and `Ctrl+C` to quit.

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
| Mouse | Select a task or control; wheel moves through the active control |

### Task editor

| Input | Action |
| --- | --- |
| `↑` / `↓` | Move between sections; move between lines in the command editor |
| `Enter` | Insert a new line in the command editor; advance or edit elsewhere |
| `Tab` / `Shift+Tab` | Move forward or backward between sections |
| `←` / `→` or `h` / `l` | Select a job policy while that section is active |
| `a` / `e` / `d` | Add, edit, or delete the selected field |
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

Taskrunner records non-empty text values when a task starts, per task and field under `.taskrunner/history/`; each field keeps its 100 newest unique values.

### Jobs and logs

Taskrunner starts a workspace daemon on demand, so a job continues after the TUI closes. The daemon exits after 10 seconds with no clients and no queued or running jobs.

Each task has one of three job policies:

- **Sequential** queues a new job until older jobs for the same task finish. This is the default.
- **Parallel** starts every job immediately.
- **Cancel previous** cancels queued and running older jobs for the same task, then starts the new job.

The **Tasks** tab marks running and queued jobs and shows the latest result for idle tasks. The **Jobs** tab lists every job with its status, command, timestamps, and combined output and error log.

| Input | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Select a job |
| `Enter` | Open the selected job log |
| `Tab` / `Shift+Tab` | Switch between the **Tasks** and **Jobs** tabs |
| `c` / `r` / `d` | Cancel, rerun, or delete the selected job |
| `q` | Quit |
| Mouse | Select a job or control; click a selected job again to open its log |

Logs update live and show elapsed execution time. Completed logs remain available after the daemon exits and restarts.

| Input | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Scroll one line |
| `Page Up` / `Page Down` | Scroll one page |
| `Home` or `g` / `End` or `G` | Jump to the top or bottom |
| `c` / `r` / `d` | Cancel, rerun, or delete this job |
| `Esc` | Return to the **Jobs** tab |
| Mouse wheel | Scroll the log |

## CLI

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

Every command provides contextual help via `--help` or `taskrunner help <command>`.

`taskrunner create` and `taskrunner edit <task>` open the task editor directly and return to the shell on save or cancel.

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

Use `--dry-run` to validate values and print the rendered command without creating a job; add `--quiet` to print only the command.

Task names match case-insensitively by display name, JSON filename, or filename without `.json`. Direct runs require every required field: optional fields default to an empty value, optional confirmations to `false`, required confirmations must be `true`, choice values must match a configured option, and unknown keys are rejected.

Quote `key=value` when the value contains whitespace or shell characters:

```sh
taskrunner run "Release" --set 'note=release candidate'
```

### Manage tasks

- `taskrunner tasks` prints task names, files, field counts, policies, and command templates. `--json` emits a JSON array.
- `taskrunner task show` prints the persisted definition; `--json` emits compact JSON.
- `taskrunner task validate` validates one task or every task; no output means success.
- `taskrunner task rm` removes only the task definition. Interactive removal requires confirmation; scripts must pass `--force`.

### Manage jobs

`taskrunner jobs` lists queued and running jobs, newest first. `--all` includes completed jobs; `--status`, `--task`, and `--limit` filter matches; `--json` emits a JSON array; `--quiet` prints one full job ID per line.

Job arguments accept a full ID or a unique prefix or suffix, including the short ID printed by the table.

- `taskrunner job logs` prints combined output; `--tail` selects the final lines, `--follow` (`-f`) streams until the job finishes and exits with its result.
- `taskrunner job wait` exits with the job result without printing logs.
- `taskrunner job inspect` prints full details; `--json` for machine-readable output.
- `taskrunner job cancel` accepts queued or running jobs.
- `taskrunner job rerun` starts a new job with the original command and policy.
- `taskrunner job rm` removes a completed job and its output; active jobs must be canceled first.
- `taskrunner job prune` removes matching completed jobs; `--status` limits to one completed status, `--before` accepts durations such as `24h` or `7d`. Interactive pruning requires confirmation; scripts must pass `--force`.

### Shell completion

Generate a completion script for Bash, Zsh, or Fish:

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

| Property | Required | Description |
| --- | --- | --- |
| `key` | Yes | Unique placeholder key. Starts with a letter and contains only letters, numbers, or underscores. |
| `label` | Yes | Prompt displayed at runtime. |
| `type` | Yes | `text`, `choice`, `file`, or `confirm`. |
| `options` | For `choice` | Allowed choice values. |
| `optional` | No | Allows the field to be skipped. Defaults to `false`. |
| `raw` | No | Inserts the value as shell syntax instead of quoting it. Defaults to `false`. |

Taskrunner shell-quotes field values before replacing command placeholders, so spaces, quotes, pipes, and substitutions remain part of one argument. A `raw: true` field bypasses quoting and executes as shell syntax; use it only with trusted task files and input. The TUI marks raw fields and displays a warning before execution.

## Workspace data

| Path | Contents |
| --- | --- |
| `.taskrunner/tasks/` | Task definitions |
| `.taskrunner/history/` | Runtime text-field history |
| `.taskrunner/runs/` | One persisted JSON record and log per job |
| `.taskrunner/daemon.sock` | Active daemon IPC socket |
| `.taskrunner/daemon.lock` | Workspace daemon lock |

Taskrunner generates task filenames, rejects paths outside `.taskrunner/tasks/`, and writes all files through temporary files and atomic renames. History and job files use owner-only permissions.

To share a task, copy its JSON file into `.taskrunner/tasks/` in another workspace. Taskrunner validates every task when loading it and reports the filename of malformed input.

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
