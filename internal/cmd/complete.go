package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/state"
)

// completeSpaces outputs space names for shell completion.
func completeSpaces(_ context.Context, _ *cli.Command) {
	spaces, err := state.List()
	if err != nil {
		return
	}
	sort.Slice(spaces, func(i, j int) bool { return spaces[i].Name < spaces[j].Name })
	for _, sp := range spaces {
		fmt.Println(sp.Name)
	}
}

// completeRepos outputs discovered repo names for shell completion.
func completeRepos(_ context.Context, cmd *cli.Command) {
	cfg, err := config.Load(cmd.Root().String("config"))
	if err != nil || cfg.Discovery.RootDir == "" {
		return
	}
	paths, err := discoverRepoPaths(cfg.Discovery.RootDir, cfg.Discovery.MaxDepth)
	if err != nil {
		return
	}
	sort.Strings(paths)
	for _, p := range paths {
		name, _ := filepath.Rel(cfg.Discovery.RootDir, p)
		fmt.Fprintln(os.Stdout, name)
	}
}

// completeSpaceAtFirst completes a space name only at position 0.
// Used for commands like exec where position 1+ is a pass-through command.
func completeSpaceAtFirst(ctx context.Context, cmd *cli.Command) {
	if cmd.NArg() <= 1 {
		completeSpaces(ctx, cmd)
	}
}

// completeReposAfterFirst completes discovered repos only at position 1+.
// Used for commands where position 0 is a new name (e.g. space create).
//
// NArg is 1 while completing position 0 (the partial token counts as an arg),
// so repos are only offered once a second token is being completed (NArg >= 2).
func completeReposAfterFirst(ctx context.Context, cmd *cli.Command) {
	if cmd.NArg() >= 2 {
		completeRepos(ctx, cmd)
	}
}

// completeSpaceThenRepos completes a space name at position 0, then discovered
// repo names at position 1+. Used for commands like space add.
func completeSpaceThenRepos(ctx context.Context, cmd *cli.Command) {
	if cmd.NArg() <= 1 {
		completeSpaces(ctx, cmd)
	} else {
		completeRepos(ctx, cmd)
	}
}

// completeSpaceMembers completes a space name at position 0, then the names of
// repos already in that space at position 1+. Used for space remove.
func completeSpaceMembers(ctx context.Context, cmd *cli.Command) {
	if cmd.NArg() <= 1 {
		completeSpaces(ctx, cmd)
		return
	}
	sp, err := state.Load(cmd.Args().First())
	if err != nil {
		return
	}
	sort.Slice(sp.Repos, func(i, j int) bool { return sp.Repos[i].Name < sp.Repos[j].Name })
	for _, r := range sp.Repos {
		fmt.Println(r.Name)
	}
}
