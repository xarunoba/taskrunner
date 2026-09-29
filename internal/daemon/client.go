package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xarunoba/taskrunner/internal/task"
)

const (
	daemonSocketName = "daemon.sock"
	connectTimeout   = 3 * time.Second
	retryInterval    = 25 * time.Millisecond
)

type Client struct {
	workspace string
	socket    string
	mu        sync.Mutex
	// warnings holds history diagnostics from the most recent response.
	warnings []string
}

// Warnings returns diagnostics from the most recent daemon response, such as
// job history records skipped at startup because they are corrupt or exceed
// the retained-output bound. Each warning names the offending path and the
// reason; records are never modified or deleted. The result is empty before
// the first call and whenever the last response carried no warnings.
func (c *Client) Warnings() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.warnings) == 0 {
		return nil
	}
	out := make([]string, len(c.warnings))
	copy(out, c.warnings)
	return out
}

func (c *Client) setWarnings(warnings []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warnings = warnings
}

func NewClient(workspace string) *Client {
	return &Client{
		workspace: workspace,
		socket:    filepath.Join(workspace, ".taskrunner", daemonSocketName),
	}
}

// Start submits a job for execution. The client process environment travels
// with the request so the job runs with the caller's environment.
func (c *Client) Start(taskID, name, command string, policy task.JobPolicy) (Job, error) {
	result, err := c.roundTrip(request{
		Action:    "start",
		TaskID:    taskID,
		Name:      name,
		Command:   command,
		JobPolicy: policy,
		Env:       os.Environ(),
	})
	if err != nil {
		return Job{}, err
	}
	if result.Job == nil {
		return Job{}, errors.New("daemon returned no job")
	}
	return *result.Job, nil
}

func (c *Client) Jobs() ([]Job, error) {
	result, err := c.roundTrip(request{Action: "list"})
	if err != nil {
		return nil, err
	}
	return result.Jobs, nil
}

func (c *Client) Job(id string, outputOffset int) (Job, error) {
	result, err := c.roundTrip(request{Action: "get", JobID: id, OutputOffset: outputOffset})
	if err != nil {
		return Job{}, err
	}
	if result.Job == nil {
		return Job{}, errors.New("daemon returned no job")
	}
	return *result.Job, nil
}

func (c *Client) Cancel(id string) (Job, error) {
	return c.jobAction("cancel", id)
}

func (c *Client) Rerun(id string) (Job, error) {
	return c.jobAction("rerun", id)
}

func (c *Client) Remove(id string) (Job, error) {
	return c.jobAction("remove", id)
}

func (c *Client) jobAction(action, id string) (Job, error) {
	result, err := c.roundTrip(request{Action: action, JobID: id})
	if err != nil {
		return Job{}, err
	}
	if result.Job == nil {
		return Job{}, errors.New("daemon returned no job")
	}
	return *result.Job, nil
}

func (c *Client) roundTrip(message request) (response, error) {
	if err := task.RejectSymlinkedDirs(filepath.Dir(c.socket)); err != nil {
		return response{}, fmt.Errorf("prepare workspace: %w", err)
	}
	conn, err := net.DialTimeout("unix", c.socket, retryInterval)
	if err != nil {
		output, startErr := c.startDaemon()
		if startErr != nil {
			return response{}, startErr
		}
		defer output.Close()
		deadline := time.Now().Add(connectTimeout)
		for {
			conn, err = net.DialTimeout("unix", c.socket, retryInterval)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				err = fmt.Errorf("connect to daemon: %w", err)
				// The freshly started daemon likely exited with a startup
				// error; surface its diagnostics instead of the generic
				// socket failure.
				if output := readDaemonStartupError(output); output != "" {
					err = fmt.Errorf("%w\n%s", err, output)
				}
				return response{}, err
			}
			time.Sleep(retryInterval)
		}
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(connectTimeout)); err != nil {
		return response{}, fmt.Errorf("set daemon deadline: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(message); err != nil {
		return response{}, fmt.Errorf("send daemon request: %w", err)
	}
	var result response
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		return response{}, fmt.Errorf("read daemon response: %w", err)
	}
	c.setWarnings(result.Warnings)
	if result.Error != "" {
		return response{}, errors.New(result.Error)
	}
	return result, nil
}

func (c *Client) startDaemon() (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(c.socket), 0o700); err != nil {
		return nil, fmt.Errorf("create daemon directory: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate taskrunner executable: %w", err)
	}
	output, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("capture daemon startup error: %w", err)
	}
	defer writer.Close()
	cmd := exec.Command(executable, "__daemon", c.workspace)
	cmd.Dir = c.workspace
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		output.Close()
		return nil, fmt.Errorf("start daemon: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		output.Close()
		return nil, fmt.Errorf("release daemon process: %w", err)
	}
	return output, nil
}

func readDaemonStartupError(output *os.File) string {
	if err := output.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
		return ""
	}
	// A slow exit may hit the deadline after writing its diagnostic. Keep
	// those bytes, but never wait indefinitely or read unbounded output.
	data, _ := io.ReadAll(io.LimitReader(output, 8192))
	return strings.TrimSpace(string(data))
}
