package main

import (
	"context"
	"log"
	"os"
	"runtime/debug"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/cmd"
	"github.com/geoffamey/wtg/internal/git"
)

func main() {
	app := &cli.Command{
		Name:                  "wtg",
		Usage:                 "manage multi-repo feature workflows using git worktrees and Go workspaces",
		Version:               buildVersion(),
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
			cmd.StatusCommand(git.New()),
			cmd.CreateCommand(git.New()),
			cmd.DeleteCommand(git.New()),
			cmd.AddCommand(git.New()),
			cmd.RemoveCommand(git.New()),
			cmd.ExecCommand(),
			cmd.PushCommand(git.New()),
			cmd.PathCommand(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return "(devel)"
	}
	return v
}
