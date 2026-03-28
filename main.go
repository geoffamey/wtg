package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/geoffamey/wtg/internal/cmd"
	"github.com/geoffamey/wtg/internal/git"
)

func main() {
	app := &cli.App{
		Name:                 "wtg",
		Usage:                "manage multi-repo feature workflows using git worktrees and Go workspaces",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				EnvVars: []string{"WTG_CONFIG"},
				Usage:   "path to config file (default: $XDG_CONFIG_HOME/wtg/config.yaml)",
			},
		},
		Commands: []*cli.Command{
			cmd.InitCommand(),
			cmd.RepoCommand(git.New()),
			cmd.SpaceCommand(git.New()),
			cmd.StatusCommand(git.New()),
			cmd.CompletionCommand(),
			cmd.CompleteCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
