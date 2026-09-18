package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SQL autocomplete (query tab): tables + columns + keywords with Tab accept.
// Pure engine lives here; popup state and rendering attach to Model in the
// Task 2 spots (model.go, view_detail.go).

// completeItem is one suggestion row.
type completeItem struct {
	Text   string // inserted on accept
	Detail string // right-side hint: table name, "table", "column", "keyword"
	Kind   string // "table" | "column" | "keyword"
}

// sqlKeywords are the static keyword candidates.
var sqlKeywords = []string{
	"SELECT", "FROM", "WHERE", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN",
	"FULL JOIN", "CROSS JOIN", "LEFT OUTER JOIN", "RIGHT OUTER JOIN", "FULL OUTER JOIN", "ON",
	"GROUP BY", "ORDER BY", "LIMIT", "OFFSET", "HAVING", "UNION",
	"INSERT", "UPDATE", "DELETE", "CREATE", "TABLE", "INDEX",
	"AND", "OR", "NOT", "NULL", "AS", "DISTINCT",
	"COUNT", "SUM", "AVG", "IN", "BETWEEN", "LIKE",
}

func isCompleteRune(r rune) bool {
	return r == '_' || r == '$' ||
		r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

// completeWord extracts the identifier prefix ending at col (rune offset).
// A dot terminates the word: "orders." yields ("", end) so the caller can
// switch to dot-column mode.
func completeWord(line string, col int) (prefix string, start int) {
	rs := []rune(line)
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	start = col
	for start > 0 && isCompleteRune(rs[start-1]) {
		start--
	}
	return string(rs[start:col]), start
}

// dotTableForPrefix returns the table name in "table.<prefix>" when fullText
// ends with that shape at the prefix position, else "".
func dotTableForPrefix(fullText, prefix string) string {
	lower := strings.ToLower(fullText)
	needle := strings.ToLower(prefix)
	idx := strings.LastIndex(lower, needle)
	if idx <= 0 {
		return ""
	}
	if lower[idx-1] != '.' {
		return ""
	}
	end := idx - 1
	start := end
	for start > 0 && isCompleteRune(rune(lower[start-1])) {
		start--
	}
	return fullText[start:end]
}

// referencedTables scans for FROM/JOIN/UPDATE/INTO <name> mentions.
func referencedTables(text string) []string {
	var out []string
	upper := strings.ToUpper(text)
	for _, kw := range []string{"FROM", "JOIN", "UPDATE", "INTO"} {
		for i := 0; i+len(kw) <= len(upper); i++ {
			if upper[i:i+len(kw)] != kw {
				continue
			}
			// Keyword must stand alone.
			if i > 0 && isCompleteRune(rune(text[i-1])) {
				continue
			}
			j := i + len(kw)
			for j < len(text) && (text[j] == ' ' || text[j] == '\t' || text[j] == '\n') {
				j++
			}
			s := j
			for j < len(text) && isCompleteRune(rune(text[j])) {
				j++
			}
			if s < j {
				out = append(out, text[s:j])
			}
		}
	}
	return out
}

// completeCandidates ranks suggestions for prefix: exact-prefix matches
// first (tables, then columns, then keywords), then substring matches.
// In "table." dot mode only that table's columns are returned.
func completeCandidates(prefix, curTable, editorText string, tables []string, colsByTable map[string][]string, curCols []string) []completeItem {
	lower := strings.ToLower(prefix)
	if t := dotTableForPrefix(editorText, prefix); t != "" {
		var out []completeItem
		for _, c := range colsByTable[strings.ToLower(t)] {
			if strings.HasPrefix(strings.ToLower(c), lower) {
				out = append(out, completeItem{Text: c, Detail: t, Kind: "column"})
			}
		}
		return out
	}
	var prefTables, prefCols, prefKw []completeItem
	var subTables, subCols, subKw []completeItem
	seen := map[string]bool{}
	for _, tb := range tables {
		key := strings.ToLower(tb)
		if seen[key] {
			continue
		}
		seen[key] = true
		it := completeItem{Text: tb, Detail: "table", Kind: "table"}
		switch {
		case strings.HasPrefix(key, lower):
			prefTables = append(prefTables, it)
		case lower != "" && strings.Contains(key, lower):
			subTables = append(subTables, it)
		}
	}
	colSet := map[string]string{}
	for _, c := range curCols {
		colSet[strings.ToLower(c)] = c
	}
	for _, tb := range referencedTables(editorText) {
		for _, c := range colsByTable[strings.ToLower(tb)] {
			if _, ok := colSet[strings.ToLower(c)]; !ok {
				colSet[strings.ToLower(c)] = c
			}
		}
	}
	_ = curTable
	for key, orig := range colSet {
		it := completeItem{Text: orig, Detail: "column", Kind: "column"}
		switch {
		case strings.HasPrefix(key, lower):
			prefCols = append(prefCols, it)
		case lower != "" && strings.Contains(key, lower):
			subCols = append(subCols, it)
		}
	}
	for _, k := range sqlKeywords {
		it := completeItem{Text: k, Detail: "keyword", Kind: "keyword"}
		key := strings.ToLower(k)
		switch {
		case strings.HasPrefix(key, lower):
			prefKw = append(prefKw, it)
		case lower != "" && strings.Contains(key, lower):
			subKw = append(subKw, it)
		}
	}
	out := append(append(append(prefTables, prefCols...), prefKw...), append(append(subTables, subCols...), subKw...)...)
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

// applyCompletion replaces the current word at col with the item text.
func applyCompletion(line string, col int, item completeItem) (string, int) {
	rs := []rune(line)
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	_, start := completeWord(line, col)
	out := string(rs[:start]) + item.Text + string(rs[col:])
	return out, start + len([]rune(item.Text))
}

// completionContext gathers candidate sources from the explorer and the
// current table's loaded columns.
func (m *Model) completionContext() (tables []string, colsByTable map[string][]string, curCols []string) {
	colsByTable = map[string][]string{}
	for _, s := range m.explorer.Schemas {
		for _, tb := range s.Tables {
			tables = append(tables, tb.Name)
			var cs []string
			for _, c := range tb.Columns {
				cs = append(cs, c.Name)
			}
			colsByTable[strings.ToLower(tb.Name)] = cs
		}
	}
	for _, c := range m.cols {
		curCols = append(curCols, c.Name)
	}
	if _, ok := colsByTable[strings.ToLower(m.table)]; !ok && len(curCols) > 0 {
		colsByTable[strings.ToLower(m.table)] = curCols
	}
	return tables, colsByTable, curCols
}

// refreshCompletion rebuilds the popup from the cursor word. It is a no-op
// unless the query editor is focused.
func (m *Model) refreshCompletion() {
	m.showComplete = false
	m.completeItems = nil
	m.completeIdx = 0
	if !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) {
		return
	}
	if m.editor.CurLine < 0 || m.editor.CurLine >= len(m.editor.Lines) {
		return
	}
	line := m.editor.Lines[m.editor.CurLine]
	prefix, start := completeWord(line, m.editor.CurCol)
	// Dot mode: "orders." with empty prefix still completes columns.
	if prefix == "" && !strings.HasSuffix(lineBeforeCol(line, m.editor.CurCol), ".") {
		return
	}
	tables, colsByTable, curCols := m.completionContext()
	items := completeCandidates(prefix, m.table, m.editor.Text(), tables, colsByTable, curCols)
	if len(items) == 0 {
		return
	}
	m.showComplete = true
	m.completeItems = items
	m.completeStart = start
	m.completePrefix = prefix
}

func lineBeforeCol(line string, col int) string {
	rs := []rune(line)
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	return string(rs[:col])
}

// popupVisibleCount reports how many popup rows are actually drawn: up to 5
// items plus a "more" footer, clipped to the editor rows below the cursor.
func (m Model) popupVisibleCount() int {
	_, _, _, n := m.popupGeometry(1 << 30)
	return n
}

// popupGeometry lays out the autocomplete box inside the 8-row editor
// budget: top/left are editor-relative rows/columns, boxW the outer width
// (borders included), nItems the visible suggestion rows. The box sits
// below the cursor when it fits, above it when the cursor is near the
// bottom, and shrinks its item count when neither side fits.
func (m Model) popupGeometry(paneW int) (top, left, boxW, nItems int) {
	nItems = len(m.completeItems)
	if nItems > 5 {
		nItems = 5
	}
	more := len(m.completeItems) > 5
	// Content width from plain text (never measure styled output).
	cw := 0
	for _, it := range m.completeItems[:min(nItems, len(m.completeItems))] {
		if w := lipgloss.Width(it.Text+" "+it.Detail); w > cw {
			cw = w
		}
	}
	footer := ""
	if more {
		footer = "… " + strconv.Itoa(len(m.completeItems)-nItems) + " more"
		if w := lipgloss.Width(footer); w > cw {
			cw = w
		}
	}
	outer := cw + 2 + 2 // padding + borders
	if outer > paneW {
		outer = paneW
	}
	if outer < 12 {
		outer = 12
	}
	cw = outer - 4
	boxH := nItems + 2 // items + top/bottom borders
	if more {
		boxH++
	}
	// The wheel can scroll the viewport past the cursor, leaving curRel
	// outside [0, queryEditorH). Clamp to the visible rows so top can
	// never escape the 8-line budget and panic the overlay copy.
	curRel := m.editor.CurLine - m.editor.OffY
	if curRel < 0 {
		curRel = 0
	}
	if curRel > queryEditorH-1 {
		curRel = queryEditorH - 1
	}
	availBelow := queryEditorH - (curRel + 1)
	switch {
	case boxH <= availBelow:
		top = curRel + 1
	case boxH <= curRel:
		top = curRel - boxH
	default:
		// Neither side fits: shrink to the space below (min 1 item).
		nItems = availBelow - 2
		if more {
			nItems--
		}
		if nItems < 1 {
			nItems = 1
		}
		more = len(m.completeItems) > nItems
		boxH = nItems + 2
		if more {
			boxH++
		}
		top = curRel + 1
	}
	_ = footer
	// Left edge at the cursor column, clamped into the pane.
	curX := 4 // gutter " %2d "
	if ln := m.editor.CurLine; ln >= 0 && ln < len(m.editor.Lines) {
		rs := []rune(m.editor.Lines[ln])
		col := m.editor.CurCol
		if col > len(rs) {
			col = len(rs)
		}
		if col < 0 {
			col = 0
		}
		curX += lipgloss.Width(string(rs[:col]))
	}
	left = curX
	if left+outer > paneW {
		left = paneW - outer
	}
	if left < 0 {
		left = 0
	}
	return top, left, outer, nItems
}
