package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/config"
	"github.com/geoffamey/wtg/internal/state"
)

func testSpace() *state.Space {
	return &state.Space{
		Name:   "feat",
		Path:   "/spaces/feat",
		Branch: "gamey/feat",
		Repos: []state.RepoEntry{
			{Name: "cloud", WorktreePath: "/spaces/feat/cloud"},
			{Name: "console", WorktreePath: "/spaces/feat/console"},
		},
	}
}

// writeScript writes an executable script to a temp dir and returns its path.
func writeScript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hook")
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunSpaceScript_NoScript(t *testing.T) {
	var out bytes.Buffer
	runSpaceScript(&config.Config{}, "create", testSpace(), []string{"cloud"}, &out)
	if out.Len() != 0 {
		t.Fatalf("expected no output, got %q", out.String())
	}
}

func TestRunSpaceScript_SetsEnv(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "env.txt")
	script := writeScript(t, "#!/bin/sh\n{ env | grep '^WTG_'; } > "+dump+"\n")

	var out bytes.Buffer
	runSpaceScript(&config.Config{Always: config.AlwaysConfig{Run: script}},
		"add", testSpace(), []string{"console"}, &out)

	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("script did not run: %v (out: %q)", err, out.String())
	}
	env := string(data)
	for _, want := range []string{
		"WTG_SPACE_NAME=feat",
		"WTG_SPACE_ROOT=/spaces/feat",
		"WTG_SPACE_BRANCH=gamey/feat",
		"WTG_SPACE_EVENT=add",
		"WTG_EVENT_REPOS=console",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q; got:\n%s", want, env)
		}
	}
	// Multi-value vars are newline-joined; env line shows only the first.
	if !strings.Contains(env, "WTG_REPOS=cloud") || !strings.Contains(env, "WTG_REPO_PATHS=/spaces/feat/cloud") {
		t.Errorf("repo vars not set; got:\n%s", env)
	}
}

func TestRunSpaceScript_FailureWarns(t *testing.T) {
	script := writeScript(t, "#!/bin/sh\nexit 1\n")
	var out bytes.Buffer
	runSpaceScript(&config.Config{Always: config.AlwaysConfig{Run: script}},
		"delete", testSpace(), nil, &out)
	if !strings.Contains(out.String(), "always.run script failed") {
		t.Errorf("expected failure warning, got %q", out.String())
	}
}
