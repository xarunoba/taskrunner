package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/xarunoba/taskrunner/internal/atomicfile"
	"github.com/xarunoba/taskrunner/internal/task"
	"golang.org/x/sys/unix"
)

const jobHistoryDirectory = "runs"

// maxPersistedRecordBytes bounds how many bytes of a single on-disk job
// record the daemon will read before allocating. The retained output window
// is maxRetainedOutput bytes; worst-case JSON escaping expands every byte to
// six (\u00xx), leaving room for ordinary command and environment metadata.
// Larger records are preserved and reported rather than read without a
// limit. This also bounds reads of legacy records with unlimited output.
const maxPersistedRecordBytes = 8 << 20

func loadJobHistory(workspace string) (map[string]*jobRecord, []string, []string, error) {
	dir := filepath.Join(workspace, ".taskrunner", jobHistoryDirectory)
	if err := task.RejectSymlinkedDirs(filepath.Dir(dir), dir); err != nil {
		return nil, nil, nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]*jobRecord), nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read job history: %w", err)
	}

	var warnings []string
	records := make([]*jobRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		stored, warn := readPersistedJob(path)
		if warn != "" {
			warnings = append(warnings, warn)
			continue
		}
		record := &jobRecord{Job: stored.Job, env: stored.Env, pid: stored.Pid, pidStart: stored.PidStart}
		output := stored.Output
		if stored.OutputBytes != nil {
			output = string(stored.OutputBytes)
		}
		record.output.restore(output, stored.OutputStart)
		record.Job.Output = ""
		record.Job.OutputStart = 0
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
	return jobs, order, warnings, nil
}

// readPersistedJob reads and decodes one on-disk job record. The read is
// bounded at maxPersistedRecordBytes so an oversized file never causes an
// unbounded allocation; the file itself is left untouched. A non-empty
// return of the second value is a warning naming the path and reason.
func readPersistedJob(path string) (persistedJob, string) {
	file, err := os.Open(path)
	if err != nil {
		return persistedJob{}, fmt.Sprintf("job history: skipping %s: %v", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return persistedJob{}, fmt.Sprintf("job history: skipping %q: %v", path, err)
	}
	if info.Size() > maxPersistedRecordBytes {
		return persistedJob{}, oversizedJobWarning(path)
	}
	// +1 so a record exactly at the limit still decodes, and anything
	// larger is detected without reading it whole.
	data, err := io.ReadAll(io.LimitReader(file, maxPersistedRecordBytes+1))
	if err != nil {
		return persistedJob{}, fmt.Sprintf("job history: skipping %s: %v", path, err)
	}
	if len(data) > maxPersistedRecordBytes {
		return persistedJob{}, oversizedJobWarning(path)
	}
	var stored persistedJob
	if err := json.Unmarshal(data, &stored); err != nil {
		return persistedJob{}, fmt.Sprintf("job history: skipping %q: corrupt record (invalid JSON or field value); file left in place", path)
	}
	if stored.ID == "" || filepath.Base(path) != stored.ID+".json" {
		return persistedJob{}, fmt.Sprintf("job history: skipping %s: invalid job id", path)
	}
	switch stored.Status {
	case StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusCanceled:
	default:
		return persistedJob{}, fmt.Sprintf("job history: skipping %q: invalid job status", path)
	}
	outputLength := len(stored.Output)
	if stored.OutputBytes != nil {
		outputLength = len(stored.OutputBytes)
	}
	if stored.OutputStart < 0 || stored.OutputStart > int(^uint(0)>>1)-outputLength {
		return persistedJob{}, fmt.Sprintf("job history: skipping %q: invalid output offset", path)
	}
	return stored, ""
}

func oversizedJobWarning(path string) string {
	return fmt.Sprintf("job history: skipping %q: record exceeds %d bytes; file left in place; move it outside runs or reduce its output before restarting", path, maxPersistedRecordBytes)
}

// persistedJob is the on-disk shape of a job record: the wire job plus the
// fields recovery and rerun need after a daemon restart.
type persistedJob struct {
	Job
	Env      []string `json:"env"`
	Pid      int      `json:"pid,omitempty"`
	PidStart uint64   `json:"pid_start,omitempty"`
	// Binary output is encoded losslessly so JSON's UTF-8 replacement does
	// not change absolute byte offsets after a restart.
	OutputBytes []byte `json:"output_bytes,omitempty"`
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
	job.Output, job.OutputStart, job.OutputSize = record.output.from(0, record.Done())
	stored := persistedJob{Job: job, Env: record.env, Pid: record.pid, PidStart: record.pidStart}
	if !utf8.ValidString(job.Output) {
		stored.OutputBytes = []byte(job.Output)
		stored.Output = ""
	}
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
