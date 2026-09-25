package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/xarunoba/taskrunner/internal/atomicfile"
)

var ErrTaskExists = errors.New("task already exists")

type Store struct {
	workspace string
	dir       string
}

func NewStore(workspace string) *Store {
	return &Store{
		workspace: workspace,
		dir:       filepath.Join(workspace, ".taskrunner", "tasks"),
	}
}

func (s *Store) Workspace() string {
	return s.workspace
}

func (s *Store) Load() ([]Task, error) {
	if err := RejectSymlinkedDirs(s.taskRunnerDir(), s.dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Task{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}

	tasks := make([]Task, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(s.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read task %q: %w", entry.Name(), err)
		}

		var item Task
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("decode task %q: %w", entry.Name(), err)
		}
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("decode task %q: %w", entry.Name(), err)
		}
		item.File = entry.Name()
		tasks = append(tasks, item)
	}

	sort.Slice(tasks, func(i, j int) bool {
		return strings.ToLower(tasks[i].Name) < strings.ToLower(tasks[j].Name)
	})
	return tasks, nil
}

func (s *Store) Save(item Task, previousFile string) (Task, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.Command = strings.TrimSpace(item.Command)
	for i := range item.Fields {
		item.Fields[i].Key = strings.TrimSpace(item.Fields[i].Key)
		item.Fields[i].Label = strings.TrimSpace(item.Fields[i].Label)
		for j := range item.Fields[i].Options {
			item.Fields[i].Options[j] = strings.TrimSpace(item.Fields[i].Options[j])
		}
	}
	if err := item.Validate(); err != nil {
		return Task{}, err
	}

	if err := RejectSymlinkedDirs(s.taskRunnerDir(), s.dir); err != nil {
		return Task{}, err
	}
	if err := os.MkdirAll(s.taskRunnerDir(), 0o700); err != nil {
		return Task{}, fmt.Errorf("create taskrunner directory: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Task{}, fmt.Errorf("create task directory: %w", err)
	}
	if err := RejectSymlinkedDirs(s.taskRunnerDir(), s.dir); err != nil {
		return Task{}, err
	}

	fileSlug := slug(item.Name)
	if fileSlug == "" {
		return Task{}, errors.New("name must contain a letter or number")
	}
	item.File = fileSlug + ".json"

	target := filepath.Join(s.dir, item.File)
	if item.File == previousFile {
		if err := atomicfile.WriteJSON(target, 0o644, item); err != nil {
			return Task{}, fmt.Errorf("save task: %w", err)
		}
		return item, nil
	}
	// New task files must not replace an existing definition, even when one
	// is created concurrently by another editor instance.
	if err := atomicfile.WriteJSONNewOnly(target, 0o644, item); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Task{}, fmt.Errorf("%w: %s", ErrTaskExists, item.Name)
		}
		return Task{}, fmt.Errorf("save task: %w", err)
	}
	if previousFile != "" {
		if err := s.remove(previousFile); err != nil {
			return Task{}, fmt.Errorf("remove renamed task: %w", err)
		}
	}
	return item, nil
}

func (s *Store) Delete(item Task) error {
	if err := RejectSymlinkedDirs(s.taskRunnerDir(), s.dir); err != nil {
		return fmt.Errorf("delete task %q: %w", item.Name, err)
	}
	if err := s.remove(item.File); err != nil {
		return fmt.Errorf("delete task %q: %w", item.Name, err)
	}
	return nil
}

func (s *Store) remove(name string) error {
	if name == "" || filepath.Base(name) != name || filepath.Ext(name) != ".json" {
		return errors.New("invalid task file")
	}
	return os.Remove(filepath.Join(s.dir, name))
}

func (s *Store) taskRunnerDir() string {
	return filepath.Join(s.workspace, ".taskrunner")
}

func slug(name string) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			separator = false
		case b.Len() > 0 && !separator:
			b.WriteByte('-')
			separator = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// RejectSymlinkedDirs errors when any directory is a symbolic link, so
// workspace storage cannot be redirected outside the workspace. Missing
// directories pass; callers create them with MkdirAll as real directories.
func RejectSymlinkedDirs(dirs ...string) error {
	for _, dir := range dirs {
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect storage directory %s: %w", dir, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("storage directory %s is a symbolic link", dir)
		}
	}
	return nil
}
