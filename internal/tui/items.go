package tui

// List items. Each picker row implements bubbles' DefaultItem so the
// fuzzy filter and the default delegate can render it.

const connIcon = "* "

// connItem is one saved connection in the picker.
type connItem struct {
	name   string
	masked string
	icon   string
}

func (c connItem) FilterValue() string { return c.name + " " + c.masked }
func (c connItem) Title() string       {
	if c.icon != "" {
		return c.icon + c.name
	}
	return c.name
}
func (c connItem) Description() string { return c.masked }
