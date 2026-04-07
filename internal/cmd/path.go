package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/state"
)

// PathCommand returns the `wtg path` command (hidden; used by shell wcd functions).
func PathCommand() *cli.Command {
	return &cli.Command{
		Name:          "path",
		Usage:         "print the root path of a workspace",
		ArgsUsage:     "<workspace>",
		Hidden:        true,
		ShellComplete: completeSpaces,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() == 0 {
				return fmt.Errorf("missing required argument: <workspace>")
			}
			sp, err := state.Load(cmd.Args().First())
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, sp.Path)
			return nil
		},
	}
}
