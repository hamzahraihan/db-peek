package tui

import (
	"strings"
)

func (m Model) formView() string {
	var b strings.Builder
	if m.editing == "" {
		b.WriteString(titleStyle.Render("peekerdb — new connection") + "\n\n")
	} else {
		b.WriteString(titleStyle.Render("peekerdb — edit "+m.editing) + "\n\n")
	}
	b.WriteString(fieldLabel("name", m.formFocus == 0) + "\n")
	b.WriteString(m.nameInput.View() + "\n\n")
	b.WriteString(fieldLabel("connection  (postgres://... • mysql://... • ./app.db)", m.formFocus == 1) + "\n")
	b.WriteString(m.connInput.View() + "\n\n")
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	}
	b.WriteString(dimStyle.Render("click/tab switch field • enter save + connect • esc cancel") + "\n")
	return b.String()
}

func fieldLabel(s string, focused bool) string {
	if focused {
		return titleStyle.Render("> " + s)
	}
	return dimStyle.Render("  " + s)
}
