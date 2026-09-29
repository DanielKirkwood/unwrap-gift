package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DanielKirkwood/unwrap-gift/internal/tasks"
)

var errUnknownTask = errors.New("cmd: unknown task")

// NewTaskCmd builds the `task` command group: listing and running
// self-registered ad-hoc jobs from internal/tasks.
func NewTaskCmd() *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "List and run ad-hoc operational tasks",
		Long: `task lists and runs the ad-hoc operational jobs registered in
internal/tasks (currently: grant-admin, revoke-admin, reseed-keto — all Keto
relation-tuple maintenance). Run "unwrap-gift task list" to see what's registered
and which --arg keys each one requires, then
"unwrap-gift task exec <name> --arg key=value" to run one.`,
	}

	taskCmd.AddCommand(newTaskListCmd())
	taskCmd.AddCommand(newTaskExecCmd())

	return taskCmd
}

func newTaskListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List every registered task",
		Example: "  unwrap-gift task list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			w := newTabWriter(cmd)

			fmt.Fprintln(w, "NAME\tREQUIRED ARGS")
			for _, t := range tasks.All() {
				fmt.Fprintf(w, "%s\t%s\n", t.Name, t.RequiredArgs)
			}

			return w.Flush()
		},
	}
}

func newTaskExecCmd() *cobra.Command {
	var args map[string]string

	cmd := &cobra.Command{
		Use:   "exec <name>",
		Short: "Run a registered task by name",
		Long: `Runs the registered task named <name>, failing with a descriptive
error if any of its required --arg keys are missing (see "task list" for each
task's required keys). Every task currently registered requires
--arg identity-id=<id>.`,
		Example: `  unwrap-gift task exec grant-admin --arg identity-id=<identity-id>
  unwrap-gift task exec revoke-admin --arg identity-id=<identity-id>
  unwrap-gift task exec reseed-keto --arg identity-id=<identity-id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, cmdArgs []string) error {
			name := cmdArgs[0]

			t, ok := tasks.Get(name)
			if !ok {
				return fmt.Errorf("%w: %s", errUnknownTask, name)
			}
			if err := t.CheckRequiredArgs(args); err != nil {
				return err
			}

			a, ok := appFromContext(cmd.Context())
			if !ok {
				return errAppNotBootstrapped
			}

			// a.Keto is a *ketoclient.Client (concrete type), nil when the
			// keto feature is disabled. Assigning a nil *ketoclient.Client
			// directly into the tasks.Deps.Keto interface field would
			// produce a non-nil interface wrapping a nil pointer — Go's
			// classic typed-nil-in-interface trap. deps.Keto == nil would
			// then be false even though there's no real client, silently
			// defeating every task's disabled-feature guard. Guarding the
			// assignment here keeps deps.Keto genuinely nil when Keto is
			// disabled.
			var deps tasks.Deps
			if a.Keto != nil {
				deps.Keto = a.Keto
			}

			return t.Run(cmd.Context(), deps, args)
		},
	}

	cmd.Flags().StringToStringVar(&args, "arg", nil, "task argument as key=value (repeatable)")

	return cmd
}
