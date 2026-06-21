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

If a config file already exists its values are offered as defaults, so pressing
enter on every prompt preserves the current configuration.

Each setting also has a flag (--repo-dir, --max-depth, --spaces-dir,
--branch-prefix, --always-repos, --always-files, --always-run) that sets it and
skips its prompt. Combine the flags with --defaults for a fully non-interactive
run: flags set what you pass, factory defaults fill the rest, and an existing
config is overwritten without confirmation.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "defaults",
				Usage: "accept defaults without prompting (also skips the overwrite confirmation)",
			},
			&cli.StringFlag{Name: "repo-dir", Usage: "set discovery.root_dir and skip its prompt"},
			&cli.StringFlag{Name: "max-depth", Usage: "set discovery.max_depth and skip its prompt"},
			&cli.StringFlag{Name: "spaces-dir", Usage: "set spaces.root_dir and skip its prompt"},
			&cli.StringFlag{Name: "branch-prefix", Usage: "set git.branch_prefix and skip its prompt"},
			&cli.StringFlag{Name: "always-repos", Usage: "set always.repos (comma-separated; - to clear) and skip its prompt"},
			&cli.StringFlag{Name: "always-files", Usage: "set always.files (comma-separated; - to clear) and skip its prompt"},
			&cli.StringFlag{Name: "always-run", Usage: "set always.run (path; - to clear) and skip its prompt"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfgPath := cmd.String("config")
			if cfgPath == "" {
				cfgPath = config.DefaultPath()
			}
			// A flag pointer is set only when the flag was passed, so an unset
			// flag falls through to the prompt or the default.
			strPtr := func(name string) *string {
				if cmd.IsSet(name) {
					v := cmd.String(name)
					return &v
				}
				return nil
			}
			ov := InitOverrides{
				DiscoveryRoot: strPtr("repo-dir"),
				MaxDepth:      strPtr("max-depth"),
				SpacesRoot:    strPtr("spaces-dir"),
				BranchPrefix:  strPtr("branch-prefix"),
				AlwaysRepos:   strPtr("always-repos"),
				AlwaysFiles:   strPtr("always-files"),
				AlwaysRun:     strPtr("always-run"),
			}
			return RunInit(cfgPath, os.Stdin, os.Stdout, cmd.Bool("defaults"), ov)
		},
	}
}

// InitOverrides carries values supplied via flags. A non-nil field overrides
// both the interactive prompt and the --defaults value for that setting; the
// list fields (AlwaysRepos/AlwaysFiles) and AlwaysRun accept "-" to clear.
type InitOverrides struct {
	DiscoveryRoot *string
	MaxDepth      *string
	SpacesRoot    *string
	BranchPrefix  *string
	AlwaysRepos   *string // comma-separated
	AlwaysFiles   *string // comma-separated
	AlwaysRun     *string
}

// RunInit runs the init wizard, reading prompts from in and writing output to out.
// When useDefaults is true all unset prompts are skipped and factory defaults are
// accepted. In interactive mode, the current config file (if any) is loaded first
// and its values are offered as prompt defaults, so pressing enter on every question
// preserves the existing configuration. Any field set in ov overrides both the
// prompt and the default, which is how the per-setting flags drive a non-interactive
// run.
func RunInit(configPath string, in io.Reader, out io.Writer, useDefaults bool, ov InitOverrides) error {
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

	// resolve returns the value for one setting: an explicit override wins;
	// otherwise the factory default in --defaults mode, or an interactive prompt
	// seeded with the current value (falling back to the factory default).
	resolve := func(override *string, factoryDef, currentVal, promptMsg string) (string, error) {
		if override != nil {
			return *override, nil
		}
		if useDefaults {
			return factoryDef, nil
		}
		def := factoryDef
		if currentVal != "" {
			def = currentVal
		}
		return prompt(r, out, promptMsg, def)
	}

	curMaxDepth := ""
	if current.Discovery.MaxDepth > 0 {
		curMaxDepth = strconv.Itoa(current.Discovery.MaxDepth)
	}

	discoveryRoot, err := resolve(ov.DiscoveryRoot, defaultDiscoveryRoot, current.Discovery.RootDir, "Discovery root (where your repos live)")
	if err != nil {
		return err
	}
	maxDepthStr, err := resolve(ov.MaxDepth, defaultMaxDepth, curMaxDepth, "Max scan depth")
	if err != nil {
		return err
	}
	spacesRoot, err := resolve(ov.SpacesRoot, defaultSpacesRoot, current.Spaces.RootDir, "Workspace root (where spaces will be created)")
	if err != nil {
		return err
	}
	branchPrefix, err := resolve(ov.BranchPrefix, defaultBranchPrefix, current.Git.BranchPrefix, `Branch prefix (optional, e.g. "yourname/")`)
	if err != nil {
		return err
	}
	alwaysRepos, err := resolve(ov.AlwaysRepos, "", joinSlice(current.Always.Repos), "Always-included repos, comma-separated (optional, - to clear)")
	if err != nil {
		return err
	}
	alwaysFiles, err := resolve(ov.AlwaysFiles, "", joinSlice(current.Always.Files), "Always-copied files, comma-separated paths (optional, - to clear)")
	if err != nil {
		return err
	}
	alwaysRun, err := resolve(ov.AlwaysRun, "", current.Always.Run, "Event script, run after a space is created/changed/deleted (optional, - to clear)")
	if err != nil {
		return err
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
