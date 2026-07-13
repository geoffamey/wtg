package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/geoffamey/wtg/internal/saga"
)

const wtgIncludeName = ".wtginclude"

// parseWtgInclude reads a .wtginclude file. Missing file returns nil, nil.
// Lines may have full-line or trailing # comments; blank lines are skipped.
func parseWtgInclude(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var paths []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}

// validateIncludePath rejects absolute paths, empty paths, and .. escapes.
func validateIncludePath(rel string) error {
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

// copyIncludeFileStep copies src to dst (creating parent dirs) and removes dst on undo.
func copyIncludeFileStep(src, dst string) saga.Step {
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

// includeCopySteps reads .wtginclude from the source repo and returns saga steps
// that copy each listed file into the worktree, preserving relative paths.
// Validates all entries and source files before returning any steps.
func includeCopySteps(t *repoTarget) ([]saga.Step, error) {
	includePath := filepath.Join(t.repoPath, wtgIncludeName)
	rels, err := parseWtgInclude(includePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", includePath, err)
	}
	if len(rels) == 0 {
		return nil, nil
	}

	repoClean := filepath.Clean(t.repoPath)
	steps := make([]saga.Step, 0, len(rels))
	for _, rel := range rels {
		if err := validateIncludePath(rel); err != nil {
			return nil, fmt.Errorf("%s: %w", includePath, err)
		}
		cleaned := filepath.Clean(rel)
		src := filepath.Join(repoClean, cleaned)
		if !pathUnderRoot(src, repoClean) {
			return nil, fmt.Errorf("%s: path escape not allowed: %s", includePath, rel)
		}
		info, err := os.Stat(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", includePath, rel, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s: %s is a directory (files only)", includePath, rel)
		}
		dst := filepath.Join(t.worktreePath, cleaned)
		steps = append(steps, copyIncludeFileStep(src, dst))
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
