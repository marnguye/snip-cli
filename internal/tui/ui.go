package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"

	"github.com/marnguye/snip-cli/internal/app"
	"github.com/marnguye/snip-cli/internal/state"
)

type mode int

const (
	modePicker mode = iota
	modeForm
	modeConfirm
)

const (
	chromeLines = 7
	itemLines   = 3
)

var (
	magenta = lipgloss.Color("#cba6f7")
	subtext = lipgloss.Color("#a6adc8")
	overlay = lipgloss.Color("#6c7086")
	surface = lipgloss.Color("#313244")

	appStyle = lipgloss.NewStyle().Padding(1, 2)

	headerTitleStyle = lipgloss.NewStyle().Foreground(magenta).Bold(true)
	headerCountStyle = lipgloss.NewStyle().Foreground(subtext)
	headerLineStyle  = lipgloss.NewStyle().Foreground(surface)
	footerStyle      = lipgloss.NewStyle().Foreground(overlay)
	footerKeyStyle   = lipgloss.NewStyle().Foreground(magenta)

	activeTitleStyle = lipgloss.NewStyle().Foreground(magenta).Bold(true)
	activeDescStyle  = lipgloss.NewStyle().Foreground(overlay)

	inactiveTitleStyle = lipgloss.NewStyle().Foreground(subtext)
	inactiveDescStyle  = lipgloss.NewStyle().Foreground(surface)

	activeBorder = lipgloss.NewStyle().
			Border(lipgloss.Border{Left: "┃"}, false, false, false, true).
			BorderForeground(magenta).
			PaddingLeft(1)

	inactiveBorder = lipgloss.NewStyle().PaddingLeft(2)
)

type form struct {
	desc, cmd               textinput.Model
	focus                   int
	edit                    bool
	initialDesc, initialCmd string
	discardWarned           bool
}

type Model struct {
	service       *app.Service
	group         string
	entries       []entry
	results       []int
	scratch       []scored
	query         textinput.Model
	cursor        int
	offset        int
	mode          mode
	form          form
	target        app.Item
	help          help.Model
	err           string
	width, height int
	Selected      string
	Execute       bool
}

func NewModel(service *app.Service, group, query string) (Model, error) {
	m := Model{service: service, group: group, width: 80, height: 24}
	items, err := m.fetch()
	if err != nil {
		return Model{}, err
	}
	m.entries = newEntries(items)

	m.query = textinput.New()
	m.query.Prompt = "> "
	m.query.Placeholder = "search snippets"
	m.query.CharLimit = 200
	m.query.Width = 60
	m.query.SetValue(query)
	m.query.Focus()

	m.form.desc = textinput.New()
	m.form.desc.Placeholder = "Description"
	m.form.desc.CharLimit = 200
	m.form.desc.Width = 40
	m.form.cmd = textinput.New()
	m.form.cmd.Placeholder = "Command"
	m.form.cmd.CharLimit = 500
	m.form.cmd.Width = 40

	m.help = help.New()
	m.help.Styles.ShortKey = footerKeyStyle
	m.help.Styles.ShortDesc = footerStyle
	m.help.Styles.ShortSeparator = footerStyle
	m.help.Width = m.width - 4

	m.refilter()
	return m, nil
}

func newEntries(items []app.Item) []entry {
	entries := make([]entry, len(items))
	for i, item := range items {
		entries[i] = newEntry(item)
	}
	return entries
}

func (m Model) fetch() ([]app.Item, error) {
	if m.group != "" {
		return m.service.Group(context.Background(), m.group)
	}
	return m.service.List(context.Background())
}

func (m *Model) reload(focusID string) error {
	items, err := m.fetch()
	if errors.Is(err, app.ErrGroupNotFound) {
		items, err = nil, nil
	}
	if err != nil {
		return err
	}
	m.entries = newEntries(items)
	m.results, m.scratch = rank(m.entries, m.query.Value(), m.results, m.scratch)
	if !m.focus(focusID) {
		m.cursor = max(0, min(m.cursor, len(m.results)-1))
	}
	m.clampScroll()
	return nil
}

func (m *Model) refilter() {
	m.results, m.scratch = rank(m.entries, m.query.Value(), m.results, m.scratch)
	m.cursor, m.offset = 0, 0
}

func (m *Model) focus(id string) bool {
	if id == "" {
		return false
	}
	for i, index := range m.results {
		if m.entries[index].item.ID == id {
			m.cursor = i
			return true
		}
	}
	return false
}

func (m Model) selected() (app.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.results) {
		return app.Item{}, false
	}
	return m.entries[m.results[m.cursor]].item, true
}

func (m Model) rows() int {
	return max(1, (m.height-chromeLines)/itemLines)
}

func (m *Model) move(delta int) {
	if len(m.results) == 0 {
		return
	}
	m.cursor = max(0, min(m.cursor+delta, len(m.results)-1))
	m.clampScroll()
}

func (m *Model) clampScroll() {
	rows := m.rows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(0, min(m.offset, len(m.results)-rows))
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

func (m Model) View() string {
	switch m.mode {
	case modeForm:
		return m.renderModal(m.formView())
	case modeConfirm:
		return m.renderModal(m.confirmView())
	}
	if len(m.entries) == 0 {
		return RenderEmptyState(m.width, m.height, m.emptyHint())
	}
	return m.pickerView()
}

func (m Model) emptyHint() emptyHint {
	if m.service.Backend == state.Legacy {
		return emptyHint{
			desc: "Add your first markdown snippet file to get started.",
			path: displayPath(m.service.LegacyDir),
			steps: []string{
				"Create a .md file there",
				"Add a ## title and a command line",
				"Or run: snip add <file.md> ...",
			},
		}
	}
	return emptyHint{
		desc: "Add your first snippet to get started.",
		path: displayPath(filepath.Join(m.service.DataDir, "snippets")),
		steps: []string{
			"Press ctrl+n to add a snippet",
			"Or run: snip add <group> ...",
			"Press enter to copy",
		},
	}
}

func displayPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.Join("~", rel)
		}
	}
	return path
}

func (m Model) pickerView() string {
	var s strings.Builder
	contentWidth := max(10, m.width-4)
	title := headerTitleStyle.Render("snip")
	count := headerCountStyle.Render(fmt.Sprintf("%d snippets", len(m.results)))
	left := fmt.Sprintf("%s  •  %s ", title, count)
	line := ""
	if width := contentWidth - lipgloss.Width(left); width > 0 {
		line = headerLineStyle.Render(strings.Repeat("─", width))
	}
	s.WriteString(left + line + "\n")
	s.WriteString(m.query.View() + "\n")
	if m.err != "" {
		s.WriteString(errorStyle.UnsetMarginTop().Render(truncate.StringWithTail(m.err, uint(contentWidth), "…")))
	}
	s.WriteString("\n")

	if len(m.results) == 0 {
		s.WriteString(inactiveTitleStyle.Render(truncate.StringWithTail(fmt.Sprintf("  No snippets match %q.", m.query.Value()), uint(contentWidth), "…")) + "\n")
	}
	textWidth := uint(max(1, contentWidth-3))
	for i := m.offset; i < min(len(m.results), m.offset+m.rows()); i++ {
		item := m.entries[m.results[i]].item
		title := truncate.StringWithTail(item.Title, textWidth, "…")
		detail := truncate.StringWithTail(fmt.Sprintf("[%s] %s", item.Group, strings.ReplaceAll(item.Command, "\n", " ")), textWidth, "…")
		if i == m.cursor {
			s.WriteString(activeBorder.Render(activeTitleStyle.Render(title)+"\n"+activeDescStyle.Render(detail)) + "\n\n")
		} else {
			s.WriteString(inactiveBorder.Render(inactiveTitleStyle.Render(title)+"\n"+inactiveDescStyle.Render(detail)) + "\n\n")
		}
	}
	s.WriteString(headerLineStyle.Render(strings.Repeat("─", contentWidth)) + "\n")
	s.WriteString(m.help.View(keys.pickerHelp()))
	return appStyle.Render(s.String())
}

func (m Model) renderModal(content string) string {
	box := modalStyle.Render(content)
	if m.width == 0 || m.height == 0 {
		return box
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) formView() string {
	title := "Add Snippet"
	if m.form.edit {
		title = "Edit Snippet"
	}

	var b strings.Builder
	b.WriteString(modalTitleStyle.Render(title))
	b.WriteString("\n")

	b.WriteString(fieldLabelStyle.Render("Description"))
	b.WriteString("\n")
	if m.form.edit {
		b.WriteString(m.target.Title)
	} else {
		b.WriteString(m.form.desc.View())
	}
	b.WriteString("\n\n")

	b.WriteString(fieldLabelStyle.Render("Command"))
	b.WriteString("\n")
	b.WriteString(m.form.cmd.View())

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.err))
	}

	b.WriteString("\n\n")
	b.WriteString(m.help.View(keys.formHelp()))

	return b.String()
}

func (m Model) confirmView() string {
	var b strings.Builder
	b.WriteString(modalTitleStyle.Render("Delete Snippet"))
	b.WriteString("\n")
	b.WriteString("Delete ")
	b.WriteString(dangerStyle.Render(m.target.Title))
	b.WriteString("?")

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.err))
	}

	b.WriteString("\n\n")
	b.WriteString(m.help.View(keys.confirmHelp()))

	return b.String()
}
