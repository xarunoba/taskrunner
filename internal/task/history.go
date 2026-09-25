package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/xarunoba/taskrunner/internal/atomicfile"
)

const valueHistoryLimit = 100

func (s *Store) ValueHistory(taskFile, fieldKey string) ([]string, error) {
	history, err := s.loadValueHistory(taskFile)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), history[fieldKey]...), nil
}

func (s *Store) RecordValueHistory(item Task, values map[string]string) error {
	path, err := s.valueHistoryPath(item.File)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create value history directory: %w", err)
	}
	// The whole read-merge-write transaction runs under an advisory lock so
	// concurrent sessions record their values instead of overwriting each
	// other's entries with stale snapshots.
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open value history lock: %w", err)
	}
	defer lock.Close() // Closing releases the flock.
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return fmt.Errorf("lock value history: %w", err)
	}
	history, err := s.loadValueHistory(item.File)
	if err != nil {
		return err
	}

	changed := false
	for _, field := range item.Fields {
		if field.Type != FieldText {
			continue
		}
		value := values[field.Key]
		if value == "" {
			continue
		}

		previous := history[field.Key]
		updated := make([]string, 0, min(valueHistoryLimit, len(previous)+1))
		updated = append(updated, value)
		for _, candidate := range previous {
			if candidate == value {
				continue
			}
			updated = append(updated, candidate)
			if len(updated) == valueHistoryLimit {
				break
			}
		}
		history[field.Key] = updated
		changed = true
	}
	if !changed {
		return nil
	}
	return s.saveValueHistory(item.File, history)
}

func (s *Store) loadValueHistory(taskFile string) (map[string][]string, error) {
	path, err := s.valueHistoryPath(taskFile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string][]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read value history: %w", err)
	}

	history := make(map[string][]string)
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("decode value history: %w", err)
	}
	if history == nil {
		// A JSON null document decodes to a nil map without an error.
		history = make(map[string][]string)
	}
	return history, nil
}

func (s *Store) saveValueHistory(taskFile string, history map[string][]string) error {
	path, err := s.valueHistoryPath(taskFile)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create value history directory: %w", err)
	}

	if err := atomicfile.WriteJSON(path, 0o600, history); err != nil {
		return fmt.Errorf("save value history: %w", err)
	}
	return nil
}

func (s *Store) valueHistoryPath(taskFile string) (string, error) {
	if taskFile == "" || filepath.Base(taskFile) != taskFile || filepath.Ext(taskFile) != ".json" {
		return "", errors.New("invalid task file")
	}
	if err := s.verifyHistoryPaths(); err != nil {
		return "", err
	}
	path := filepath.Join(s.workspace, ".taskrunner", "history", taskFile)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("history file %s is a symbolic link", path)
	}
	return path, nil
}

func (s *Store) verifyHistoryPaths() error {
	return RejectSymlinkedDirs(
		filepath.Join(s.workspace, ".taskrunner"),
		filepath.Join(s.workspace, ".taskrunner", "history"),
	)
}
