// Package export writes query results to CSV, TSV, JSON or Markdown.
//
// It takes the *untruncated* driver values (db.Sample.Raw) rather than
// the display strings (db.Sample.Rows), which are cut at 60 characters:
// exporting the display text would silently lose data.
package export

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// Format is a supported export encoding, chosen by file extension.
type Format string

const (
	FormatCSV  Format = "csv"
	FormatTSV  Format = "tsv"
	FormatJSON Format = "json"
	FormatMD   Format = "md"
)

// FormatFromExt maps a file path to a format. The extension is matched
// case-insensitively; anything else is an error rather than a guess.
func FormatFromExt(path string) (Format, error) {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case "csv":
		return FormatCSV, nil
	case "tsv", "tab":
		return FormatTSV, nil
	case "json":
		return FormatJSON, nil
	case "md", "markdown":
		return FormatMD, nil
	}
	return "", fmt.Errorf("cannot export to %q: want .csv, .tsv, .json or .md", filepath.Ext(path))
}

// Write emits cols and raw in the receiver's format and returns the
// number of data rows written (the header is not counted). A nil value is
// SQL NULL and becomes an empty field, except in JSON where it stays null.
func (f Format) Write(w io.Writer, cols []string, raw [][]any) (int, error) {
	switch f {
	case FormatCSV:
		return f.writeSeparated(w, cols, raw, ',')
	case FormatTSV:
		return f.writeSeparated(w, cols, raw, '\t')
	case FormatJSON:
		return f.writeJSON(w, cols, raw)
	case FormatMD:
		return f.writeMarkdown(w, cols, raw)
	}
	return 0, fmt.Errorf("unknown export format %q", f)
}

func (f Format) writeSeparated(w io.Writer, cols []string, raw [][]any, comma rune) (int, error) {
	cw := csv.NewWriter(w)
	cw.Comma = comma
	if err := cw.Write(cols); err != nil {
		return 0, err
	}
	for _, row := range raw {
		rec := make([]string, len(cols))
		for i := range cols {
			rec[i] = scalarString(cell(row, i))
		}
		if err := cw.Write(rec); err != nil {
			return 0, err
		}
	}
	cw.Flush()
	return len(raw), cw.Error()
}

func (f Format) writeJSON(w io.Writer, cols []string, raw [][]any) (int, error) {
	out := make([]map[string]any, 0, len(raw))
	for _, row := range raw {
		obj := make(map[string]any, len(cols))
		for i, c := range cols {
			obj[c] = jsonValue(cell(row, i))
		}
		out = append(out, obj)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return 0, err
	}
	return len(raw), nil
}

func (f Format) writeMarkdown(w io.Writer, cols []string, raw [][]any) (int, error) {
	var b strings.Builder
	b.WriteString("| " + strings.Join(escapeMD(cols), " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", len(cols)) + "\n")
	for _, row := range raw {
		cells := make([]string, len(cols))
		for i := range cols {
			cells[i] = scalarString(cell(row, i))
		}
		b.WriteString("| " + strings.Join(escapeMD(cells), " | ") + " |\n")
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return 0, err
	}
	return len(raw), nil
}

// cell reads column i, padding a short row so a ragged result cannot
// panic the export.
func cell(row []any, i int) any {
	if i >= len(row) {
		return nil
	}
	return row[i]
}

// scalarString renders one value for the text formats. NULL is empty,
// never the literal "NULL" the grid shows.
func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		if isText(t) {
			return string(t)
		}
		return base64.StdEncoding.EncodeToString(t)
	case time.Time:
		return t.Format(time.RFC3339Nano)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// jsonValue keeps numbers and booleans typed, and renders []byte as a
// base64 string so binary survives the round-trip.
func jsonValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		if isText(t) {
			return string(t)
		}
		return base64.StdEncoding.EncodeToString(t)
	case time.Time:
		return t.Format(time.RFC3339Nano)
	case string, bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, float32, float64:
		return t
	default:
		// Named integer types (driver.Value implementations) do not type
		// assert above; render them as their literal text rather than
		// letting encoding/json refuse them.
		return fmt.Sprintf("%v", t)
	}
}

func isText(b []byte) bool {
	printable := 0
	for _, c := range b {
		if c == '\t' || c == '\n' || c == '\r' || (c >= 0x20 && c < 0x7f) {
			printable++
		}
	}
	return len(b) == 0 || printable*10 >= len(b)*8
}

func escapeMD(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		c = strings.ReplaceAll(c, "\n", " ")
		c = strings.ReplaceAll(c, "\r", " ")
		out[i] = strings.ReplaceAll(c, "|", `\|`)
	}
	return out
}
