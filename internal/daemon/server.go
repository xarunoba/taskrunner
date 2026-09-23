package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const DefaultIdleTimeout = 10 * time.Second

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
		s.workers.Go(func() {
			defer func() {
				s.mu.Lock()
				s.handlers--
				s.lastActivity = time.Now()
				s.mu.Unlock()
			}()
			s.handle(conn)
		})
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
