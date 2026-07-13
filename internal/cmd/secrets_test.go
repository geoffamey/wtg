package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/config"
)

func TestValidateSecretPath(t *testing.T) {
	cases := []struct {
		path    string
		wantErr string
	}{
		{".env", ""},
		{"config/local.env", ""},
		{"", "empty path"},
		{".", "empty path"},
		{"/abs/path", "absolute"},
		{"../escape", "path escape"},
		{"foo/../../etc/passwd", "path escape"},
	}
	for _, tc := range cases {
		err := validateSecretPath(tc.path)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("validateSecretPath(%q): unexpected error %v", tc.path, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("validateSecretPath(%q): expected error containing %q", tc.path, tc.wantErr)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("validateSecretPath(%q): got %v, want substring %q", tc.path, err, tc.wantErr)
		}
	}
}

func TestSecretCopySteps_MissingSkipped(t *testing.T) {
	repo := t.TempDir()
	cfg := &config.Config{Always: config.AlwaysConfig{Secrets: []string{"missing.env"}}}
	tgt := &repoTarget{repoPath: repo, worktreePath: filepath.Join(t.TempDir(), "wt")}
	steps, err := secretCopySteps(cfg, tgt)
	if err != nil {
		t.Fatalf("secretCopySteps: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected no steps for missing file, got %d", len(steps))
	}
}

func TestSecretCopySteps_DirectoryRejected(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := &config.Config{Always: config.AlwaysConfig{Secrets: []string{"configs"}}}
	tgt := &repoTarget{repoPath: repo, worktreePath: filepath.Join(t.TempDir(), "wt")}
	_, err := secretCopySteps(cfg, tgt)
	if err == nil {
		t.Fatal("expected error for directory entry")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error should mention directory: %v", err)
	}
}

func TestSecretCopySteps_AbsoluteRejected(t *testing.T) {
	repo := t.TempDir()
	cfg := &config.Config{Always: config.AlwaysConfig{Secrets: []string{"/etc/passwd"}}}
	tgt := &repoTarget{repoPath: repo, worktreePath: filepath.Join(t.TempDir(), "wt")}
	_, err := secretCopySteps(cfg, tgt)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error should mention absolute: %v", err)
	}
}
