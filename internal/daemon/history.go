package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const jobHistoryDirectory = "runs"

func loadJobHistory(workspace string) (map[string]*jobRecord, []string, error) {
	dir := filepath.Join(workspace, ".taskrunner", jobHistoryDirectory)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]*jobRecord), nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read job history: %w", err)
	}

	records := make([]*jobRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, nil, fmt.Errorf("read job %q: %w", entry.Name(), err)
		}
		var job Job
		if err := json.Unmarshal(data, &job); err != nil {
			return nil, nil, fmt.Errorf("decode job %q: %w", entry.Name(), err)
		}
		if job.ID == "" || entry.Name() != job.ID+".json" {
			return nil, nil, fmt.Errorf("decode job %q: invalid job id", entry.Name())
		}
		record := &jobRecord{Job: job}
		record.output.Set(job.Output)
		record.Job.Output = ""
		if record.Status == StatusQueued || record.Status == StatusRunning {
			record.Status = StatusFailed
			record.Error = "daemon stopped before job completed"
			record.EndedAt = time.Now()
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})

	jobs := make(map[string]*jobRecord, len(records))
	order := make([]string, 0, len(records))
	for _, record := range records {
		jobs[record.ID] = record
		order = append(order, record.ID)
	}
	return jobs, order, nil
}

func (s *server) persistLocked(record *jobRecord) error {
	dir := filepath.Join(s.workspace, ".taskrunner", jobHistoryDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create job history: %w", err)
	}
	file, err := os.CreateTemp(dir, ".job-*.json")
	if err != nil {
		return fmt.Errorf("create temporary job: %w", err)
	}
	tempName := file.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()

	job := record.Job
	job.Output, job.OutputSize = record.output.Slice(0)
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(job); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode job: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("set job permissions: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close job: %w", err)
	}
	path := filepath.Join(dir, record.ID+".json")
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("save job: %w", err)
	}
	return nil
}

func (s *server) removeLocked(record *jobRecord) error {
	path := filepath.Join(s.workspace, ".taskrunner", jobHistoryDirectory, record.ID+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove job: %w", err)
	}
	delete(s.jobs, record.ID)
	for i, id := range s.order {
		if id == record.ID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return nil
}
