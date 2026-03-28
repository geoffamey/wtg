package state

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"
)

// override DataDir to a temp dir for all tests in this package.
func withTempDataDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func exampleSpace() *Space {
	return &Space{
		Name:      "myfeature",
		Path:      "/Users/geoff/workspaces/myfeature",
		Branch:    "geoff/myfeature",
		CreatedAt: time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
		Repos: []RepoEntry{
			{
				Name:         "api",
				RepoPath:     "/Users/geoff/repos/api",
				WorktreePath: "/Users/geoff/workspaces/myfeature/api",
			},
			{
				Name:         "myorg/frontend",
				RepoPath:     "/Users/geoff/repos/myorg/frontend",
				WorktreePath: "/Users/geoff/workspaces/myfeature/myorg/frontend",
			},
		},
		GoWorkspace: true,
	}
}

func TestSaveAndLoad(t *testing.T) {
	withTempDataDir(t)
	want := exampleSpace()

	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(want.Name)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Name != want.Name {
		t.Errorf("Name: got %q, want %q", got.Name, want.Name)
	}
	if got.Branch != want.Branch {
		t.Errorf("Branch: got %q, want %q", got.Branch, want.Branch)
	}
	if got.Path != want.Path {
		t.Errorf("Path: got %q, want %q", got.Path, want.Path)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if got.GoWorkspace != want.GoWorkspace {
		t.Errorf("GoWorkspace: got %v, want %v", got.GoWorkspace, want.GoWorkspace)
	}
	if len(got.Repos) != len(want.Repos) {
		t.Fatalf("Repos: got %d, want %d", len(got.Repos), len(want.Repos))
	}
	for i, r := range got.Repos {
		w := want.Repos[i]
		if r.Name != w.Name || r.RepoPath != w.RepoPath || r.WorktreePath != w.WorktreePath {
			t.Errorf("Repos[%d]: got %+v, want %+v", i, r, w)
		}
	}
}

func TestLoad_NotFound(t *testing.T) {
	withTempDataDir(t)
	_, err := Load("no-such-space")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	withTempDataDir(t)
	// Write a corrupt file directly.
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(spacePath("bad"), []byte("{not valid yaml: ["), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Load("bad")
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestList_Empty(t *testing.T) {
	withTempDataDir(t)
	spaces, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(spaces) != 0 {
		t.Errorf("got %d spaces, want 0", len(spaces))
	}
}

func TestList_MissingDir(t *testing.T) {
	// XDG_DATA_HOME points to a non-existent subdir.
	t.Setenv("XDG_DATA_HOME", t.TempDir()+"/nonexistent")
	spaces, err := List()
	if err != nil {
		t.Fatalf("List with missing dir: %v", err)
	}
	if len(spaces) != 0 {
		t.Errorf("got %d spaces, want 0", len(spaces))
	}
}

func TestList_Multiple(t *testing.T) {
	withTempDataDir(t)

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		s := exampleSpace()
		s.Name = name
		if err := Save(s); err != nil {
			t.Fatalf("Save %q: %v", name, err)
		}
	}

	spaces, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(spaces) != len(names) {
		t.Fatalf("got %d spaces, want %d", len(spaces), len(names))
	}
}

func TestDelete(t *testing.T) {
	withTempDataDir(t)
	s := exampleSpace()
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Delete(s.Name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := Load(s.Name)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after delete, expected fs.ErrNotExist, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	withTempDataDir(t)
	err := Delete("no-such-space")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}
