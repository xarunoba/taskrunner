package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xarunoba/taskrunner/internal/daemon"
)

func parseRunCLI(args []string) (cliOptions, error) {
	if (len(args) == 1 && isHelpArgument(args[0])) || (len(args) == 2 && isHelpArgument(args[1])) {
		return helpOptions("run")
	}
	if len(args) == 0 {
		return cliOptions{}, errors.New("usage: taskrunner run <task> [--set <key>=<value>]... [--detach|--dry-run]")
	}

	options := cliOptions{
		mode:   modeRun,
		task:   args[0],
		values: make(map[string]string),
	}
	for i := 1; i < len(args); i++ {
		argument := args[i]
		switch {
		case argument == "--detach":
			options.detach = true
		case argument == "--dry-run":
			options.dryRun = true
		case argument == "--quiet":
			options.quiet = true
		case argument == "--set":
			i++
			if i == len(args) {
				return cliOptions{}, errors.New("--set requires <key>=<value>")
			}
			if err := addFieldAssignment(options.values, args[i]); err != nil {
				return cliOptions{}, err
			}
		case strings.HasPrefix(argument, "--set="):
			if err := addFieldAssignment(options.values, strings.TrimPrefix(argument, "--set=")); err != nil {
				return cliOptions{}, err
			}
		default:
			return cliOptions{}, fmt.Errorf("unknown run option %q", argument)
		}
	}
	if options.detach && options.dryRun {
		return cliOptions{}, errors.New("--detach and --dry-run cannot be used together")
	}
	if options.quiet && !options.dryRun {
		return cliOptions{}, errors.New("--quiet requires --dry-run")
	}
	return options, nil
}

func addFieldAssignment(values map[string]string, assignment string) error {
	key, value, ok := strings.Cut(assignment, "=")
	if !ok || key == "" {
		return fmt.Errorf("invalid field assignment %q; expected <key>=<value>", assignment)
	}
	values[key] = value
	return nil
}

func parseTasksCLI(args []string) (cliOptions, error) {
	switch {
	case len(args) == 0:
		return cliOptions{mode: modeTasks}, nil
	case len(args) == 1 && isHelpArgument(args[0]):
		return helpOptions("tasks")
	case len(args) == 1 && args[0] == "--json":
		return cliOptions{mode: modeTasks, json: true}, nil
	default:
		return cliOptions{}, errors.New("usage: taskrunner tasks [--json]")
	}
}

func parseTaskCLI(args []string) (cliOptions, error) {
	if len(args) == 0 || isHelpArgument(args[0]) {
		return helpOptions("task")
	}
	if hasHelpArgument(args[1:]) {
		return helpOptions("task " + args[0])
	}
	switch args[0] {
	case "show":
		options := cliOptions{mode: modeTaskShow}
		for _, argument := range args[1:] {
			if argument == "--json" {
				options.json = true
				continue
			}
			if options.task != "" {
				return cliOptions{}, errors.New("usage: taskrunner task show <task> [--json]")
			}
			options.task = argument
		}
		if options.task == "" {
			return cliOptions{}, errors.New("usage: taskrunner task show <task> [--json]")
		}
		return options, nil
	case "validate":
		if len(args) > 2 {
			return cliOptions{}, errors.New("usage: taskrunner task validate [task]")
		}
		options := cliOptions{mode: modeTaskValidate}
		if len(args) == 2 {
			options.task = args[1]
		}
		return options, nil
	case "rm":
		options := cliOptions{mode: modeTaskRemove}
		for _, argument := range args[1:] {
			if argument == "--force" {
				options.force = true
				continue
			}
			if options.task != "" {
				return cliOptions{}, errors.New("usage: taskrunner task rm <task> [--force]")
			}
			options.task = argument
		}
		if options.task == "" {
			return cliOptions{}, errors.New("usage: taskrunner task rm <task> [--force]")
		}
		return options, nil
	default:
		return cliOptions{}, fmt.Errorf("unknown task command %q", args[0])
	}
}

func parseJobsCLI(args []string) (cliOptions, error) {
	if hasHelpArgument(args) {
		return helpOptions("jobs")
	}
	options := cliOptions{mode: modeJobs}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all", "-a":
			options.all = true
		case "--json":
			options.json = true
		case "--quiet":
			options.quiet = true
		case "--status":
			value, err := nextOptionValue(args, &i, "--status")
			if err != nil {
				return cliOptions{}, err
			}
			status, err := parseJobStatus(value)
			if err != nil {
				return cliOptions{}, err
			}
			options.status = status
		case "--task":
			value, err := nextOptionValue(args, &i, "--task")
			if err != nil {
				return cliOptions{}, err
			}
			options.filterTask = value
		case "--limit":
			value, err := nextOptionValue(args, &i, "--limit")
			if err != nil {
				return cliOptions{}, err
			}
			options.limit, err = strconv.Atoi(value)
			if err != nil || options.limit < 1 {
				return cliOptions{}, errors.New("--limit requires a positive integer")
			}
		default:
			return cliOptions{}, fmt.Errorf("unknown jobs option %q", args[i])
		}
	}
	if options.json && options.quiet {
		return cliOptions{}, errors.New("--json and --quiet cannot be used together")
	}
	return options, nil
}

func parseJobCLI(args []string) (cliOptions, error) {
	if len(args) == 0 || isHelpArgument(args[0]) {
		return helpOptions("job")
	}
	if hasHelpArgument(args[1:]) {
		return helpOptions("job " + args[0])
	}

	switch args[0] {
	case "logs":
		options := cliOptions{mode: modeJobLogs, tail: -1}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--follow", "-f":
				options.follow = true
			case "--tail":
				value, err := nextOptionValue(args, &i, "--tail")
				if err != nil {
					return cliOptions{}, err
				}
				options.tail, err = strconv.Atoi(value)
				if err != nil || options.tail < 0 {
					return cliOptions{}, errors.New("--tail requires a non-negative integer")
				}
				options.tailSet = true
			default:
				if options.job != "" {
					return cliOptions{}, errors.New("usage: taskrunner job logs [--follow] [--tail <lines>] <job>")
				}
				options.job = args[i]
			}
		}
		if options.job == "" {
			return cliOptions{}, errors.New("usage: taskrunner job logs [--follow] [--tail <lines>] <job>")
		}
		return options, nil
	case "wait":
		return parseJobReferenceCommand(args, modeJobWait)
	case "inspect":
		options := cliOptions{mode: modeJobInspect}
		for _, argument := range args[1:] {
			if argument == "--json" {
				options.json = true
				continue
			}
			if options.job != "" {
				return cliOptions{}, errors.New("usage: taskrunner job inspect <job> [--json]")
			}
			options.job = argument
		}
		if options.job == "" {
			return cliOptions{}, errors.New("usage: taskrunner job inspect <job> [--json]")
		}
		return options, nil
	case "cancel":
		return parseJobReferenceCommand(args, modeJobCancel)
	case "rerun":
		return parseJobReferenceCommand(args, modeJobRerun)
	case "rm":
		return parseJobReferenceCommand(args, modeJobRemove)
	case "prune":
		options := cliOptions{mode: modeJobPrune}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--force":
				options.force = true
			case "--status":
				value, err := nextOptionValue(args, &i, "--status")
				if err != nil {
					return cliOptions{}, err
				}
				status, err := parseJobStatus(value)
				if err != nil {
					return cliOptions{}, err
				}
				if status == daemon.StatusQueued || status == daemon.StatusRunning {
					return cliOptions{}, errors.New("job prune only accepts completed statuses")
				}
				options.status = status
			case "--before":
				value, err := nextOptionValue(args, &i, "--before")
				if err != nil {
					return cliOptions{}, err
				}
				options.before, err = parseAge(value)
				if err != nil {
					return cliOptions{}, fmt.Errorf("invalid --before value: %w", err)
				}
			default:
				return cliOptions{}, fmt.Errorf("unknown job prune option %q", args[i])
			}
		}
		return options, nil
	default:
		return cliOptions{}, fmt.Errorf("unknown job command %q", args[0])
	}
}

func parseJobReferenceCommand(args []string, mode cliMode) (cliOptions, error) {
	if len(args) != 2 {
		return cliOptions{}, fmt.Errorf("usage: taskrunner job %s <job>", args[0])
	}
	return cliOptions{mode: mode, job: args[1]}, nil
}

func parseCompletionCLI(args []string) (cliOptions, error) {
	if len(args) == 0 || len(args) == 1 && isHelpArgument(args[0]) {
		return helpOptions("completion")
	}
	if len(args) != 1 {
		return cliOptions{}, errors.New("usage: taskrunner completion <bash|zsh|fish>")
	}
	switch args[0] {
	case "bash", "zsh", "fish":
		return cliOptions{mode: modeCompletion, shell: args[0]}, nil
	default:
		return cliOptions{}, fmt.Errorf("unsupported shell %q", args[0])
	}
}

func nextOptionValue(args []string, index *int, option string) (string, error) {
	*index = *index + 1
	if *index >= len(args) {
		return "", fmt.Errorf("%s requires a value", option)
	}
	return args[*index], nil
}

func parseJobStatus(value string) (daemon.Status, error) {
	status := daemon.Status(value)
	switch status {
	case daemon.StatusQueued, daemon.StatusRunning, daemon.StatusSucceeded, daemon.StatusFailed, daemon.StatusCanceled:
		return status, nil
	default:
		return "", fmt.Errorf("unknown job status %q", value)
	}
}

func parseAge(value string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(value, "d"); ok {
		count, err := strconv.Atoi(days)
		if err != nil || count < 1 {
			return 0, errors.New("day duration must be a positive integer such as 7d")
		}
		return time.Duration(count) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, errors.New("duration must be positive, such as 24h or 7d")
	}
	return duration, nil
}
