package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateGitignoreIfMissing(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, ".taskrunner", ".gitignore")

	if err := CreateGitignoreIfMissing(workspace); err != nil {
		t.Fatalf("create gitignore: %v", err)
	}

	custom := "# keep tasks only\n"
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CreateGitignoreIfMissing(workspace); err != nil {
		t.Fatalf("recreate gitignore: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reread gitignore: %v", err)
	}
	if string(raw) != custom {
		t.Fatalf("existing gitignore overwritten: %q", raw)
	}
}
