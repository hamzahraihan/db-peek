package saved

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Session is the state db-peek restores on the next launch. It records the
// saved profile *name*, never the connection string, so no new secret is
// written to disk; an ad-hoc connection restores nothing connection-wise.
type Session struct {
	ConnProfile string   `json:"conn_profile,omitempty"`
	ConnMasked  string   `json:"conn_masked,omitempty"`
	Table       string   `json:"table,omitempty"`
	Tab         int      `json:"tab,omitempty"`
	PageSize    int      `json:"page_size,omitempty"`
	Filter      string   `json:"filter,omitempty"`
	Buffers     []string `json:"buffers,omitempty"` // editor text per query buffer
	Cur         int      `json:"cur,omitempty"`

	// path overrides the default location; empty means the config dir.
	path string
}

func DefaultSessionPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

func (s *Session) file() (string, error) {
	if s.path != "" {
		return s.path, nil
	}
	return DefaultSessionPath()
}

// LoadSession reads the session file. A missing file is an empty session,
// not an error: a fresh install must start normally.
func LoadSession() (*Session, error) {
	p, err := DefaultSessionPath()
	if err != nil {
		return &Session{}, err
	}
	s := &Session{path: p}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return &Session{path: p}, err
	}
	return s, nil
}

// Save writes the session atomically. Called on quit, where a blocking
// write costs nothing.
func (s *Session) Save() error {
	p, err := s.file()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(p, append(data, '\n'), 0o600)
}
