package task

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xarunoba/taskrunner/internal/atomicfile"
)

// gitignoreContent keeps only task definitions shareable; value history,
// job runs, and daemon state stay untracked.
const gitignoreContent = "/*\n!/tasks\n!/.gitignore\n"

// CreateGitignoreIfMissing writes .taskrunner/.gitignore when absent and
// never touches an existing file.
func CreateGitignoreIfMissing(workspace string) error {
	dir := filepath.Join(workspace, ".taskrunner")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create taskrunner directory: %w", err)
	}
	if err := RejectSymlinkedDirs(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Lstat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect gitignore: %w", err)
	}
	if err := atomicfile.WriteStringNewOnly(path, 0o644, gitignoreContent); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("create gitignore: %w", err)
	}
	return nil
}
