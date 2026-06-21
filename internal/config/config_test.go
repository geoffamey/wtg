package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(nonexistentPath(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Discovery.MaxDepth != 2 {
		t.Errorf("MaxDepth: got %d, want 2", cfg.Discovery.MaxDepth)
	}
	if cfg.Discovery.RootDir != expandTilde("~/repos") {
		t.Errorf("RootDir: got %q, want default ~/repos", cfg.Discovery.RootDir)
	}
	if cfg.Spaces.RootDir != expandTilde("~/spaces") {
		t.Errorf("Spaces.RootDir: got %q, want default ~/spaces", cfg.Spaces.RootDir)
	}
}

func TestLoad_File(t *testing.T) {
	path := writeConfig(t, `
discovery:
  root_dir: ~/repos
  max_depth: 3
spaces:
  root_dir: ~/workspaces
git:
  branch_prefix: "me/"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Discovery.MaxDepth != 3 {
		t.Errorf("MaxDepth: got %d, want 3", cfg.Discovery.MaxDepth)
	}
	if cfg.Spaces.RootDir != expandTilde("~/workspaces") {
		t.Errorf("Spaces.RootDir: got %q", cfg.Spaces.RootDir)
	}
	if cfg.Git.BranchPrefix != "me/" {
		t.Errorf("BranchPrefix: got %q, want %q", cfg.Git.BranchPrefix, "me/")
	}
}

func TestLoad_FilePartial_DefaultsPreserved(t *testing.T) {
	// File only overrides max_depth; default for other fields stays.
	path := writeConfig(t, `
discovery:
  max_depth: 5
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Discovery.MaxDepth != 5 {
		t.Errorf("MaxDepth: got %d, want 5", cfg.Discovery.MaxDepth)
	}
	if cfg.Discovery.RootDir != expandTilde("~/repos") {
		t.Errorf("RootDir should fall back to default ~/repos, got %q", cfg.Discovery.RootDir)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	path := writeConfig(t, `
discovery:
  max_depth: 3
git:
  branch_prefix: "file/"
`)

	t.Setenv("WTG_GIT_BRANCH_PREFIX", "env/")
	t.Setenv("WTG_DISCOVERY_MAX_DEPTH", "7")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Git.BranchPrefix != "env/" {
		t.Errorf("BranchPrefix: got %q, want %q", cfg.Git.BranchPrefix, "env/")
	}
	if cfg.Discovery.MaxDepth != 7 {
		t.Errorf("MaxDepth: got %d, want 7", cfg.Discovery.MaxDepth)
	}
}

func TestLoad_TildeExpansion(t *testing.T) {
	path := writeConfig(t, `
discovery:
  root_dir: ~/myrepos
spaces:
  root_dir: ~/myspaces
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != filepath.Join(home, "myrepos") {
		t.Errorf("Discovery.RootDir: got %q", cfg.Discovery.RootDir)
	}
	if cfg.Spaces.RootDir != filepath.Join(home, "myspaces") {
		t.Errorf("Spaces.RootDir: got %q", cfg.Spaces.RootDir)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeConfig(t, `{not valid yaml: [`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_MissingFile_NotAnError(t *testing.T) {
	_, err := Load(nonexistentPath(t))
	if err != nil {
		t.Fatalf("missing config file should not be an error, got: %v", err)
	}
}

func TestDefaultPath_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	if got := DefaultPath(); got != "/custom/config/wtg/config.toml" {
		t.Errorf("DefaultPath: got %q", got)
	}
}

func TestDefaultPath_FallbackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	got := DefaultPath()
	if got == "" {
		t.Error("DefaultPath should not be empty")
	}
	if filepath.Base(got) != "config.toml" {
		t.Errorf("DefaultPath should end in config.toml, got %q", got)
	}
}

func TestLoad_TOMLFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[discovery]
root_dir = "~/repos"
max_depth = 4
[git]
branch_prefix = "me/"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load TOML: %v", err)
	}
	if cfg.Discovery.MaxDepth != 4 {
		t.Errorf("MaxDepth: got %d, want 4", cfg.Discovery.MaxDepth)
	}
	if cfg.Git.BranchPrefix != "me/" {
		t.Errorf("BranchPrefix: got %q", cfg.Git.BranchPrefix)
	}
}

func TestResolvePath(t *testing.T) {
	// Explicit path wins unchanged.
	if got := ResolvePath("/some/where.toml"); got != "/some/where.toml" {
		t.Errorf("explicit: got %q", got)
	}

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	wtgDir := filepath.Join(dir, "wtg")
	if err := os.MkdirAll(wtgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := filepath.Join(wtgDir, "config.toml")
	yamlPath := filepath.Join(wtgDir, "config.yaml")

	// Neither exists → default toml path.
	if got := ResolvePath(""); got != toml {
		t.Errorf("none: got %q, want %q", got, toml)
	}
	// Only yaml exists → legacy yaml.
	if err := os.WriteFile(yamlPath, []byte("discovery:\n  max_depth: 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolvePath(""); got != yamlPath {
		t.Errorf("legacy yaml: got %q, want %q", got, yamlPath)
	}
	// toml present → toml wins over yaml.
	if err := os.WriteFile(toml, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolvePath(""); got != toml {
		t.Errorf("toml precedence: got %q, want %q", got, toml)
	}
}

func TestLoad_LegacyYAMLViaResolve(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	wtgDir := filepath.Join(dir, "wtg")
	if err := os.MkdirAll(wtgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Only a legacy config.yaml exists; Load("") must find and parse it.
	if err := os.WriteFile(filepath.Join(wtgDir, "config.yaml"), []byte("discovery:\n  max_depth: 6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Discovery.MaxDepth != 6 {
		t.Errorf("MaxDepth: got %d, want 6", cfg.Discovery.MaxDepth)
	}
}

func TestLoad_UsesDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// No file written — should succeed with defaults.
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with empty path: %v", err)
	}
	if cfg.Discovery.MaxDepth != 2 {
		t.Errorf("MaxDepth: got %d, want 2", cfg.Discovery.MaxDepth)
	}
}

func TestLoad_AlwaysSection(t *testing.T) {
	path := writeConfig(t, `
always:
  repos: [docs, shared]
  files: [~/.config/wtg/CLAUDE.md]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Always.Repos) != 2 || cfg.Always.Repos[0] != "docs" || cfg.Always.Repos[1] != "shared" {
		t.Errorf("Always.Repos: got %v", cfg.Always.Repos)
	}
	if len(cfg.Always.Files) != 1 {
		t.Fatalf("Always.Files: got %v", cfg.Always.Files)
	}
	home, _ := os.UserHomeDir()
	wantFile := filepath.Join(home, ".config/wtg/CLAUDE.md")
	if cfg.Always.Files[0] != wantFile {
		t.Errorf("Always.Files[0]: got %q, want %q", cfg.Always.Files[0], wantFile)
	}
}

func TestLoad_AlwaysEmpty_ByDefault(t *testing.T) {
	cfg, err := Load(nonexistentPath(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Always.Repos) != 0 {
		t.Errorf("Always.Repos should be empty by default, got %v", cfg.Always.Repos)
	}
	if len(cfg.Always.Files) != 0 {
		t.Errorf("Always.Files should be empty by default, got %v", cfg.Always.Files)
	}
}

// writeConfig writes YAML content to a temp file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "wtg-config-*.yaml")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close config: %v", err)
	}
	return f.Name()
}

// nonexistentPath returns a path that does not exist inside a temp dir.
func nonexistentPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "no-such-file.yaml")
}
