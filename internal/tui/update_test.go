package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/marnguye/snip-cli/internal/app"
	"github.com/marnguye/snip-cli/internal/state"
)

func TestFilteredEditAndDeleteUseGlobalIndex(t *testing.T) {
	items := []list.Item{
		NewItem(app.Item{ID: "legacy:test.md:0", Description: "first", Command: "one", Group: "test.md", Legacy: true}),
		NewItem(app.Item{ID: "legacy:test.md:1", Description: "second", Command: "two", Group: "test.md", Legacy: true}),
	}
	service, err := app.New(state.TOML, t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(items, service, "")
	m.list.SetFilterText("second")
	selected, ok := m.list.SelectedItem().(item)
	if !ok || m.list.Index() != 0 || m.list.GlobalIndex() != 1 {
		t.Fatalf("unexpected filtered selection: %#v index=%d global=%d", selected, m.list.Index(), m.list.GlobalIndex())
	}
	m.startEdit(selected)
	if m.targetIndex != 1 {
		t.Fatalf("edit index=%d, want 1", m.targetIndex)
	}
	m.state = stateList
	updated, _ := m.updateList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model := updated.(Model)
	if model.targetIndex != 1 {
		t.Fatalf("delete index=%d, want 1", model.targetIndex)
	}
}

func TestEmptyStateMatchesBackend(t *testing.T) {
	for backend, want := range map[state.Backend]string{state.TOML: "Press a to add", state.Legacy: "Create a .md file"} {
		service, err := app.New(backend, t.TempDir(), t.TempDir(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		model, _ := NewModel(nil, service, "").Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		view := model.View()
		if !strings.Contains(view, want) || strings.Contains(strings.ToLower(view), "reload") {
			t.Fatalf("backend %s: unexpected empty state %q", backend, view)
		}
	}
}
