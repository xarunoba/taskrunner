package atomicfile

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteJSON atomically replaces path with an indented JSON encoding of value.
// The destination directory must already exist.
func WriteJSON(path string, mode fs.FileMode, value any) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".json-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempName := file.Name()
	defer func() {
		_ = os.Remove(tempName) // Rename removes this path on success.
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode JSON: %w", err)
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("set file permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	return nil
}
