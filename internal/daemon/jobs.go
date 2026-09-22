package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

func (b *synchronizedBuffer) Set(value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b.Reset()
	b.b.WriteString(value)
}

func (b *synchronizedBuffer) Slice(offset int) (string, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data := b.b.Bytes()
	if offset < 0 || offset > len(data) {
		offset = 0
	}
	return string(data[offset:]), len(data)
}

type jobRecord struct {
	Job
	cancel       context.CancelFunc
	cancelReason string
	output       synchronizedBuffer
}

func (s *server) startJobLocked(taskID, name, command string, policy task.JobPolicy) response {
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
	if includeOutput {
		job.Output, job.OutputSize = record.output.Slice(outputOffset)
	} else {
		_, job.OutputSize = record.output.Slice(0)
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
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		defer cancel()
		s.execute(ctx, record)
	}()
}

func (s *server) execute(ctx context.Context, record *jobRecord) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", record.Command)
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
	err := cmd.Run()

	s.mu.Lock()
	defer s.mu.Unlock()
	record.cancel = nil
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
