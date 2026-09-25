package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/xarunoba/taskrunner/internal/atomicfile"
	"github.com/xarunoba/taskrunner/internal/task"
	"golang.org/x/sys/unix"
)

const jobHistoryDirectory = "runs"

func loadJobHistory(workspace string) (map[string]*jobRecord, []string, error) {
	dir := filepath.Join(workspace, ".taskrunner", jobHistoryDirectory)
	if err := task.RejectSymlinkedDirs(filepath.Dir(dir), dir); err != nil {
		return nil, nil, err
	}
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
		var stored persistedJob
		if err := json.Unmarshal(data, &stored); err != nil {
			return nil, nil, fmt.Errorf("decode job %q: %w", entry.Name(), err)
		}
		if stored.ID == "" || entry.Name() != stored.ID+".json" {
			return nil, nil, fmt.Errorf("decode job %q: invalid job id", entry.Name())
		}
		record := &jobRecord{Job: stored.Job, env: stored.Env, pid: stored.Pid, pidStart: stored.PidStart}
		record.output.store(stored.Output)
		record.Job.Output = ""
		if record.Status == StatusQueued || record.Status == StatusRunning {
			// The previous daemon died without reaping this job. Kill its
			// surviving process group before reporting failure so a
			// restarted queue cannot run on top of it.
			if sameProcess(record.pid, record.pidStart) {
				_ = unix.Kill(-record.pid, unix.SIGKILL)
			}
			record.Status = StatusFailed
			record.Error = "daemon stopped before job completed"
			record.EndedAt = time.Now()
			record.pid = 0
			record.pidStart = 0
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

// persistedJob is the on-disk shape of a job record: the wire job plus the
// fields recovery and rerun need after a daemon restart.
type persistedJob struct {
	Job
	Env      []string `json:"env"`
	Pid      int      `json:"pid,omitempty"`
	PidStart uint64   `json:"pid_start,omitempty"`
}

func (s *server) persistLocked(record *jobRecord) error {
	dir := filepath.Join(s.workspace, ".taskrunner", jobHistoryDirectory)
	if err := task.RejectSymlinkedDirs(filepath.Dir(dir), dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create job history: %w", err)
	}
	job := record.Job
	job.Output, job.OutputSize = record.output.from(0, record.Done())
	stored := persistedJob{Job: job, Env: record.env, Pid: record.pid, PidStart: record.pidStart}
	path := filepath.Join(dir, record.ID+".json")
	if err := atomicfile.WriteJSON(path, 0o600, stored); err != nil {
		return fmt.Errorf("save job: %w", err)
	}
	return nil
}

func (s *server) removeLocked(record *jobRecord) error {
	path := filepath.Join(s.workspace, ".taskrunner", jobHistoryDirectory, record.ID+".json")
	if err := task.RejectSymlinkedDirs(filepath.Dir(filepath.Dir(path)), filepath.Dir(path)); err != nil {
		return err
	}
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
