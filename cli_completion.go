package main

import (
	"fmt"
	"io"
)

const bashCompletion = `# bash completion for taskrunner
_taskrunner_complete() {
  local current previous
  current="${COMP_WORDS[COMP_CWORD]}"
  previous="${COMP_WORDS[COMP_CWORD-1]}"
  case "${COMP_WORDS[1]}" in
    job)
      if [[ ${COMP_CWORD} -eq 2 ]]; then
        COMPREPLY=($(compgen -W "logs wait inspect cancel rerun rm prune" -- "$current"))
      fi
      ;;
    task)
      if [[ ${COMP_CWORD} -eq 2 ]]; then
        COMPREPLY=($(compgen -W "show validate rm" -- "$current"))
      fi
      ;;
    completion)
      COMPREPLY=($(compgen -W "bash zsh fish" -- "$current"))
      ;;
    *)
      if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "create edit run tasks task jobs job completion help" -- "$current"))
      fi
      ;;
  esac
}
complete -F _taskrunner_complete taskrunner
`

const zshCompletion = `#compdef taskrunner
_taskrunner() {
  local -a commands job_commands task_commands
  commands=(create edit run tasks task jobs job completion help)
  job_commands=(logs wait inspect cancel rerun rm prune)
  task_commands=(show validate rm)
  if (( CURRENT == 2 )); then
    _describe 'command' commands
  elif [[ ${words[2]} == job && CURRENT == 3 ]]; then
    _describe 'job command' job_commands
  elif [[ ${words[2]} == task && CURRENT == 3 ]]; then
    _describe 'task command' task_commands
  elif [[ ${words[2]} == completion ]]; then
    _values 'shell' bash zsh fish
  fi
}
compdef _taskrunner taskrunner
`

const fishCompletion = `complete -c taskrunner -f
complete -c taskrunner -n '__fish_use_subcommand' -a 'create edit run tasks task jobs job completion help'
complete -c taskrunner -n '__fish_seen_subcommand_from job; and not __fish_seen_subcommand_from logs wait inspect cancel rerun rm prune' -a 'logs wait inspect cancel rerun rm prune'
complete -c taskrunner -n '__fish_seen_subcommand_from task; and not __fish_seen_subcommand_from show validate rm' -a 'show validate rm'
complete -c taskrunner -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`

func executeCompletionCLI(shell string, stdout io.Writer) error {
	var source string
	switch shell {
	case "bash":
		source = bashCompletion
	case "zsh":
		source = zshCompletion
	case "fish":
		source = fishCompletion
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
	if _, err := io.WriteString(stdout, source); err != nil {
		return fmt.Errorf("write %s completion: %w", shell, err)
	}
	return nil
}
