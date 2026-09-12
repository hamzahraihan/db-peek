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
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "db-peek", "connections.json"), nil
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
	done := false
	for i, p := range s.profiles {
		if strings.EqualFold(p.Name, name) {
			s.profiles[i] = Profile{Name: p.Name, Conn: conn, UpdatedAt: now}
			done = true
		}
	}
	if !done {
		s.profiles = append(s.profiles, Profile{Name: name, Conn: conn, UpdatedAt: now})
	}
	return s.persist()
}

func (s *Store) Delete(name string) error {
	kept := s.profiles[:0]
	found := false
	for _, p := range s.profiles {
		if strings.EqualFold(p.Name, name) {
			found = true
			continue
		}
		kept = append(kept, p)
	}
	if !found {
		return fmt.Errorf("no saved connection %q", name)
	}
	s.profiles = kept
	return s.persist()
}

func (s *Store) persist() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, append(data, '\n'), 0o600)
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
