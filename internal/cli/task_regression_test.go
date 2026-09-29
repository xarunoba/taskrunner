package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xarunoba/taskrunner/internal/task"
)

func TestFindTaskRejectsAmbiguousName(t *testing.T) {
	t.Parallel()

	items := []task.Task{
		{Name: "Deploy", Command: "one", File: "deploy.json"},
		{Name: "deploy", Command: "two", File: "deploy-2.json"},
	}
	_, err := findTask(items, "deploy")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("findTask(deploy) error = %v, want ambiguity rejection", err)
	}
	for _, file := range []string{"deploy.json", "deploy-2.json"} {
		if !strings.Contains(err.Error(), file) {
			t.Fatalf("findTask(deploy) error %v does not list %q", err, file)
		}
	}
}

func TestFindTaskRejectsAmbiguousCaseInsensitiveFile(t *testing.T) {
	t.Parallel()

	items := []task.Task{
		{Name: "One", Command: "one", File: "Build.json"},
		{Name: "Two", Command: "two", File: "build.json"},
	}
	if _, err := findTask(items, "BUILD.JSON"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("findTask(BUILD.JSON) error = %v, want ambiguity rejection", err)
	}

	item, err := findTask(items, "Build.json")
	if err != nil || item.File != "Build.json" {
		t.Fatalf("findTask(Build.json) = %#v, %v; want exact filename match", item, err)
	}
	item, err = findTask(items, "build.json")
	if err != nil || item.File != "build.json" {
		t.Fatalf("findTask(build.json) = %#v, %v; want exact filename match", item, err)
	}
}

func TestFindTaskRejectsAmbiguousStem(t *testing.T) {
	t.Parallel()

	items := []task.Task{
		{Name: "Unrelated", Command: "one", File: "deploy-prod.json"},
		{Name: "Also Unrelated", Command: "two", File: "Deploy-Prod.json"},
	}
	_, err := findTask(items, "deploy-prod")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("findTask(deploy-prod) error = %v, want ambiguity rejection", err)
	}
}

func TestFindTaskPriorityAndUniqueFallbacks(t *testing.T) {
	t.Parallel()

	items := []task.Task{
		{Name: "Zebra", Command: "zebra", File: "b.json"},
		{Name: "Other", Command: "other", File: "zebra.json"},
	}

	// Exact filename wins over every fallback.
	item, err := findTask(items, "zebra.json")
	if err != nil || item.File != "zebra.json" {
		t.Fatalf("findTask(zebra.json) = %#v, %v; want zebra.json", item, err)
	}
	// Case-insensitive filename outranks task name.
	item, err = findTask(items, "B.JSON")
	if err != nil || item.File != "b.json" {
		t.Fatalf("findTask(B.JSON) = %#v, %v; want b.json", item, err)
	}
	// Task name outranks stem.
	item, err = findTask(items, "ZEBRA")
	if err != nil || item.Name != "Zebra" {
		t.Fatalf("findTask(ZEBRA) = %#v, %v; want the Zebra task", item, err)
	}
	// Stem resolves when unique.
	item, err = findTask(items, "b")
	if err != nil || item.File != "b.json" {
		t.Fatalf("findTask(b) = %#v, %v; want b.json", item, err)
	}
}

func TestTasksJSONStaysCleanWithWarningsOnStderr(t *testing.T) {
	workspace := t.TempDir()
	store := task.NewStore(workspace)
	if _, err := store.Save(task.Task{Name: "Build", Command: "true"}, ""); err != nil {
		t.Fatalf("save valid task: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workspace, ".taskrunner", "tasks", "broken.json"),
		[]byte("{not json"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	var stdout, stderr bytes.Buffer
	if err := Execute([]string{"tasks", "--json"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	var views []taskView
	if err := json.Unmarshal(stdout.Bytes(), &views); err != nil {
		t.Fatalf("invalid JSON stdout: %v", err)
	}
	if len(views) != 1 || views[0].File != "build.json" {
		t.Fatalf("valid task missing from JSON output: %+v", views)
	}
	if !strings.Contains(stderr.String(), "broken.json") {
		t.Fatalf("stderr %q does not name broken.json", stderr.String())
	}
}

func TestValidateFailsForInvalidNamedFile(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := task.NewStore(workspace)
	if _, err := store.Save(task.Task{Name: "Build", Command: "true"}, ""); err != nil {
		t.Fatalf("save valid task: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workspace, ".taskrunner", "tasks", "broken.json"),
		[]byte(`{"name":"Broken"}`), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	items, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if err := executeTaskValidate(store, items, "broken.json"); err == nil {
		t.Fatal("validate(broken.json) = nil, want validation failure")
	} else if !strings.Contains(err.Error(), "broken.json") {
		t.Fatalf("validate(broken.json) error = %v, want the filename", err)
	}
	if err := executeTaskValidate(store, items, ""); err == nil {
		t.Fatal("validate all = nil, want failure for the invalid sibling")
	}
	if err := executeTaskValidate(store, items, "Build"); err != nil {
		t.Fatalf("validate(Build) error = %v", err)
	}
}

func TestRemoveMalformedFileByExactFilename(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := task.NewStore(workspace)
	if _, err := store.Save(task.Task{Name: "Build", Command: "true"}, ""); err != nil {
		t.Fatalf("save valid task: %v", err)
	}
	brokenPath := filepath.Join(workspace, ".taskrunner", "tasks", "broken.json")
	if err := os.WriteFile(brokenPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := executeTaskRemove(store, items, "broken.json", true, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("executeTaskRemove(broken.json) error = %v", err)
	}
	if _, err := os.Stat(brokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("broken.json still exists after removal: %v", err)
	}
}

func TestRemoveRejectsTraversalForInvalidFiles(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	store := task.NewStore(workspace)
	if _, err := store.Save(task.Task{Name: "Build", Command: "true"}, ""); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(workspace, ".taskrunner", "escape.json")
	if err := os.WriteFile(outside, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := executeTaskRemove(store, items, "../escape.json", true, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("remove(../escape.json) = nil, want rejection")
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "preserve" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
}

func TestInvalidExactFilenameCannotResolveToAnotherTask(t *testing.T) {
	workspace := t.TempDir()
	store := task.NewStore(workspace)
	valid, err := store.Save(task.Task{Name: "broken.json", Command: "printf wrong-task"}, "")
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(workspace, ".taskrunner", "tasks", "broken.json")
	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(workspace)
	for _, args := range [][]string{
		{"run", "broken.json", "--dry-run", "--quiet"},
		{"task", "show", "broken.json"},
		{"task", "validate", "broken.json"},
	} {
		var stdout, stderr bytes.Buffer
		if err := Execute(args, strings.NewReader(""), &stdout, &stderr); err == nil {
			t.Fatalf("%v resolved invalid filename to another task", args)
		}
		if strings.Contains(stdout.String(), "wrong-task") {
			t.Fatalf("%v exposed the wrong command", args)
		}
	}
	var stdout, stderr bytes.Buffer
	if err := Execute([]string{"task", "rm", "broken.json", "--force"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(broken); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid file was not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".taskrunner", "tasks", valid.File)); err != nil {
		t.Fatalf("valid task was removed instead: %v", err)
	}
}
