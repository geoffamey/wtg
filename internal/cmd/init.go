package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
	"go.yaml.in/yaml/v3"

	"github.com/geoffamey/wtg/internal/config"
)

// InitCommand returns the `wtg init` command.
func InitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "interactive setup wizard — creates the wtg config file",
		Description: `Prompts for core settings and writes them to the config file
(default: $XDG_CONFIG_HOME/wtg/config.yaml):

  discovery.root_dir   where wtg scans for git repos
  spaces.root_dir      where new workspaces are created
  git.branch_prefix    prefix prepended to workspace names to form branch names
  always.repos         repos symlinked into every new space
  always.files         files copied into every new space root
  always.run           executable run after a space is created/changed/deleted

The always.repos and always.files prompts take a comma-separated list on a
single line; enter a single - to clear an existing value.

If a config file already exists its values are offered as defaults, so pressing
enter on every prompt preserves the current configuration.

Use --defaults to accept all factory defaults without prompting.`,
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
// When useDefaults is true all prompts are skipped and factory defaults are accepted.
// In interactive mode, the current config file (if any) is loaded first and its
// values are offered as prompt defaults, so pressing enter on every question
// preserves the existing configuration.
func RunInit(configPath string, in io.Reader, out io.Writer, useDefaults bool) error {
	r := bufio.NewReader(in)

	const (
		defaultDiscoveryRoot = "~/"
		defaultMaxDepth      = "2"
		defaultSpacesRoot    = "~/spaces"
		defaultBranchPrefix  = ""
	)

	// Load current config to seed interactive defaults. Missing file is fine.
	current, _ := config.Load(configPath)

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
		alwaysRepos   string
		alwaysFiles   string
		alwaysRun     string
		err           error
	)

	if useDefaults {
		discoveryRoot = defaultDiscoveryRoot
		maxDepthStr = defaultMaxDepth
		spacesRoot = defaultSpacesRoot
		branchPrefix = defaultBranchPrefix
	} else {
		discRootDef := defaultDiscoveryRoot
		if current.Discovery.RootDir != "" {
			discRootDef = current.Discovery.RootDir
		}
		discoveryRoot, err = prompt(r, out, "Discovery root (where your repos live)", discRootDef)
		if err != nil {
			return err
		}

		maxDepthDef := defaultMaxDepth
		if current.Discovery.MaxDepth > 0 {
			maxDepthDef = strconv.Itoa(current.Discovery.MaxDepth)
		}
		maxDepthStr, err = prompt(r, out, "Max scan depth", maxDepthDef)
		if err != nil {
			return err
		}

		spacesRootDef := defaultSpacesRoot
		if current.Spaces.RootDir != "" {
			spacesRootDef = current.Spaces.RootDir
		}
		spacesRoot, err = prompt(r, out, "Workspace root (where spaces will be created)", spacesRootDef)
		if err != nil {
			return err
		}

		branchPrefix, err = prompt(r, out, `Branch prefix (optional, e.g. "yourname/")`, current.Git.BranchPrefix)
		if err != nil {
			return err
		}

		alwaysRepos, err = prompt(r, out, "Always-included repos, comma-separated (optional, - to clear)", joinSlice(current.Always.Repos))
		if err != nil {
			return err
		}

		alwaysFiles, err = prompt(r, out, "Always-copied files, comma-separated paths (optional, - to clear)", joinSlice(current.Always.Files))
		if err != nil {
			return err
		}

		alwaysRun, err = prompt(r, out, "Event script, run after a space is created/changed/deleted (optional, - to clear)", current.Always.Run)
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
		Always: config.AlwaysConfig{
			Repos: splitSlice(alwaysRepos),
			Files: splitSlice(alwaysFiles),
			Run:   singleValue(alwaysRun),
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

// joinSlice formats a string slice as a comma-separated string for display in prompts.
func joinSlice(s []string) string {
	return strings.Join(s, ", ")
}

// singleValue trims a single-value prompt result. Empty input (keep current) and
// the sentinel "-" (explicit clear) both yield "".
func singleValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "-" {
		return ""
	}
	return s
}

// splitSlice parses a comma-separated string into a trimmed string slice.
// Returns nil for empty/whitespace input (keep current) or the sentinel "-" (explicit clear).
func splitSlice(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
