package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/geoffamey/wtg/internal/state"
)

func saveSpace(t *testing.T, name, branch, path string, repoCount int) {
	t.Helper()
	sp := &state.Space{
		Name:      name,
		Branch:    branch,
		Path:      path,
		CreatedAt: time.Now(),
	}
	for i := range repoCount {
		sp.Repos = append(sp.Repos, state.RepoEntry{
			Name:         "repo" + string(rune('a'+i)),
			RepoPath:     "/repos/repo",
			WorktreePath: path + "/repo",
		})
	}
	if err := state.Save(sp); err != nil {
		t.Fatalf("saveSpace %q: %v", name, err)
	}
}

func TestRunSpaceList_Empty(t *testing.T) {
	isolateState(t)
	var out bytes.Buffer
	if err := RunSpaceList(&out); err != nil {
		t.Fatalf("RunSpaceList: %v", err)
	}
	if got := out.String(); got != "" {
		t.Errorf("expected empty output with no spaces, got %q", got)
	}
}

func TestRunSpaceList_Single(t *testing.T) {
	isolateState(t)
	saveSpace(t, "feat", "geoff/feat", "/workspaces/feat", 2)

	var out bytes.Buffer
	if err := RunSpaceList(&out); err != nil {
		t.Fatalf("RunSpaceList: %v", err)
	}
	got := out.String()
	for _, want := range []string{"feat", "geoff/feat", "/workspaces/feat", "2 repos"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q: %q", want, got)
		}
	}
}

func TestRunSpaceList_Multiple_Sorted(t *testing.T) {
	isolateState(t)
	saveSpace(t, "zebra", "geoff/zebra", "/workspaces/zebra", 1)
	saveSpace(t, "alpha", "geoff/alpha", "/workspaces/alpha", 3)
	saveSpace(t, "middle", "geoff/middle", "/workspaces/middle", 2)

	var out bytes.Buffer
	if err := RunSpaceList(&out); err != nil {
		t.Fatalf("RunSpaceList: %v", err)
	}
	got := out.String()
	posAlpha := strings.Index(got, "alpha")
	posMiddle := strings.Index(got, "middle")
	posZebra := strings.Index(got, "zebra")
	if posAlpha > posMiddle || posMiddle > posZebra {
		t.Errorf("spaces not sorted alphabetically:\n%s", got)
	}
}

func TestRunSpaceList_RepoCount(t *testing.T) {
	isolateState(t)
	saveSpace(t, "big", "main", "/workspaces/big", 5)

	var out bytes.Buffer
	if err := RunSpaceList(&out); err != nil {
		t.Fatalf("RunSpaceList: %v", err)
	}
	if !strings.Contains(out.String(), "5 repos") {
		t.Errorf("output missing repo count: %q", out.String())
	}
}
