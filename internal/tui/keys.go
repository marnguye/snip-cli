package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up, Down, PageUp, PageDown   key.Binding
	Copy, Run, Add, Edit, Delete key.Binding
	Clear, Quit                  key.Binding
	Next, Submit, Cancel         key.Binding
	Confirm, Deny                key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "ctrl+k"), key.WithHelp("↑", "move")),
	Down:     key.NewBinding(key.WithKeys("down", "ctrl+j"), key.WithHelp("↓", "move")),
	PageUp:   key.NewBinding(key.WithKeys("pgup")),
	PageDown: key.NewBinding(key.WithKeys("pgdown")),
	Copy:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "copy")),
	Run:      key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "run")),
	Add:      key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "add")),
	Edit:     key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "edit")),
	Delete:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete")),
	Clear:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
	Quit:     key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Next:     key.NewBinding(key.WithKeys("tab", "shift+tab", "down", "up"), key.WithHelp("tab", "next field")),
	Submit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
	Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Confirm:  key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "confirm")),
	Deny:     key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "cancel")),
}

type shortHelp []key.Binding

func (h shortHelp) ShortHelp() []key.Binding  { return h }
func (h shortHelp) FullHelp() [][]key.Binding { return [][]key.Binding{h} }

func (k keyMap) pickerHelp() shortHelp {
	return shortHelp{k.Up, k.Copy, k.Run, k.Add, k.Edit, k.Delete, k.Clear}
}

func (k keyMap) formHelp() shortHelp { return shortHelp{k.Submit, k.Next, k.Cancel} }

func (k keyMap) confirmHelp() shortHelp { return shortHelp{k.Confirm, k.Deny} }
