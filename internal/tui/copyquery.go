package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
)

// Copy-query support: the TUI captures the mouse, so terminal text
// selection is impossible while running. Ctrl+S copies the editor SQL
// plus the last error to the system clipboard (file fallback when the
// clipboard is unavailable), so failing queries can be pasted elsewhere.

// clipboardWriteAll is a seam for tests.
var clipboardWriteAll = clipboard.WriteAll

// queryDumpText formats the editor SQL and last error for pasting.
func queryDumpText(sql, errStr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "-- db-peek query dump %s\n", time.Now().Format(time.RFC3339))
	b.WriteString(sql + "\n")
	if strings.TrimSpace(errStr) != "" {
		b.WriteString("-- error:\n" + strings.TrimSpace(errStr) + "\n")
	}
	return b.String()
}

// copyQueryDump copies the dump; on clipboard failure it writes
// db-peek-debug.txt in the working directory instead. It reports the
// human status line for the caller to display. It never panics: NUL bytes
// (fatal to the Windows clipboard API) are stripped, and a panicking
// clipboard backend falls through to the file fallback.
func (m *Model) copyQueryDump() (status string) {
	text := strings.ReplaceAll(queryDumpText(m.editor.Text(), m.err), "\x00", "")
	defer func() {
		if recover() != nil {
			status = writeQueryDumpFile(text)
		}
	}()
	if err := clipboardWriteAll(text); err == nil {
		return "query copied to clipboard"
	}
	return writeQueryDumpFile(text)
}

func writeQueryDumpFile(text string) string {
	if err := os.WriteFile("db-peek-debug.txt", []byte(text), 0o644); err != nil {
		return "copy failed: " + err.Error()
	}
	return "clipboard unavailable — wrote db-peek-debug.txt"
}

// copySelectionText copies the raw selected lines (no dump header, no
// error trailer). It shares the never-panic clipboard path.
func (m *Model) copySelectionText(text string) (status string) {
	text = strings.ReplaceAll(text, "\x00", "")
	n := len(strings.Split(text, "\n"))
	defer func() {
		if recover() != nil {
			status = writeQueryDumpFile(text)
		}
	}()
	if err := clipboardWriteAll(text); err == nil {
		if n == 1 {
			return "1 line copied to clipboard"
		}
		return fmt.Sprintf("%d lines copied to clipboard", n)
	}
	return writeQueryDumpFile(text)
}
