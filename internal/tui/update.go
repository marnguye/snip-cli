package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		m.help.Width = max(1, size.Width-4)
		m.query.Width = max(10, size.Width-8)
		m.clampScroll()
		return m, nil
	}
	if k, ok := msg.(tea.KeyMsg); ok && key.Matches(k, keys.Quit) {
		return m, tea.Quit
	}
	switch m.mode {
	case modeForm:
		return m.updateForm(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	}
	return m.updatePicker(msg)
}

func (m Model) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		m.err = ""
		item, hasItem := m.selected()
		switch {
		case key.Matches(k, keys.Clear):
			if m.query.Value() == "" {
				return m, tea.Quit
			}
			m.query.SetValue("")
			m.refilter()
			return m, nil
		case key.Matches(k, keys.Up):
			m.move(-1)
			return m, nil
		case key.Matches(k, keys.Down):
			m.move(1)
			return m, nil
		case key.Matches(k, keys.PageUp):
			m.move(-m.rows())
			return m, nil
		case key.Matches(k, keys.PageDown):
			m.move(m.rows())
			return m, nil
		case key.Matches(k, keys.Copy), key.Matches(k, keys.Run):
			if !hasItem {
				return m, nil
			}
			m.Selected = item.Command
			m.Execute = key.Matches(k, keys.Run)
			return m, tea.Quit
		case key.Matches(k, keys.Add):
			return m, m.startAdd(item.Group)
		case key.Matches(k, keys.Edit):
			if !hasItem {
				return m, nil
			}
			return m, m.startEdit()
		case key.Matches(k, keys.Delete):
			if hasItem {
				m.target = item
				m.mode = modeConfirm
			}
			return m, nil
		}
	}
	before := m.query.Value()
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	if m.query.Value() != before {
		m.refilter()
	}
	return m, cmd
}

func (m *Model) startAdd(group string) tea.Cmd {
	m.mode = modeForm
	m.err = ""
	m.target.Group = group
	m.form.edit = false
	m.form.discardWarned = false
	m.form.initialDesc, m.form.initialCmd = "", ""
	m.form.desc.Reset()
	m.form.cmd.Reset()
	m.form.focus = 0
	m.form.cmd.Blur()
	return m.form.desc.Focus()
}

func (m *Model) startEdit() tea.Cmd {
	item, _ := m.selected()
	m.mode = modeForm
	m.err = ""
	m.target = item
	m.form.edit = true
	m.form.discardWarned = false
	m.form.initialDesc, m.form.initialCmd = item.Title, item.Command
	m.form.desc.SetValue(item.Title)
	m.form.desc.Blur()
	m.form.cmd.SetValue(item.Command)
	m.form.focus = 1
	return m.form.cmd.Focus()
}

func (f form) dirty() bool {
	return f.desc.Value() != f.initialDesc || f.cmd.Value() != f.initialCmd
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		cancel := key.Matches(k, keys.Cancel)
		if !cancel {
			m.form.discardWarned = false
		}
		switch {
		case cancel:
			if m.form.dirty() && !m.form.discardWarned {
				m.form.discardWarned = true
				m.err = "unsaved changes: press esc again to discard"
				return m, nil
			}
			m.mode = modePicker
			m.err = ""
			return m, nil
		case key.Matches(k, keys.Next):
			if m.form.edit {
				return m, nil
			}
			var cmd tea.Cmd
			m.form.focus = 1 - m.form.focus
			if m.form.focus == 0 {
				cmd = m.form.desc.Focus()
				m.form.cmd.Blur()
			} else {
				cmd = m.form.cmd.Focus()
				m.form.desc.Blur()
			}
			return m, cmd
		case key.Matches(k, keys.Submit):
			return m.submitForm()
		}
	}
	var cmd tea.Cmd
	if m.form.focus == 0 && !m.form.edit {
		m.form.desc, cmd = m.form.desc.Update(msg)
	} else {
		m.form.cmd, cmd = m.form.cmd.Update(msg)
	}
	return m, cmd
}

func (m Model) submitForm() (tea.Model, tea.Cmd) {
	command := strings.TrimSpace(m.form.cmd.Value())
	if command == "" {
		m.err = "command is required"
		return m, nil
	}
	if m.form.edit {
		updated, err := m.service.Update(context.Background(), m.target, command)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		return m.finishForm(updated.ID, false), nil
	}
	desc := strings.TrimSpace(m.form.desc.Value())
	if desc == "" {
		m.err = "description is required"
		return m, nil
	}
	created, err := m.service.Create(context.Background(), m.target.Group, desc, command)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	return m.finishForm(created.ID, true), nil
}

func (m Model) finishForm(id string, reveal bool) Model {
	m.mode = modePicker
	m.err = ""
	if err := m.reload(id); err != nil {
		m.err = err.Error()
		return m
	}
	if reveal && !m.focus(id) {
		m.query.SetValue("")
		m.refilter()
		m.focus(id)
		m.clampScroll()
	}
	return m
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(k, keys.Confirm):
		deleteErr := m.service.Delete(context.Background(), m.target)
		m.mode = modePicker
		m.err = ""
		if err := m.reload(""); err != nil {
			m.err = err.Error()
		} else if deleteErr != nil {
			m.err = deleteErr.Error()
		}
	case key.Matches(k, keys.Deny):
		m.mode = modePicker
		m.err = ""
	}
	return m, nil
}
