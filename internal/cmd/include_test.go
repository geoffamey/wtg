package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWtgInclude_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".wtginclude")
	content := `
# full-line comment
.env
config/local.env  # trailing comment

# another
.secret
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := parseWtgInclude(path)
	if err != nil {
		t.Fatalf("parseWtgInclude: %v", err)
	}
	want := []string{".env", "config/local.env", ".secret"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseWtgInclude_MissingFile(t *testing.T) {
	got, err := parseWtgInclude(filepath.Join(t.TempDir(), ".wtginclude"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil paths, got %v", got)
	}
}

func TestValidateIncludePath(t *testing.T) {
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
		err := validateIncludePath(tc.path)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("validateIncludePath(%q): unexpected error %v", tc.path, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("validateIncludePath(%q): expected error containing %q", tc.path, tc.wantErr)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("validateIncludePath(%q): got %v, want substring %q", tc.path, err, tc.wantErr)
		}
	}
}

func TestIncludeCopySteps_DirectoryRejected(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	include := filepath.Join(repo, ".wtginclude")
	if err := os.WriteFile(include, []byte("configs\n"), 0o644); err != nil {
		t.Fatalf("write include: %v", err)
	}
	tgt := &repoTarget{repoPath: repo, worktreePath: filepath.Join(t.TempDir(), "wt")}
	_, err := includeCopySteps(tgt)
	if err == nil {
		t.Fatal("expected error for directory entry")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error should mention directory: %v", err)
	}
}

func TestIncludeCopySteps_AbsoluteRejected(t *testing.T) {
	repo := t.TempDir()
	include := filepath.Join(repo, ".wtginclude")
	if err := os.WriteFile(include, []byte("/etc/passwd\n"), 0o644); err != nil {
		t.Fatalf("write include: %v", err)
	}
	tgt := &repoTarget{repoPath: repo, worktreePath: filepath.Join(t.TempDir(), "wt")}
	_, err := includeCopySteps(tgt)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error should mention absolute: %v", err)
	}
}
