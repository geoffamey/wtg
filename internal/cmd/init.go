package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/urfave/cli/v3"
	"go.yaml.in/yaml/v3"

	"github.com/geoffamey/wtg/internal/config"
)

// InitCommand returns the `wtg init` command.
func InitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "interactive setup wizard — creates the wtg config file",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "defaults",
				Usage: "accept all defaults without prompting",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfgPath := cmd.String("config")
			if cfgPath == "" {
				cfgPath = config.DefaultPath()
			}
			return RunInit(cfgPath, os.Stdin, os.Stdout, cmd.Bool("defaults"))
		},
	}
}

// RunInit runs the init wizard, reading prompts from in and writing output to out.
// When useDefaults is true all prompts are skipped and defaults are accepted.
// Keeping IO injectable makes it straightforward to test.
func RunInit(configPath string, in io.Reader, out io.Writer, useDefaults bool) error {
	r := bufio.NewReader(in)

	const (
		defaultDiscoveryRoot = "~/"
		defaultMaxDepth      = "2"
		defaultSpacesRoot    = "~/spaces"
		defaultBranchPrefix  = ""
	)

	// If a config already exists, confirm before overwriting (skip in defaults mode).
	if _, err := os.Stat(configPath); err == nil && !useDefaults {
		fmt.Fprintf(out, "Config already exists at %s\n", configPath)
		ok, err := confirm(r, out, "Overwrite?")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Aborted.")
			return nil
		}
	}

	var (
		discoveryRoot string
		maxDepthStr   string
		spacesRoot    string
		branchPrefix  string
		err           error
	)

	if useDefaults {
		discoveryRoot = defaultDiscoveryRoot
		maxDepthStr = defaultMaxDepth
		spacesRoot = defaultSpacesRoot
		branchPrefix = defaultBranchPrefix
	} else {
		discoveryRoot, err = prompt(r, out, "Discovery root (where your repos live)", defaultDiscoveryRoot)
		if err != nil {
			return err
		}

		maxDepthStr, err = prompt(r, out, "Max scan depth", defaultMaxDepth)
		if err != nil {
			return err
		}

		spacesRoot, err = prompt(r, out, "Workspace root (where spaces will be created)", defaultSpacesRoot)
		if err != nil {
			return err
		}

		branchPrefix, err = prompt(r, out, `Branch prefix (optional, e.g. "yourname/")`, defaultBranchPrefix)
		if err != nil {
			return err
		}
	}

	maxDepth, err := strconv.Atoi(maxDepthStr)
	if err != nil || maxDepth < 1 {
		return fmt.Errorf("max scan depth must be a positive integer, got %q", maxDepthStr)
	}

	cfg := config.Config{
		Discovery: config.DiscoveryConfig{
			RootDir:  discoveryRoot,
			MaxDepth: maxDepth,
		},
		Spaces: config.SpacesConfig{
			RootDir: spacesRoot,
		},
		Git: config.GitConfig{
			BranchPrefix: branchPrefix,
		},
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Fprintf(out, "\nConfig written to %s\n", configPath)
	return nil
}
