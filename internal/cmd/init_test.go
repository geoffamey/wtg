package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/geoffamey/wtg/internal/config"
)

// runInitWith is a helper that feeds input lines to RunInit and captures output.
func runInitWith(t *testing.T, configPath, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := RunInit(configPath, strings.NewReader(input), &out, false, InitOverrides{})
	return out.String(), err
}

// runInitDefaults runs RunInit with --defaults mode.
func runInitDefaults(t *testing.T, configPath string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := RunInit(configPath, strings.NewReader(""), &out, true, InitOverrides{})
	return out.String(), err
}

// ptr returns a pointer to s, for building InitOverrides in tests.
func ptr(s string) *string { return &s }

func TestInit_WritesConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "wtg", "config.yaml")
	input := strings.Join([]string{
		"~/myrepos",  // discovery root
		"3",          // max scan depth
		"~/myspaces", // workspace root
		"geoff/",     // branch prefix
		"",           // always repos (none)
		"",           // always files (none)
	}, "\n") + "\n"

	out, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	if !strings.Contains(out, cfgPath) {
		t.Errorf("output should mention config path, got: %q", out)
	}

	// Reload via config.Load to verify round-trip.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load after init: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != filepath.Join(home, "myrepos") {
		t.Errorf("Discovery.RootDir: %q", cfg.Discovery.RootDir)
	}
	if cfg.Discovery.MaxDepth != 3 {
		t.Errorf("MaxDepth: got %d, want 3", cfg.Discovery.MaxDepth)
	}
	if cfg.Spaces.RootDir != filepath.Join(home, "myspaces") {
		t.Errorf("Spaces.RootDir: %q", cfg.Spaces.RootDir)
	}
	if cfg.Git.BranchPrefix != "geoff/" {
		t.Errorf("BranchPrefix: %q", cfg.Git.BranchPrefix)
	}
}

func TestInit_DefaultMaxDepth(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Empty line for max depth → use default (2).
	input := "~/repos\n\n~/workspaces\n\n\n\n"

	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Discovery.MaxDepth != 2 {
		t.Errorf("MaxDepth: got %d, want 2", cfg.Discovery.MaxDepth)
	}
}

func TestInit_NoBranchPrefix(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := "~/repos\n2\n~/workspaces\n\n\n\n" // empty branch prefix, no always entries

	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Git.BranchPrefix != "" {
		t.Errorf("BranchPrefix: got %q, want empty", cfg.Git.BranchPrefix)
	}
}

func TestInit_ExistingConfig_Overwrite(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Write a placeholder config.
	if err := os.WriteFile(cfgPath, []byte("existing: true\n"), 0o644); err != nil {
		t.Fatalf("write placeholder: %v", err)
	}

	// "y" to confirm overwrite, then valid inputs.
	input := "y\n~/repos\n2\n~/workspaces\n\n\n\n"
	out, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	if !strings.Contains(out, "Config written") {
		t.Errorf("expected success message, got: %q", out)
	}
	// Verify the file was actually overwritten with valid config.
	if _, err := config.Load(cfgPath); err != nil {
		t.Errorf("config.Load after overwrite: %v", err)
	}
}

func TestInit_ExistingConfig_Decline(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("existing: true\n")
	if err := os.WriteFile(cfgPath, original, 0o644); err != nil {
		t.Fatalf("write placeholder: %v", err)
	}

	out, err := runInitWith(t, cfgPath, "n\n")
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	if !strings.Contains(out, "Aborted") {
		t.Errorf("expected abort message, got: %q", out)
	}
	// Original file must be untouched.
	got, _ := os.ReadFile(cfgPath)
	if !bytes.Equal(got, original) {
		t.Error("original config was modified after decline")
	}
}

func TestInit_DefaultDiscoveryRoot(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Empty discovery root → use default (~/).
	input := "\n2\n~/workspaces\n\n\n\n"

	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != home {
		t.Errorf("Discovery.RootDir: got %q, want %q", cfg.Discovery.RootDir, home)
	}
}

func TestInit_DefaultSpacesRoot(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Empty spaces root → use default (~/spaces).
	input := "~/repos\n2\n\n\n\n\n"

	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "spaces")
	if cfg.Spaces.RootDir != want {
		t.Errorf("Spaces.RootDir: got %q, want %q", cfg.Spaces.RootDir, want)
	}
}

func TestInit_InvalidMaxDepth(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := "~/repos\nnot-a-number\n\n\n\n\n"
	_, err := runInitWith(t, cfgPath, input)
	if err == nil {
		t.Fatal("expected error for invalid max depth")
	}
}

func TestInit_ZeroMaxDepth(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := "~/repos\n0\n\n\n\n\n"
	_, err := runInitWith(t, cfgPath, input)
	if err == nil {
		t.Fatal("expected error for zero max depth")
	}
}

func TestInit_Defaults(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "wtg", "config.yaml")

	out, err := runInitDefaults(t, cfgPath)
	if err != nil {
		t.Fatalf("RunInit --defaults: %v", err)
	}
	if !strings.Contains(out, "Config written") {
		t.Errorf("expected success message, got: %q", out)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != home {
		t.Errorf("Discovery.RootDir: got %q, want %q", cfg.Discovery.RootDir, home)
	}
	if cfg.Discovery.MaxDepth != 2 {
		t.Errorf("MaxDepth: got %d, want 2", cfg.Discovery.MaxDepth)
	}
	if cfg.Spaces.RootDir != filepath.Join(home, "spaces") {
		t.Errorf("Spaces.RootDir: got %q, want %q", cfg.Spaces.RootDir, filepath.Join(home, "spaces"))
	}
	if cfg.Git.BranchPrefix != "" {
		t.Errorf("BranchPrefix: got %q, want empty", cfg.Git.BranchPrefix)
	}
}

func TestInit_Defaults_OverwritesExistingConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("existing: true\n"), 0o644); err != nil {
		t.Fatalf("write placeholder: %v", err)
	}

	_, err := runInitDefaults(t, cfgPath)
	if err != nil {
		t.Fatalf("RunInit --defaults: %v", err)
	}
	// Verify the file was overwritten with valid config.
	if _, err := config.Load(cfgPath); err != nil {
		t.Errorf("config.Load after defaults overwrite: %v", err)
	}
}

// --- always.repos / always.files prompts ---

func TestInit_AlwaysReposAndFiles(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := strings.Join([]string{
		"~/repos",             // discovery root
		"2",                   // max depth
		"~/spaces",            // workspace root
		"",                    // branch prefix
		"docs, shared",        // always repos
		"~/.config/CLAUDE.md", // always files
	}, "\n") + "\n"

	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.Always.Repos) != 2 || cfg.Always.Repos[0] != "docs" || cfg.Always.Repos[1] != "shared" {
		t.Errorf("Always.Repos: got %v", cfg.Always.Repos)
	}
	if len(cfg.Always.Files) != 1 {
		t.Fatalf("Always.Files: got %v", cfg.Always.Files)
	}
	home, _ := os.UserHomeDir()
	wantFile := filepath.Join(home, ".config/CLAUDE.md")
	if cfg.Always.Files[0] != wantFile {
		t.Errorf("Always.Files[0]: got %q, want %q", cfg.Always.Files[0], wantFile)
	}
}

func TestInit_AlwaysRun(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := strings.Join([]string{
		"~/repos",            // discovery root
		"2",                  // max depth
		"~/spaces",           // workspace root
		"",                   // branch prefix
		"",                   // always repos
		"",                   // always files
		"~/bin/wtg-on-event", // always run
	}, "\n") + "\n"

	if _, err := runInitWith(t, cfgPath, input); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "bin/wtg-on-event") // config.Load expands ~/
	if cfg.Always.Run != want {
		t.Errorf("Always.Run: got %q, want %q", cfg.Always.Run, want)
	}
}

func TestInit_AlwaysRun_SentinelClears(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	existing := strings.Join([]string{
		"discovery:",
		"  root_dir: ~/repos",
		"  max_depth: 2",
		"spaces:",
		"  root_dir: ~/spaces",
		"always:",
		"  run: ~/bin/wtg-on-event",
	}, "\n") + "\n"
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	// Confirm overwrite, enter through everything, type "-" at the run prompt.
	input := "y\n\n\n\n\n\n\n-\n"
	if _, err := runInitWith(t, cfgPath, input); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Always.Run != "" {
		t.Errorf("Always.Run should be empty after -, got %q", cfg.Always.Run)
	}
}

func TestInit_ExistingConfig_UsedAsDefaults(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Write an existing config with non-default values including always entries.
	existing := strings.Join([]string{
		"discovery:",
		"  root_dir: ~/myrepos",
		"  max_depth: 4",
		"spaces:",
		"  root_dir: ~/myspaces",
		"git:",
		"  branch_prefix: team/",
		"always:",
		"  repos: [docs]",
		"  files: [~/.config/wtg/CLAUDE.md]",
	}, "\n") + "\n"
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	// Confirm overwrite, then press enter on every prompt to keep all current values.
	input := "y\n\n\n\n\n\n\n"
	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != filepath.Join(home, "myrepos") {
		t.Errorf("Discovery.RootDir: got %q", cfg.Discovery.RootDir)
	}
	if cfg.Discovery.MaxDepth != 4 {
		t.Errorf("MaxDepth: got %d, want 4", cfg.Discovery.MaxDepth)
	}
	if cfg.Spaces.RootDir != filepath.Join(home, "myspaces") {
		t.Errorf("Spaces.RootDir: got %q", cfg.Spaces.RootDir)
	}
	if cfg.Git.BranchPrefix != "team/" {
		t.Errorf("BranchPrefix: got %q, want team/", cfg.Git.BranchPrefix)
	}
	if len(cfg.Always.Repos) != 1 || cfg.Always.Repos[0] != "docs" {
		t.Errorf("Always.Repos: got %v, want [docs]", cfg.Always.Repos)
	}
	if len(cfg.Always.Files) != 1 {
		t.Fatalf("Always.Files: got %v", cfg.Always.Files)
	}
	wantFile := filepath.Join(home, ".config/wtg/CLAUDE.md")
	if cfg.Always.Files[0] != wantFile {
		t.Errorf("Always.Files[0]: got %q, want %q", cfg.Always.Files[0], wantFile)
	}
}

func TestInit_AlwaysRepos_EmptyInput_ClearsExisting(t *testing.T) {
	// Explicitly entering a single space (then trimmed to "") clears the list.
	// More practically: the user types nothing and the default keeps the value —
	// this test verifies that entering whitespace does clear the field.
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := strings.Join([]string{
		"~/repos",
		"2",
		"~/spaces",
		"",
		" ", // whitespace-only → treated as empty → no always repos
		"",
	}, "\n") + "\n"

	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.Always.Repos) != 0 {
		t.Errorf("Always.Repos should be empty, got %v", cfg.Always.Repos)
	}
}

func TestInit_AlwaysSentinel_ClearsExistingValues(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	existing := strings.Join([]string{
		"discovery:",
		"  root_dir: ~/repos",
		"  max_depth: 2",
		"spaces:",
		"  root_dir: ~/spaces",
		"always:",
		"  repos: [docs]",
		"  files: [~/.config/CLAUDE.md]",
	}, "\n") + "\n"
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	// Confirm overwrite, press enter on everything except always fields where we type "-".
	input := "y\n\n\n\n\n-\n-\n"
	_, err := runInitWith(t, cfgPath, input)
	if err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.Always.Repos) != 0 {
		t.Errorf("Always.Repos should be empty after -, got %v", cfg.Always.Repos)
	}
	if len(cfg.Always.Files) != 0 {
		t.Errorf("Always.Files should be empty after -, got %v", cfg.Always.Files)
	}
}

// --- non-interactive overrides / flags ---

func TestInit_Overrides_NonInteractive(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Empty stdin: every value must come from the overrides, not a prompt.
	var out bytes.Buffer
	ov := InitOverrides{
		DiscoveryRoot: ptr("~/over-repos"),
		MaxDepth:      ptr("4"),
		SpacesRoot:    ptr("~/over-spaces"),
		BranchPrefix:  ptr("ovr/"),
		AlwaysRepos:   ptr("docs, shared"),
		AlwaysFiles:   ptr("~/.config/X.md"),
		AlwaysRun:     ptr("~/bin/hook"),
	}
	if err := RunInit(cfgPath, strings.NewReader(""), &out, false, ov); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != filepath.Join(home, "over-repos") {
		t.Errorf("Discovery.RootDir: %q", cfg.Discovery.RootDir)
	}
	if cfg.Discovery.MaxDepth != 4 {
		t.Errorf("MaxDepth: %d", cfg.Discovery.MaxDepth)
	}
	if cfg.Spaces.RootDir != filepath.Join(home, "over-spaces") {
		t.Errorf("Spaces.RootDir: %q", cfg.Spaces.RootDir)
	}
	if cfg.Git.BranchPrefix != "ovr/" {
		t.Errorf("BranchPrefix: %q", cfg.Git.BranchPrefix)
	}
	if len(cfg.Always.Repos) != 2 || cfg.Always.Repos[0] != "docs" {
		t.Errorf("Always.Repos: %v", cfg.Always.Repos)
	}
	if cfg.Always.Run != filepath.Join(home, "bin/hook") {
		t.Errorf("Always.Run: %q", cfg.Always.Run)
	}
}

func TestInit_Overrides_SkipOnlySetPrompts(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Override discovery root and branch prefix; the remaining five prompts must
	// still fire in order and consume these five input lines.
	input := strings.Join([]string{
		"3",            // max depth
		"~/in-spaces",  // workspace root
		"a, b",         // always repos
		"~/in-file.md", // always files
		"~/in-run",     // always run
	}, "\n") + "\n"
	var out bytes.Buffer
	ov := InitOverrides{DiscoveryRoot: ptr("~/ov-repos"), BranchPrefix: ptr("ov/")}
	if err := RunInit(cfgPath, strings.NewReader(input), &out, false, ov); err != nil {
		t.Fatalf("RunInit: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != filepath.Join(home, "ov-repos") { // from override
		t.Errorf("Discovery.RootDir: %q", cfg.Discovery.RootDir)
	}
	if cfg.Git.BranchPrefix != "ov/" { // from override
		t.Errorf("BranchPrefix: %q", cfg.Git.BranchPrefix)
	}
	if cfg.Discovery.MaxDepth != 3 { // from prompt
		t.Errorf("MaxDepth: %d", cfg.Discovery.MaxDepth)
	}
	if cfg.Spaces.RootDir != filepath.Join(home, "in-spaces") { // from prompt
		t.Errorf("Spaces.RootDir: %q", cfg.Spaces.RootDir)
	}
	if len(cfg.Always.Repos) != 2 || cfg.Always.Repos[1] != "b" { // from prompt
		t.Errorf("Always.Repos: %v", cfg.Always.Repos)
	}
	if cfg.Always.Run != filepath.Join(home, "in-run") { // from prompt
		t.Errorf("Always.Run: %q", cfg.Always.Run)
	}
}

func TestInit_Override_ClearsAlways(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	existing := strings.Join([]string{
		"discovery:",
		"  root_dir: ~/repos",
		"  max_depth: 2",
		"spaces:",
		"  root_dir: ~/spaces",
		"always:",
		"  repos: [docs]",
		"  run: ~/bin/hook",
	}, "\n") + "\n"
	if err := os.WriteFile(cfgPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	var out bytes.Buffer
	ov := InitOverrides{AlwaysRepos: ptr("-"), AlwaysRun: ptr("-")}
	// useDefaults skips the overwrite confirmation; overrides clear the two fields.
	if err := RunInit(cfgPath, strings.NewReader(""), &out, true, ov); err != nil {
		t.Fatalf("RunInit: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if len(cfg.Always.Repos) != 0 {
		t.Errorf("Always.Repos should be cleared, got %v", cfg.Always.Repos)
	}
	if cfg.Always.Run != "" {
		t.Errorf("Always.Run should be cleared, got %q", cfg.Always.Run)
	}
}

func TestInit_Override_InvalidMaxDepth(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var out bytes.Buffer
	ov := InitOverrides{MaxDepth: ptr("not-a-number")}
	if err := RunInit(cfgPath, strings.NewReader(""), &out, true, ov); err == nil {
		t.Fatal("expected error for invalid --max-depth override")
	}
}

// initApp wraps InitCommand in a root with the global --config flag.
func initApp() *cli.Command {
	return &cli.Command{
		Name:     "wtg",
		Flags:    []cli.Flag{&cli.StringFlag{Name: "config"}},
		Commands: []*cli.Command{InitCommand()},
	}
}

func TestInit_Flags_Wired(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	err := initApp().Run(context.Background(), []string{
		"wtg", "--config", cfgPath, "init", "--defaults",
		"--repo-dir", "~/cli-repos", "--branch-prefix", "cli/", "--max-depth", "5",
		"--always-run", "~/cli-hook",
	})
	if err != nil {
		t.Fatalf("init run: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	home, _ := os.UserHomeDir()
	if cfg.Discovery.RootDir != filepath.Join(home, "cli-repos") {
		t.Errorf("Discovery.RootDir: %q", cfg.Discovery.RootDir)
	}
	if cfg.Discovery.MaxDepth != 5 {
		t.Errorf("MaxDepth: %d", cfg.Discovery.MaxDepth)
	}
	if cfg.Git.BranchPrefix != "cli/" {
		t.Errorf("BranchPrefix: %q", cfg.Git.BranchPrefix)
	}
	if cfg.Always.Run != filepath.Join(home, "cli-hook") {
		t.Errorf("Always.Run: %q", cfg.Always.Run)
	}
	// Unset flags fall back to factory defaults under --defaults.
	if cfg.Spaces.RootDir != filepath.Join(home, "spaces") {
		t.Errorf("Spaces.RootDir should be default, got %q", cfg.Spaces.RootDir)
	}
}

// --- joinSlice / splitSlice ---

func TestJoinSlice(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"docs"}, "docs"},
		{[]string{"docs", "shared"}, "docs, shared"},
	}
	for _, c := range cases {
		if got := joinSlice(c.in); got != c.want {
			t.Errorf("joinSlice(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitSlice(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"-", nil},
		{"  -  ", nil},
		{"docs", []string{"docs"}},
		{"docs, shared", []string{"docs", "shared"}},
		{"docs,shared", []string{"docs", "shared"}},
		{" docs , shared ", []string{"docs", "shared"}},
		{",,,", nil},
	}
	for _, c := range cases {
		got := splitSlice(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitSlice(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitSlice(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}
