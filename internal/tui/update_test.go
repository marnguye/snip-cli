package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/marnguye/snip-cli/internal/app"
	"github.com/marnguye/snip-cli/internal/state"
)

type snip struct{ group, title, command string }

func newTestModel(t *testing.T, query string, snippets ...snip) Model {
	t.Helper()
	service, err := app.New(state.TOML, t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snippets {
		if _, err := service.Create(context.Background(), s.group, s.title, s.command); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewModel(service, "", query)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func send(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

func typed(m Model, text string) Model {
	for _, r := range text {
		m, _ = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func press(m Model, keys ...tea.KeyType) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = send(m, tea.KeyMsg{Type: k})
	}
	return m, cmd
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func selectedTitle(m Model) string {
	item, ok := m.selected()
	if !ok {
		return ""
	}
	return item.Title
}

func resultTitles(m Model) []string {
	var out []string
	for _, index := range m.results {
		out = append(out, m.entries[index].item.Title)
	}
	return out
}

var sample = []snip{
	{"docker", "List containers", "docker ps"},
	{"docker", "Prune volumes", "docker volume prune -f"},
	{"go", "Run tests", "go test ./..."},
}

func TestSearchFocusedOnStartupAndTypingFilters(t *testing.T) {
	m := newTestModel(t, "", sample...)
	if !m.query.Focused() || len(m.results) != 3 {
		t.Fatalf("focused=%t results=%d", m.query.Focused(), len(m.results))
	}
	m = typed(m, "prune")
	if m.query.Value() != "prune" || selectedTitle(m) != "Prune volumes" || len(m.results) != 1 {
		t.Fatalf("query=%q results=%v", m.query.Value(), resultTitles(m))
	}
}

func TestShortcutLettersAreTypedNotExecuted(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "adexjkq")
	if m.mode != modePicker || m.query.Value() != "adexjkq" || m.Selected != "" {
		t.Fatalf("mode=%d query=%q selected=%q", m.mode, m.query.Value(), m.Selected)
	}
}

func TestInitialQueryFromFind(t *testing.T) {
	m := newTestModel(t, "go", sample...)
	if got := resultTitles(m); len(got) != 1 || got[0] != "Run tests" {
		t.Fatalf("results=%v", got)
	}
}

func TestNavigationKeys(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m, _ = press(m, tea.KeyDown)
	if m.cursor != 1 {
		t.Fatalf("cursor=%d", m.cursor)
	}
	m, _ = press(m, tea.KeyCtrlJ)
	if m.cursor != 2 {
		t.Fatalf("cursor=%d", m.cursor)
	}
	m, _ = press(m, tea.KeyDown)
	if m.cursor != 2 {
		t.Fatalf("cursor past end=%d", m.cursor)
	}
	m, _ = press(m, tea.KeyCtrlK, tea.KeyUp, tea.KeyUp)
	if m.cursor != 0 {
		t.Fatalf("cursor=%d", m.cursor)
	}
	if m.query.Value() != "" {
		t.Fatalf("navigation changed query: %q", m.query.Value())
	}
}

func TestEnterCopiesAndCtrlRRuns(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "containers")
	copied, cmd := press(m, tea.KeyEnter)
	if copied.Selected != "docker ps" || copied.Execute || !quits(cmd) {
		t.Fatalf("selected=%q execute=%t", copied.Selected, copied.Execute)
	}
	ran, cmd := press(m, tea.KeyCtrlR)
	if ran.Selected != "docker ps" || !ran.Execute || !quits(cmd) {
		t.Fatalf("selected=%q execute=%t", ran.Selected, ran.Execute)
	}
}

func TestNoResultsHasNoSelectionAndEnterDoesNothing(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "zzzz")
	if _, ok := m.selected(); ok || len(m.results) != 0 {
		t.Fatal("expected no selection")
	}
	if !strings.Contains(m.View(), "No snippets match") {
		t.Fatalf("view=%q", m.View())
	}
	for _, k := range []tea.KeyType{tea.KeyEnter, tea.KeyCtrlR, tea.KeyCtrlE, tea.KeyCtrlD} {
		next, cmd := press(m, k)
		if cmd != nil || next.mode != modePicker || next.Selected != "" {
			t.Fatalf("key %v acted without selection", k)
		}
	}
}

func TestSelectionResetsToBestMatchAndStaysValid(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m, _ = press(m, tea.KeyDown, tea.KeyDown)
	m = typed(m, "docker")
	if m.cursor != 0 || selectedTitle(m) == "" {
		t.Fatalf("cursor=%d selected=%q", m.cursor, selectedTitle(m))
	}
	m, _ = press(m, tea.KeyDown)
	m = typed(m, " vol")
	if got := selectedTitle(m); got != "Prune volumes" {
		t.Fatalf("selected=%q results=%v", got, resultTitles(m))
	}
}

func TestEscClearsThenQuits(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "go")
	m, cmd := press(m, tea.KeyEsc)
	if m.query.Value() != "" || len(m.results) != 3 || quits(cmd) {
		t.Fatalf("query=%q results=%d", m.query.Value(), len(m.results))
	}
	_, cmd = press(m, tea.KeyEsc)
	if !quits(cmd) {
		t.Fatal("second esc should quit")
	}
}

func TestCtrlCQuitsFromEveryMode(t *testing.T) {
	m := newTestModel(t, "", sample...)
	if _, cmd := press(m, tea.KeyCtrlC); !quits(cmd) {
		t.Fatal("picker")
	}
	form, _ := press(m, tea.KeyCtrlN)
	if _, cmd := press(form, tea.KeyCtrlC); !quits(cmd) {
		t.Fatal("form")
	}
	confirm, _ := press(m, tea.KeyCtrlD)
	if _, cmd := press(confirm, tea.KeyCtrlC); !quits(cmd) {
		t.Fatal("confirm")
	}
}

func TestModalEntryShortcuts(t *testing.T) {
	m := newTestModel(t, "", sample...)
	add, _ := press(m, tea.KeyCtrlN)
	if add.mode != modeForm || add.form.edit || add.target.Group != "docker" {
		t.Fatalf("add mode=%d edit=%t group=%q", add.mode, add.form.edit, add.target.Group)
	}
	edit, _ := press(m, tea.KeyCtrlE)
	if edit.mode != modeForm || !edit.form.edit || edit.form.cmd.Value() != "docker ps" {
		t.Fatalf("edit mode=%d cmd=%q", edit.mode, edit.form.cmd.Value())
	}
	del, _ := press(m, tea.KeyCtrlD)
	if del.mode != modeConfirm || del.target.Title != "List containers" {
		t.Fatalf("delete mode=%d target=%q", del.mode, del.target.Title)
	}
}

func TestFormInputsIgnorePickerShortcuts(t *testing.T) {
	m := newTestModel(t, "keep", sample...)
	m, _ = press(m, tea.KeyCtrlN)
	m = typed(m, "adexjkq")
	if m.mode != modeForm || m.form.desc.Value() != "adexjkq" || m.query.Value() != "keep" || m.Selected != "" {
		t.Fatalf("mode=%d desc=%q query=%q", m.mode, m.form.desc.Value(), m.query.Value())
	}
	m, _ = press(m, tea.KeyCtrlR, tea.KeyCtrlD)
	if m.mode != modeForm || m.Selected != "" || m.Execute {
		t.Fatal("picker shortcut fired inside form")
	}
}

func TestCancelModalRestoresPickerAndQuery(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "docker")
	form, _ := press(m, tea.KeyCtrlN)
	form, cmd := press(form, tea.KeyEsc)
	if form.mode != modePicker || form.query.Value() != "docker" || quits(cmd) || !form.query.Focused() {
		t.Fatalf("mode=%d query=%q", form.mode, form.query.Value())
	}
	confirm, _ := press(m, tea.KeyCtrlD)
	confirm, cmd = press(confirm, tea.KeyEsc)
	if confirm.mode != modePicker || confirm.query.Value() != "docker" || quits(cmd) || len(resultTitles(confirm)) != 2 {
		t.Fatalf("mode=%d", confirm.mode)
	}
}

func TestUnsavedFormEditsNeedSecondEsc(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m, _ = press(m, tea.KeyCtrlN)
	m = typed(m, "draft")
	m, _ = press(m, tea.KeyEsc)
	if m.mode != modeForm || m.form.desc.Value() != "draft" || m.err == "" {
		t.Fatalf("mode=%d desc=%q err=%q", m.mode, m.form.desc.Value(), m.err)
	}
	m, _ = press(m, tea.KeyEsc)
	if m.mode != modePicker {
		t.Fatal("second esc should discard")
	}
}

func TestDeleteUpdatesSelectionAndEmptyState(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m, _ = press(m, tea.KeyDown, tea.KeyCtrlD)
	if m.target.Title != "Prune volumes" {
		t.Fatalf("target=%q", m.target.Title)
	}
	m = typed(m, "y")
	if m.mode != modePicker || len(m.results) != 2 {
		t.Fatalf("mode=%d results=%v", m.mode, resultTitles(m))
	}
	got := selectedTitle(m)
	if got == "" || got == "Prune volumes" {
		t.Fatalf("stale selection %q", got)
	}
	for len(m.results) > 0 {
		m, _ = press(m, tea.KeyCtrlD, tea.KeyEnter)
	}
	if _, ok := m.selected(); ok || !strings.Contains(m.View(), "No snippets yet") {
		t.Fatal("expected empty state after deleting everything")
	}
	if cmd := func() tea.Cmd { _, c := press(m, tea.KeyEnter); return c }(); cmd != nil {
		t.Fatal("enter acted on empty collection")
	}
}

func TestDeletingLastMatchKeepsQueryAndShowsNoResults(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "run tests")
	m, _ = press(m, tea.KeyCtrlD)
	m = typed(m, "y")
	if len(m.results) != 0 || m.query.Value() != "run tests" || len(m.entries) != 2 {
		t.Fatalf("results=%v query=%q", resultTitles(m), m.query.Value())
	}
}

func TestEditRefreshesResultsAndKeepsSelectionByID(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "containers")
	id := m.entries[m.results[0]].item.ID
	m, _ = press(m, tea.KeyCtrlE)
	m, _ = press(m, tea.KeyBackspace)
	m = typed(m, "s -a")
	m, _ = press(m, tea.KeyEnter)
	if m.mode != modePicker || m.err != "" {
		t.Fatalf("mode=%d err=%q", m.mode, m.err)
	}
	item, ok := m.selected()
	if !ok || item.ID != id || item.Command != "docker ps -a" {
		t.Fatalf("selected=%+v", item)
	}
	m, _ = press(m, tea.KeyEsc)
	m = typed(m, "ps -a")
	m, _ = press(m, tea.KeyCtrlE)
	m, _ = press(m, tea.KeyBackspace, tea.KeyBackspace, tea.KeyBackspace, tea.KeyBackspace, tea.KeyBackspace)
	m = typed(m, "docker images")
	m, _ = press(m, tea.KeyEnter)
	if len(m.results) != 0 {
		t.Fatalf("edited snippet should leave results: %v", resultTitles(m))
	}
}

func TestAddWhileQueryActiveRevealsNewSnippet(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "volumes")
	m, _ = press(m, tea.KeyCtrlN)
	m = typed(m, "Brand new")
	m, _ = press(m, tea.KeyTab)
	m = typed(m, "echo new")
	m, _ = press(m, tea.KeyEnter)
	if m.mode != modePicker || m.err != "" {
		t.Fatalf("mode=%d err=%q", m.mode, m.err)
	}
	if selectedTitle(m) != "Brand new" || m.query.Value() != "" || len(m.entries) != 4 {
		t.Fatalf("selected=%q query=%q entries=%d", selectedTitle(m), m.query.Value(), len(m.entries))
	}
}

func TestAddKeepsMatchingQuery(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m = typed(m, "docker")
	m, _ = press(m, tea.KeyCtrlN)
	m = typed(m, "Docker logs")
	m, _ = press(m, tea.KeyTab)
	m = typed(m, "docker logs")
	m, _ = press(m, tea.KeyEnter)
	if m.query.Value() != "docker" || selectedTitle(m) != "Docker logs" {
		t.Fatalf("query=%q selected=%q", m.query.Value(), selectedTitle(m))
	}
}

func TestFormValidationErrorsStayInForm(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m, _ = press(m, tea.KeyCtrlN, tea.KeyEnter)
	if m.mode != modeForm || m.err == "" {
		t.Fatalf("mode=%d err=%q", m.mode, m.err)
	}
}

func TestDuplicateTitlesAndCommandsStayDistinct(t *testing.T) {
	m := newTestModel(t, "", snip{"a", "Same", "echo 1"}, snip{"b", "Same", "echo 1"})
	if len(m.results) != 2 || m.entries[0].item.ID == m.entries[1].item.ID {
		t.Fatal("duplicates collapsed")
	}
	m, _ = press(m, tea.KeyDown, tea.KeyCtrlD)
	target := m.target.ID
	m = typed(m, "y")
	if len(m.results) != 1 || m.entries[m.results[0]].item.ID == target {
		t.Fatal("deleted wrong duplicate")
	}
}

func TestLargeCollectionScrollsAndStaysInRange(t *testing.T) {
	var items []app.Item
	for i := 0; i < 3000; i++ {
		items = append(items, app.Item{ID: fmt.Sprint(i), Title: fmt.Sprintf("snippet %d", i), Command: "echo"})
	}
	m := newTestModel(t, "")
	m.entries = newEntries(items)
	m.refilter()
	m, _ = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(m, tea.KeyPgDown, tea.KeyPgDown, tea.KeyPgDown)
	if m.cursor < m.offset || m.cursor >= m.offset+m.rows() {
		t.Fatalf("cursor=%d offset=%d rows=%d", m.cursor, m.offset, m.rows())
	}
	m = typed(m, "snippet 2999")
	if selectedTitle(m) != "snippet 2999" {
		t.Fatalf("selected=%q", selectedTitle(m))
	}
}

func TestLongTextAndTinyTerminalDoNotPanic(t *testing.T) {
	long := strings.Repeat("ž", 90)
	m := newTestModel(t, "", snip{"g", long, strings.Repeat("echo ", 200)}, snip{"g", "short", "x"})
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 10, Height: 3}, {Width: 1, Height: 1}, {Width: 0, Height: 0}, {Width: 200, Height: 60}} {
		m, _ = send(m, size)
		for _, mode := range []mode{modePicker, modeForm, modeConfirm} {
			m.mode = mode
			if m.View() == "" {
				t.Fatalf("empty view for %v at %v", mode, size)
			}
		}
		m.mode = modePicker
		if rows := m.rows(); rows < 1 || m.offset < 0 || m.offset > max(0, len(m.results)-1) {
			t.Fatalf("rows=%d offset=%d", rows, m.offset)
		}
	}
	for _, line := range strings.Split(func() string { m, _ = send(m, tea.WindowSizeMsg{Width: 60, Height: 24}); return m.View() }(), "\n") {
		if width := lipgloss.Width(line); width > 60 {
			t.Fatalf("line wider than terminal: %d", width)
		}
	}
}

func TestEmptyStateMatchesBackend(t *testing.T) {
	for backend, want := range map[state.Backend]string{state.TOML: "Press ctrl+n to add", state.Legacy: "Create a .md file"} {
		service, err := app.New(backend, t.TempDir(), t.TempDir(), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		m, err := NewModel(service, "", "")
		if err != nil {
			t.Fatal(err)
		}
		m, _ = send(m, tea.WindowSizeMsg{Width: 120, Height: 40})
		view := m.View()
		if !strings.Contains(view, want) || strings.Contains(strings.ToLower(view), "reload") {
			t.Fatalf("backend %s: unexpected empty state %q", backend, view)
		}
	}
}

func TestFooterShowsSearchFirstShortcuts(t *testing.T) {
	m := newTestModel(t, "", sample...)
	m, _ = send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	view := m.View()
	for _, want := range []string{"enter copy", "ctrl+r run", "ctrl+n add", "ctrl+e edit", "ctrl+d delete", "> "} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
