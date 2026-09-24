# Taskrunner

Save shell commands as tasks with prompts, run them from a terminal UI or CLI, and watch job output. Jobs keep running after you close the terminal.

## Install

Requires Linux and Go 1.27.1 or newer. From the repository root:

```sh
go install .
```

## Usage

Run from your workspace directory:

```sh
taskrunner
```

Press `?` for keybinds (`F1` while editing text). For CLI commands and options:

```sh
taskrunner --help
taskrunner <command> --help
```

Commands run through `$SHELL -c`, falling back to `/bin/sh`. Field values are shell-quoted by default; enable raw mode only for trusted shell syntax.

## Files

- Tasks: `.taskrunner/tasks/`, one JSON file per task. Copy these files to share tasks between workspaces.
- Runtime history and daemon state: `.taskrunner/` in each workspace.
- Preferences: `$XDG_CONFIG_HOME/taskrunner/`, falling back to `~/.config/taskrunner/`. The TUI creates `config.json` and `themes/default.json` when missing. Choose a theme in Settings; copy `themes/default.json` to `themes/<name>.json` and edit its colors to create a custom theme.

For development, run `gofmt -w .`, `go test ./...`, `go vet ./...`, and `go build ./...`. See [CONTRIBUTING.md](CONTRIBUTING.md) before submitting changes.

[MIT License](LICENSE). Include [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES) with source and binary distributions.
