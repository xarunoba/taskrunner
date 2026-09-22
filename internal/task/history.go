package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	file, err := os.CreateTemp(dir, ".history-*.json")
	if err != nil {
		return fmt.Errorf("create temporary value history: %w", err)
	}
	tempName := file.Name()
	defer func() {
		_ = os.Remove(tempName) // The rename below removes this path on success.
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(history); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode value history: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("set value history permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close value history: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("save value history: %w", err)
	}
	return nil
}

func (s *Store) valueHistoryPath(taskFile string) (string, error) {
	if taskFile == "" || filepath.Base(taskFile) != taskFile || filepath.Ext(taskFile) != ".json" {
		return "", errors.New("invalid task file")
	}
	return filepath.Join(s.workspace, ".taskrunner", "history", taskFile), nil
}
