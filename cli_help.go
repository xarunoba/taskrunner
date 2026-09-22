package main

import (
	"fmt"
	"strings"
)

const createHelpText = `Usage:
  taskrunner create

Description:
  Open a standalone task creation form. Saving or canceling returns to the shell.

Options:
  -h, --help  Show this help.
`

const editHelpText = `Usage:
  taskrunner edit <task>

Description:
  Open a task in a standalone editor. Task names match display names and JSON filenames.

Options:
  -h, --help  Show this help.
`

const runHelpText = `Usage:
  taskrunner run <task> [options]

Options:
  --set k=v   Set a task field. Repeat for multiple fields.
  --detach    Print the job ID and return without waiting.
  --dry-run   Print the safely rendered command without creating a job.
  --quiet     With --dry-run, print only the rendered command.
  -h, --help  Show this help.
`

const tasksHelpText = `Usage:
  taskrunner tasks [options]

Description:
  List task definitions in the current workspace.

Options:
  --json      Emit a JSON array.
  -h, --help  Show this help.
`

const taskHelpText = `Usage:
  taskrunner task <command> [options]

Commands:
  show      Print a persisted task definition.
  validate  Validate one task or every task.
  rm        Remove a task definition without removing job history.

Options:
  -h, --help  Show this help.

Run "taskrunner task <command> --help" for command-specific options.
`

const taskShowHelpText = `Usage:
  taskrunner task show <task> [options]

Options:
  --json      Emit compact JSON instead of indented JSON.
  -h, --help  Show this help.
`

const taskValidateHelpText = `Usage:
  taskrunner task validate [task]

Description:
  Validate one task or every task. Successful validation produces no output.

Options:
  -h, --help  Show this help.
`

const taskRemoveHelpText = `Usage:
  taskrunner task rm <task> [options]

Options:
  --force     Remove without interactive confirmation.
  -h, --help  Show this help.
`

const jobsHelpText = `Usage:
  taskrunner jobs [options]

Description:
  List queued and running jobs, newest first.

Options:
  --all, -a       Include completed jobs.
  --status value  Filter by queued, running, succeeded, failed, or canceled.
  --task value    Filter by task display name or JSON filename.
  --limit count   Limit the number of matching jobs.
  --json          Emit a JSON array.
  --quiet         Print one full job ID per line.
  -h, --help      Show this help.
`

const jobHelpText = `Usage:
  taskrunner job <command> [options]

Commands:
  logs     Print or follow combined standard output and standard error.
  wait     Wait for a job to finish without printing its log.
  inspect  Print job status, command, timestamps, and errors.
  cancel   Cancel a queued or running job.
  rerun    Start a new job with the original command and policy.
  rm       Remove a completed job and its output.
  prune    Remove matching completed jobs.

Options:
  -h, --help  Show this help.

Job references accept a full ID or a unique ID prefix or suffix.
Run "taskrunner job <command> --help" for command-specific options.
`

const jobLogsHelpText = `Usage:
  taskrunner job logs <job> [options]

Options:
  --follow, -f  Follow new output until the job finishes.
  --tail lines  Print only the final number of existing lines.
  -h, --help    Show this help.
`

const jobWaitHelpText = `Usage:
  taskrunner job wait <job>

Description:
  Wait for completion without printing logs. The exit status reflects the job result.

Options:
  -h, --help  Show this help.
`

const jobInspectHelpText = `Usage:
  taskrunner job inspect <job> [options]

Options:
  --json      Emit JSON.
  -h, --help  Show this help.
`

const jobCancelHelpText = `Usage:
  taskrunner job cancel <job>

Description:
  Request cancellation of a queued or running job.

Options:
  -h, --help  Show this help.
`

const jobRerunHelpText = `Usage:
  taskrunner job rerun <job>

Description:
  Start a new job with the original command and job policy.

Options:
  -h, --help  Show this help.
`

const jobRemoveHelpText = `Usage:
  taskrunner job rm <job>

Description:
  Remove a completed job and its output. Active jobs must be canceled first.

Options:
  -h, --help  Show this help.
`

const jobPruneHelpText = `Usage:
  taskrunner job prune [options]

Options:
  --status value  Remove only succeeded, failed, or canceled jobs.
  --before age    Remove jobs older than an age such as 24h or 7d.
  --force         Remove without interactive confirmation.
  -h, --help      Show this help.
`

const completionHelpText = `Usage:
  taskrunner completion <bash|zsh|fish>

Description:
  Generate shell completion source on standard output.

Options:
  -h, --help  Show this help.
`

var commandHelp = map[string]string{
	"":              helpText,
	"create":        createHelpText,
	"edit":          editHelpText,
	"run":           runHelpText,
	"tasks":         tasksHelpText,
	"task":          taskHelpText,
	"task show":     taskShowHelpText,
	"task validate": taskValidateHelpText,
	"task rm":       taskRemoveHelpText,
	"jobs":          jobsHelpText,
	"job":           jobHelpText,
	"job logs":      jobLogsHelpText,
	"job wait":      jobWaitHelpText,
	"job inspect":   jobInspectHelpText,
	"job cancel":    jobCancelHelpText,
	"job rerun":     jobRerunHelpText,
	"job rm":        jobRemoveHelpText,
	"job prune":     jobPruneHelpText,
	"completion":    completionHelpText,
}

func parseHelpCLI(args []string) (cliOptions, error) {
	return helpOptions(strings.Join(args, " "))
}

func helpOptions(topic string) (cliOptions, error) {
	if _, ok := commandHelp[topic]; !ok {
		return cliOptions{}, fmt.Errorf("unknown help topic %q", topic)
	}
	return cliOptions{mode: modeHelp, helpTopic: topic}, nil
}

func helpTextFor(topic string) string {
	return commandHelp[topic]
}

func isHelpArgument(argument string) bool {
	return argument == "-h" || argument == "--help"
}

func hasHelpArgument(args []string) bool {
	for _, argument := range args {
		if isHelpArgument(argument) {
			return true
		}
	}
	return false
}
