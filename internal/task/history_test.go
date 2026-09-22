package task

import (
	"os"
	"path/filepath"
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
