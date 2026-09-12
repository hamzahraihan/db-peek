package tui

// List items. Each picker row implements bubbles' DefaultItem so the
// fuzzy filter and the default delegate can render it.

// tableItem is one row in the fuzzy-filterable table list.
type tableItem struct{ name string }

func (t tableItem) FilterValue() string { return t.name }
func (t tableItem) Title() string       { return t.name }
func (t tableItem) Description() string { return "table" }

// connItem is one saved connection in the picker.
type connItem struct {
	name   string
	masked string
}

func (c connItem) FilterValue() string { return c.name + " " + c.masked }
func (c connItem) Title() string       { return c.name }
func (c connItem) Description() string { return c.masked }
