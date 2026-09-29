package daemon

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xarunoba/taskrunner/internal/task"
)

func TestOutputFromWithholdsIncompleteUTF8Sequence(t *testing.T) {
	t.Parallel()

	var buffer synchronizedBuffer
	buffer.Write([]byte("a\xE2"))
	chunk, _, size := buffer.from(0, false)
	if chunk != "a" || size != 1 {
		t.Fatalf("partial rune: chunk = %q size = %d, want %q 1", chunk, size, "a")
	}
	buffer.Write([]byte("\x82\xAC\n"))
	chunk, _, size = buffer.from(size, false)
	if chunk != "€\n" || size != 5 {
		t.Fatalf("completed rune: chunk = %q size = %d, want %q 5", chunk, size, "€\\n")
	}
}

func TestOutputBoundaryPassesPermanentlyInvalidSequences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantText string
		wantSize int
	}{
		// Surrogate lead: ED A0 can never become a valid rune.
		{"surrogate head", "a\xED\xA0", "a\xED\xA0", 3},
		// F0 requires its first continuation byte in 0x90..0xBF.
		{"overlong 4-byte head", "a\xF0\x80", "a\xF0\x80", 3},
		// F4 requires its first continuation byte in 0x80..0x8F.
		{"out of range lead F4", "a\xF4\x90", "a\xF4\x90", 3},
		// A still-possible sequence stays withheld.
		{"valid prefix withheld", "a\xF0\x9F", "a", 1},
		// A complete 4-byte rune passes.
		{"complete astral rune", "a\xf0\x9f\x98\x8a", "a\xf0\x9f\x98\x8a", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buffer synchronizedBuffer
			buffer.Write([]byte(tt.input))
			chunk, _, size := buffer.from(0, false)
			if chunk != tt.wantText || size != tt.wantSize {
				t.Fatalf("from(0) = %q %d, want %q %d", chunk, size, tt.wantText, tt.wantSize)
			}
		})
	}
}

func TestFinalOutputFlushesTruncatedTail(t *testing.T) {
	t.Parallel()

	var buffer synchronizedBuffer
	buffer.Write([]byte("a\xE2\x82"))
	if chunk, _, size := buffer.from(0, false); chunk != "a" || size != 1 {
		t.Fatalf("running: chunk = %q size = %d, want %q 1", chunk, size, "a")
	}
	chunk, _, size := buffer.from(1, true)
	if chunk != "\xE2\x82" || size != 3 {
		t.Fatalf("flushed: chunk = %q size = %d, want %q 3", chunk, size, "\xe2\x82")
	}
}

func TestJobsRunWithEachInvocationEnvironment(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	t.Setenv("TASKRUNNER_ENV_TEST", "first")
	first, err := client.Start("env.json", "Env", `printf %s "$TASKRUNNER_ENV_TEST"`, task.JobSequential)
	if err != nil {
		t.Fatalf("start first job: %v", err)
	}
	t.Setenv("TASKRUNNER_ENV_TEST", "second")
	second, err := client.Start("env.json", "Env", `printf %s "$TASKRUNNER_ENV_TEST"`, task.JobParallel)
	if err != nil {
		t.Fatalf("start second job: %v", err)
	}
	waitForJobs(t, client, 2)

	firstDone, err := client.Job(first.ID, 0)
	if err != nil {
		t.Fatalf("get first job: %v", err)
	}
	if firstDone.Output != "first" {
		t.Fatalf("first job output = %q, want first", firstDone.Output)
	}
	secondDone, err := client.Job(second.ID, 0)
	if err != nil {
		t.Fatalf("get second job: %v", err)
	}
	if secondDone.Output != "second" {
		t.Fatalf("second job output = %q, want second", secondDone.Output)
	}
	waitForServerExit(t, stopped)
}

func TestRecoveryKillsOrphanedProcessGroup(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".taskrunner", jobHistoryDirectory), 0o700); err != nil {
		t.Fatal(err)
	}

	orphan := exec.Command("sleep", "30")
	orphan.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := orphan.Start(); err != nil {
		t.Fatalf("start orphan: %v", err)
	}
	defer func() { _ = orphan.Process.Kill() }()
	waitOrphan := make(chan error, 1)
	go func() { waitOrphan <- orphan.Wait() }()

	id := "recovery-orphan"
	stored := persistedJob{
		ID:        id,
		TaskID:    "orphan.json",
		Name:      "Orphan",
		Command:   "sleep 30",
		Status:    StatusRunning,
		CreatedAt: time.Now(),
		StartedAt: time.Now(),
		Pid:       orphan.Process.Pid,
		PidStart:  procStartTime(orphan.Process.Pid),
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".taskrunner", jobHistoryDirectory, id+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	jobs, order, _, err := loadJobHistory(workspace)
	if err != nil {
		t.Fatalf("loadJobHistory() error = %v", err)
	}
	if len(order) != 1 || jobs[id].Status != StatusFailed {
		t.Fatalf("recovered job = %+v, want failed", jobs[id])
	}
	select {
	case <-waitOrphan:
	case <-time.After(3 * time.Second):
		t.Fatal("orphaned process group survived recovery")
	}
}

func TestRecoveryLeavesUnrelatedPidAlone(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".taskrunner", jobHistoryDirectory), 0o700); err != nil {
		t.Fatal(err)
	}

	outside := exec.Command("sleep", "30")
	if err := outside.Start(); err != nil {
		t.Fatalf("start outside process: %v", err)
	}
	defer func() { _ = outside.Process.Kill() }()

	// A stale record whose PID points at a process with a different start
	// time (PID reuse) must not be killed.
	id := "recovery-stale"
	stored := persistedJob{
		ID:        id,
		TaskID:    "stale.json",
		Name:      "Stale",
		Command:   "sleep 30",
		Status:    StatusRunning,
		CreatedAt: time.Now(),
		StartedAt: time.Now(),
		Pid:       outside.Process.Pid,
		PidStart:  procStartTime(outside.Process.Pid) + 1,
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".taskrunner", jobHistoryDirectory, id+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := loadJobHistory(workspace); err != nil {
		t.Fatalf("loadJobHistory() error = %v", err)
	}
	if err := outside.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("unrelated process was killed during recovery: %v", err)
	}
	_ = outside.Process.Kill()
	_ = outside.Wait()
}

func TestRerunReplaysRecordedEnvironment(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	t.Setenv("TASKRUNNER_ENV_TEST", "original")
	original, err := client.Start("env.json", "Env", `printf %s "$TASKRUNNER_ENV_TEST"`, task.JobSequential)
	if err != nil {
		t.Fatalf("start original job: %v", err)
	}
	waitForJobs(t, client, 1)

	t.Setenv("TASKRUNNER_ENV_TEST", "changed")
	rerun, err := client.Rerun(original.ID)
	if err != nil {
		t.Fatalf("rerun job: %v", err)
	}
	waitForJobs(t, client, 2)
	rerunDone, err := client.Job(rerun.ID, 0)
	if err != nil {
		t.Fatalf("get rerun job: %v", err)
	}
	if !strings.Contains(rerunDone.Output, "original") {
		t.Fatalf("rerun output = %q, want original environment", rerunDone.Output)
	}
	waitForServerExit(t, stopped)
}

func TestCompletedJobDeliversTruncatedFinalRune(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	// The job exits after writing a lead byte with no continuation, so no
	// complete rune can follow. The completed job must still deliver those
	// bytes instead of silently withholding them. JSON transport replaces
	// the dangling lead byte with U+FFFD; OutputSize still reports the raw
	// byte count so resume offsets stay consistent.
	job, err := client.Start("tail.json", "Tail", `printf 'a\302'`, task.JobSequential)
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	waitForJobs(t, client, 1)

	done, err := client.Job(job.ID, 0)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if done.OutputSize != 2 || done.Output != "a\uFFFD" {
		t.Fatalf("output = %q size = %d, want %q 2", done.Output, done.OutputSize, "a\uFFFD")
	}
	waitForServerExit(t, stopped)
}

func TestEmptyEnvironmentSurvivesRerunAfterRestart(t *testing.T) {
	t.Setenv("TASKRUNNER_ENV_TEST", "daemon-only")
	t.Setenv("SHELL", filepath.Join(t.TempDir(), "missing-shell"))
	workspace := t.TempDir()
	client := NewClient(workspace)
	stopped := startTestServer(t, workspace)
	result, err := client.roundTrip(request{
		Action: "start", TaskID: "empty.json", Name: "Empty environment",
		Command: `printf %s "${TASKRUNNER_ENV_TEST-absent}"`,
		Env:     []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForJobs(t, client, 1)
	first, err := client.Job(result.Job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	waitForServerExit(t, stopped)
	if first.Status != StatusSucceeded || first.Output != "absent" {
		t.Fatalf("empty environment job: status=%s output=%q error=%q", first.Status, first.Output, first.Error)
	}

	stopped = startTestServer(t, workspace)
	rerun, err := client.Rerun(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitForJobs(t, client, 2)
	replayed, err := client.Job(rerun.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	waitForServerExit(t, stopped)
	if replayed.Status != StatusSucceeded || replayed.Output != "absent" {
		t.Fatalf("replayed environment: status=%s output=%q error=%q", replayed.Status, replayed.Output, replayed.Error)
	}
}

func TestDaemonRejectsSymlinkBeforeCreatingHistory(t *testing.T) {
	t.Parallel()
	workspace, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, ".taskrunner")); err != nil {
		t.Fatal(err)
	}
	if err := Serve(workspace, time.Millisecond); err == nil {
		t.Fatal("Serve accepted a symlinked storage directory")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("daemon wrote %d entries outside the workspace", len(entries))
	}
}
