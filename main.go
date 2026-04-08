package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/cmd"
	"github.com/geoffamey/wtg/internal/git"
)

// fishCompletion is a hand-written fish completion script that works around a
// bug in urfave/cli v3.7.0–v3.8.0 which produces invalid fish function names
// in the auto-generated output. See https://github.com/urfave/cli/issues/2285.
//
// When that bug is fixed in a released version, this constant and the
// ConfigureShellCompletionCommand hook below can be removed.
const fishCompletion = `function __wtg_perform_completion
    set -l args (commandline -opc)
    set -l lastArg (commandline -ct)

    set -l results ($args[1] $args[2..-1] $lastArg --generate-shell-completion 2>/dev/null)

    for line in $results[-1..1]
        if test (string trim -- $line) = ""
            set results $results[1..-2]
        else
            break
        end
    end

    for line in $results
        if not string match -q -- "wtg*" $line
            set -l parts (string split -m 1 ":" -- "$line")
            if test (count $parts) -eq 2
                printf "%s\t%s\n" "$parts[1]" "$parts[2]"
            else
                printf "%s\n" "$line"
            end
        end
    end
end

complete -c wtg -e
complete -c wtg -f -a '(__wtg_perform_completion)'
`

func main() {
	app := &cli.Command{
		Name:                  "wtg",
		Usage:                 "manage multi-repo feature workflows using git worktrees and Go workspaces",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		ConfigureShellCompletionCommand: func(completionCmd *cli.Command) {
			orig := completionCmd.Action
			completionCmd.Action = func(ctx context.Context, cmd *cli.Command) error {
				if cmd.Args().First() == "fish" {
					_, err := io.WriteString(os.Stdout, fishCompletion)
					return err
				}
				if orig != nil {
					return orig(ctx, cmd)
				}
				return nil
			}
		},
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
			cmd.NewCommand(git.New()),
			cmd.DeleteCommand(git.New()),
			cmd.AddCommand(git.New()),
			cmd.RemoveCommand(git.New()),
			cmd.ExecCommand(),
			cmd.PushCommand(git.New()),
			cmd.PathCommand(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "wtg: %s\n", err)
		os.Exit(1)
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
