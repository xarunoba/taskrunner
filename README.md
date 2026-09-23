# Taskrunner

Taskrunner runs workspace-specific shell commands from a terminal UI or direct CLI commands. Create tasks once with prompts (text, choice, file picker, confirmation), then run them interactively or from scripts, with live logs and a background daemon so jobs keep running after you close the terminal.

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

Run `taskrunner` without arguments to open the **Tasks** tab. Press `?` for keybinds on the current screen; while editing text, use `F1` for help and `Ctrl+C` to quit. The footer shows **Back** outside each tab's home screen. Click **Back** to return or **? keybinds** to toggle help.

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
| Mouse | Select a task or control; click the selected task to run it; wheel moves through the active control |

### Task editor

The editor walks you through the command, an optional job policy, and any prompt fields; fields can reference earlier answers (for example a confirmation label like `Deploy {{version}} to {{environment}}?`).

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

Fields run in their displayed order and can be required or optional. Field values are shell-quoted before substitution, so spaces and special characters stay part of one argument.

### Running a task

| Input | Action |
| --- | --- |
| `Enter` | Accept the current value |
| `↑` / `↓` | Browse previous values for the active text field |
| `Shift+Tab` or `Ctrl+←` | Return to the previous field |
| `s` | Skip an optional file field |
| `Esc` | Cancel the run |

Moving backward preserves values already entered. Recent values per field are remembered, so repeated runs pick up where you left off.

### Jobs and logs

Taskrunner starts a workspace daemon on demand, so a job continues after the TUI closes. Each task has one of three job policies: **Sequential** (default; queues new jobs until older ones finish), **Parallel** (starts every job immediately), or **Cancel previous** (cancels older jobs, then starts the new one).

The **Tasks** tab marks running and queued jobs and shows the latest result for idle tasks. The **Jobs** tab lists every job with its status, command, timestamps, and log.

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

Use `--dry-run` to validate values and print the rendered command without creating a job.

Task names match case-insensitively by display name or filename. Direct runs require every required field; choice values must match a configured option, and unknown keys are rejected.

### Manage tasks and jobs

- `taskrunner tasks` lists tasks; `taskrunner task show` prints one task; `taskrunner task validate` checks one task or every task; `taskrunner task rm` removes a task.
- `taskrunner jobs` lists jobs (newest first) with `--status`, `--task`, `--limit`, and `--json`/`--quiet` filters.
- Job arguments accept a full ID or a unique prefix or suffix, including the short ID printed by the table.
- `taskrunner job logs --follow` streams a job until it finishes; `taskrunner job wait` exits with the job result.
- `taskrunner job cancel`, `rerun`, `rm`, and `prune` manage the job list. Interactive removal and pruning ask for confirmation; scripts pass `--force`.

### Shell completion

Generate a completion script for Bash, Zsh, or Fish:

```sh
taskrunner completion bash
taskrunner completion zsh
taskrunner completion fish
```

## Tasks

Tasks live in `.taskrunner/` inside the workspace, one JSON file per task. You normally never edit them by hand: `taskrunner create` and `taskrunner edit` open a guided editor, and `taskrunner task show <task>` prints the current definition. A task is a command with `{{placeholders}}` plus the fields that fill them in:

```json
{
  "name": "Deploy",
  "command": "deploy --environment {{environment}} --version {{version}}",
  "job_policy": "parallel",
  "fields": [
    { "key": "environment", "label": "Environment", "type": "choice", "options": ["staging", "production"] },
    { "key": "version", "label": "Version", "type": "text" },
    { "key": "confirmed", "label": "Deploy {{version}} to {{environment}}?", "type": "confirm" }
  ]
}
```

To share a task, copy its JSON file into `.taskrunner/tasks/` in another workspace. Every task is validated when loaded, and malformed files are reported by filename.

Field values are shell-quoted before substitution, so task files cannot accidentally inject shell syntax. A field can deliberately opt out for trusted shell snippets; the TUI marks such fields and warns before execution.

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
