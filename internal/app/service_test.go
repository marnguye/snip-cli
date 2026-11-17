package app

import (
	"context"
	"testing"

	"github.com/marnguye/snip-cli/internal/state"
)

func TestTOMLServiceUsesStableIDs(t *testing.T) {
	service, err := New(state.TOML, t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Create(context.Background(), "work.md", "same", "printf one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), "work.md", "same", "printf two")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Legacy || second.Legacy {
		t.Fatal("toml records need separate stable IDs")
	}
	if err := service.Delete(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	items, err := service.List(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestTOMLCutoverRequiresMigration(t *testing.T) {
	legacy := t.TempDir()
	service, err := New(state.Legacy, legacy, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), "work.md", "title", "printf ok"); err != nil {
		t.Fatal(err)
	}
	if err := service.Use(context.Background(), state.TOML, false); err == nil {
		t.Fatal("expected verified migration requirement")
	}
}
