package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/xarunoba/taskrunner/internal/task"
)

const DefaultIdleTimeout = 10 * time.Second

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

type server struct {
	workspace string
	idle      time.Duration

	mu           sync.Mutex
	jobs         map[string]*jobRecord
	order        []string
	running      map[string]int
	handlers     int
	sequence     uint64
	lastActivity time.Time
	workers      sync.WaitGroup
}

func Serve(workspace string, idle time.Duration) error {
	root := filepath.Join(workspace, ".taskrunner")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create daemon directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(root, "daemon.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon lock: %w", err)
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil
		}
		return fmt.Errorf("lock daemon: %w", err)
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)

	jobs, order, err := loadJobHistory(workspace)
	if err != nil {
		return err
	}

	socket := filepath.Join(root, daemonSocketName)
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale daemon socket: %w", err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen for daemon clients: %w", err)
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0o600); err != nil {
		return fmt.Errorf("set daemon socket permissions: %w", err)
	}

	s := &server{
		workspace:    workspace,
		idle:         idle,
		jobs:         jobs,
		order:        order,
		running:      make(map[string]int),
		sequence:     uint64(len(order)),
		lastActivity: time.Now(),
	}
	for _, id := range order {
		record := jobs[id]
		if err := s.persistLocked(record); err != nil {
			record.StorageError = err.Error()
		}
	}
	return s.serve(listener)
}

func (s *server) serve(listener *net.UnixListener) error {
	for {
		if err := listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
			return fmt.Errorf("set daemon accept deadline: %w", err)
		}
		conn, err := listener.AcceptUnix()
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				if s.isIdle() {
					s.workers.Wait()
					return nil
				}
				continue
			}
			return fmt.Errorf("accept daemon client: %w", err)
		}
		s.mu.Lock()
		s.handlers++
		s.lastActivity = time.Now()
		s.mu.Unlock()
		s.workers.Add(1)
		go func() {
			defer s.workers.Done()
			defer func() {
				s.mu.Lock()
				s.handlers--
				s.lastActivity = time.Now()
				s.mu.Unlock()
			}()
			s.handle(conn)
		}()
	}
}

func (s *server) isIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handlers > 0 || time.Since(s.lastActivity) < s.idle {
		return false
	}
	for _, record := range s.jobs {
		if record.Status == StatusQueued || record.Status == StatusRunning {
			return false
		}
	}
	return true
}

func (s *server) handle(conn *net.UnixConn) {
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	var message request
	if err := json.NewDecoder(conn).Decode(&message); err != nil {
		_ = json.NewEncoder(conn).Encode(response{Error: fmt.Sprintf("decode daemon request: %v", err)})
		return
	}
	result := s.dispatch(message)
	_ = json.NewEncoder(conn).Encode(result)
}

func (s *server) dispatch(message request) response {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()

	switch message.Action {
	case "start":
		return s.startJobLocked(message.TaskID, message.Name, message.Command, message.JobPolicy)
	case "list":
		jobs := make([]Job, 0, len(s.order))
		for _, id := range s.order {
			jobs = append(jobs, s.snapshotLocked(s.jobs[id], 0, false))
		}
		return response{Jobs: jobs}
	case "get":
		record, ok := s.jobs[message.JobID]
		if !ok {
			return response{Error: fmt.Sprintf("job %q not found", message.JobID)}
		}
		job := s.snapshotLocked(record, message.OutputOffset, true)
		return response{Job: &job}
	case "cancel":
		record, ok := s.jobs[message.JobID]
		if !ok {
			return response{Error: fmt.Sprintf("job %q not found", message.JobID)}
		}
		if err := s.cancelJobLocked(record, "canceled by user"); err != nil {
			return response{Error: err.Error()}
		}
		job := s.snapshotLocked(record, 0, false)
		return response{Job: &job}
	case "rerun":
		record, ok := s.jobs[message.JobID]
		if !ok {
			return response{Error: fmt.Sprintf("job %q not found", message.JobID)}
		}
		return s.startJobLocked(record.TaskID, record.Name, record.Command, record.Policy)
	case "remove":
		record, ok := s.jobs[message.JobID]
		if !ok {
			return response{Error: fmt.Sprintf("job %q not found", message.JobID)}
		}
		if !record.Done() {
			return response{Error: "cannot remove an active job"}
		}
		if err := s.removeLocked(record); err != nil {
			return response{Error: err.Error()}
		}
		job := s.snapshotLocked(record, 0, false)
		return response{Job: &job}
	default:
		return response{Error: fmt.Sprintf("unknown daemon action %q", message.Action)}
	}
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
