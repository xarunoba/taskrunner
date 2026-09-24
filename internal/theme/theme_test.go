package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWithoutConfigReturnsDefaultAndCreatesNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewStore(root)
	selected, err := store.Load()
	if err != nil {
		t.Fatalf("load with missing config: %v", err)
	}
	if selected != Default() {
		t.Fatalf("missing config selected %#v, want %#v", selected, Default())
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("loading a missing config created %v", entries)
	}
}

func TestLoadExplicitDefaultAndConfiguredTheme(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		theme   string
		wantNow bool
	}{
		{name: "empty string", theme: "", wantNow: true},
		{name: "default", theme: "default", wantNow: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			writeConfig(t, root, `{"theme": "`+tc.theme+`"}`)
			selected, err := NewStore(root).Load()
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if selected.Name != "default" {
				t.Fatalf("selected %q, want default", selected.Name)
			}
			if tc.wantNow && selected != Default() {
				t.Fatalf("selected %#v, want %#v", selected, Default())
			}
		})
	}
}

func TestSelectCustomThemePersists(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTheme(t, root, "violet", validVioletTheme)
	store := NewStore(root)

	selected, err := store.Select("violet")
	if err != nil {
		t.Fatalf("select violet: %v", err)
	}
	if selected.Name != "violet" {
		t.Fatalf("selected %q, want violet", selected.Name)
	}
	if selected.Palette.Accent != "63" {
		t.Fatalf("selected accent %q, want 63", selected.Palette.Accent)
	}

	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded != selected {
		t.Fatalf("reloaded %#v, want %#v", reloaded, selected)
	}
}

func TestSelectDefaultThemeNeedsNoFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := NewStore(root).Select("default"); err != nil {
		t.Fatalf("select default: %v", err)
	}
	raw := readConfig(t, root)
	if !strings.Contains(raw, `"theme": "default"`) {
		t.Fatalf("config = %s, want persisted default theme", raw)
	}
}

func TestNamesSortsCustomThemesAfterDefault(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTheme(t, root, "zeta", validVioletTheme)
	writeTheme(t, root, "alpha", validVioletTheme)
	writeTheme(t, root, "default", validVioletTheme) // Skipped.
	writeTheme(t, root, "with space", validVioletTheme)
	if err := os.Mkdir(filepath.Join(root, "themes", "nested.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	names, err := NewStore(root).Names()
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	want := []string{"default", "alpha", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", names, want)
		}
	}
}

func TestNamesWithoutThemesDirectory(t *testing.T) {
	t.Parallel()

	names, err := NewStore(t.TempDir()).Names()
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if len(names) != 1 || names[0] != "default" {
		t.Fatalf("names = %#v, want [default]", names)
	}
}

func TestSelectRejectsUnsafeNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"", "with space", "slash/slash", "back\\slash", "..", "../escape",
		"dot.name", "uniçode", "quote\"", "tab\t", "newline\n", "emoji🎁",
	} {
		store := NewStore(t.TempDir())
		if _, err := store.Select(name); err == nil {
			t.Fatalf("select %q succeeded, want rejection", name)
		}
	}
}

func TestLoadMissingThemeFileFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeConfig(t, root, `{"theme": "absent"}`)
	if _, err := NewStore(root).Load(); err == nil {
		t.Fatal("load with missing theme file succeeded, want failure")
	}
}

func TestLoadRejectsMalformedConfigAndExtraDocuments(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"truncated":        `{"theme": `,
		"not an object":    `"theme"`,
		"non-string theme": `{"theme": 5}`,
		"second document":  `{"theme": "default"} {"later": true}`,
		"trailing garbage": `{} garbage`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeConfig(t, root, raw)
			if _, err := NewStore(root).Load(); err == nil {
				t.Fatalf("load %s succeeded, want failure", name)
			}
			if _, err := NewStore(root).Select("default"); err == nil {
				t.Fatalf("select over %s config succeeded, want abort", name)
			}
		})
	}
}

func TestLoadRejectsMalformedAndExtraThemeDocuments(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"truncated":       `{"colors": {`,
		"second document": validVioletTheme + "\n{}",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTheme(t, root, "broken", raw)
			writeConfig(t, root, `{"theme": "broken"}`)
			if _, err := NewStore(root).Load(); err == nil {
				t.Fatalf("load %s theme succeeded, want failure", name)
			}
		})
	}
}

func TestLoadRejectsIncompletePalettes(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"missing colors":       `{}`,
		"missing scalar role":  `{"colors": {"accent": "34"}}`,
		"missing pair role":    `{"colors": {"accent": "34", "muted": "241", "cursor": "46", "error": "196", "selection": {"foreground": "1", "background": "2"}}}`,
		"missing pair half":    `{"colors": {"accent": "34", "muted": "241", "cursor": "46", "error": "196", "selection": {"foreground": "1"}, "inactive_tab": {"foreground": "1", "background": "2"}, "status": {"foreground": "1", "background": "2"}, "error_badge": {"foreground": "1", "background": "2"}, "queued": {"foreground": "1", "background": "2"}, "succeeded": {"foreground": "1", "background": "2"}, "canceled": {"foreground": "1", "background": "2"}}}`,
		"blank color":          strings.Replace(validVioletTheme, `"accent": "63"`, `"accent": "  "`, 1),
		"non-string pair half": strings.Replace(validVioletTheme, `"background": "57"`, `"background": 57`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTheme(t, root, "incomplete", raw)
			writeConfig(t, root, `{"theme": "incomplete"}`)
			if _, err := NewStore(root).Load(); err == nil {
				t.Fatalf("load %s palette succeeded, want failure", name)
			}
		})
	}
}

func TestSelectIgnoresUnknownThemeKeys(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTheme(t, root, "violet", `{"future": true, "colors": {"accent": "63", "muted": "241", "cursor": "212", "error": "196", "selection": {"foreground": "230", "background": "57"}, "inactive_tab": {"foreground": "250", "background": "236"}, "status": {"foreground": "250", "background": "236"}, "error_badge": {"foreground": "230", "background": "196"}, "queued": {"foreground": "230", "background": "63"}, "succeeded": {"foreground": "0", "background": "42"}, "canceled": {"foreground": "255", "background": "241"}}}`)
	writeConfig(t, root, `{"theme": "violet"}`)
	selected, err := NewStore(root).Load()
	if err != nil {
		t.Fatalf("load theme with unknown key: %v", err)
	}
	if selected.Palette.Accent != "63" {
		t.Fatalf("accent = %q, want 63", selected.Palette.Accent)
	}
}

func TestSelectReplacesOnlyThemeKey(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTheme(t, root, "violet", validVioletTheme)
	writeConfig(t, root, `{"future-setting": {"nested": [1, 2]}, "theme": "default"}`)

	if _, err := NewStore(root).Select("violet"); err != nil {
		t.Fatalf("select violet: %v", err)
	}
	raw := readConfig(t, root)
	if !strings.Contains(raw, `"future-setting"`) {
		t.Fatalf("select dropped the unrelated configuration key:\n%s", raw)
	}
	if !strings.Contains(raw, `"theme": "violet"`) {
		t.Fatalf("select did not persist the theme:\n%s", raw)
	}
}

func TestCreateDefaultIfMissingOnlyWhenMissing(t *testing.T) {
	t.Parallel()

	t.Run("missing config is created with the default theme", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		store := NewStore(root)
		if err := store.CreateDefaultIfMissing(); err != nil {
			t.Fatalf("create default: %v", err)
		}
		if raw := readConfig(t, root); !strings.Contains(raw, `"theme": "default"`) {
			t.Fatalf("config = %s, want persisted default theme", raw)
		}
		selected, err := store.Load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if selected != Default() {
			t.Fatalf("loaded %#v, want %#v", selected, Default())
		}
		raw, err := os.ReadFile(filepath.Join(root, "themes", "default.json"))
		if err != nil {
			t.Fatalf("read default theme: %v", err)
		}
		var generated Theme
		if err := json.Unmarshal(raw, &generated); err != nil {
			t.Fatalf("decode generated default theme: %v", err)
		}
		if generated.Palette != Default().Palette {
			t.Fatalf("generated palette %#v, want %#v", generated.Palette, Default().Palette)
		}
	})

	t.Run("existing config with unknown keys is untouched", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeConfig(t, root, `{"theme":"violet","future-setting":42}`)
		before := readConfig(t, root)
		if err := NewStore(root).CreateDefaultIfMissing(); err != nil {
			t.Fatalf("create default: %v", err)
		}
		if after := readConfig(t, root); after != before {
			t.Fatalf("CreateDefaultIfMissing rewrote the config: %s", after)
		}
	})

	t.Run("existing default theme file is untouched", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTheme(t, root, "default", `{"colors": {"accent": "200"}}`)
		before, err := os.ReadFile(filepath.Join(root, "themes", "default.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := NewStore(root).CreateDefaultIfMissing(); err != nil {
			t.Fatalf("create default: %v", err)
		}
		after, err := os.ReadFile(filepath.Join(root, "themes", "default.json"))
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Fatalf("CreateDefaultIfMissing rewrote themes/default.json: %s", after)
		}
	})

	t.Run("missing default theme file is generated with existing config", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeConfig(t, root, `{"theme":"default","future-setting":42}`)
		if err := NewStore(root).CreateDefaultIfMissing(); err != nil {
			t.Fatalf("create default: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(root, "themes", "default.json"))
		if err != nil {
			t.Fatalf("read generated default theme: %v", err)
		}
		var generated Theme
		if err := json.Unmarshal(raw, &generated); err != nil {
			t.Fatalf("decode generated default theme: %v", err)
		}
		if generated.Palette != Default().Palette {
			t.Fatalf("generated palette %#v, want %#v", generated.Palette, Default().Palette)
		}
	})

	t.Run("stale generated default theme is refreshed", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTheme(t, root, "default", `{
  "colors": {
    "accent": "34",
    "canceled": {
      "background": "241",
      "foreground": "255"
    },
    "cursor": "46",
    "error": "196",
    "error_badge": {
      "background": "196",
      "foreground": "230"
    },
    "inactive_tab": {
      "background": "236",
      "foreground": "250"
    },
    "muted": "241",
    "queued": {
      "background": "34",
      "foreground": "230"
    },
    "selection": {
      "background": "22",
      "foreground": "230"
    },
    "status": {
      "background": "236",
      "foreground": "250"
    },
    "succeeded": {
      "background": "42",
      "foreground": "0"
    }
  }
}`)
		if err := NewStore(root).CreateDefaultIfMissing(); err != nil {
			t.Fatalf("create default: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(root, "themes", "default.json"))
		if err != nil {
			t.Fatal(err)
		}
		var generated Theme
		if err := json.Unmarshal(raw, &generated); err != nil {
			t.Fatal(err)
		}
		if generated.Palette != Default().Palette {
			t.Fatalf("stale file was not refreshed: %#v", generated.Palette)
		}
	})

	t.Run("malformed config is untouched", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeConfig(t, root, `{not json`)
		before := readConfig(t, root)
		if err := NewStore(root).CreateDefaultIfMissing(); err != nil {
			t.Fatalf("create default: %v", err)
		}
		if after := readConfig(t, root); after != before {
			t.Fatalf("CreateDefaultIfMissing rewrote a malformed config: %s", after)
		}
	})
}

func TestDefaultThemeFileOverridesBuiltinRoles(t *testing.T) {
	t.Parallel()

	t.Run("partial override merges over the built-in palette", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTheme(t, root, "default", `{"colors": {"accent": "200"}}`)
		selected, err := NewStore(root).Load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		want := Default()
		want.Palette.Accent = "200"
		if selected != want {
			t.Fatalf("loaded %#v, want %#v", selected, want)
		}
		if selected.Name != "default" {
			t.Fatalf("name %q, want default", selected.Name)
		}
	})

	t.Run("full override replaces every role it names", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTheme(t, root, "default", `{"colors": {"accent": "200", "selection": {"foreground": "15", "background": "24"}}}`)
		selected, err := NewStore(root).Load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if selected.Palette.Accent != "200" || selected.Palette.Selection != (ColorPair{Foreground: "15", Background: "24"}) {
			t.Fatalf("unexpected palette: %#v", selected.Palette)
		}
		if selected.Palette.Muted != Default().Palette.Muted {
			t.Fatalf("muted %q, want built-in %q", selected.Palette.Muted, Default().Palette.Muted)
		}
	})

	t.Run("unknown color role is rejected", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTheme(t, root, "default", `{"colors": {"sparkle": "1"}}`)
		if _, err := NewStore(root).Load(); err == nil {
			t.Fatal("load accepted an unknown default color role")
		}
	})

	t.Run("malformed default file is an error", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeTheme(t, root, "default", `{bad`)
		if _, err := NewStore(root).Load(); err == nil {
			t.Fatal("load accepted a malformed default theme file")
		}
	})

	t.Run("missing default file keeps the built-in palette", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		selected, err := NewStore(root).Load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if selected != Default() {
			t.Fatalf("loaded %#v, want %#v", selected, Default())
		}
	})
}

func TestSelectWritesAtomicFilePermissions(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "config")
	writeTheme(t, root, "violet", validVioletTheme)

	if _, err := NewStore(root).Select("violet"); err != nil {
		t.Fatalf("select violet: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config permissions = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(root)
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("configuration directory permissions = %o, want 700", perm)
	}

	// The temporary file is gone; only the atomic replacement remains.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".json-") {
			t.Fatalf("select left a temporary file behind: %s", entry.Name())
		}
	}
}

func writeConfig(t *testing.T, root, raw string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeTheme(t *testing.T, root, name, raw string) {
	t.Helper()
	dir := filepath.Join(root, "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readConfig(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(raw)
}

const validVioletTheme = `{
	"colors": {
		"accent": "63",
		"muted": "241",
		"cursor": "212",
		"error": "196",
		"selection": {"foreground": "230", "background": "57"},
		"inactive_tab": {"foreground": "250", "background": "236"},
		"status": {"foreground": "250", "background": "236"},
		"error_badge": {"foreground": "230", "background": "196"},
		"queued": {"foreground": "230", "background": "63"},
		"succeeded": {"foreground": "0", "background": "42"},
		"canceled": {"foreground": "255", "background": "241"}
	}
}`
