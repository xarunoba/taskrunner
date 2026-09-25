package atomicfile

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// WriteJSON atomically replaces path with an indented JSON encoding of value.
// The destination directory must already exist.
func WriteJSON(path string, mode fs.FileMode, value any) error {
	return writeJSON(path, mode, value, false)
}

// WriteJSONNewOnly atomically creates path with an indented JSON encoding of
// value and fails without touching an existing destination, so concurrent
// creators cannot silently overwrite each other. The destination directory
// must already exist.
func WriteJSONNewOnly(path string, mode fs.FileMode, value any) error {
	return writeJSON(path, mode, value, true)
}

func writeJSON(path string, mode fs.FileMode, value any, newOnly bool) error {
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
	// Chmod before Sync: the permission change is metadata and must hit
	// disk together with the data, or a crash can leave the old mode.
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("set file permissions: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync temporary directory entry: %w", err)
	}
	if newOnly {
		err = unix.Renameat2(unix.AT_FDCWD, tempName, unix.AT_FDCWD, path, unix.RENAME_NOREPLACE)
	} else {
		err = os.Rename(tempName, path)
	}
	if err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync destination directory: %w", err)
	}
	return nil
}

// syncDir flushes directory entries and reports durability failures.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	return err
}
