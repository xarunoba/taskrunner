package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xarunoba/taskrunner/internal/task"
)

func TestDaemonSchedulesPerTaskAndPersistsJobs(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	first, err := client.Start("build.json", "Build", "printf 'first'; sleep 0.2", task.JobSequential)
	if err != nil {
		t.Fatalf("start first job: %v", err)
	}
	second, err := client.Start("build.json", "Build", "printf 'second'", task.JobSequential)
	if err != nil {
		t.Fatalf("start second job: %v", err)
	}
	other, err := client.Start("test.json", "Test", "printf 'other'; sleep 0.2", task.JobSequential)
	if err != nil {
		t.Fatalf("start other job: %v", err)
	}
	parallel, err := client.Start("build.json", "Build", "printf 'parallel'; sleep 0.2", task.JobParallel)
	if err != nil {
		t.Fatalf("start parallel job: %v", err)
	}

	jobs, err := client.Jobs()
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	statuses := make(map[string]Status, len(jobs))
	for _, job := range jobs {
		statuses[job.ID] = job.Status
	}
	if statuses[first.ID] != StatusRunning {
		t.Fatalf("first job status = %q, want running", statuses[first.ID])
	}
	if statuses[second.ID] != StatusQueued {
		t.Fatalf("second same-task job status = %q, want queued", statuses[second.ID])
	}
	if statuses[other.ID] != StatusRunning {
		t.Fatalf("different-task job status = %q, want running", statuses[other.ID])
	}
	if statuses[parallel.ID] != StatusRunning {
		t.Fatalf("parallel same-task job status = %q, want running", statuses[parallel.ID])
	}

	waitForJobs(t, client, 4)
	firstDone, err := client.Job(first.ID, 0)
	if err != nil {
		t.Fatalf("get first job: %v", err)
	}
	secondDone, err := client.Job(second.ID, 0)
	if err != nil {
		t.Fatalf("get second job: %v", err)
	}
	parallelDone, err := client.Job(parallel.ID, 0)
	if err != nil {
		t.Fatalf("get parallel job: %v", err)
	}
	if secondDone.StartedAt.Before(firstDone.EndedAt) {
		t.Fatalf("queued same-task job started at %s before first ended at %s", secondDone.StartedAt, firstDone.EndedAt)
	}
	if !parallelDone.StartedAt.Before(firstDone.EndedAt) {
		t.Fatalf("parallel same-task job started at %s after first ended at %s", parallelDone.StartedAt, firstDone.EndedAt)
	}
	waitForServerExit(t, stopped)
	restarted := startTestServer(t, workspace)
	jobs, err = client.Jobs()
	if err != nil {
		t.Fatalf("list persisted jobs: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("persisted jobs = %d, want 4", len(jobs))
	}
	for _, job := range jobs {
		if !job.Done() {
			t.Fatalf("persisted job %q status = %q, want terminal", job.ID, job.Status)
		}
		current, err := client.Job(job.ID, 0)
		if err != nil {
			t.Fatalf("get persisted job %q: %v", job.ID, err)
		}
		if current.Output == "" {
			t.Fatalf("persisted job %q has no output", job.ID)
		}
	}
	waitForServerExit(t, restarted)
}

func TestCancelPreviousPolicyReplacesActiveAndQueuedJobs(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	first, err := client.Start("build.json", "Build", "sleep 5", task.JobSequential)
	if err != nil {
		t.Fatalf("start first job: %v", err)
	}
	queued, err := client.Start("build.json", "Build", "printf queued", task.JobSequential)
	if err != nil {
		t.Fatalf("start queued job: %v", err)
	}
	replacement, err := client.Start("build.json", "Build", "printf replacement", task.JobCancelPrevious)
	if err != nil {
		t.Fatalf("start replacement job: %v", err)
	}

	waitForJobs(t, client, 3)
	for id, want := range map[string]Status{
		first.ID:       StatusCanceled,
		queued.ID:      StatusCanceled,
		replacement.ID: StatusSucceeded,
	} {
		job, err := client.Job(id, 0)
		if err != nil {
			t.Fatalf("get job %q: %v", id, err)
		}
		if job.Status != want {
			t.Fatalf("job %q status = %q, want %q", id, job.Status, want)
		}
	}
	waitForServerExit(t, stopped)
}

func TestCancelAndRerunCreateNewJob(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	original, err := client.Start("build.json", "Build", "sleep 5", task.JobSequential)
	if err != nil {
		t.Fatalf("start original job: %v", err)
	}
	if _, err := client.Cancel(original.ID); err != nil {
		t.Fatalf("cancel original job: %v", err)
	}
	waitForJobStatus(t, client, original.ID, StatusCanceled)

	rerun, err := client.Rerun(original.ID)
	if err != nil {
		t.Fatalf("rerun job: %v", err)
	}
	if rerun.ID == original.ID {
		t.Fatal("rerun reused the original job id")
	}
	if rerun.Policy != original.Policy {
		t.Fatalf("rerun policy = %q, want %q", rerun.Policy, original.Policy)
	}
	if _, err := client.Cancel(rerun.ID); err != nil {
		t.Fatalf("cancel rerun job: %v", err)
	}
	waitForJobStatus(t, client, rerun.ID, StatusCanceled)
	waitForServerExit(t, stopped)
}

func TestRemoveDeletesCompletedJobHistory(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	job, err := client.Start("build.json", "Build", "sleep 5", task.JobSequential)
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	if _, err := client.Remove(job.ID); err == nil {
		t.Fatal("Remove() accepted an active job")
	}
	if _, err := client.Cancel(job.ID); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	waitForJobStatus(t, client, job.ID, StatusCanceled)
	if _, err := client.Remove(job.ID); err != nil {
		t.Fatalf("remove completed job: %v", err)
	}
	if _, err := client.Job(job.ID, 0); err == nil {
		t.Fatal("removed job remains available")
	}
	waitForServerExit(t, stopped)

	restarted := startTestServer(t, workspace)
	jobs, err := client.Jobs()
	if err != nil {
		t.Fatalf("list jobs after restart: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs after restart = %d, want 0", len(jobs))
	}
	waitForServerExit(t, restarted)
}

func waitForJobStatus(t *testing.T, client *Client, id string, want Status) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		job, err := client.Job(id, 0)
		if err != nil {
			t.Fatalf("get job %q: %v", id, err)
		}
		if job.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %q status = %q, want %q", id, job.Status, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestJobFinishesWhenBackgroundChildHoldsOutputPipes(t *testing.T) {
	workspace := t.TempDir()
	previousDelay := waitDelay
	waitDelay = 200 * time.Millisecond
	t.Cleanup(func() { waitDelay = previousDelay })
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	// The shell exits immediately, but `sleep 30 &` inherits the output
	// pipes. Without a bounded post-exit wait the job stays running until
	// the background child exits.
	job, err := client.Start("spawn.json", "Spawn", "printf 'spawned output'; sleep 30 &", task.JobParallel)
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	waitForJobStatus(t, client, job.ID, StatusSucceeded)

	done, err := client.Job(job.ID, 0)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if done.Error != "" {
		t.Fatalf("job error = %q, want none", done.Error)
	}
	if !strings.Contains(done.Output, "spawned output") {
		t.Fatalf("job output = %q, want spawned output", done.Output)
	}

	waitForServerExit(t, stopped)
}

func startTestServer(t *testing.T, workspace string) <-chan error {
	t.Helper()
	stopped := make(chan error, 1)
	go func() {
		stopped <- Serve(workspace, 100*time.Millisecond)
	}()
	socket := filepath.Join(workspace, ".taskrunner", daemonSocketName)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			return stopped
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon socket was not created")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForJobs(t *testing.T, client *Client, count int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		jobs, err := client.Jobs()
		if err != nil {
			t.Fatalf("list jobs: %v", err)
		}
		done := 0
		for _, job := range jobs {
			if job.Done() {
				done++
			}
		}
		if done == count {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("completed jobs = %d, want %d", done, count)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForServerExit(t *testing.T, stopped <-chan error) {
	t.Helper()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not exit when idle")
	}
}
