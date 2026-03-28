// Package config handles loading and validating wtg configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config holds all wtg configuration.
type Config struct {
	Discovery DiscoveryConfig `koanf:"discovery" yaml:"discovery"`
	Spaces    SpacesConfig    `koanf:"spaces"    yaml:"spaces"`
	Git       GitConfig       `koanf:"git"       yaml:"git"`
}

// DiscoveryConfig controls repo scanning.
type DiscoveryConfig struct {
	RootDir  string `koanf:"root_dir"  yaml:"root_dir"`
	MaxDepth int    `koanf:"max_depth" yaml:"max_depth"`
}

// SpacesConfig controls workspace directory placement.
type SpacesConfig struct {
	RootDir string `koanf:"root_dir" yaml:"root_dir"`
}

// GitConfig controls git operation behaviour.
type GitConfig struct {
	BranchPrefix string `koanf:"branch_prefix" yaml:"branch_prefix"`
}

// DefaultPath returns the default config file path following the XDG Base Directory
// spec: $XDG_CONFIG_HOME/wtg/config.yaml, falling back to ~/.config/wtg/config.yaml.
func DefaultPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "wtg", "config.yaml")
}

// Load loads configuration from (in order): built-in defaults, the YAML file at path,
// and WTG_* environment variables. If path is empty, DefaultPath() is used.
// A missing config file is not an error.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	k := koanf.New(".")

	// 1. Built-in defaults.
	if err := k.Load(confmap.Provider(map[string]any{
		"discovery.max_depth": 2,
	}, "."), nil); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	// 2. Config file (optional — missing is not an error).
	if _, err := os.Stat(path); err == nil {
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load config %s: %w", path, err)
		}
	}

	// 3. Environment variables.
	// Mapping: WTG_<SECTION>_<KEY> → <section>.<key>
	// The section is always a single word, so we split on the first underscore only.
	// Examples:
	//   WTG_GIT_BRANCH_PREFIX     → git.branch_prefix
	//   WTG_DISCOVERY_ROOT_DIR    → discovery.root_dir
	//   WTG_DISCOVERY_MAX_DEPTH   → discovery.max_depth
	if err := k.Load(env.Provider("WTG_", ".", func(s string) string {
		s = strings.ToLower(strings.TrimPrefix(s, "WTG_"))
		section, key, ok := strings.Cut(s, "_")
		if ok {
			return section + "." + key
		}
		return s
	}), nil); err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.Discovery.RootDir = expandTilde(cfg.Discovery.RootDir)
	cfg.Spaces.RootDir = expandTilde(cfg.Spaces.RootDir)

	return &cfg, nil
}

// expandTilde replaces a leading ~/ with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
