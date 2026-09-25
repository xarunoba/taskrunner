package daemon

import (
	"time"

	"github.com/xarunoba/taskrunner/internal/task"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type Job struct {
	ID           string         `json:"id"`
	TaskID       string         `json:"task_id"`
	Name         string         `json:"name"`
	Command      string         `json:"command"`
	Policy       task.JobPolicy `json:"job_policy,omitempty"`
	Status       Status         `json:"status"`
	Output       string         `json:"output,omitempty"`
	OutputSize   int            `json:"output_size"`
	Error        string         `json:"error,omitempty"`
	StorageError string         `json:"storage_error,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	StartedAt    time.Time      `json:"started_at"`
	EndedAt      time.Time      `json:"ended_at"`
}

func (j Job) Done() bool {
	return j.Status == StatusSucceeded || j.Status == StatusFailed || j.Status == StatusCanceled
}

// ShortID returns the final eight characters of the job ID for display.
func (j Job) ShortID() string {
	if len(j.ID) <= 8 {
		return j.ID
	}
	return j.ID[len(j.ID)-8:]
}

type request struct {
	Action       string         `json:"action"`
	TaskID       string         `json:"task_id,omitempty"`
	Name         string         `json:"name,omitempty"`
	Command      string         `json:"command,omitempty"`
	JobPolicy    task.JobPolicy `json:"job_policy,omitempty"`
	JobID        string         `json:"job_id,omitempty"`
	OutputOffset int            `json:"output_offset,omitempty"`
	// Env is the invoking process environment for "start" requests, so a
	// persistent daemon executes the job with the caller's PATH, virtualenv,
	// and exported values instead of the daemon's startup environment.
	Env []string `json:"env"`
}

type response struct {
	Job   *Job   `json:"job,omitempty"`
	Jobs  []Job  `json:"jobs,omitempty"`
	Error string `json:"error,omitempty"`
}
