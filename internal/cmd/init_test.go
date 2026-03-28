package cmd

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/config"
)

// runInit is a helper that feeds input lines to RunInit and captures output.
func runInitWith(t *testing.T, configPath, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := RunInit(configPath, strings.NewReader(input), &out)
	return out.String(), err
}

func TestInit_WritesConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "wtg", "config.yaml")
	input := strings.Join([]string{
		"~/myrepos",   // discovery root
		"3",           // max scan depth
		"~/myspaces",  // workspace root
		"geoff/",      // branch prefix
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
	input := "~/repos\n\n~/workspaces\n\n"

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
	input := "~/repos\n2\n~/workspaces\n\n" // empty branch prefix

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
	input := "y\n~/repos\n2\n~/workspaces\n\n"
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

func TestInit_MissingDiscoveryRoot(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	// Empty discovery root → error.
	_, err := runInitWith(t, cfgPath, "\n")
	if err == nil {
		t.Fatal("expected error for empty discovery root")
	}
	if _, statErr := os.Stat(cfgPath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Error("config file should not have been created")
	}
}

func TestInit_InvalidMaxDepth(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := "~/repos\nnot-a-number\n"
	_, err := runInitWith(t, cfgPath, input)
	if err == nil {
		t.Fatal("expected error for invalid max depth")
	}
}

func TestInit_ZeroMaxDepth(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := "~/repos\n0\n"
	_, err := runInitWith(t, cfgPath, input)
	if err == nil {
		t.Fatal("expected error for zero max depth")
	}
}

func TestInit_MissingSpacesRoot(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	input := "~/repos\n2\n\n" // empty spaces root
	_, err := runInitWith(t, cfgPath, input)
	if err == nil {
		t.Fatal("expected error for empty spaces root")
	}
}
