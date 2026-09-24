# Taskrunner

Save shell commands as tasks with prompts, run them from a terminal UI or CLI, and watch job output. Jobs keep running after you close the terminal.

## Installation

Requires Linux and Go 1.27.1 or newer. From the repository root:

```sh
go install .
```

Or build a local binary:

```sh
go build -o taskrunner .
```

## Usage

Run from the directory whose tasks you want to use:

```sh
taskrunner
```

Press `?` for keybinds (`F1` while editing text). For CLI commands and options:

```sh
taskrunner --help
taskrunner run --help
```

## Storage

The current working directory is your workspace. Taskrunner keeps its tasks and runtime data under `.taskrunner/`:

| Path | Contents |
| --- | --- |
| `.taskrunner/tasks/` | Task definitions, one readable JSON file per task |
| `.taskrunner/history/` | Recent input values for each task |
| `.taskrunner/runs/` | Saved job records and output |
| `.taskrunner/daemon.sock` | Workspace daemon's Unix socket |
| `.taskrunner/daemon.lock` | Workspace daemon's lock file |

Create and edit tasks in the application, or edit their JSON directly. Copy files from `.taskrunner/tasks/` to another workspace to share tasks. Keep runtime history, job output, and daemon files out of version control; saved inputs and output may contain sensitive data.

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

Field values are shell-quoted by default. Raw mode inserts values as shell syntax without quoting; enable it only for trusted input.

## Development

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
