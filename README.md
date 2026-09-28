# Taskrunner

Save shell commands as tasks with prompts, run them from a terminal UI or CLI, and watch job output. Jobs keep running after you close the terminal.

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

Press `?` for keybinds and the application version (`F1` while editing text). For the CLI version, commands, and options:

```sh
taskrunner --version
taskrunner --help
taskrunner run --help
```

Task references prefer an exact filename, then a case-insensitive filename, task name, or filename without `.json`.

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

AI tools are used in the development of Taskrunner, including to generate code and documentation. Maintainers remain responsible for reviewing contributions and checking their licensing.

Run these checks from the repository root:

```sh
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

For TUI changes, also exercise the binary in narrow/short and wide/tall terminals. Update `THIRD_PARTY_NOTICES` when changing dependencies.

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution and code-provenance requirements.

## License

Taskrunner is licensed under the [MIT License](LICENSE). Dependencies retain their own licenses. Include [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES) with source and binary distributions.
