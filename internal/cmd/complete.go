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
