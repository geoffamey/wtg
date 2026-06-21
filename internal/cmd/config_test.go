package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/geoffamey/wtg/internal/config"
)

func TestConfigInit_WritesTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wtg", "config.toml")
	var out bytes.Buffer
	if err := runConfigInit(path, false, &out); err != nil {
		t.Fatalf("runConfigInit: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if string(data) != configTemplate {
		t.Errorf("written content does not match template")
	}
	if !strings.Contains(out.String(), path) {
		t.Errorf("output should mention path, got %q", out.String())
	}
	// The required keys ship uncommented, so a fresh scaffold loads with them set.
	cfg, err := config.Load(path)
	if err != nil {
		t.Errorf("Load scaffolded config: %v", err)
	}
	if cfg.Discovery.RootDir == "" {
		t.Error("scaffold should set discovery.root_dir")
	}
	if cfg.Spaces.RootDir == "" {
		t.Error("scaffold should set spaces.root_dir")
	}
}

func TestConfigInit_RefusesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runConfigInit(path, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should name the path, got %q", err.Error())
	}
}

func TestConfigInit_ForceOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runConfigInit(path, true, &bytes.Buffer{}); err != nil {
		t.Fatalf("runConfigInit --force: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != configTemplate {
		t.Errorf("force did not overwrite with template")
	}
}

func TestConfigInit_Stdout_NeverRefuses(t *testing.T) {
	var out bytes.Buffer
	// "-" writes to out and must not refuse even though the target "exists" notionally.
	if err := runConfigInit("-", false, &out); err != nil {
		t.Fatalf("runConfigInit -: %v", err)
	}
	if out.String() != configTemplate {
		t.Errorf("stdout output should be the template")
	}
}

func TestConfigPrint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("hello = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, note bytes.Buffer
	if err := runConfigPrint(path, &out, &note); err != nil {
		t.Fatalf("runConfigPrint: %v", err)
	}
	if out.String() != "hello = 1\n" {
		t.Errorf("contents: got %q", out.String())
	}
	if note.Len() != 0 {
		t.Errorf("note should be empty when file exists, got %q", note.String())
	}
}

func TestConfigPrint_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such.toml")
	var out, note bytes.Buffer
	if err := runConfigPrint(path, &out, &note); err != nil {
		t.Fatalf("runConfigPrint missing: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty for missing file, got %q", out.String())
	}
	if !strings.Contains(note.String(), path) {
		t.Errorf("note should name the path, got %q", note.String())
	}
}

// TestConfigTemplate_CoversAllKeys guards against the hand-written template drifting
// from the Config struct: every koanf key (section and leaf) must appear in it.
func TestConfigTemplate_CoversAllKeys(t *testing.T) {
	for _, key := range koanfTags(reflect.TypeOf(config.Config{})) {
		if !strings.Contains(configTemplate, key) {
			t.Errorf("config template is missing key %q", key)
		}
	}
}

func koanfTags(t reflect.Type) []string {
	var tags []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := strings.Split(f.Tag.Get("koanf"), ",")[0]
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
		if f.Type.Kind() == reflect.Struct {
			tags = append(tags, koanfTags(f.Type)...)
		}
	}
	return tags
}
