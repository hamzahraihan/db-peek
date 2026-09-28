package saved

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Profile is one named connection: a DBeaver-style saved entry.
type Profile struct {
	Name      string    `json:"name"`
	Conn      string    `json:"conn"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persists profiles as JSON under the OS user config dir.
// Passwords live in the conn strings, so the file is created 0600.
type Store struct {
	Path     string
	profiles []Profile
	pathErr  error // why Path is empty, when the config dir was unavailable
}

// DefaultDir is the directory db-peek keeps all of its state in.
func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "db-peek"), nil
}

func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "connections.json"), nil
}

// New returns a store bound to the default path. If the config dir is
// unavailable the store keeps an empty Path and persists fail with an
// actionable message instead of "open : no such file or directory".
func New() *Store {
	p, err := DefaultPath()
	if err != nil {
		return &Store{pathErr: err}
	}
	return &Store{Path: p}
}

// writeAtomic writes data to path via a same-directory temp file and
// os.Rename, so a crash mid-write can never truncate the real file.
// os.Rename replaces an existing destination on Windows (MoveFileEx
// with MOVEFILE_REPLACE_EXISTING), POSIX, and every other platform Go
// supports — no remove-then-rename is needed.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func Load() (*Store, error) {
	p, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	s := &Store{Path: p}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var profiles []Profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	s.profiles = profiles
	return s, nil
}

func (s *Store) List() []Profile {
	out := append([]Profile(nil), s.profiles...)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (s *Store) Get(name string) (Profile, bool) {
	for _, p := range s.profiles {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Profile{}, false
}

// Upsert adds or replaces the profile with the same (case-insensitive) name.
func (s *Store) Upsert(name, conn string) error {
	name = strings.TrimSpace(name)
	conn = strings.TrimSpace(conn)
	if name == "" {
		return fmt.Errorf("profile name is empty")
	}
	if conn == "" {
		return fmt.Errorf("connection string is empty")
	}
	now := time.Now().UTC()
	next := make([]Profile, 0, len(s.profiles)+1)
	replaced := false
	for _, p := range s.profiles {
		if !replaced && strings.EqualFold(p.Name, name) {
			next = append(next, Profile{Name: p.Name, Conn: conn, UpdatedAt: now})
			replaced = true
			continue
		}
		next = append(next, p)
	}
	if !replaced {
		next = append(next, Profile{Name: name, Conn: conn, UpdatedAt: now})
	}
	return s.persist(next)
}

func (s *Store) Delete(name string) error {
	next := make([]Profile, 0, len(s.profiles))
	found := false
	for i, p := range s.profiles {
		if strings.EqualFold(p.Name, name) {
			next = append(next, s.profiles[i+1:]...)
			found = true
			break
		}
		next = append(next, p)
	}
	if !found {
		return fmt.Errorf("no saved connection %q", name)
	}
	return s.persist(next)
}

// persist writes next and, only on success, adopts it as the store's
// profiles. On failure s.profiles is untouched, so the UI list and the
// disk file can never diverge.
func (s *Store) persist(next []Profile) error {
	if s.Path == "" {
		return fmt.Errorf("saved connections have no path (config dir unavailable): %w", s.pathErr)
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(s.Path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	s.profiles = next
	return nil
}

// Mask renders a conn string for display with any password removed.
func Mask(raw string) string {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		if u.User != nil {
			u.User = url.User(u.User.Username())
		}
		return u.Redacted()
	}
	return raw
}
