package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/config"
)

// configTemplate is the commented config scaffold written by `wtg config init`.
// Every parameter is shown commented out with its default; commented means the
// built-in default applies, so future default changes still reach users.
const configTemplate = `# wtg configuration. Every setting is shown commented out with its default value.
# Uncomment a line and edit it to override that default.

[discovery]
# Directory wtg scans to find your git repos. Anything matching under here can be
# pulled into a space by its short name. (default: ~/repos)
# root_dir = "~/repos"

# How many directory levels below root_dir to descend while scanning. Raise this
# if your clones are nested under org or group subdirectories. (default: 2)
# max_depth = 2

[spaces]
# Directory where new spaces are created, one subdirectory per space.
# (default: ~/spaces)
# root_dir = "~/spaces"

[git]
# Prefix prepended to a space name to form its branch name, so space "login" with
# prefix "alice/" branches as "alice/login". Leave empty to branch on the space
# name alone. (default: none)
# branch_prefix = ""

[always]
# Repos symlinked into every new space without getting their own worktree. Use it
# for shared docs or tooling you want on hand but never branch.
# repos = ["docs"]

# Files copied into the root of every new space. Good for editor configs, direnv
# files, or a CLAUDE.md you want present in every workspace.
# files = ["~/.config/wtg/CLAUDE.md"]

# Executable run after a space is created, changed, or deleted. It receives the
# event type and the space path through environment variables. See docs/always.md.
# run = "~/.config/wtg/on-event.sh"
`

// ConfigCommand returns the `wtg config` command group. With no subcommand it
// prints the resolved config file's raw contents.
func ConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "show and manage the wtg config file",
		Description: `With no subcommand, prints the resolved config file's raw contents
(or a note if none exists). Use the subcommands to scaffold a config
file or print its resolved path.`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runConfigPrint(config.ResolvePath(cmd.String("config")), os.Stdout, os.Stderr)
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "write a commented config template",
				Description: `Writes a commented TOML config template. Refuses if the target
already exists; pass --force to overwrite. -o sets the output path;
-o - writes to stdout (and never refuses).`,
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "force", Usage: "overwrite an existing config file"},
					&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "output path, or - for stdout"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					out := cmd.String("output")
					if out == "" {
						out = config.ResolvePath(cmd.String("config"))
					}
					return runConfigInit(out, cmd.Bool("force"), os.Stdout)
				},
			},
			{
				Name:  "path",
				Usage: "print the resolved config file path",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					fmt.Fprintln(os.Stdout, config.ResolvePath(cmd.String("config")))
					return nil
				},
			},
		},
	}
}

// runConfigPrint writes the raw contents of the file at path to out, or a note to
// note if it does not exist.
func runConfigPrint(path string, out, note io.Writer) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(note, "no config file at %s (run `wtg config init` to create one)\n", path)
		return nil
	}
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

// runConfigInit writes the config template. With path "-" it writes to stdout and
// never refuses. Otherwise it refuses if the target exists unless force is set.
func runConfigInit(path string, force bool, out io.Writer) error {
	if path == "-" {
		_, err := io.WriteString(out, configTemplate)
		return err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(configTemplate), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Fprintf(out, "Config written to %s\n", path)
	return nil
}
