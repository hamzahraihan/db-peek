package export

import (
	"bytes"
	"strings"
	"testing"
)

var (
	exportCols = []string{"a", "b"}
	exportRaw  = [][]any{
		{int64(1), nil},
		{int64(2), []byte("x")},
	}
)

func write(t *testing.T, f Format) string {
	t.Helper()
	var b bytes.Buffer
	n, err := f.Write(&b, exportCols, exportRaw)
	if err != nil {
		t.Fatalf("%s: %v", f, err)
	}
	if n != len(exportRaw) {
		t.Fatalf("%s: wrote %d rows, want %d", f, n, len(exportRaw))
	}
	return b.String()
}

func TestExportCSV(t *testing.T) {
	// SQL NULL is an empty field, never the literal "NULL" the grid shows.
	if got, want := write(t, FormatCSV), "a,b\n1,\n2,x\n"; got != want {
		t.Fatalf("csv = %q, want %q", got, want)
	}
}

func TestExportTSVUsesTabs(t *testing.T) {
	got := write(t, FormatTSV)
	if !strings.Contains(got, "a\tb") || strings.Contains(got, "a,b") {
		t.Fatalf("tsv must be tab separated, got %q", got)
	}
}

func TestExportJSONTypes(t *testing.T) {
	got := write(t, FormatJSON)
	if !strings.Contains(got, `"a": 1`) {
		t.Fatalf("numbers must stay numeric:\n%s", got)
	}
	if !strings.Contains(got, `"b": null`) {
		t.Fatalf("SQL NULL must stay null in JSON:\n%s", got)
	}
	if !strings.Contains(got, `"b": "x"`) {
		t.Fatalf("text bytes must decode to a string:\n%s", got)
	}
}

func TestExportJSONBase64ForBinary(t *testing.T) {
	var b bytes.Buffer
	raw := [][]any{{[]byte{0x00, 0x01, 0x02}}}
	if _, err := FormatJSON.Write(&b, []string{"v"}, raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"v": "AAEC"`) {
		t.Fatalf("binary must be base64:\n%s", b.String())
	}
}

func TestExportMarkdownEscapesPipes(t *testing.T) {
	var b bytes.Buffer
	raw := [][]any{{"a|b"}}
	if _, err := FormatMD.Write(&b, []string{"col|1"}, raw); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if !strings.Contains(got, `col\|1`) || !strings.Contains(got, `a\|b`) {
		t.Fatalf("pipes must be escaped:\n%s", got)
	}
	if !strings.HasPrefix(got, "| col\\|1 |\n| --- |\n| a\\|b |\n") {
		t.Fatalf("unexpected markdown shape:\n%s", got)
	}
}

// A ragged result (a driver returning fewer columns than headers) must
// pad rather than panic.
func TestExportPadsShortRows(t *testing.T) {
	var b bytes.Buffer
	if _, err := FormatCSV.Write(&b, []string{"a", "b", "c"}, [][]any{{int64(1)}}); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), "a,b,c\n1,,\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatFromExt(t *testing.T) {
	cases := map[string]Format{
		"out.csv": FormatCSV, "OUT.CSV": FormatCSV,
		"out.tsv": FormatTSV, "out.json": FormatJSON,
		"out.md": FormatMD, "out.markdown": FormatMD,
	}
	for path, want := range cases {
		got, err := FormatFromExt(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", path, got, want)
		}
	}
	for _, path := range []string{"out.xml", "out", "out.exe"} {
		if _, err := FormatFromExt(path); err == nil {
			t.Fatalf("%s must be rejected", path)
		}
	}
}

// The whole point of Raw: an export must not inherit the grid's 60-char
// display truncation.
func TestExportKeepsLongValues(t *testing.T) {
	long := strings.Repeat("x", 300)
	var b bytes.Buffer
	if _, err := FormatCSV.Write(&b, []string{"c"}, [][]any{{long}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), long) {
		t.Fatalf("export must keep the full value, got %d bytes", b.Len())
	}
}
