package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// openThemeModal initializes and displays the theme picker modal.
func (m *Model) openThemeModal() bool {
	m.themeModal = true
	m.themeOriginal = m.theme
	if m.themeOriginal == "" {
		m.themeOriginal = "default"
	}
	m.themeCursor = 0
	for i, t := range AvailableThemes {
		if t.ID == CanonicalThemeID(m.themeOriginal) {
			m.themeCursor = i
			break
		}
	}
	return true
}

// cancelThemeModal dismisses the theme picker and restores the original theme.
func (m *Model) cancelThemeModal() {
	m.themeModal = false
	m.applyTheme(m.themeOriginal)
	m.status = "theme unchanged"
}

// confirmThemeModal commits the selected theme and closes the picker.
func (m *Model) confirmThemeModal() {
	if m.themeCursor >= 0 && m.themeCursor < len(AvailableThemes) {
		chosen := AvailableThemes[m.themeCursor]
		m.applyTheme(chosen.ID)
		m.theme = chosen.ID
		m.status = fmt.Sprintf("theme: %s", chosen.Name)
	}
	m.themeModal = false
}

// moveThemeCursor steps the cursor by delta and immediately previews the theme.
func (m *Model) moveThemeCursor(delta int) {
	n := len(AvailableThemes)
	if n == 0 {
		return
	}
	m.themeCursor = (m.themeCursor + delta) % n
	if m.themeCursor < 0 {
		m.themeCursor += n
	}
	m.previewTheme(AvailableThemes[m.themeCursor].ID)
}

// previewTheme applies a theme immediately for live visual feedback.
func (m *Model) previewTheme(id string) {
	m.applyTheme(id)
}

// applyTheme configures all components and global styles for the given theme.
func (m *Model) applyTheme(id string) {
	t := ApplyTheme(id)
	m.theme = t.ID
	// Refresh sidebar filter input styling
	m.filterInput.SetStyles(textinput.Styles{
		Focused: textinput.StyleState{
			Prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Primary)),
			Text:        lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Text)),
			Placeholder: dimStyle,
		},
		Blurred: textinput.StyleState{
			Prompt:      dimStyle,
			Text:        lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.TextDimField)),
			Placeholder: dimStyle,
		},
		Cursor: m.filterInput.Styles().Cursor,
	})

	// Refresh table headers and column roles
	m.refreshTableStyles()

	// Refresh connection list delegate styles
	cdelegate := list.NewDefaultDelegate()
	cdelegate.SetSpacing(0)
	cstyles := list.NewDefaultItemStyles(true)
	cstyles.SelectedTitle = selTitle
	cstyles.SelectedDesc = selDesc
	cdelegate.Styles = cstyles
	m.conns.SetDelegate(cdelegate)
}

// refreshTableStyles reapplies the active theme's column and header roles to all tables.
func (m *Model) refreshTableStyles() {
	if len(m.colTable.cols) > 0 {
		m.colTable.SetColStyles(
			[]lipgloss.Style{colFieldStyle, colTypeStyle, colMetaStyle, colMetaStyle, colMetaStyle},
			[]lipgloss.Style{dimColFieldStyle, dimColTypeStyle, dimColMetaStyle, dimColMetaStyle, dimColMetaStyle},
		)
		m.colTable.SetCellHook(func(col int, val string, dim bool) (lipgloss.Style, bool) {
			if col != 4 {
				return lipgloss.Style{}, false
			}
			if strings.HasPrefix(val, "PK") || strings.Contains(val, "PRI") {
				if dim {
					return dimColPKStyle, true
				}
				return colPKStyle, true
			}
			return lipgloss.Style{}, false
		})
	}
	if len(m.idxTable.cols) > 0 {
		m.idxTable.SetColStyles(
			[]lipgloss.Style{colFieldStyle, colMetaStyle, colTypeStyle},
			[]lipgloss.Style{dimColFieldStyle, dimColMetaStyle, dimColTypeStyle},
		)
		m.idxTable.SetCellHook(func(col int, val string, dim bool) (lipgloss.Style, bool) {
			if col != 1 {
				return lipgloss.Style{}, false
			}
			if dim {
				return dimColUniqueStyle, true
			}
			if strings.EqualFold(strings.TrimSpace(val), "yes") {
				return colUniqueYesStyle, true
			}
			return colUniqueNoStyle, true
		})
	}
	m.rowTable.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
	m.queryTable.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
	for i := range m.qbufs {
		m.qbufs[i].table.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
	}
}

// themeRect computes the bounding box (x, y, w, h) of the theme picker dialog.
func (m Model) themeRect() (int, int, int, int) {
	termW := m.width
	if termW < 40 {
		termW = 40
	}
	termH := m.height
	if termH < 12 {
		termH = 12
	}

	boxW := 56
	if boxW > termW-4 {
		boxW = termW - 4
	}
	if boxW < 36 {
		boxW = 36
	}

	boxH := len(AvailableThemes) + 6

	x := (termW - boxW) / 2
	if x < 0 {
		x = 0
	}
	y := (termH - boxH) / 2
	if y < 0 {
		y = 0
	}
	return x, y, boxW, boxH
}

// themeModalView renders the theme picker overlay on top of the active screen.
func (m Model) themeModalView() string {
	var base string
	switch m.screen {
	case screenConns:
		base = m.connsView()
	case screenForm:
		base = m.formView()
	default:
		base = m.browseView()
	}

	termW := m.width
	if termW < 40 {
		termW = 40
	}
	termH := m.height
	if termH < 12 {
		termH = 12
	}

	x, y, boxW, _ := m.themeRect()
	innerW := boxW - 2 // space inside borders

	var b strings.Builder
	title := titleStyle.Render(" Color Schemes ")
	b.WriteString(fitText(title, innerW))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	for i, t := range AvailableThemes {
		isCursor := i == m.themeCursor
		isActive := t.ID == CanonicalThemeID(m.theme)

		prefix := "  "
		if isCursor {
			prefix = "> "
		}

		check := " "
		if isActive {
			check = "✓"
		}

		// Color preview dots using theme palette
		dot1 := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Primary)).Render("●")
		dot2 := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Secondary)).Render("●")
		dot3 := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Success)).Render("●")
		dot4 := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.ActiveTabBg)).Render("●")
		dots := fmt.Sprintf("%s %s %s %s", dot1, dot2, dot3, dot4)

		namePart := fmt.Sprintf("%-11s", t.Name)
		lineContent := fmt.Sprintf("%s%s %s %s  %s", prefix, check, namePart, dots, t.Description)

		if isCursor {
			b.WriteString(dataSelectedStyle.Render(fitText(lineContent, innerW)))
		} else {
			b.WriteString(fitText(lineContent, innerW))
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(fitText(dimStyle.Render(" ↑/↓ / j/k: preview • enter: select • esc: cancel "), innerW))

	box := paneBorder(true).Render(b.String())
	boxLines := strings.Split(box, "\n")
	baseLines := strings.Split(base, "\n")

	actualH := len(boxLines)
	actualW := 0
	for _, l := range boxLines {
		if w := lipgloss.Width(l); w > actualW {
			actualW = w
		}
	}

	for i := range actualH {
		lineIdx := y + i
		if lineIdx >= len(baseLines) {
			break
		}
		baseLines[lineIdx] = spliceCells(baseLines[lineIdx], boxLines[i], x, actualW, termW)
	}

	return strings.Join(baseLines, "\n")
}
