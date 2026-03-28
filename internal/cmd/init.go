package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/urfave/cli/v2"
	"go.yaml.in/yaml/v3"

	"github.com/geoffamey/wtg/internal/config"
)

// InitCommand returns the `wtg init` command.
func InitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "interactive setup wizard — creates the wtg config file",
		Action: func(c *cli.Context) error {
			cfgPath := c.String("config")
			if cfgPath == "" {
				cfgPath = config.DefaultPath()
			}
			return RunInit(cfgPath, os.Stdin, os.Stdout)
		},
	}
}

// RunInit runs the init wizard, reading prompts from in and writing output to out.
// Keeping IO injectable makes it straightforward to test.
func RunInit(configPath string, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)

	// If a config already exists, confirm before overwriting.
	if _, err := os.Stat(configPath); err == nil {
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

	discoveryRoot, err := prompt(r, out, "Discovery root (where your repos live)", "")
	if err != nil {
		return err
	}
	if discoveryRoot == "" {
		return fmt.Errorf("discovery root is required")
	}

	maxDepthStr, err := prompt(r, out, "Max scan depth", "2")
	if err != nil {
		return err
	}
	maxDepth, err := strconv.Atoi(maxDepthStr)
	if err != nil || maxDepth < 1 {
		return fmt.Errorf("max scan depth must be a positive integer, got %q", maxDepthStr)
	}

	spacesRoot, err := prompt(r, out, "Workspace root (where spaces will be created)", "")
	if err != nil {
		return err
	}
	if spacesRoot == "" {
		return fmt.Errorf("workspace root is required")
	}

	branchPrefix, err := prompt(r, out, `Branch prefix (optional, e.g. "yourname/")`, "")
	if err != nil {
		return err
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
