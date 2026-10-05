package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"charm.land/lipgloss/v2"
)

type Editor struct {
	Lines     []string
	CurLine   int
	CurCol    int // rune offset within line
	OffY      int // first visible line
	selAnchor int // anchor line
	selAnchorCol int // anchor column (rune offset); only used when !selLineWise
	selActive bool
	selLineWise bool // true = whole lines lo..hi (shift+up/down, shift+click, vertical drag)
}

func NewEditor() Editor { return Editor{Lines: []string{""}} }

func (e *Editor) runes() []rune { return []rune(e.Lines[e.CurLine]) }

func (e *Editor) Insert(r rune) {
	rs := e.runes()
	if e.CurCol > len(rs) {
		e.CurCol = len(rs)
	}
	e.Lines[e.CurLine] = string(rs[:e.CurCol]) + string(r) + string(rs[e.CurCol:])
	e.CurCol++
}

// RunSQL chooses the SQL to execute: the active selection wins, else
// the semicolon-delimited statement under the cursor, else the buffer.
// Statement splitting skips quoted semicolons and comments; $-quoting
// is out of scope and splits conservatively.
func (e *Editor) RunSQL() string {
	if s := e.SelectionText(); strings.TrimSpace(s) != "" {
		return s
	}
	if s := statementAtCursor(e.Text(), e.CurLine, e.CurCol); strings.TrimSpace(s) != "" {
		return s
	}
	return e.Text()
}

// stmtSpan is one raw statement body with rune offsets into the source.
type stmtSpan struct {
	body       string
	start, end int
}

func splitStatements(src string) []stmtSpan {
	rs := []rune(src)
	var out []stmtSpan
	start, i := 0, 0
	inSingle, inDouble := false, false
	inLine, inBlock := false, false
	bodyOf := func(end int) {
		out = append(out, stmtSpan{body: string(rs[start:end]), start: start, end: end})
	}
	for i < len(rs) {
		c := rs[i]
		var next rune
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		switch {
		case inLine:
			if c == '\n' {
				inLine = false
			}
		case inBlock:
			if c == '*' && next == '/' {
				inBlock = false
				i++
			}
		case inSingle:
			if c == '\'' {
				if next == '\'' {
					i++
				} else {
					inSingle = false
				}
			}
		case inDouble:
			if c == '"' {
				if next == '"' {
					i++
				} else {
					inDouble = false
				}
			}
		case c == '-' && next == '-':
			inLine = true
			i++
		case c == '/' && next == '*':
			inBlock = true
			i++
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == ';':
			bodyOf(i)
			start = i + 1
		}
		i++
	}
	bodyOf(len(rs))
	return out
}

func statementAtCursor(text string, line, col int) string {
	rs := []rune(text)
	lines := strings.Split(text, "\n")
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	off := 0
	for i := 0; i < line && i < len(lines); i++ {
		off += len([]rune(lines[i])) + 1
	}
	lrs := []rune(lines[line])
	if col < 0 {
		col = 0
	}
	if col > len(lrs) {
		col = len(lrs)
	}
	off += col
	_ = rs
	var prev string
	for _, sp := range splitStatements(text) {
		body := strings.TrimSpace(sp.body)
		if body == "" {
			continue
		}
		if sp.start <= off && off < sp.end {
			return body
		}
		if sp.end <= off {
			prev = body
			continue
		}
		if prev != "" {
			return prev
		}
		return body
	}
	return prev
}

// deleteSelection removes the current selection (line-wise or
// character-precise) and parks the cursor at its start. It reports
// whether a selection was active.
func (e *Editor) deleteSelection() bool {
	alo, aco, chi, cco, active := e.SelectionSpan()
	if !active {
		return false
	}
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	if alo < 0 {
		alo = 0
	}
	if chi >= len(e.Lines) {
		chi = len(e.Lines) - 1
	}
	if alo < 0 || chi < 0 || alo >= len(e.Lines) || chi >= len(e.Lines) {
		e.selActive = false
		return true
	}
	ars := []rune(e.Lines[alo])
	crs := []rune(e.Lines[chi])
	if aco < 0 {
		aco = 0
	}
	if cco < 0 {
		cco = 0
	}
	if aco > len(ars) {
		aco = len(ars)
	}
	if cco > len(crs) {
		cco = len(crs)
	}
	if alo == chi {
		e.Lines[alo] = string(ars[:aco]) + string(ars[cco:])
		e.CurLine, e.CurCol = alo, aco
	} else {
		head := string(ars[:aco])
		tail := string(crs[cco:])
		out := make([]string, 0, len(e.Lines)-(chi-alo))
		out = append(out, e.Lines[:alo]...)
		out = append(out, head+tail)
		out = append(out, e.Lines[chi+1:]...)
		if len(out) == 0 {
			out = []string{""}
		}
		e.Lines = out
		e.CurLine = alo
		e.CurCol = len([]rune(head))
	}
	e.selActive = false
	return true
}

// InsertText pastes s at the cursor, replacing any selection. It
// normalizes Windows (\r\n) and old-Mac (\r) line endings, strips NUL
// bytes (fatal to the Windows clipboard API), and splits multi-line
// input into new editor rows like Newline does.
func (e *Editor) InsertText(s string) {
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	if e.CurLine < 0 {
		e.CurLine = 0
	}
	if e.CurLine >= len(e.Lines) {
		e.CurLine = len(e.Lines) - 1
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\x00", "")
	e.deleteSelection()
	if s == "" {
		return
	}
	rs := []rune(e.Lines[e.CurLine])
	if e.CurCol < 0 {
		e.CurCol = 0
	}
	if e.CurCol > len(rs) {
		e.CurCol = len(rs)
	}
	parts := strings.Split(s, "\n")
	if len(parts) == 1 {
		pr := []rune(parts[0])
		e.Lines[e.CurLine] = string(rs[:e.CurCol]) + parts[0] + string(rs[e.CurCol:])
		e.CurCol += len(pr)
		return
	}
	head := string(rs[:e.CurCol])
	tail := string(rs[e.CurCol:])
	out := make([]string, 0, len(e.Lines)+len(parts)-1)
	out = append(out, e.Lines[:e.CurLine]...)
	out = append(out, head+parts[0])
	if len(parts) > 2 {
		out = append(out, parts[1:len(parts)-1]...)
	}
	out = append(out, parts[len(parts)-1]+tail)
	out = append(out, e.Lines[e.CurLine+1:]...)
	e.Lines = out
	e.CurLine = e.CurLine + len(parts) - 1
	e.CurCol = len([]rune(parts[len(parts)-1]))
}

func (e *Editor) Backspace() {
	rs := e.runes()
	if e.CurCol > 0 {
		e.Lines[e.CurLine] = string(rs[:e.CurCol-1]) + string(rs[e.CurCol:])
		e.CurCol--
		return
	}
	if e.CurLine > 0 {
		prev := len([]rune(e.Lines[e.CurLine-1]))
		e.Lines[e.CurLine-1] += e.Lines[e.CurLine]
		e.Lines = append(e.Lines[:e.CurLine], e.Lines[e.CurLine+1:]...)
		e.CurLine--
		e.CurCol = prev
	}
}

func (e *Editor) Delete() {
	rs := e.runes()
	if e.CurCol < len(rs) {
		e.Lines[e.CurLine] = string(rs[:e.CurCol]) + string(rs[e.CurCol+1:])
		return
	}
	if e.CurLine < len(e.Lines)-1 {
		e.Lines[e.CurLine] += e.Lines[e.CurLine+1]
		e.Lines = append(e.Lines[:e.CurLine+1], e.Lines[e.CurLine+2:]...)
	}
}

func (e *Editor) Newline() {
	rs := e.runes()
	if e.CurCol > len(rs) {
		e.CurCol = len(rs)
	}
	e.Lines = append(e.Lines[:e.CurLine+1], append([]string{string(rs[e.CurCol:])}, e.Lines[e.CurLine+1:]...)...)
	e.Lines[e.CurLine] = string(rs[:e.CurCol])
	e.CurLine++
	e.CurCol = 0
}

func (e *Editor) MoveUp() {
	if e.CurLine > 0 {
		e.CurLine--
		e.CurCol = min(e.CurCol, len([]rune(e.Lines[e.CurLine])))
	}
}

func (e *Editor) MoveDown() {
	if e.CurLine < len(e.Lines)-1 {
		e.CurLine++
		e.CurCol = min(e.CurCol, len([]rune(e.Lines[e.CurLine])))
	}
}

func (e *Editor) MoveLeft() {
	if e.CurCol > 0 {
		e.CurCol--
	} else if e.CurLine > 0 {
		e.CurLine--
		e.CurCol = len([]rune(e.Lines[e.CurLine]))
	}
}

func (e *Editor) MoveRight() {
	if e.CurCol < len([]rune(e.Lines[e.CurLine])) {
		e.CurCol++
	} else if e.CurLine < len(e.Lines)-1 {
		e.CurLine++
		e.CurCol = 0
	}
}

func (e *Editor) Home() { e.CurCol = 0 }
func (e *Editor) End()  { e.CurCol = len([]rune(e.Lines[e.CurLine])) }

func (e *Editor) Text() string { return strings.Join(e.Lines, "\n") }

// MoveToEnd parks the cursor after the last character.
func (e *Editor) MoveToEnd() {
	e.CurLine = len(e.Lines) - 1
	if e.CurLine < 0 {
		e.CurLine = 0
	}
	e.End()
}

func (e *Editor) SetText(s string) {
	e.Lines = strings.Split(s, "\n")
	e.CurLine, e.CurCol, e.OffY = 0, 0, 0
	e.selActive = false
}

func (e *Editor) CursorXY() (int, int) { return e.CurLine, e.CurCol }

func (e *Editor) ClearSelection() { e.selActive = false }

func (e *Editor) SelectedRange() (int, int, bool) {
	if !e.selActive {
		return 0, 0, false
	}
	lo, hi := e.selAnchor, e.CurLine
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(e.Lines) {
		hi = len(e.Lines) - 1
	}
	return lo, hi, true
}

func (e *Editor) ExtendSelectionTo(line int) {
	if !e.selActive {
		e.selAnchor, e.selActive = e.CurLine, true
	}
	e.selLineWise = true
	if line < 0 {
		line = 0
	}
	if line >= len(e.Lines) {
		line = len(e.Lines) - 1
	}
	e.CurLine = line
	rs := []rune(e.Lines[e.CurLine])
	if e.CurCol > len(rs) {
		e.CurCol = len(rs)
	}
}

// ExtendSelectionToPos extends a character-precise selection to (line,
// col). The first call anchors at the pre-move cursor; a zero-width span
// deactivates so taps never leave selections behind.
func (e *Editor) ExtendSelectionToPos(line, col int) {
	if line < 0 {
		line = 0
	}
	if line >= len(e.Lines) {
		line = len(e.Lines) - 1
	}
	rs := []rune(e.Lines[line])
	if col < 0 {
		col = 0
	}
	if col > len(rs) {
		col = len(rs)
	}
	if !e.selActive {
		e.selAnchor, e.selAnchorCol, e.selActive = e.CurLine, e.CurCol, true
	}
	e.selLineWise = false
	e.CurLine, e.CurCol = line, col
	if e.selAnchor == e.CurLine && e.selAnchorCol == e.CurCol {
		e.selActive = false
	}
}

// ExtendSelectionChar extends a character-precise selection one rune
// left (right=false) or right (right=true), wrapping across line
// boundaries like MoveLeft/MoveRight. A zero-width result deactivates.
func (e *Editor) ExtendSelectionChar(right bool) {
	line, col := e.CurLine, e.CurCol
	if line < 0 {
		line = 0
	}
	if line >= len(e.Lines) {
		line = len(e.Lines) - 1
	}
	if right {
		if col < len([]rune(e.Lines[line])) {
			col++
		} else if line < len(e.Lines)-1 {
			line++
			col = 0
		}
	} else {
		if col > 0 {
			col--
		} else if line > 0 {
			line--
			col = len([]rune(e.Lines[line]))
		}
	}
	e.ExtendSelectionToPos(line, col)
}

// SelectionSpan returns the normalized character-precise endpoints
// (anchor-first ordering): lines alo..chi with rune columns aco..cco on
// the outer lines. Middle lines are fully included. Line-wise selections
// report (lo,0)-(hi,eol) so callers share one code path.
func (e *Editor) SelectionSpan() (alo, aco, chi, cco int, active bool) {
	if !e.selActive {
		return 0, 0, 0, 0, false
	}
	lo, hi, _ := e.SelectedRange()
	if e.selLineWise {
		return lo, 0, hi, len([]rune(e.Lines[hi])), true
	}
	alo, aco = e.selAnchor, e.selAnchorCol
	chi, cco = e.CurLine, e.CurCol
	if alo > chi || (alo == chi && aco > cco) {
		alo, aco, chi, cco = chi, cco, alo, aco
	}
	return alo, aco, chi, cco, true
}

// SelectionEmpty reports a zero-width (or inactive) selection.
func (e *Editor) SelectionEmpty() bool {
	alo, aco, chi, cco, active := e.SelectionSpan()
	return !active || (alo == chi && aco == cco)
}

func (e *Editor) SelectionText() string {
	lo, hi, active := e.SelectedRange()
	if !active {
		return ""
	}
	if e.selLineWise {
		return strings.Join(e.Lines[lo:hi+1], "\n")
	}
	alo, aco, chi, cco, _ := e.SelectionSpan()
	if alo == chi {
		rs := []rune(e.Lines[alo])
		return string(rs[aco:cco])
	}
	out := string([]rune(e.Lines[alo])[aco:])
	for _, ln := range e.Lines[alo+1 : chi] {
		out += "\n" + ln
	}
	out += "\n" + string([]rune(e.Lines[chi])[:cco])
	return out
}

func (e *Editor) DeleteCurrentLine() {
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	if e.CurLine < 0 {
		e.CurLine = 0
	}
	if e.CurLine >= len(e.Lines) {
		e.CurLine = len(e.Lines) - 1
	}
	e.Lines = append(e.Lines[:e.CurLine], e.Lines[e.CurLine+1:]...)
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	e.CurLine = min(e.CurLine, len(e.Lines)-1)
	e.CurCol = 0
	e.selActive = false
}

func (e *Editor) DeleteRange() {
	lo, hi, active := e.SelectedRange()
	if !active {
		return
	}
	e.Lines = append(e.Lines[:lo], e.Lines[hi+1:]...)
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	e.CurLine = min(lo, len(e.Lines)-1)
	e.CurCol = 0
	e.selActive = false
}

func commentLine(s string) string { return "-- " + s }

func uncommentLine(s string) (string, bool) {
	if strings.HasPrefix(s, "-- ") {
		return s[3:], true
	}
	if strings.HasPrefix(s, "--") {
		return s[2:], true
	}
	return s, false
}

func (e *Editor) ToggleCommentRange() {
	lo, hi, active := e.SelectedRange()
	if !active {
		e.ToggleCommentLine()
		return
	}
	commented := true
	for _, ln := range e.Lines[lo : hi+1] {
		if !strings.HasPrefix(ln, "--") {
			commented = false
			break
		}
	}
	for i := lo; i <= hi; i++ {
		if commented {
			u, _ := uncommentLine(e.Lines[i])
			e.Lines[i] = u
		} else {
			e.Lines[i] = commentLine(e.Lines[i])
		}
	}
}

func (e *Editor) ToggleCommentLine() {
	u, ok := uncommentLine(e.Lines[e.CurLine])
	if ok {
		e.Lines[e.CurLine] = u
		return
	}
	e.Lines[e.CurLine] = commentLine(e.Lines[e.CurLine])
}

type hlCell struct {
	Text  string
	Style lipgloss.Style
}

var (
	sqlKeyword = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EAB308"))
	sqlString  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
	sqlNumber  = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
	sqlComment = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sqlPlain   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
)

func HighlightSQL(src string) [][]hlCell {
	lx := lexers.Get("sql")
	if lx == nil {
		return [][]hlCell{{{Text: src, Style: sqlPlain}}}
	}
	it, err := lx.Tokenise(nil, src)
	if err != nil {
		return [][]hlCell{{{Text: src, Style: sqlPlain}}}
	}
	var out [][]hlCell
	cur := []hlCell{}
	flush := func() { out = append(out, cur); cur = []hlCell{} }
	for _, tok := range it.Tokens() {
		st := sqlPlain
		switch {
		case tok.Type.InCategory(chroma.Keyword):
			st = sqlKeyword
		case tok.Type.InCategory(chroma.LiteralString):
			st = sqlString
		case tok.Type.InCategory(chroma.LiteralNumber):
			st = sqlNumber
		case tok.Type.InCategory(chroma.Comment):
			st = sqlComment
		}
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				flush()
			}
			if p != "" {
				cur = append(cur, hlCell{Text: p, Style: st})
			}
		}
	}
	out = append(out, cur)
	return out
}
