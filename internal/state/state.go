// Package state handles reading and writing space metadata to XDG data files.
package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Space holds the persisted metadata for a workspace.
type Space struct {
	Name        string      `yaml:"name"`
	Path        string      `yaml:"path"`   // absolute path to the workspace root
	Branch      string      `yaml:"branch"` // branch shared across all repos
	CreatedAt   time.Time   `yaml:"created_at"`
	Repos       []RepoEntry `yaml:"repos"`
	GoWorkspace bool        `yaml:"go_workspace"` // whether a go.work was written
}

// RepoEntry describes one repo's participation in a space.
type RepoEntry struct {
	Name         string `yaml:"name"`              // short name relative to discovery.root_dir
	RepoPath     string `yaml:"repo_path"`         // absolute path to the main clone
	WorktreePath string `yaml:"worktree_path"`     // absolute path to the linked worktree or symlink
	Symlink      bool   `yaml:"symlink,omitempty"` // true if this entry is a symlink to the main clone
}

// DataDir returns the directory where space state files are stored, following
// the XDG Base Directory spec: $XDG_DATA_HOME/wtg/spaces (default ~/.local/share/wtg/spaces).
func DataDir() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(dir, "wtg", "spaces")
}

// spacePath returns the file path for the named space.
func spacePath(name string) string {
	return filepath.Join(DataDir(), name+".yaml")
}

// Load reads the state for the named space. Returns a wrapped fs.ErrNotExist
// if no state file exists for that name.
func Load(name string) (*Space, error) {
	data, err := os.ReadFile(spacePath(name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("space %q not found: %w", name, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("read space %q: %w", name, err)
	}
	var s Space
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse space %q: %w", name, err)
	}
	return &s, nil
}

// Save writes the space state to disk, creating the state directory if needed.
func Save(space *Space) error {
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := yaml.Marshal(space)
	if err != nil {
		return fmt.Errorf("marshal space %q: %w", space.Name, err)
	}
	if err := os.WriteFile(spacePath(space.Name), data, 0o644); err != nil {
		return fmt.Errorf("write space %q: %w", space.Name, err)
	}
	return nil
}

// List returns all spaces found in the state directory. A missing state
// directory is treated as empty (not an error).
func List() ([]*Space, error) {
	entries, err := os.ReadDir(DataDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state dir: %w", err)
	}

	var spaces []*Space
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".yaml")
		if !ok || entry.IsDir() {
			continue
		}
		s, err := Load(name)
		if err != nil {
			return nil, err
		}
		spaces = append(spaces, s)
	}
	return spaces, nil
}

// Delete removes the state file for the named space. Returns a wrapped
// fs.ErrNotExist if no state file exists.
func Delete(name string) error {
	err := os.Remove(spacePath(name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("space %q not found: %w", name, fs.ErrNotExist)
		}
		return fmt.Errorf("delete space %q: %w", name, err)
	}
	return nil
}
