// Package theme owns the user's display preferences: the selected theme and
// its color palette. Preferences live under the user configuration directory,
// never inside a workspace, so task definitions stay shareable.
package theme

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/xarunoba/taskrunner/internal/atomicfile"
)

// defaultName is the built-in theme selected without configuration.
const defaultName = "default"

// ColorPair is a foreground and background color role.
type ColorPair struct {
	Foreground string `json:"foreground"`
	Background string `json:"background"`
}

// Palette holds every color role the TUI renders.
type Palette struct {
	Accent      string    `json:"accent"`
	Muted       string    `json:"muted"`
	Cursor      string    `json:"cursor"`
	Error       string    `json:"error"`
	Selection   ColorPair `json:"selection"`
	InactiveTab ColorPair `json:"inactive_tab"`
	Status      ColorPair `json:"status"`
	ErrorBadge  ColorPair `json:"error_badge"`
	Queued      ColorPair `json:"queued"`
	Succeeded   ColorPair `json:"succeeded"`
	Canceled    ColorPair `json:"canceled"`
}

// Theme is a named palette. The name comes from the configuration value and
// custom theme file names; it is never read from JSON.
type Theme struct {
	Name    string  `json:"-"`
	Palette Palette `json:"colors"`
}

// Default returns the built-in green-hued theme.
func Default() Theme {
	return Theme{
		Name: defaultName,
		Palette: Palette{
			Accent:      "34",
			Muted:       "241",
			Cursor:      "46",
			Error:       "196",
			Selection:   ColorPair{Foreground: "230", Background: "22"},
			InactiveTab: ColorPair{Foreground: "250", Background: "236"},
			Status:      ColorPair{Foreground: "250", Background: "236"},
			ErrorBadge:  ColorPair{Foreground: "230", Background: "196"},
			Queued:      ColorPair{Foreground: "230", Background: "34"},
			Succeeded:   ColorPair{Foreground: "0", Background: "42"},
			Canceled:    ColorPair{Foreground: "255", Background: "241"},
		},
	}
}

// Store resolves and persists themes under a Taskrunner configuration root.
type Store struct {
	root string
}

// Open returns the store for the current user's configuration directory.
func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user configuration directory: %w", err)
	}
	return NewStore(filepath.Join(dir, "taskrunner")), nil
}

// NewStore returns a store rooted at dir; tests use temporary directories.
func NewStore(dir string) *Store {
	return &Store{root: dir}
}

func (s *Store) configPath() string {
	return filepath.Join(s.root, "config.json")
}

func (s *Store) themesPath(name string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("invalid theme name %q", name)
	}
	return filepath.Join(s.root, "themes", name+".json"), nil
}

// CreateDefaultIfMissing persists the default configuration when no configuration file
// exists. An existing file, including a malformed one, is left untouched.
func (s *Store) CreateDefaultIfMissing() error {
	_, err := os.Stat(s.configPath())
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("stat %s: %w", s.configPath(), err)
	}
	if _, err := s.Select(defaultName); err != nil {
		return fmt.Errorf("create %s: %w", s.configPath(), err)
	}
	return nil
}

// Load returns the configured theme, or Default when no configuration exists.
// A missing configuration never creates the configuration directory.
func (s *Store) Load() (Theme, error) {
	raw, err := os.ReadFile(s.configPath())
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Theme{}, fmt.Errorf("read %s: %w", s.configPath(), err)
	}
	cfg, err := decodeConfig(raw)
	if err != nil {
		return Theme{}, fmt.Errorf("decode %s: %w", s.configPath(), err)
	}
	if cfg.theme == "" || cfg.theme == defaultName {
		return Default(), nil
	}
	return s.load(cfg.theme)
}

// Names lists the built-in theme followed by custom theme names in lexical
// order. A missing themes directory yields only the default.
func (s *Store) Names() ([]string, error) {
	names := []string{defaultName}
	dir := filepath.Join(s.root, "themes")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return names, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		stem, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || stem == defaultName || !validName(stem) {
			continue
		}
		names = append(names, stem)
	}
	slices.Sort(names[1:])
	return names, nil
}

// Select persists name as the chosen theme and returns it. Only the theme key
// is replaced; every other top-level configuration key is preserved.
func (s *Store) Select(name string) (Theme, error) {
	selected, err := s.load(name)
	if err != nil {
		return Theme{}, err
	}
	cfg := config{unknown: map[string]json.RawMessage{}}
	raw, err := os.ReadFile(s.configPath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Theme{}, fmt.Errorf("read %s: %w", s.configPath(), err)
	default:
		cfg, err = decodeConfig(raw)
		if err != nil {
			return Theme{}, fmt.Errorf("decode %s: %w", s.configPath(), err)
		}
	}
	cfg.theme = name
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return Theme{}, fmt.Errorf("create %s: %w", s.root, err)
	}
	if err := atomicfile.WriteJSON(s.configPath(), 0o600, cfg.object()); err != nil {
		return Theme{}, fmt.Errorf("write %s: %w", s.configPath(), err)
	}
	return selected, nil
}

// load resolves a theme by name; the default needs no file.
func (s *Store) load(name string) (Theme, error) {
	if name == defaultName {
		return Default(), nil
	}
	path, err := s.themesPath(name)
	if err != nil {
		return Theme{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, fmt.Errorf("read %s: %w", path, err)
	}
	var parsed Theme
	if err := decodeSingleDocument(raw, &parsed); err != nil {
		return Theme{}, fmt.Errorf("decode %s: %w", path, err)
	}
	parsed.Name = name
	if err := parsed.Palette.validate(); err != nil {
		return Theme{}, fmt.Errorf("theme %s: %w", name, err)
	}
	return parsed, nil
}

// config mirrors the persisted configuration object. Unknown top-level keys
// are retained so future settings survive a theme change.
type config struct {
	theme   string
	unknown map[string]json.RawMessage
}

func decodeConfig(raw []byte) (config, error) {
	var fields map[string]json.RawMessage
	if err := decodeSingleDocument(raw, &fields); err != nil {
		return config{}, err
	}
	cfg := config{unknown: make(map[string]json.RawMessage, len(fields))}
	for key, value := range fields {
		if key != "theme" {
			cfg.unknown[key] = value
			continue
		}
		if err := json.Unmarshal(value, &cfg.theme); err != nil {
			return config{}, fmt.Errorf("field %q: %w", key, err)
		}
	}
	return cfg, nil
}

// object returns the configuration as a raw JSON object with the theme key
// replaced and every unknown key preserved.
func (c config) object() map[string]json.RawMessage {
	object := make(map[string]json.RawMessage, len(c.unknown)+1)
	for key, value := range c.unknown {
		object[key] = value
	}
	object["theme"], _ = json.Marshal(c.theme)
	return object
}

// decodeSingleDocument decodes one JSON value and rejects trailing data.
func decodeSingleDocument(raw []byte, value any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(value); err != nil {
		return err
	}
	var extra json.RawMessage
	switch err := dec.Decode(&extra); {
	case err == nil:
		return errors.New("unexpected second JSON document")
	case !errors.Is(err, io.EOF):
		return err
	}
	return nil
}

func (p Palette) validate() error {
	for _, role := range []struct {
		name  string
		value string
	}{
		{"accent", p.Accent},
		{"muted", p.Muted},
		{"cursor", p.Cursor},
		{"error", p.Error},
	} {
		if strings.TrimSpace(role.value) == "" {
			return fmt.Errorf("color %q is required", role.name)
		}
	}
	for _, role := range []struct {
		name string
		pair ColorPair
	}{
		{"selection", p.Selection},
		{"inactive_tab", p.InactiveTab},
		{"status", p.Status},
		{"error_badge", p.ErrorBadge},
		{"queued", p.Queued},
		{"succeeded", p.Succeeded},
		{"canceled", p.Canceled},
	} {
		if strings.TrimSpace(role.pair.Foreground) == "" {
			return fmt.Errorf("color %q foreground is required", role.name)
		}
		if strings.TrimSpace(role.pair.Background) == "" {
			return fmt.Errorf("color %q background is required", role.name)
		}
	}
	return nil
}

// validName reports whether name is safe to use as a theme file stem.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
