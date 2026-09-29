package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xarunoba/taskrunner/internal/daemon"
)

func TestJobCLIReportsSkippedHistoryAndTruncatedOutput(t *testing.T) {
	workspace := t.TempDir()
	runs := filepath.Join(workspace, ".taskrunner", "runs")
	if err := os.MkdirAll(runs, 0o700); err != nil {
		t.Fatal(err)
	}
	output := strings.Repeat("x", 2<<20) + "final line\n"
	stored, err := json.Marshal(daemon.Job{
		ID: "legacy", Name: "Legacy", Status: daemon.StatusSucceeded,
		Output: output, OutputSize: len(output),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runs, "legacy.json"), stored, 0o600); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(runs, "broken.json")
	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	serverErr := startCLITestDaemon(t, workspace)
	t.Cleanup(func() {
		select {
		case err := <-serverErr:
			if err != nil {
				t.Errorf("daemon stopped: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("daemon did not become idle")
		}
	})

	var stdout, stderr bytes.Buffer
	if err := executeJobs(workspace, jobListOptions{all: true, json: true}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var jobs []daemon.Job
	if err := json.Unmarshal(stdout.Bytes(), &jobs); err != nil {
		t.Fatalf("warnings contaminated JSON output: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != "legacy" || jobs[0].OutputSize != len(output) {
		t.Fatalf("legacy job metadata lost: %+v", jobs)
	}
	if !strings.Contains(stderr.String(), "broken.json") {
		t.Fatalf("missing corrupt-record diagnostic: %q", stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if err := executeJobLogsByReference(workspace, "legacy", jobLogsOptions{tail: -1}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	const retainedBytes = 1 << 20
	if stdout.String() != output[len(output)-retainedBytes:] {
		t.Fatalf("logs did not return the retained tail: got %d bytes", stdout.Len())
	}
	if !strings.Contains(stderr.String(), "truncated") || !strings.Contains(stderr.String(), "broken.json") {
		t.Fatalf("missing truncation or history diagnostic: %q", stderr.String())
	}
	if data, err := os.ReadFile(broken); err != nil || string(data) != "{" {
		t.Fatalf("corrupt record was modified: %q, %v", data, err)
	}
}
