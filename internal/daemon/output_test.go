package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xarunoba/taskrunner/internal/task"
)

func TestRingEvictsBeyondCapacity(t *testing.T) {
	t.Parallel()

	var buffer synchronizedBuffer
	chunk := 64 << 10
	writes := (32 << 20) / chunk
	payload := strings.Repeat("a", chunk)
	for range writes {
		if n, err := buffer.Write([]byte(payload)); err != nil || n != chunk {
			t.Fatalf("Write() = %d, %v", n, err)
		}
	}
	total := writes * chunk
	start, end := buffer.boundaries(true)
	if end != total {
		t.Fatalf("boundaries() end = %d, want %d", end, total)
	}
	if start != total-maxRetainedOutput {
		t.Fatalf("boundaries() start = %d, want %d", start, total-maxRetainedOutput)
	}
	text, gotStart, gotEnd := buffer.from(0, true)
	if gotStart != start || gotEnd != end {
		t.Fatalf("from(0) = window [%d,%d), want [%d,%d)", gotStart, gotEnd, start, end)
	}
	if len(text) != maxRetainedOutput {
		t.Fatalf("retained window = %d bytes, want %d", len(text), maxRetainedOutput)
	}
	if text != strings.Repeat("a", maxRetainedOutput) {
		t.Fatal("retained window content mismatch")
	}
}

func TestUTF8AroundEvictedBoundary(t *testing.T) {
	t.Parallel()
	for evicted := 1; evicted <= 3; evicted++ {
		var buffer synchronizedBuffer
		initial := "\U0001d11e" + strings.Repeat("a", maxRetainedOutput-4)
		if _, err := buffer.Write([]byte(initial)); err != nil {
			t.Fatal(err)
		}
		suffix := strings.Repeat("b", evicted)
		if _, err := buffer.Write([]byte(suffix)); err != nil {
			t.Fatal(err)
		}
		text, start, end := buffer.from(0, true)
		if start != 4 || end != maxRetainedOutput+evicted || text != initial[4:]+suffix {
			t.Fatalf("evict %d rune bytes: window [%d,%d), %d bytes", evicted, start, end, len(text))
		}
	}
}

// TestFromOffsetsAcrossEviction verifies stale offsets deliver the whole
// window and current offsets deliver the correct suffix.
func TestFromOffsetsAcrossEviction(t *testing.T) {
	t.Parallel()

	var buffer synchronizedBuffer
	if _, err := buffer.Write([]byte(strings.Repeat("a", maxRetainedOutput+10))); err != nil {
		t.Fatal(err)
	}
	start, end := buffer.boundaries(true)
	if start != 10 || end != maxRetainedOutput+10 {
		t.Fatalf("boundaries() = [%d,%d), want [10,%d)", start, end, maxRetainedOutput+10)
	}
	if text, _, _ := buffer.from(0, true); len(text) != maxRetainedOutput {
		t.Fatalf("stale offset 0 delivered %d bytes, want %d", len(text), maxRetainedOutput)
	}
	if text, gotStart, _ := buffer.from(10, true); gotStart != start || len(text) != maxRetainedOutput {
		t.Fatalf("offset at window start delivered %d bytes from %d", len(text), gotStart)
	}
	suffix, _, _ := buffer.from(start+5, true)
	if len(suffix) != maxRetainedOutput-5 {
		t.Fatalf("suffix = %d bytes, want %d", len(suffix), maxRetainedOutput-5)
	}
	if text, _, _ := buffer.from(end, true); text != "" {
		t.Fatalf("offset at end delivered %q, want empty", text)
	}
}

// TestRestoreTruncatesLegacyWindow verifies a restored window larger than
// the retention cap keeps its tail with correct absolute offsets.
func TestRestoreTruncatesLegacyWindow(t *testing.T) {
	t.Parallel()

	var buffer synchronizedBuffer
	legacy := strings.Repeat("b", 2<<20)
	buffer.restore(legacy, 0)
	start, end := buffer.boundaries(true)
	if end != 2<<20 {
		t.Fatalf("boundaries() end = %d, want %d", end, 2<<20)
	}
	if start != (2<<20)-maxRetainedOutput {
		t.Fatalf("boundaries() start = %d, want %d", start, (2<<20)-maxRetainedOutput)
	}
	text, _, _ := buffer.from(0, true)
	if len(text) != maxRetainedOutput || text != legacy[(2<<20)-maxRetainedOutput:] {
		t.Fatal("restored window is not the bounded tail of the legacy output")
	}
}

func writeJobRecord(t *testing.T, dir, id string, stored persistedJob) string {
	t.Helper()
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLegacyRecordWithoutNewFieldsLoads verifies a v0.1.0 record lacking
// output_start, pid, and pid_start still loads with its output tail.
func TestLegacyRecordWithoutNewFieldsLoads(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".taskrunner", jobHistoryDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "legacy-job"
	writeJobRecord(t, dir, id, persistedJob{
		ID:         id,
		TaskID:     "legacy.json",
		Name:       "Legacy",
		Command:    "printf legacy",
		Status:     StatusSucceeded,
		Output:     strings.Repeat("x", 2<<20),
		OutputSize: 2 << 20,
		CreatedAt:  time.Now(),
		StartedAt:  time.Now(),
		EndedAt:    time.Now(),
	})
	jobs, order, warnings, err := loadJobHistory(workspace)
	if err != nil {
		t.Fatalf("loadJobHistory() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if len(order) != 1 {
		t.Fatalf("order = %v, want [%s]", order, id)
	}
	record := jobs[id]
	if record == nil || record.Status != StatusSucceeded {
		t.Fatalf("legacy job = %+v, want loaded and succeeded", record)
	}
	text, start, end := record.output.from(0, true)
	if end != 2<<20 {
		t.Fatalf("output end = %d, want %d", end, 2<<20)
	}
	if len(text) != maxRetainedOutput || start != end-len(text) {
		t.Fatalf("restored window = %d bytes at %d, want %d at %d", len(text), start, maxRetainedOutput, end-maxRetainedOutput)
	}
}

// TestCorruptSiblingRecordIsSkipped verifies one corrupt record neither
// stops valid records from loading nor leaks record content into warnings.
func TestCorruptSiblingRecordIsSkipped(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".taskrunner", jobHistoryDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	goodID := "good-job"
	goodPath := writeJobRecord(t, dir, goodID, persistedJob{
		ID:        goodID,
		TaskID:    "good.json",
		Name:      "Good",
		Command:   "printf good",
		Status:    StatusSucceeded,
		CreatedAt: time.Now(),
		StartedAt: time.Now(),
		EndedAt:   time.Now(),
	})
	corruptPath := filepath.Join(dir, "broken-job.json")
	// The corrupt record pretends to hold a secret; the warning must carry
	// the path and reason only, never file content.
	if err := os.WriteFile(corruptPath, []byte(`{"id":"broken-job", "TASKRUNNER_SECRET_TOKEN=hunter2`), 0o600); err != nil {
		t.Fatal(err)
	}
	mismatchedPath := filepath.Join(dir, "other-name.json")
	if err := os.WriteFile(mismatchedPath, []byte(`{"id":"different-id"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	jobs, order, warnings, err := loadJobHistory(workspace)
	if err != nil {
		t.Fatalf("loadJobHistory() error = %v", err)
	}
	if len(order) != 1 || jobs[goodID] == nil {
		t.Fatalf("valid record not loaded: order = %v", order)
	}
	joined := strings.Join(warnings, "\n")
	for _, path := range []string{corruptPath, mismatchedPath} {
		if !strings.Contains(joined, path) {
			t.Fatalf("warning missing offending path %s:\n%s", path, joined)
		}
	}
	if !strings.Contains(joined, "corrupt record") || !strings.Contains(joined, "invalid job id") {
		t.Fatalf("warnings missing reasons:\n%s", joined)
	}
	if strings.Contains(joined, "hunter2") {
		t.Fatalf("warning leaked record content:\n%s", joined)
	}
	_ = goodPath
}

// TestOversizedRecordPreservedAndSkipped verifies a record beyond the read
// bound is skipped with an actionable warning and left byte-for-byte intact.
func TestOversizedRecordPreservedAndSkipped(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".taskrunner", jobHistoryDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	goodID := "good-job"
	writeJobRecord(t, dir, goodID, persistedJob{
		ID:        goodID,
		TaskID:    "good.json",
		Name:      "Good",
		Command:   "printf good",
		Status:    StatusSucceeded,
		CreatedAt: time.Now(),
		StartedAt: time.Now(),
		EndedAt:   time.Now(),
	})
	hugePath := filepath.Join(dir, "huge-job.json")
	huge := fmt.Sprintf(`{"TASKRUNNER_SECRET_TOKEN=hunter2","pad":"%s"}`, strings.Repeat("p", maxPersistedRecordBytes))
	if err := os.WriteFile(hugePath, []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(hugePath)
	if err != nil {
		t.Fatal(err)
	}

	jobs, _, warnings, err := loadJobHistory(workspace)
	if err != nil {
		t.Fatalf("loadJobHistory() error = %v", err)
	}
	if len(jobs) != 1 || jobs[goodID] == nil {
		t.Fatal("valid record not loaded alongside oversized record")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, hugePath) || !strings.Contains(joined, "exceeds") || !strings.Contains(joined, "left in place") {
		t.Fatalf("oversized record warning missing path or guidance:\n%s", joined)
	}
	if strings.Contains(joined, "hunter2") {
		t.Fatalf("warning leaked oversized record content:\n%s", joined)
	}
	after, err := os.Stat(hugePath)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("oversized record was modified: %d bytes, want %d", after.Size(), before.Size())
	}
}

// TestWarningsSurfaceToClient verifies startup history warnings reach the
// client accessor while normal jobs still work.
func TestWarningsSurfaceToClient(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".taskrunner", jobHistoryDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(dir, "broken-job.json")
	if err := os.WriteFile(corruptPath, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}

	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)
	if _, err := client.Start("build.json", "Build", "printf ok", task.JobSequential); err != nil {
		t.Fatalf("start job alongside corrupt record: %v", err)
	}
	waitForJobs(t, client, 1)
	warnings := client.Warnings()
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), corruptPath) {
		t.Fatalf("Warnings() = %v, want the corrupt record path", warnings)
	}
	waitForServerExit(t, stopped)
}

// TestLargeOutputRetainsBoundedTail is the end-to-end acceptance check: a
// 32 MiB job retains at most 1 MiB with correct absolute size and start,
// follow consumers can detect the omission, and the bounded tail survives a
// daemon restart.
func TestLargeOutputRetainsBoundedTail(t *testing.T) {
	workspace := t.TempDir()
	stopped := startTestServer(t, workspace)
	client := NewClient(workspace)

	const total = 32 << 20
	job, err := client.Start("big.json", "Big", fmt.Sprintf("head -c %d /dev/zero", total), task.JobSequential)
	if err != nil {
		t.Fatalf("start large job: %v", err)
	}
	waitForJobs(t, client, 1)

	done, err := client.Job(job.ID, 0)
	if err != nil {
		t.Fatalf("get large job: %v", err)
	}
	if done.OutputSize != total {
		t.Fatalf("OutputSize = %d, want %d", done.OutputSize, total)
	}
	if len(done.Output) > maxRetainedOutput {
		t.Fatalf("retained output = %d bytes, exceeds %d", len(done.Output), maxRetainedOutput)
	}
	if done.OutputStart != total-len(done.Output) {
		t.Fatalf("OutputStart = %d, want %d", done.OutputStart, total-len(done.Output))
	}
	if done.OutputStart == 0 {
		t.Fatal("follow consumer requesting offset 0 cannot detect the omission")
	}
	waitForServerExit(t, stopped)

	stopped = startTestServer(t, workspace)
	restored, err := client.Job(job.ID, 0)
	if err != nil {
		t.Fatalf("get large job after restart: %v", err)
	}
	waitForServerExit(t, stopped)
	if restored.OutputSize != done.OutputSize || restored.OutputStart != done.OutputStart || restored.Output != done.Output {
		t.Fatalf("restored job diverged: size %d start %d len %d, want size %d start %d len %d",
			restored.OutputSize, restored.OutputStart, len(restored.Output),
			done.OutputSize, done.OutputStart, len(done.Output))
	}
	if restored.Status != StatusSucceeded {
		t.Fatalf("restored status = %s, want %s", restored.Status, StatusSucceeded)
	}
}

func TestOutputSnapshotStaysConsistentDuringEviction(t *testing.T) {
	var buffer synchronizedBuffer
	var writer sync.WaitGroup
	ready := make(chan struct{})
	writer.Go(func() {
		chunk := make([]byte, 4096)
		for offset := 0; offset < 4*maxRetainedOutput; offset += len(chunk) {
			for i := range chunk {
				chunk[i] = byte('a' + (offset+i)%26)
			}
			buffer.Write(chunk)
			if offset+len(chunk) == maxRetainedOutput {
				close(ready)
			}
		}
	})
	defer writer.Wait()
	<-ready
	for range 50 {
		text, start, end := buffer.from(0, true)
		if len(text) != end-start || len(text) > maxRetainedOutput {
			t.Fatalf("inconsistent window [%d,%d) with %d bytes", start, end, len(text))
		}
		for i := range text {
			if text[i] != byte('a'+(start+i)%26) {
				t.Fatalf("output changed within snapshot at absolute byte %d", start+i)
			}
		}
	}
}

func TestBinaryOutputOffsetsSurviveRestart(t *testing.T) {
	workspace := t.TempDir()
	record := &jobRecord{ID: "binary", Status: StatusSucceeded}
	raw := string([]byte{0x80, 0xff, 'x', 0xe2, 0x82})
	record.output.restore(raw, 0)
	s := &server{workspace: workspace}
	if err := s.persistLocked(record); err != nil {
		t.Fatal(err)
	}
	jobs, _, warnings, err := loadJobHistory(workspace)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("reload failed: %v, %v", err, warnings)
	}
	text, start, end := jobs["binary"].output.from(0, true)
	if text != raw || start != 0 || end != len(raw) {
		t.Fatalf("binary output changed: %x [%d,%d)", text, start, end)
	}
}

func TestMalformedTimestampDoesNotLeakRecordContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte(`{"id":"broken","created_at":"private-test-value"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, warning := readPersistedJob(path)
	if !strings.Contains(warning, path) || strings.Contains(warning, "private-test-value") {
		t.Fatalf("unsafe or missing diagnostic: %q", warning)
	}
}
