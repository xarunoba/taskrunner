package atomicfile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteJSONAppliesModeAndRoundTrips(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := WriteJSON(path, 0o600, map[string]string{"k": "v"}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}

	var got map[string]string
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode written file: %v", err)
	}
	if got["k"] != "v" {
		t.Fatalf("content = %v, want k=v", got)
	}
	if entries, err := os.ReadDir(dir); err != nil {
		t.Fatal(err)
	} else if len(entries) != 1 {
		t.Fatalf("temporary files leaked: %d entries", len(entries))
	}
}

func TestWriteJSONNewOnlyNeverReplaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSONNewOnly(path, 0o600, map[string]string{}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("WriteJSONNewOnly() error = %v, want os.ErrExist", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original\n" {
		t.Fatalf("existing file overwritten: %q", data)
	}
	if entries, err := os.ReadDir(dir); err != nil {
		t.Fatal(err)
	} else if len(entries) != 1 {
		t.Fatalf("temporary files leaked: %d entries", len(entries))
	}
}
