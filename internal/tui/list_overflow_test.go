package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

func TestListOverflowPositionAndMouseBounds(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{24, 10}, {80, 24}, {140, 50}} {
		for _, jobs := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/jobs=%t", size[0], size[1], jobs), func(t *testing.T) {
				m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
				m.resize(size[0], size[1])
				for i := range 70 {
					m.tasks = append(m.tasks, task.Task{Name: fmt.Sprintf("Task %02d %s", i, strings.Repeat("long", 40)), Command: "true\nprintf ok"})
					m.jobs = append(m.jobs, daemon.Job{ID: fmt.Sprintf("job-%08d", i), Name: strings.Repeat("long", 40), Status: daemon.StatusSucceeded})
				}
				if jobs {
					m.screen = screenJobs
				}
				for _, cursor := range []int{0, 35, 69} {
					m.cursor, m.jobCursor = cursor, cursor
					view := m.View()
					assertFillsTerminal(t, view, size[0], size[1])
					lines := strings.Split(ansi.Strip(view), "\n")
					thumbRow := -1
					lastTrackRow := -1
					for y, line := range lines {
						cell := ansi.Cut(line, size[0]-4, size[0]-3)
						if cell == "│" || cell == "█" {
							lastTrackRow = y
						}
						if strings.Contains(line, "█") {
							thumbRow = y
							if got := ansi.Cut(line, size[0]-4, size[0]-3); got != "█" {
								t.Fatalf("thumb is not at content right edge: %q", line)
							}
						}
					}
					if !strings.Contains(ansi.Strip(view), "›") {
						t.Fatalf("selected row hidden:\n%s", view)
					}
					if cursor == 0 && thumbRow != 2 || cursor == 69 && thumbRow != lastTrackRow || cursor == 35 && (thumbRow <= 2 || thumbRow >= lastTrackRow) {
						t.Fatalf("cursor %d: invalid thumb row %d:\n%s", cursor, thumbRow, ansi.Strip(view))
					}
				}

				m.cursor, m.jobCursor = 0, 0
				row := 5 // Second task follows two detail rows.
				if jobs {
					row = 4 // Jobs have one detail row.
				}
				updated, _ := m.updateMouse(mouseClickAt(4, row))
				m = updated.(model)
				if jobs && m.jobCursor != 1 || !jobs && m.cursor != 1 {
					t.Fatal("click on truncated row did not select the second item")
				}
				updated, cmd := m.updateMouse(mouseClickAt(size[0]-4, 2))
				after := updated.(model)
				if after.cursor != m.cursor || after.jobCursor != m.jobCursor || cmd != nil {
					t.Fatal("clicking the indicator selected or opened an item")
				}
				updated, _ = m.updateMouse(tea.MouseMsg(tea.MouseEvent{Button: tea.MouseButtonWheelDown}))
				m = updated.(model)
				if jobs && m.jobCursor != 2 || !jobs && m.cursor != 2 {
					t.Fatal("wheel did not advance the selected item")
				}
				m.tasks = m.tasks[:1]
				m.jobs = m.jobs[:1]
				m.cursor, m.jobCursor = 0, 0
				if strings.Contains(ansi.Strip(m.View()), "█") {
					t.Fatal("indicator remains when the list fits")
				}
			})
		}
	}
}
