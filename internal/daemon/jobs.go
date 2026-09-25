package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xarunoba/taskrunner/internal/task"
	"golang.org/x/sys/unix"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(data)
}

func (b *synchronizedBuffer) store(value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b.Reset()
	b.b.WriteString(value)
}

// from returns the buffered output from offset up to the last complete
// UTF-8 sequence, plus the boundary length clients use as their next
// offset. A multibyte character still being written is withheld so JSON
// encoding never replaces a split rune with U+FFFD. Once the job is done
// no further bytes can arrive, so flush delivers the trailing bytes with
// any incomplete sequence replaced by U+FFFD rather than dropping them.
func (b *synchronizedBuffer) from(offset int, flush bool) (string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data := b.b.Bytes()
	if offset < 0 || offset > len(data) {
		offset = 0
	}
	end := utf8Boundary(data)
	if flush {
		end = len(data)
	}
	if offset > end {
		offset = end
	}
	return string(data[offset:end]), end
}

// utf8Boundary returns the length of data cut after the last complete or
// permanently invalid UTF-8 sequence. An incomplete trailing sequence is
// withheld until its remaining bytes arrive.
func utf8Boundary(data []byte) int {
	for back := 1; back <= 3 && back <= len(data); back++ {
		b := data[len(data)-back]
		if b < 0x80 {
			// ASCII: any continuation bytes before it are invalid, not
			// incomplete, so pass them through.
			return len(data)
		}
		if b < 0xC0 {
			// Continuation byte: keep looking for its lead byte.
			continue
		}
		var expected int
		var secondMin, secondMax byte
		switch {
		case b >= 0xC2 && b <= 0xDF:
			expected, secondMin, secondMax = 2, 0x80, 0xBF
		case b >= 0xE0 && b <= 0xEF:
			expected, secondMin, secondMax = 3, 0x80, 0xBF
			if b == 0xE0 {
				secondMin = 0xA0
			} else if b == 0xED {
				secondMax = 0x9F
			}
		case b >= 0xF0 && b <= 0xF4:
			expected, secondMin, secondMax = 4, 0x80, 0xBF
			if b == 0xF0 {
				secondMin = 0x90
			} else if b == 0xF4 {
				secondMax = 0x8F
			}
		default:
			return len(data) // Invalid lead byte: not an incomplete sequence.
		}
		if back < expected {
			// Only withhold if the bytes so far could still become a valid
			// sequence. A continuation byte outside the lead's range makes
			// the sequence permanently invalid, so pass it through instead
			// of holding it until the next write (or forever).
			if back >= 2 {
				second := data[len(data)-back+1]
				if second < secondMin || second > secondMax {
					return len(data)
				}
			}
			return len(data) - back
		}
		return len(data)
	}
	return len(data)
}

type jobRecord struct {
	Job
	cancel       context.CancelFunc
	cancelReason string
	output       synchronizedBuffer
	// env is the invoking process environment for this job, replayed on
	// rerun. A nil env falls back to the daemon's own environment.
	env []string
	// pid and pidStart identify the job's process group across daemon
	// restarts: pid is the group leader, pidStart its kernel start time,
	// checked before signaling so a reused PID is never killed.
	pid      int
	pidStart uint64
}

func (s *server) startJobLocked(taskID, name, command string, policy task.JobPolicy, env []string) response {
	if taskID == "" || name == "" || command == "" {
		return response{Error: "task id, name, and command are required"}
	}
	switch policy {
	case task.JobSequential, task.JobParallel, task.JobCancelPrevious:
	default:
		return response{Error: fmt.Sprintf("unknown job policy %q", policy)}
	}
	s.sequence++
	id := strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(s.sequence, 36)
	record := &jobRecord{
		Job: Job{
			ID:        id,
			TaskID:    taskID,
			Name:      name,
			Command:   command,
			Policy:    policy,
			Status:    StatusQueued,
			CreatedAt: time.Now(),
		},
		env: env,
	}
	s.jobs[id] = record
	s.order = append(s.order, id)
	if err := s.persistLocked(record); err != nil {
		delete(s.jobs, id)
		s.order = s.order[:len(s.order)-1]
		return response{Error: err.Error()}
	}
	switch policy {
	case task.JobParallel:
		s.startLocked(record)
	case task.JobCancelPrevious:
		s.cancelPreviousLocked(taskID, id)
		s.startLocked(record)
	default:
		if s.running[taskID] == 0 {
			s.startLocked(record)
		}
	}
	job := s.snapshotLocked(record, 0, false)
	return response{Job: &job}
}

func (s *server) snapshotLocked(record *jobRecord, outputOffset int, includeOutput bool) Job {
	job := record.Job
	flush := record.Done()
	if includeOutput {
		job.Output, job.OutputSize = record.output.from(outputOffset, flush)
	} else {
		_, job.OutputSize = record.output.from(0, flush)
	}
	return job
}

func (s *server) cancelPreviousLocked(taskID, exceptID string) {
	for _, id := range s.order {
		record := s.jobs[id]
		if id == exceptID || record.TaskID != taskID || record.Done() {
			continue
		}
		_ = s.cancelJobLocked(record, "canceled by newer job")
	}
}

func (s *server) cancelJobLocked(record *jobRecord, reason string) error {
	switch record.Status {
	case StatusQueued:
		record.Status = StatusCanceled
		record.Error = reason
		record.EndedAt = time.Now()
		if err := s.persistLocked(record); err != nil {
			record.StorageError = err.Error()
			return err
		}
		return nil
	case StatusRunning:
		record.cancelReason = reason
		if record.cancel != nil {
			record.cancel()
		}
		return nil
	default:
		return errors.New("job already finished")
	}
}

func (s *server) startLocked(record *jobRecord) {
	ctx, cancel := context.WithCancel(context.Background())
	record.cancel = cancel
	record.Status = StatusRunning
	record.StartedAt = time.Now()
	s.running[record.TaskID]++
	s.lastActivity = time.Now()
	if err := s.persistLocked(record); err != nil {
		record.StorageError = err.Error()
	}
	s.workers.Go(func() {
		defer cancel()
		s.execute(ctx, record)
	})
}

func (s *server) execute(ctx context.Context, record *jobRecord) {
	shell := envValue(record.env, "SHELL")
	if record.env == nil {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", record.Command)
	if record.env != nil {
		cmd.Env = record.env
	}
	cmd.Dir = s.workspace
	cmd.Stdout = &record.output
	cmd.Stderr = &record.output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := unix.Kill(-cmd.Process.Pid, unix.SIGKILL); err != nil {
			if errors.Is(err, unix.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
	var err error
	if startErr := cmd.Start(); startErr != nil {
		err = startErr
	} else {
		s.mu.Lock()
		record.pid = cmd.Process.Pid
		record.pidStart = procStartTime(record.pid)
		if persistErr := s.persistLocked(record); persistErr != nil {
			record.StorageError = persistErr.Error()
		}
		s.mu.Unlock()
		err = cmd.Wait()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	record.cancel = nil
	record.pid = 0
	record.pidStart = 0
	record.EndedAt = time.Now()
	switch {
	case err == nil:
		record.Status = StatusSucceeded
	case ctx.Err() != nil:
		record.Status = StatusCanceled
		record.Error = record.cancelReason
		if record.Error == "" {
			record.Error = "canceled"
		}
	default:
		record.Status = StatusFailed
		record.Error = err.Error()
	}
	s.running[record.TaskID]--
	if s.running[record.TaskID] == 0 {
		delete(s.running, record.TaskID)
		for _, id := range s.order {
			next := s.jobs[id]
			if next.TaskID == record.TaskID && next.Status == StatusQueued {
				s.startLocked(next)
				break
			}
		}
	}
	s.lastActivity = time.Now()
	if err := s.persistLocked(record); err != nil {
		record.StorageError = err.Error()
	}
}

// envValue returns the value of name in a "key=value" environment list.
func envValue(env []string, name string) string {
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, name+"="); ok {
			return value
		}
	}
	return ""
}

// procStartTime returns the kernel start time of pid from /proc, used to
// detect PID reuse before signaling a recovered job's process group. A
// missing or unreadable entry returns 0.
func procStartTime(pid int) uint64 {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	// The comm field can contain spaces; numbered fields start after the
	// last ')'. parts[0] is state (field 3); starttime is field 22.
	parts := strings.Fields(string(raw[bytes.LastIndexByte(raw, ')')+1:]))
	if len(parts) < 20 {
		return 0
	}
	value, err := strconv.ParseUint(parts[19], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// sameProcess reports whether pid still names the process whose kernel
// start time is start. A zero start time never matches, so an unverified
// PID is left alone rather than risk killing a reused PID.
func sameProcess(pid int, start uint64) bool {
	if pid <= 0 || start == 0 {
		return false
	}
	return procStartTime(pid) == start
}
