package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeLegacy(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDryRunMigrationAndRepeat(t *testing.T) {
	legacy, data := t.TempDir(), filepath.Join(t.TempDir(), "data")
	writeLegacy(t, legacy, "work.md", "## Same\nprintf one\n\n## Same\nprintf two\n")
	writeLegacy(t, legacy, "ops.md", "## Deploy\nprintf deploy\n")
	dry, err := Run(context.Background(), Options{LegacyDir: legacy, DataDir: data, DryRun: true})
	if err != nil || dry.LegacyFiles != 2 || dry.Snippets != 3 || dry.Duplicates != 1 {
		t.Fatalf("report=%#v err=%v", dry, err)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote data: %v", err)
	}
	first, err := Run(context.Background(), Options{LegacyDir: legacy, DataDir: data})
	if err != nil || first.Migrated != 3 || first.BackupDir == "" {
		t.Fatalf("report=%#v err=%v", first, err)
	}
	second, err := Run(context.Background(), Options{LegacyDir: legacy, DataDir: data})
	if err != nil || second.AlreadyMigrated != 3 {
		t.Fatalf("report=%#v err=%v", second, err)
	}
	content, err := os.ReadFile(filepath.Join(legacy, "work.md"))
	if err != nil || string(content) != "## Same\nprintf one\n\n## Same\nprintf two\n" {
		t.Fatalf("legacy changed: %q err=%v", content, err)
	}
}

func TestMigrationRejectsSymlinkInput(t *testing.T) {
	legacy, data := t.TempDir(), t.TempDir()
	target := filepath.Join(legacy, "target")
	writeLegacy(t, legacy, "target", "safe")
	if err := os.Symlink(target, filepath.Join(legacy, "bad.md")); err != nil {
		t.Skip(err)
	}
	if _, err := Run(context.Background(), Options{LegacyDir: legacy, DataDir: data, DryRun: true}); err == nil {
		t.Fatal("symlink accepted")
	}
}
