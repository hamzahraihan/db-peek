package tui

// ER render memoization: laying out and drawing the diagram is O(schema),
// and the TUI re-renders on every key, mouse motion and blink tick. The
// canvas is therefore cached across frames and keyed on the inputs that
// can change it; panning and clipping stay per frame.

import dbpkg "db-peek/internal/db"

// genPosKey covers everything the layouts depend on: the schema data
// generation, whether the focused subset is used, its center, and the
// viewport size the grid layout is computed against.
type genPosKey struct {
	gen     int
	focused bool
	center  string
	w, h    int
}

// hlMemo caches the SQL highlighting of the editor buffer. It hangs off a
// pointer because Model is copied by value on every Update.
type hlMemo struct {
	text  string
	cells [][]hlCell
}

// genCanvasKey adds the selection and hover, which change box styling.
type genCanvasKey struct {
	genPosKey
	sel, hover string
}

// erMemo caches the ER layout and the unclipped canvas across frames.
// Model is copied by value on every Update, so the cache hangs off a
// pointer and is keyed on the inputs that affect the render.
type erMemo struct {
	// posKey covers everything erFocusLayout/erGridLayout depend on.
	posKey genPosKey
	pos    map[string]erRect
	tables []erTable
	links  []dbpkg.ForeignKey
	// canvasKey adds selection and hover, which change box styling.
	canvasKey genCanvasKey
	canvas    []string
}

// erLayoutFor returns the (tables, links, positions) for the current ER
// state, recomputing only when the layout key changes. Shared by the
// renderers and the hit-testers so they can never disagree.
func (m *Model) erLayoutFor(w, h int) ([]erTable, []dbpkg.ForeignKey, map[string]erRect) {
	if m.erMemo == nil {
		m.erMemo = &erMemo{}
	}
	key := genPosKey{gen: m.erDataGen, focused: m.erIsFocused(), center: m.erCenter, w: w, h: h}
	if m.erMemo.posKey == key && m.erMemo.pos != nil {
		return m.erMemo.tables, m.erMemo.links, m.erMemo.pos
	}
	var tables []erTable
	var links []dbpkg.ForeignKey
	var pos map[string]erRect
	if key.focused {
		tables, links = erFocusedSubset(m.erCenter, m.erSchema.tables, m.erSchema.links)
		pos = erFocusLayout(m.erCenter, tables, links)
	} else {
		tables, links = m.erSchema.tables, m.erSchema.links
		pos = erGridLayout(tables, w, h)
	}
	m.erMemo.posKey = key
	m.erMemo.tables, m.erMemo.links, m.erMemo.pos = tables, links, pos
	return tables, links, pos
}

// erCanvasFor returns the unclipped canvas rows, recomputing only when
// the canvas key changes. erSliceViewport still runs per frame — it is
// cheap and pan-dependent.
func (m *Model) erCanvasFor(w, h int) []string {
	tables, links, pos := m.erLayoutFor(w, h)
	key := genCanvasKey{genPosKey: m.erMemo.posKey, sel: m.erSel, hover: m.hoverER}
	if m.erMemo.canvasKey == key && m.erMemo.canvas != nil {
		return m.erMemo.canvas
	}
	var canvas []string
	if len(tables) == 0 {
		canvas = []string{"(no tables)"}
	} else {
		boxFn := erBoxLinesEx
		if m.erIsFocused() {
			boxFn = erFocusBoxLinesEx
		}
		canvas = erRenderWithPos(tables, links, pos, boxFn, m.erSel, m.hoverER)
	}
	m.erMemo.canvasKey = key
	m.erMemo.canvas = canvas
	return canvas
}
