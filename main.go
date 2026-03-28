package main

import (
	"context"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/cmd"
	"github.com/geoffamey/wtg/internal/git"
)

func main() {
	app := &cli.Command{
		Name:                  "wtg",
		Usage:                 "manage multi-repo feature workflows using git worktrees and Go workspaces",
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Sources: cli.EnvVars("WTG_CONFIG"),
				Usage:   "path to config file (default: $XDG_CONFIG_HOME/wtg/config.yaml)",
			},
		},
		Commands: []*cli.Command{
			cmd.InitCommand(),
			cmd.RepoCommand(git.New()),
			cmd.SpaceCommand(git.New()),
			cmd.StatusCommand(git.New()),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
