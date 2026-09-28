package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
	"github.com/xarunoba/taskrunner/internal/tui"
	"github.com/xarunoba/taskrunner/internal/version"
)

type cliApp struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	root := newRootCommand(stdin, stdout, stderr)
	root.SetArgs(args)
	return root.Execute()
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	app := &cliApp{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
	root := &cobra.Command{
		Use:           "taskrunner",
		Version:       version.Current(),
		Short:         "Run workspace tasks",
		Long:          "Taskrunner stores and runs tasks for the current workspace. Run without a command to open the TUI.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.runTUI("", false)
		},
	}
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(
		app.newCreateCommand(),
		app.newEditCommand(),
		app.newRunCommand(),
		app.newTasksCommand(),
		app.newTaskCommand(),
		app.newJobsCommand(),
		app.newJobCommand(),
		newCompletionCommand(root),
		newDaemonCommand(),
	)
	return root
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "completion <bash|zsh|fish>",
		Short:                 "Generate shell completion source",
		Args:                  cobra.ExactArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}

func newDaemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "__daemon <workspace>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return daemon.Serve(args[0], daemon.DefaultIdleTimeout)
		},
	}
}

func (a *cliApp) runTUI(taskName string, create bool) error {
	_, store, items, err := loadTaskStore()
	if err != nil {
		return err
	}
	themes, err := theme.Open()
	if err != nil {
		return fmt.Errorf("open theme store: %w", err)
	}
	if err := themes.CreateDefaultIfMissing(); err != nil {
		return fmt.Errorf("prepare configuration: %w", err)
	}
	selected, err := themes.Load()
	if err != nil {
		return fmt.Errorf("load theme: %w", err)
	}
	if create {
		return tui.Create(store, items, themes, selected, a.stdin, a.stdout)
	}
	if taskName == "" {
		return tui.Run(store, items, themes, selected, a.stdin, a.stdout)
	}
	item, err := findTask(items, taskName)
	if err != nil {
		return err
	}
	return tui.Edit(store, items, item, themes, selected, a.stdin, a.stdout)
}

func workspacePath() (string, error) {
	workspace, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get workspace: %w", err)
	}
	return workspace, nil
}

func loadTaskStore() (string, *task.Store, []task.Task, error) {
	workspace, err := workspacePath()
	if err != nil {
		return "", nil, nil, err
	}
	store := task.NewStore(workspace)
	items, err := store.Load()
	if err != nil {
		return "", nil, nil, err
	}
	return workspace, store, items, nil
}
