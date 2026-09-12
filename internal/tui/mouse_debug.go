package tui

// Mouse diagnostics, off by default. Set DBPEEK_MOUSE_DEBUG=1 to append
// one line per mouse event to %TEMP%/db-peek-mouse.log, showing exactly
// what the terminal delivered and how the app resolved it.

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var mouseDebug = os.Getenv("DBPEEK_MOUSE_DEBUG") != ""
var mouseLogMu sync.Mutex

func mouseLogPath() string { return filepath.Join(os.TempDir(), "db-peek-mouse.log") }

func logMouse(format string, args ...any) {
	if !mouseDebug {
		return
	}
	mouseLogMu.Lock()
	defer mouseLogMu.Unlock()
	f, err := os.OpenFile(mouseLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"\n", args...)
}
