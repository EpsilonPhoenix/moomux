package tui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up            key.Binding
	Down          key.Binding
	MoveUp        key.Binding
	MoveDown      key.Binding
	MoveProjLeft  key.Binding
	MoveProjRight key.Binding
	Open          key.Binding
	New           key.Binding
	Delete        key.Binding
	Archive       key.Binding
	ShowArchived  key.Binding
	Kill          key.Binding
	Refresh       key.Binding
	Tab           key.Binding
	ShiftTab      key.Binding
	NextProject   key.Binding
	PrevProject   key.Binding
	Quit          key.Binding
	Cancel        key.Binding
	Confirm       key.Binding
	NewProject    key.Binding
	DelProject    key.Binding
	EditSession   key.Binding
	EditProject   key.Binding
	Tag           key.Binding
	Enter         key.Binding
	Left          key.Binding
	Right         key.Binding
	No            key.Binding
	FormUp        key.Binding
	FormDown      key.Binding
	Help          key.Binding
	RemoteLinks   key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		// Each reorder action also takes a plain-letter alternate: shift+arrow
		// chords need extended-keys support, which mobile/remote terminal clients
		// often can't send with a single keypress.
		MoveUp:        key.NewBinding(key.WithKeys("shift+up", "K"), key.WithHelp("shift+↑/K", "move up")),
		MoveDown:      key.NewBinding(key.WithKeys("shift+down", "J"), key.WithHelp("shift+↓/J", "move down")),
		MoveProjLeft:  key.NewBinding(key.WithKeys("shift+left", "H"), key.WithHelp("shift+←/H", "move project left")),
		MoveProjRight: key.NewBinding(key.WithKeys("shift+right", "L"), key.WithHelp("shift+→/L", "move project right")),
		Open:          key.NewBinding(key.WithKeys("enter", "o"), key.WithHelp("enter/o", "open")),
		New:           key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		Delete:        key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Archive:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "archive")),
		ShowArchived:  key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "archived")),
		Kill:          key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "park")),
		Refresh:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Tab:           key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next project")),
		ShiftTab:      key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev project")),
		// Plain-letter alternates to tab/shift+tab for switching project. Kept as
		// separate bindings rather than extra keys on Tab/ShiftTab because those
		// two double as field navigation in the forms, where "[" / "]" must stay
		// ordinary text input.
		NextProject: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next project")),
		PrevProject: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev project")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Cancel:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Confirm:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "confirm")),
		NewProject:  key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "add project")),
		DelProject:  key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "remove project")),
		EditSession: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit session")),
		EditProject: key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "edit project")),
		Tag:         key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tag")),
		Enter:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		Left:        key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "left")),
		Right:       key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "right")),
		No:          key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "no")),
		// Arrow-only (no j/k) for forms with text inputs, so typing "j"/"k" isn't hijacked as navigation.
		FormUp:      key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		FormDown:    key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		RemoteLinks: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "toggle ticket/PR link copy")),
	}
}
