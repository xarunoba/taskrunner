package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
}

func NewClient(workspace string) *Client {
	return &Client{
		workspace: workspace,
		socket:    filepath.Join(workspace, ".taskrunner", daemonSocketName),
	}
}

func (c *Client) Start(taskID, name, command string, policy task.JobPolicy) (Job, error) {
	result, err := c.do(request{
		Action:    "start",
		TaskID:    taskID,
		Name:      name,
		Command:   command,
		JobPolicy: policy,
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
	result, err := c.do(request{Action: "list"})
	if err != nil {
		return nil, err
	}
	return result.Jobs, nil
}

func (c *Client) Job(id string, outputOffset int) (Job, error) {
	result, err := c.do(request{Action: "get", JobID: id, OutputOffset: outputOffset})
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
	result, err := c.do(request{Action: action, JobID: id})
	if err != nil {
		return Job{}, err
	}
	if result.Job == nil {
		return Job{}, errors.New("daemon returned no job")
	}
	return *result.Job, nil
}

func (c *Client) do(message request) (response, error) {
	conn, err := net.DialTimeout("unix", c.socket, retryInterval)
	if err != nil {
		if err := c.startDaemon(); err != nil {
			return response{}, err
		}
		deadline := time.Now().Add(connectTimeout)
		for {
			conn, err = net.DialTimeout("unix", c.socket, retryInterval)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return response{}, fmt.Errorf("connect to daemon: %w", err)
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
	if result.Error != "" {
		return response{}, errors.New(result.Error)
	}
	return result, nil
}

func (c *Client) startDaemon() error {
	if err := os.MkdirAll(filepath.Dir(c.socket), 0o700); err != nil {
		return fmt.Errorf("create daemon directory: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate taskrunner executable: %w", err)
	}
	cmd := exec.Command(executable, "__daemon", c.workspace)
	cmd.Dir = c.workspace
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release daemon process: %w", err)
	}
	return nil
}
