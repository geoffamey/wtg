package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/urfave/cli/v2"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/state"
)

// fishDynamicCompletions is appended to the output of app.ToFishCompletion().
// It adds position-aware argument completions for repo and space names that
// the built-in ToFishCompletion cannot generate.
const fishDynamicCompletions = `
# Disable file completions globally for wtg
complete -c wtg -f

# Position-aware helpers

# True when completing the first positional arg after 'space add' (the space name).
function __fish_wtg_space_add_completing_name
    not __fish_seen_subcommand_from space; and return 1
    not __fish_seen_subcommand_from add;   and return 1
    set -l tokens (commandline -opc)
    set -l idx 0
    for i in (seq (count $tokens))
        if test $tokens[$i] = add; set idx $i; break; end
    end
    test $idx -eq 0; and return 1
    set -l pos 0
    for i in (seq (math $idx + 1) (count $tokens))
        string match -q -- '-*' $tokens[$i]; and continue
        set pos (math $pos + 1)
    end
    test $pos -eq 0
end

# True when completing subsequent positional args after 'space add' (repo names).
function __fish_wtg_space_add_completing_repos
    not __fish_seen_subcommand_from space; and return 1
    not __fish_seen_subcommand_from add;   and return 1
    set -l tokens (commandline -opc)
    set -l idx 0
    for i in (seq (count $tokens))
        if test $tokens[$i] = add; set idx $i; break; end
    end
    test $idx -eq 0; and return 1
    set -l pos 0
    for i in (seq (math $idx + 1) (count $tokens))
        string match -q -- '-*' $tokens[$i]; and continue
        set pos (math $pos + 1)
    end
    test $pos -ge 1
end

# True when completing the 2nd+ positional arg after 'space create' (repo names).
function __fish_wtg_space_create_completing_repos
    not __fish_seen_subcommand_from space;  and return 1
    not __fish_seen_subcommand_from create; and return 1
    set -l tokens (commandline -opc)
    set -l idx 0
    for i in (seq (count $tokens))
        if test $tokens[$i] = create; set idx $i; break; end
    end
    test $idx -eq 0; and return 1
    set -l pos 0
    for i in (seq (math $idx + 1) (count $tokens))
        string match -q -- '-*' $tokens[$i]; and continue
        set pos (math $pos + 1)
    end
    test $pos -ge 1
end

# Dynamic argument completions
complete -c wtg -f -n '__fish_seen_subcommand_from status; and not __fish_seen_subcommand_from repo space' -a '(wtg _complete spaces 2>/dev/null)'
complete -c wtg -f -n '__fish_seen_subcommand_from repo; and __fish_seen_subcommand_from sync'   -a '(wtg _complete repos 2>/dev/null)'
complete -c wtg -f -n '__fish_seen_subcommand_from repo; and __fish_seen_subcommand_from status' -a '(wtg _complete repos 2>/dev/null)'
complete -c wtg -f -n '__fish_seen_subcommand_from space; and __fish_seen_subcommand_from delete rm'   -a '(wtg _complete spaces 2>/dev/null)'
complete -c wtg -f -n '__fish_seen_subcommand_from space; and __fish_seen_subcommand_from push'      -a '(wtg _complete spaces 2>/dev/null)'
complete -c wtg -f -n '__fish_seen_subcommand_from space; and __fish_seen_subcommand_from rebase'    -a '(wtg _complete spaces 2>/dev/null)'
complete -c wtg -f -n '__fish_seen_subcommand_from space; and __fish_seen_subcommand_from path'      -a '(wtg _complete spaces 2>/dev/null)'
complete -c wtg -f -n '__fish_seen_subcommand_from space; and __fish_seen_subcommand_from exec'      -a '(wtg _complete spaces 2>/dev/null)'
complete -c wtg -f -n __fish_wtg_space_add_completing_name     -a '(wtg _complete spaces 2>/dev/null)'
complete -c wtg -f -n __fish_wtg_space_add_completing_repos    -a '(wtg _complete repos 2>/dev/null)'
complete -c wtg -f -n __fish_wtg_space_create_completing_repos -a '(wtg _complete repos 2>/dev/null)'
`

// CompletionCommand returns the `wtg completion` command for generating shell
// completion scripts.
func CompletionCommand() *cli.Command {
	return &cli.Command{
		Name:  "completion",
		Usage: "generate shell completion scripts",
		Subcommands: []*cli.Command{
			{
				Name:  "fish",
				Usage: "generate fish shell completion script",
				Action: func(c *cli.Context) error {
					script, err := c.App.ToFishCompletion()
					if err != nil {
						return fmt.Errorf("generate fish completion: %w", err)
					}
					fmt.Print(script)
					fmt.Print(fishDynamicCompletions)
					return nil
				},
			},
		},
	}
}

// CompleteCommand returns the hidden `wtg _complete` plumbing command used by
// shell completion scripts to fetch dynamic candidates at tab-completion time.
func CompleteCommand() *cli.Command {
	return &cli.Command{
		Name:   "_complete",
		Hidden: true,
		Usage:  "output completion candidates (used by shell completion scripts)",
		Subcommands: []*cli.Command{
			{
				Name:  "spaces",
				Usage: "output space names, one per line",
				Action: func(c *cli.Context) error {
					spaces, err := state.List()
					if err != nil {
						return nil // silently fail during completion
					}
					sort.Slice(spaces, func(i, j int) bool { return spaces[i].Name < spaces[j].Name })
					for _, sp := range spaces {
						fmt.Fprintln(os.Stdout, sp.Name)
					}
					return nil
				},
			},
			{
				Name:  "repos",
				Usage: "output discovered repo names, one per line",
				Action: func(c *cli.Context) error {
					cfg, err := config.Load(c.String("config"))
					if err != nil || cfg.Discovery.RootDir == "" {
						return nil // silently fail during completion
					}
					paths, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
					if err != nil {
						return nil
					}
					sort.Strings(paths)
					for _, p := range paths {
						name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
						fmt.Fprintln(os.Stdout, name)
					}
					return nil
				},
			},
		},
	}
}
