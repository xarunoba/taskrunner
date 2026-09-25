package task

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	file, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create gitignore: %w", err)
	}
	if _, err := file.WriteString(gitignoreContent); err != nil {
		file.Close()
		return fmt.Errorf("write gitignore: %w", err)
	}
	return file.Close()
}
