# Taskrunner

Taskrunner saves shell commands as tasks you can run from a terminal UI or the command line. Tasks can prompt for text, choices, files, and confirmations. You can watch job output live, and jobs keep running after you close the terminal.

## Requirements

- Go 1.27.1 or newer
- Linux: the job daemon communicates over a Unix domain socket

Taskrunner executes commands through `$SHELL -c`. It uses `/bin/sh` when `$SHELL` is empty.

## Install

```sh
go install .
```

Or build a binary in the repository:

```sh
go build -o taskrunner .
```

Run Taskrunner from your workspace directory:

```sh
taskrunner
```

## TUI

Run `taskrunner` without arguments to open the **Tasks** tab. Press `?` for keybinds on the current screen. While editing text, use `F1` for keybinds, `F4` for Settings, and `Ctrl+C` to quit.

Footer buttons show their shortcuts: `esc back`, `s settings`, and `? keybinds`. During text entry, Settings and keybinds use `f4` and `f1` so you can type `s` and `?`. Modals use `esc close`; direct create/edit screens use `esc cancel`. Buttons wrap in narrow terminals. Click a button or press its displayed key.

Lists, pickers, editors, help, and logs show a position bar on the right when content extends beyond the visible area. Use the keyboard or mouse wheel to scroll; the bar cannot be dragged. Editors keep the focused control visible as you move forward or backward.

### Settings

| Input | Action |
| --- | --- |
| `s` outside text entry / `F4` / Settings footer button | Open Settings |
| `↑` / `↓` | Move between setting rows |
| `←` / `→`, `h` / `l`, or `Enter` | Cycle the selected row to its previous or next value |
| `Esc` / `s` / **esc close** | Close Settings |
| Mouse | Click a row's label for the previous value or its value for the next |

Changing **Theme** applies it immediately and saves your choice; see [Configuration](#configuration).

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

Use the editor to set the command, job policy, and prompt fields. Field labels can use earlier answers, such as `Deploy {{version}} to {{environment}}?`.

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

In the command editor, `↑` on the first displayed line moves to Fields, and `↓` on the last moves to Job policy. Otherwise, the arrows move the cursor within the command, including wrapped lines. Leaving the editor preserves the command and cursor position.

Fields run in their displayed order and can be required or optional. Field values are shell-quoted before substitution, so spaces and special characters stay part of one argument.

### Running a task

| Input | Action |
| --- | --- |
| `Enter` | Accept the current value |
| `↑` / `↓` | Browse previous values for the active text field |
| `Shift+Tab` or `Ctrl+←` | Return to the previous field |
| `Esc` | Cancel the run |
| Mouse | Select a file or control; click **Skip** for an optional file field |

Moving backward preserves the values you entered. Taskrunner also saves recent values for each field so you can reuse them on later runs.

### Jobs and logs

Taskrunner starts a workspace daemon when needed, so jobs continue after the TUI closes. Each task has one of three job policies:

- **Sequential** (default): Queue new jobs until older ones finish.
- **Parallel**: Start every job immediately.
- **Cancel previous**: Cancel older jobs, then start the new one.

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

Use `--help` or `taskrunner help <command>` for command help.

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
job_id=$(taskrunner run "Deploy" --detach \
  --set environment=staging \
  --set version=1.4.0 \
  --set confirmed=true)
taskrunner job wait "$job_id"
```

Use `--dry-run` to validate values and print the rendered command without creating a job.

Select a task by name or filename; matching ignores case. Direct runs need a value for every required field. Choice values must match a configured option, and unknown field keys are rejected.

### Manage tasks and jobs

- `taskrunner tasks` lists tasks; `taskrunner task show` prints one task; `taskrunner task validate` checks one task or every task; `taskrunner task rm` removes a task.
- `taskrunner jobs` lists jobs, newest first. Filter with `--status` and `--task`, cap the count with `--limit`, and choose the output format with `--json` or `--quiet`.
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

Tasks live in `.taskrunner/tasks/`, one JSON file per task. Use `taskrunner create` and `taskrunner edit` to open the editor, or edit the JSON directly. `taskrunner task show <task>` prints the saved definition. Each task has a command with `{{placeholders}}` and fields that supply their values:

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

Field values are shell-quoted by default, so shell syntax in a value stays literal. Set `"raw": true` on a field only when you intend to insert trusted shell syntax. The TUI marks raw fields and warns before execution.

## Configuration

Display preferences apply across your workspaces. Taskrunner reads them from `$XDG_CONFIG_HOME/taskrunner/`, or `~/.config/taskrunner/` when `XDG_CONFIG_HOME` is unset. Opening the TUI creates `config.json` with the default theme if the file does not exist; an existing file is never touched at startup, and a malformed one is reported as an error. Without a configuration file, the TUI uses the built-in `default` theme with green accents.

`config.json` selects the theme:

```json
{ "theme": "violet" }
```

`themes/default.json` overrides the built-in `default` theme: name only the roles you want to change; unnamed roles keep their built-in values. Unknown color names are rejected.

Save custom themes as `themes/<name>.json` in the configuration directory. The filename sets the theme name and may contain only letters, digits, `_`, and `-` before `.json`. Include every color entry shown below. Each entry takes either a single color or a `foreground` and `background` pair. Colors can be ANSI 256 numbers or hex values:

```json
{
  "colors": {
    "accent": "63",
    "muted": "241",
    "cursor": "212",
    "error": "196",
    "selection": { "foreground": "230", "background": "57" },
    "inactive_tab": { "foreground": "250", "background": "236" },
    "status": { "foreground": "250", "background": "236" },
    "error_badge": { "foreground": "230", "background": "196" },
    "queued": { "foreground": "230", "background": "63" },
    "succeeded": { "foreground": "0", "background": "42" },
    "canceled": { "foreground": "255", "background": "241" }
  }
}
```

Choosing a theme in Settings updates only the `theme` key in `config.json`; other top-level keys stay unchanged. If `config.json` is malformed, Taskrunner reports the error at startup and leaves the file untouched. Settings lists `default` first, then valid custom themes sorted by name.

## Development

```sh
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

After changing dependencies, update `THIRD_PARTY_NOTICES` by hand with the new component's version and copyright lines. CI checks dependency licenses with [go-licenses](https://github.com/google/go-licenses).

Run TUI changes in a PTY. Check narrow and short terminals as well as wide and tall terminals.

See [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a change.

## License

Taskrunner is available under the [MIT License](LICENSE). Dependencies retain their own licenses; source and binary distributions must include [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES).
