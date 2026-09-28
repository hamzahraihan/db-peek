package tui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"

	"db-peek/internal/saved"
)

// Copy-query support: the TUI captures the mouse, so terminal text
// selection is impossible while running. Ctrl+S copies the editor SQL
// plus the last error to the system clipboard (file fallback when the
// clipboard is unavailable), so failing queries can be pasted elsewhere.

// clipboardWriteAll is a seam for tests.
var clipboardWriteAll = clipboard.WriteAll

// userCacheDir is a seam for tests; the dump file follows the platform
// cache dir so the text never lands in the working directory.
var userCacheDir = os.UserCacheDir

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
	text := scrubConnSecrets(strings.ReplaceAll(queryDumpText(m.editor.Text(), m.err), "\x00", ""), m.connStr)
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

// scrubConnSecrets removes credential material from text destined for the
// clipboard or disk. Driver errors embed the DSN, so the dump is scrubbed
// at the source rather than trusting the driver.
func scrubConnSecrets(text, connStr string) string {
	connStr = strings.TrimSpace(connStr)
	if connStr == "" {
		return text
	}
	text = strings.ReplaceAll(text, connStr, saved.Mask(connStr))
	if pass := connPassword(connStr); pass != "" {
		text = strings.ReplaceAll(text, pass, "***")
	}
	return text
}

// connPassword returns the password embedded in a URL-style conn string,
// or "" when there is none.
func connPassword(connStr string) string {
	u, err := url.Parse(connStr)
	if err != nil || u.User == nil {
		return ""
	}
	pass, ok := u.User.Password()
	if !ok {
		return ""
	}
	return pass
}

// writeQueryDumpFile writes the dump outside the working directory at
// 0600: the text can embed the DSN, so it must not sit world-readable
// next to the project.
func writeQueryDumpFile(text string) string {
	dir, err := userCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "db-peek")
	path := filepath.Join(dir, "last-dump.txt")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "copy failed: " + err.Error()
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "copy failed: " + err.Error()
	}
	return "clipboard unavailable — wrote " + path
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
