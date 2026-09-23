package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
)

func TestTabClicksHitTabsNotContentOrPath(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	tasks := []task.Task{
		{Name: "Jobs pipeline", Command: "true"},
		{Name: "Build", Command: "true"},
	}
	m := newModel(task.NewStore(workspace), tasks)
	m.resize(80, 24)

	// Header columns: "╭─ " then padded "Tasks" (cols 4-8), "Jobs" (cols 12-15).
	cases := []struct {
		x    int
		want screen
		hit  bool
	}{
		{x: 4, want: screenList, hit: true},
		{x: 8, want: screenList, hit: true},
		{x: 9, want: 0, hit: false}, // gap between tabs
		{x: 12, want: screenJobs, hit: true},
		{x: 15, want: screenJobs, hit: true},
		{x: 16, want: 0, hit: false}, // gap before path
		{x: 30, want: 0, hit: false}, // workspace path

	}
	for _, tc := range cases {
		got, ok := m.topLevelTabAt(0, tc.x)
		if ok != tc.hit || (ok && got != tc.want) {
			t.Fatalf("topLevelTabAt(0, %d) = %d %v, want screen %d hit %v", tc.x, got, ok, tc.want, tc.hit)
		}
	}
	if _, ok := m.topLevelTabAt(3, 4); ok {
		t.Fatal("tab hit test matched outside the header row")
	}

	// Clicking the Jobs tab header switches tabs.
	updated, _ := m.updateMouse(mouseClickAt(12, 0))
	if got := updated.(model).screen; got != screenJobs {
		t.Fatalf("clicking the Jobs tab produced screen %d, want %d", got, screenJobs)
	}

	// Clicking a task row whose name contains "Jobs" must not switch tabs.
	updated, _ = m.updateMouse(mouseClickAt(12, 0)) // back to Jobs
	m = updated.(model)
	updated, _ = m.updateMouse(mouseClickAt(4, 0)) // Tasks tab
	m = updated.(model)
	view := strings.Split(ansi.Strip(m.View()), "\n")
	row := -1
	for y, line := range view {
		if y > 0 && strings.Contains(line, "Jobs pipeline") {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("task row not visible:\n%s", m.View())
	}
	updated, _ = m.updateMouse(mouseClickAt(16, row))
	m = updated.(model)
	if m.screen != screenList {
		t.Fatalf("clicking task %q switched to screen %d", "Jobs pipeline", m.screen)
	}
	if m.cursor != 0 {
		t.Fatalf("clicking the task row set cursor to %d, want 0", m.cursor)
	}
}
