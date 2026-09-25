package task

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSaveRejectsSymlinkedTasksDirectory(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".taskrunner"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, ".taskrunner", "tasks")); err != nil {
		t.Fatal(err)
	}

	store := NewStore(workspace)
	_, err := store.Save(Task{Name: "Build", Command: "true"}, "")
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Save() error = %v, want symbolic link rejection", err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Load() error = %v, want symbolic link rejection", err)
	}
	if err := store.Delete(Task{Name: "Build", File: "build.json"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Delete() error = %v, want symbolic link rejection", err)
	}
}

func TestSaveRejectsSymlinkedTaskRunnerRoot(t *testing.T) {
	t.Parallel()

	workspace, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, ".taskrunner")); err != nil {
		t.Fatal(err)
	}

	store := NewStore(workspace)
	if _, err := store.Save(Task{Name: "Build", Command: "true"}, ""); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Save() error = %v, want symbolic link rejection", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("Save wrote %d entries outside the workspace", len(entries))
	}
}

func TestConcurrentSaveCreatesOnlyOneTask(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := NewStore(workspace)

	const savers = 4
	results := make(chan error, savers)
	var start sync.WaitGroup
	start.Add(1)
	for i := range savers {
		go func(i int) {
			start.Wait()
			_, err := store.Save(Task{Name: "Build", Command: "true"}, "")
			results <- err
		}(i)
	}
	start.Done()

	succeeded := 0
	for range savers {
		if err := <-results; err == nil {
			succeeded++
		} else if !errors.Is(err, ErrTaskExists) {
			t.Fatalf("Save() error = %v, want success or ErrTaskExists", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful saves = %d, want 1", succeeded)
	}
	items, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("stored tasks = %d, want 1", len(items))
	}
}

func TestSaveRenameKeepsOldFileOnCollision(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := NewStore(workspace)
	if _, err := store.Save(Task{Name: "Build", Command: "true"}, ""); err != nil {
		t.Fatalf("save first task: %v", err)
	}
	if _, err := store.Save(Task{Name: "Other", Command: "true"}, ""); err != nil {
		t.Fatalf("save second task: %v", err)
	}

	// Renaming "Other" to "Build" collides with the existing file and must
	// leave both original definitions untouched.
	if _, err := store.Save(Task{Name: "Build", Command: "false"}, "other.json"); !errors.Is(err, ErrTaskExists) {
		t.Fatalf("rename collision error = %v, want ErrTaskExists", err)
	}
	items, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("stored tasks = %d, want 2", len(items))
	}
	for _, item := range items {
		if item.Command != "true" {
			t.Fatalf("stored command for %q = %q, want true", item.Name, item.Command)
		}
	}
}
