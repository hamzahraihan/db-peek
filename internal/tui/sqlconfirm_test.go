package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDirtyBufferAsksFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.sql")
	if err := os.WriteFile(path, []byte("SELECT 2"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := queryTabModel()
	m.tab, m.queryFocus, m.focusDetail = 3, 0, true
	m.editor.SetText("SELECT 1")
	m.loadBufferFromPath(path)
	if got := m.editor.Text(); got != "SELECT 1" {
		t.Fatalf("dirty buffer must not load yet, got %q", got)
	}
	if m.sqlConfirmPath != path {
		t.Fatalf("pending path must arm, got %q", m.sqlConfirmPath)
	}
	u, _ := m.Update(testKey("y"))
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT 2" {
		t.Fatalf("y must replace buffer, got %q", got)
	}
}

func TestOpenDirtyBufferDeclined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.sql")
	if err := os.WriteFile(path, []byte("SELECT 2"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := queryTabModel()
	m.tab = 3
	m.editor.SetText("SELECT 1")
	m.loadBufferFromPath(path)
	u, _ := m.Update(testKey("n"))
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT 1" {
		t.Fatalf("n must keep buffer, got %q", got)
	}
	if m.sqlConfirmPath != "" {
		t.Fatalf("pending path must clear, got %q", m.sqlConfirmPath)
	}
}

func TestOpenCleanBufferLoadsDirectly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.sql")
	if err := os.WriteFile(path, []byte("SELECT 2"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := queryTabModel()
	m.tab = 3
	m.loadBufferFromPath(path)
	if got := m.editor.Text(); got != "SELECT 2" {
		t.Fatalf("clean buffer must load directly, got %q", got)
	}
	if m.sqlConfirmPath != "" {
		t.Fatalf("no confirm must arm, got %q", m.sqlConfirmPath)
	}
}
