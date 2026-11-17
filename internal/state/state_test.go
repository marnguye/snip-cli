package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultsAndExplicitSelection(t *testing.T) {
	config := t.TempDir()
	legacy := filepath.Join(t.TempDir(), "snippets")
	resolved, err := Resolve(config, legacy)
	if err != nil || resolved.Backend != TOML || resolved.Explicit {
		t.Fatalf("fresh=%+v err=%v", resolved, err)
	}
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "work.md"), []byte("## title\nprintf ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err = Resolve(config, legacy)
	if err != nil || resolved.Backend != Legacy {
		t.Fatalf("legacy=%+v err=%v", resolved, err)
	}
	if err := Save(config, TOML); err != nil {
		t.Fatal(err)
	}
	resolved, err = Resolve(config, legacy)
	if err != nil || resolved.Backend != TOML || !resolved.Explicit {
		t.Fatalf("explicit=%+v err=%v", resolved, err)
	}
}

func TestResolveRejectsInvalidState(t *testing.T) {
	config := t.TempDir()
	if err := os.WriteFile(filepath.Join(config, "state.toml"), []byte("storage_backend = 'bad'\nschema_version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(config, filepath.Join(t.TempDir(), "snippets")); err == nil {
		t.Fatal("expected invalid state error")
	}
}
