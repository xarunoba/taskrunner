package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestValueHistoryPersistsRecentUniqueTextValues(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := NewStore(workspace)
	item := Task{
		Name: "Deploy",
		File: "deploy.json",
		Fields: []Field{
			{Key: "version", Label: "Version", Type: FieldText},
			{Key: "environment", Label: "Environment", Type: FieldChoice, Options: []string{"staging"}},
		},
	}
	for _, value := range []string{"1.0.0", "1.1.0", "1.0.0"} {
		if err := store.RecordValueHistory(item, map[string]string{
			"version":     value,
			"environment": "staging",
		}); err != nil {
			t.Fatalf("RecordValueHistory() error = %v", err)
		}
	}

	history, err := store.ValueHistory(item.File, "version")
	if err != nil {
		t.Fatalf("ValueHistory() error = %v", err)
	}
	if len(history) != 2 || history[0] != "1.0.0" || history[1] != "1.1.0" {
		t.Fatalf("history = %#v, want [1.0.0 1.1.0]", history)
	}
	choices, err := store.ValueHistory(item.File, "environment")
	if err != nil {
		t.Fatalf("ValueHistory() choice error = %v", err)
	}
	if len(choices) != 0 {
		t.Fatalf("choice history = %#v, want empty", choices)
	}

	info, err := os.Stat(filepath.Join(workspace, ".taskrunner", "history", item.File))
	if err != nil {
		t.Fatalf("stat history file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("history permissions = %o, want 600", got)
	}
}

func TestValueHistoryRejectsTaskPathTraversal(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	if _, err := store.ValueHistory("../task.json", "value"); err == nil {
		t.Fatal("ValueHistory() accepted task path traversal")
	}
}

func TestRecordValueHistorySurvivesNullFile(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := NewStore(workspace)
	item := Task{
		Name: "Deploy",
		File: "deploy.json",
		Fields: []Field{
			{Key: "version", Label: "Version", Type: FieldText},
		},
	}
	historyDir := filepath.Join(workspace, ".taskrunner", "history")
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(historyDir, item.File), []byte("null"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.RecordValueHistory(item, map[string]string{"version": "1.0.0"}); err != nil {
		t.Fatalf("RecordValueHistory() error = %v", err)
	}
	history, err := store.ValueHistory(item.File, "version")
	if err != nil {
		t.Fatalf("ValueHistory() error = %v", err)
	}
	if len(history) != 1 || history[0] != "1.0.0" {
		t.Fatalf("history = %#v, want [1.0.0]", history)
	}
}

func TestConcurrentRecordValueHistoryKeepsEveryValue(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	item := Task{
		Name: "Deploy",
		File: "deploy.json",
		Fields: []Field{
			{Key: "version", Label: "Version", Type: FieldText},
		},
	}

	const writers = 4
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	for i := range writers {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			store := NewStore(workspace)
			start.Wait()
			if err := store.RecordValueHistory(item, map[string]string{"version": fmt.Sprintf("1.%d.0", i)}); err != nil {
				t.Errorf("RecordValueHistory() error = %v", err)
			}
		}(i)
	}
	start.Done()
	done.Wait()

	history, err := NewStore(workspace).ValueHistory(item.File, "version")
	if err != nil {
		t.Fatalf("ValueHistory() error = %v", err)
	}
	if len(history) != writers {
		t.Fatalf("history = %#v, want %d recorded values", history, writers)
	}
	seen := make(map[string]bool, writers)
	for _, value := range history {
		seen[value] = true
	}
	for i := range writers {
		if !seen[fmt.Sprintf("1.%d.0", i)] {
			t.Fatalf("history = %#v, missing value from writer %d", history, i)
		}
	}
}

func TestValueHistoryRejectsSymlinkedHistoryFile(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := NewStore(workspace)
	item := Task{Name: "Deploy", Command: "true", File: "deploy.json"}
	if err := os.MkdirAll(filepath.Join(workspace, ".taskrunner", "history"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(workspace, ".taskrunner", "history", "deploy.json")); err != nil {
		t.Fatal(err)
	}

	if err := store.RecordValueHistory(item, map[string]string{"version": "1.0.0"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("RecordValueHistory() error = %v, want symbolic link rejection", err)
	}
	if _, err := store.ValueHistory(item.File, "version"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("ValueHistory() error = %v, want symbolic link rejection", err)
	}
}

func TestValueHistoryRejectsSymlinkedDirectories(t *testing.T) {
	t.Parallel()
	for _, directory := range []string{".taskrunner", ".taskrunner/history"} {
		t.Run(directory, func(t *testing.T) {
			workspace, outside := t.TempDir(), t.TempDir()
			target := outside
			if directory == ".taskrunner" {
				target = filepath.Join(outside, "history")
			}
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "deploy.json"), []byte(`{"version":["outside"]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(workspace, directory)
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			if values, err := NewStore(workspace).ValueHistory("deploy.json", "version"); err == nil {
				t.Fatalf("read values through symlinked %s: %v", directory, values)
			}
		})
	}
}
