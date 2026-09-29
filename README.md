# Taskrunner

Taskrunner is a Linux terminal UI and CLI for saved shell commands. Define input prompts, reuse the task, and inspect its job output. A local daemon keeps jobs running after the terminal closes.

## Installation

Requires Linux and Go 1.27.1 or newer. Install or update to the latest release:

```sh
go install github.com/xarunoba/taskrunner@latest
```

To install a specific release:

```sh
go install github.com/xarunoba/taskrunner@v0.1.0
```

## Versions

Releases use SemVer tags. During 0.x development, incompatible changes to the CLI, task JSON, or persisted job records increment the minor version; compatible fixes increment the patch version. Version 1.0.0 will mark the stable public contract.

## Usage

Run from the directory whose tasks you want to use:

```sh
taskrunner
```

Press `?` for keybinds and the application version (`F1` while editing text). When files cannot be loaded, this button is labeled `warnings`; open it for the file paths and reasons. For the CLI version, commands, and options:

```sh
taskrunner --version
taskrunner --help
taskrunner run --help
```

Task references prefer a byte-exact filename, then a case-insensitive filename, task name, or filename without `.json`. Multiple matches at a fallback step are an error listing the candidate filenames. Use an exact filename to distinguish tasks with the same name or filenames differing only by case.

### Example task

Create `.taskrunner/tasks/find-files.json` in your project, including the parent directories:

```json
{
  "name": "Find files",
  "command": "find . -type f -name {{pattern}}",
  "fields": [
    {"key": "pattern", "label": "File name pattern", "type": "text"}
  ]
}
```

Open `taskrunner` to select the task and enter a pattern, or run it directly:

```sh
taskrunner run find-files --set 'pattern=*.go'
```

This lists Go files under the current directory. Taskrunner quotes the pattern so the shell passes it to `find` without expanding it first.

### Commands and output

| Command | Purpose |
| --- | --- |
| `create`, `edit <task>` | Open the task editor |
| `tasks [--json]` | List valid task definitions |
| `task show <task> [--json]`, `task validate [task]` | Inspect or validate definitions |
| `task rm <task> [--force]` | Remove a definition |
| `run <task> --set key=value` | Run with supplied field values; repeat `--set` |
| `jobs [--all] [--json]` | List active jobs, or include completed jobs |
| `job logs <job> [--follow] [--tail N]` | Read retained output |
| `job inspect <job> [--json]`, `job wait <job>` | Inspect a job or wait for completion |
| `job cancel <job>`, `job rerun <job>`, `job rm <job>`, `job prune` | Manage jobs and saved history |

Job references accept full IDs or unique ID prefixes or suffixes. `run --dry-run --quiet` prints the rendered command without execution; `run --detach` prints the job ID and returns. Each command's `--help` lists its flags.

JSON commands write data to stdout and warnings to stderr. List commands return arrays, including `[]` when empty; inspect/show commands return objects. Task objects include the definition and its `file`. Job objects include the ID, task, command, status, timestamps, output offsets, and any errors; they do not expose the saved environment. Job statuses are `queued`, `running`, `succeeded`, `failed`, and `canceled`. Consumers should allow additional JSON properties.

Successful CLI operations exit with 0; errors exit with 1. Attached `run`, `job wait`, and `job logs --follow` also exit with 1 for failed or canceled jobs. The shell's failure status is reported in the error text, not forwarded as the CLI exit code.

### Task format

Each task requires `name` and `command`. The optional `fields` array is ordered; each field requires a unique `key`, a `label`, and a `type`:

| Type | Value and additional properties |
| --- | --- |
| `text` | A text value; the TUI offers previously entered values |
| `choice` | One of the strings in the required, non-empty `options` array |
| `file` | A path; the TUI provides a file picker |
| `confirm` | A boolean confirmation; required confirmations must be true |
| `refer` | Mirrors the earlier field named by `from`; never prompted or set directly |

Fields are required unless `"optional": true`. Omitted optional values are empty; omitted optional confirmations are false. An absent value removes its placeholder and its affixes. `prefix` and `suffix` add literal text around a present value. Confirmations insert only their affixes, never the boolean itself; a reference to a confirmation behaves the same way. Fields absent from the command still gate execution. `"raw": true` disables value quoting and produces a runtime warning; use it only for trusted shell syntax.

`job_policy` controls jobs for the same task file: omitted or `""` queues them sequentially, `"parallel"` allows concurrent jobs, and `"cancel_previous"` cancels earlier queued/running jobs before starting the new one.

## Storage

The current working directory is your workspace. Taskrunner keeps its tasks and runtime data under `.taskrunner/`:

| Path | Contents |
| --- | --- |
| `.taskrunner/tasks/` | Task definitions, one readable JSON file per task |
| `.taskrunner/history/` | Recent input values for each task |
| `.taskrunner/runs/` | Saved job records and output |
| `.taskrunner/daemon.sock` | Workspace daemon's Unix socket |
| `.taskrunner/daemon.lock` | Workspace daemon's lock file |
| `.taskrunner/.gitignore` | Generated ignore file; shares `tasks/` and the ignore file |

Create and edit tasks in the application, or edit their JSON directly. Copy files from `.taskrunner/tasks/` to another workspace to share tasks. Saving a task or starting the daemon creates `.taskrunner/.gitignore` when missing; listing and validating tasks do not change the workspace. Existing ignore files are preserved. The generated rules exclude saved inputs, environments, output, and daemon state, which may contain sensitive data. They do not remove files already tracked by Git.

### Output retention and recovery

Each job retains at most the last **1 MiB of output bytes**, ending and starting on complete UTF-8 boundaries where possible. Earlier output is discarded. CLI output readers warn on stderr when bytes were missed; the TUI log shows a truncation notice. JSON `output_size` is the absolute byte cursor, including discarded bytes; `output_start` is the first retained byte's offset and is omitted when zero. Invalid UTF-8 is displayed with replacement characters.

The limit also applies to loaded job history. Redirect commands to your own files when full logs are required. Retention is per job; use `job rm` or `job prune` to reduce the number of saved jobs.

Invalid task files and corrupt job records are skipped with warnings, leaving valid tasks and jobs usable. Files are preserved for repair, not deleted. `task validate` fails if any task file is invalid; a named validation checks that task. Remove an invalid task by its exact filename, for example `task rm broken.json --force`.

Job records larger than **8 MiB** are preserved but not loaded. Move such a record outside `runs/` for inspection, or reduce its output before restoring it. Directory access failures and symlinked storage directories still stop the operation. After repairing files, reopen the TUI; for job records, first let all jobs finish and close workspace clients so the daemon can exit after about 10 seconds of inactivity.

## Configuration

Display preferences apply across workspaces. They live in `$XDG_CONFIG_HOME/taskrunner/`, falling back to `~/.config/taskrunner/`:

| Path | Purpose |
| --- | --- |
| `config.json` | Selected theme |
| `themes/default.json` | Colors for the default theme |
| `themes/<name>.json` | Custom themes |

The default theme uses near-black text on mint highlights. A true-color terminal displays its exact colors.

Opening the TUI creates the configuration and default theme files when missing. Choose a theme in Settings to apply and save it. Direct CLI commands do not read display preferences.

At TUI startup, Taskrunner refreshes outdated generated default themes and recognized palettes from older releases, then loads the selected theme. To preserve custom colors across these updates, copy `themes/default.json` to `themes/<name>.json`, edit the copy, and select it in Settings.

Theme names accept letters, digits, `_`, and `-`. Keep every color role and its existing structure: either a single color string or a `foreground`/`background` pair. Use quoted ANSI 256 numbers (such as `"48"`) or hex colors (such as `"#00ff87"`).

## Security

Tasks execute shell commands through `$SHELL -c`, falling back to `/bin/sh`. Review shared task definitions before running them.

Argument-mode fields use POSIX/Bash template syntax. Placeholders can appear in shell arguments, quoted strings, and heredoc bodies. Values are quoted for their syntax context and are never rescanned for placeholders. Placeholders in shell expressions or identifiers are rejected because argument quoting cannot make those positions safe. Raw mode inserts trusted shell syntax without quoting; enable it only for trusted input.

Jobs run with the invoking process's environment, including an empty environment. Job records save that environment in owner-only files for reruns; it may contain secrets. On restart, the daemon kills surviving job process groups whose leader PID and start time match the saved record, then marks interrupted jobs failed. A crash before that identity is saved can leave a job running. Taskrunner rejects symlinked storage directories and value-history files.

## Development

The [CI workflow](.github/workflows/ci.yml) checks formatting, runs `go vet` and race tests, and builds the project. The [license workflow](.github/workflows/license.yml) checks dependency licenses when `go.mod` or `go.sum` changes.

Run these checks from the repository root:

```sh
gofmt -w .
go test -race ./...
go vet ./...
go build ./...
```

For TUI changes, also exercise the binary in narrow/short and wide/tall terminals. Update `THIRD_PARTY_NOTICES` when changing dependencies.

AI tools are used to generate and revise code and documentation in this project. Maintainers are responsible for reviewing contributions and checking their licensing. See [CONTRIBUTING.md](CONTRIBUTING.md) for review, verification, and disclosure requirements.

## License

Taskrunner is licensed under the [MIT License](LICENSE). Dependencies retain their own licenses. Include [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES) with source and binary distributions.
