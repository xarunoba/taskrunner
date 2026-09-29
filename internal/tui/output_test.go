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

func TestOpenResultReplacesEvictedOutput(t *testing.T) {
	m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
	m.resize(80, 24)
	m.openResult(daemon.Job{ID: "live", Status: daemon.StatusRunning, Output: "discarded\nretained\n", OutputSize: 19})
	m.updateOpenResult(daemon.Job{
		ID: "live", Status: daemon.StatusSucceeded,
		Output: "retained\nlast 世界\n", OutputStart: 10, OutputSize: 31,
	})
	if m.result.Output != "retained\nlast 世界\n" {
		t.Fatalf("evicted output remained or new output was duplicated: %q", m.result.Output)
	}
	if strings.Contains(m.resultViewport.View(), "discarded") {
		t.Fatal("viewport still displays evicted output")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Output truncated") {
		t.Fatal("retained tail has no visible truncation warning")
	}
}

func TestResultTruncationLayoutAndScrolling(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {100, 36}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
			m.resize(size[0], size[1])
			m.openResult(daemon.Job{
				ID: "large", Status: daemon.StatusSucceeded,
				Output: strings.Repeat("row 世界\n", 100), OutputStart: 1000, OutputSize: 2100,
			})
			for _, resized := range [][2]int{size, {40, 12}, size} {
				m.resize(resized[0], resized[1])
				view := ansi.Strip(m.View())
				if !strings.Contains(view, "Output truncated") {
					t.Fatalf("missing warning after resize: %s", view)
				}
				lines := strings.Split(view, "\n")
				if len(lines) > resized[1] {
					t.Fatalf("view exceeds height: %d > %d", len(lines), resized[1])
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > resized[0] {
						t.Fatalf("view exceeds width: %q", line)
					}
				}
				updated, _ := m.updateResult(tea.KeyMsg{Type: tea.KeyHome})
				m = updated.(model)
				if !m.resultViewport.AtTop() {
					t.Fatal("home did not reach retained output start")
				}
				updated, _ = m.updateMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
				m = updated.(model)
				if m.resultViewport.AtTop() {
					t.Fatal("wheel did not scroll retained output")
				}
			}
		})
	}
}

func TestLoadWarningsRemainReachableWhileEditing(t *testing.T) {
	m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
	m.openTaskForm(task.Task{Name: "draft s?", Command: "printf preserved"})
	m.applyDaemonPoll(daemonPollMsg{warnings: []string{`skip job "bad.json": invalid JSON`}})
	for _, size := range [][2]int{{24, 10}, {100, 36}, {24, 10}} {
		m.resize(size[0], size[1])
		if !strings.Contains(ansi.Strip(m.View()), "f1 warnings") {
			t.Fatalf("warning control is not visible at %v", size)
		}
		layout := m.footerLayout()
		var warningButton footerButton
		for i := range layout.count {
			if layout.buttons[i].descriptor == "warnings" {
				warningButton = layout.buttons[i]
			}
		}
		updated, _ := m.updateMouse(tea.MouseMsg{
			Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
			X: warningButton.x, Y: size[1] - layout.rows + warningButton.row,
		})
		m = updated.(model)
		if !m.helpOpen {
			t.Fatal("warning button did not open diagnostics")
		}
		if !strings.Contains(strings.ReplaceAll(ansi.Strip(m.helpContent()), "\n", ""), "bad.json") {
			t.Fatal("wrapped diagnostics lost the offending filename")
		}
		updated, _ = m.updateHelp(tea.KeyMsg{Type: tea.KeyEsc})
		m = updated.(model)
		if m.taskNameInput.Value() != "draft s?" || m.taskCommandInput.Value() != "printf preserved" {
			t.Fatal("opening warnings changed the task draft")
		}
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
		m = updated.(model)
		if !m.helpOpen {
			t.Fatal("advertised warning key did not open diagnostics")
		}
		updated, _ = m.updateHelp(tea.KeyMsg{Type: tea.KeyEsc})
		m = updated.(model)
	}
	m.applyDaemonPoll(daemonPollMsg{})
	if strings.Contains(ansi.Strip(m.View()), "f1 warnings") {
		t.Fatal("warning control remained after warnings cleared")
	}
}
