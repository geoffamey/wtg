package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/saga"
)

// validateSecretPath rejects absolute paths, empty paths, and .. escapes.
func validateSecretPath(rel string) error {
	if rel == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(rel) {
		return fmt.Errorf("absolute path not allowed: %s", rel)
	}
	cleaned := filepath.Clean(rel)
	if cleaned == "." || cleaned == "" {
		return fmt.Errorf("empty path")
	}
	for _, p := range strings.Split(cleaned, string(filepath.Separator)) {
		if p == ".." {
			return fmt.Errorf("path escape not allowed: %s", rel)
		}
	}
	return nil
}

// copySecretFileStep copies src to dst (creating parent dirs) and removes dst on undo.
func copySecretFileStep(src, dst string) saga.Step {
	return saga.Step{
		Name: fmt.Sprintf("copy %s", filepath.Base(src)),
		Do: func(ctx context.Context) error {
			data, err := os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("read %s: %w", src, err)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
			}
			return os.WriteFile(dst, data, 0o644)
		},
		Undo: func(ctx context.Context) error {
			if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		},
	}
}

// secretCopySteps returns saga steps that copy always.secrets entries from the
// source repo into the worktree when present. Missing sources are skipped;
// directories and invalid paths are hard errors.
func secretCopySteps(cfg *config.Config, t *repoTarget) ([]saga.Step, error) {
	if cfg == nil || len(cfg.Always.Secrets) == 0 {
		return nil, nil
	}

	repoClean := filepath.Clean(t.repoPath)
	steps := make([]saga.Step, 0, len(cfg.Always.Secrets))
	for _, rel := range cfg.Always.Secrets {
		if err := validateSecretPath(rel); err != nil {
			return nil, fmt.Errorf("always.secrets: %w", err)
		}
		cleaned := filepath.Clean(rel)
		src := filepath.Join(repoClean, cleaned)
		if !pathUnderRoot(src, repoClean) {
			return nil, fmt.Errorf("always.secrets: path escape not allowed: %s", rel)
		}
		info, err := os.Stat(src)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // skip missing secrets in this repo
			}
			return nil, fmt.Errorf("always.secrets: %s: %w", rel, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("always.secrets: %s is a directory (files only)", rel)
		}
		dst := filepath.Join(t.worktreePath, cleaned)
		steps = append(steps, copySecretFileStep(src, dst))
	}
	return steps, nil
}

// pathUnderRoot reports whether path is equal to root or a path within it.
func pathUnderRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	prefix := root + string(filepath.Separator)
	return strings.HasPrefix(path, prefix)
}
