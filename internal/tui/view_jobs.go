package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
)

func jobPolicySummary(policy task.JobPolicy) string {
	switch policy {
	case task.JobParallel:
		return "parallel jobs"
	case task.JobCancelPrevious:
		return "cancel previous job"
	default:
		return "sequential jobs"
	}
}

func (m model) taskActivity(taskID string) string {
	running := m.running[taskID]
	queued := m.queued[taskID]
	switch {
	case running > 0 && queued > 0:
		return fmt.Sprintf("  [running %d, queued %d]", running, queued)
	case running > 0:
		return fmt.Sprintf("  [running %d]", running)
	case queued > 0:
		return fmt.Sprintf("  [queued %d]", queued)
	case m.latest[taskID] == daemon.StatusSucceeded:
		return "  [succeeded]"
	case m.latest[taskID] == daemon.StatusFailed:
		return "  [failed]"
	case m.latest[taskID] == daemon.StatusCanceled:
		return "  [canceled]"
	default:
		return ""
	}
}

func (m model) jobsView() string {
	var body strings.Builder
	if len(m.jobs) == 0 {
		body.WriteString(m.styles.muted.Render("No jobs yet."))
	} else {
		start, end := m.listRange(len(m.jobs), m.jobCursor, 1)
		for i := start; i < end; i++ {
			job := m.jobs[len(m.jobs)-1-i]
			line := fmt.Sprintf("%-9s %s  %s", strings.ToUpper(string(job.Status)), strings.ReplaceAll(job.Name, "\n", " "), job.ShortID())
			if i == m.jobCursor {
				line = m.styles.selected.Render("› " + line)
			} else {
				line = "  " + line
			}
			body.WriteString(line)
			body.WriteByte('\n')
			if i == m.jobCursor && m.contentHeight() > 1 {
				body.WriteString("  ")
				body.WriteString(m.styles.muted.Render(fmt.Sprintf(
					"%s • %d bytes output • %s",
					job.CreatedAt.Local().Format("2006-01-02 15:04:05"),
					job.OutputSize,
					strings.ReplaceAll(job.Command, "\n", " "),
				)))
				body.WriteByte('\n')
			}
		}
		return m.renderWorkspacePanel(m.listWithScrollBar(body.String(), start, end, len(m.jobs)), screenJobs)
	}
	return m.renderWorkspacePanel(body.String(), screenJobs)
}

func (m model) resultHeader(now time.Time) string {
	status := strings.ToUpper(string(m.result.Status))
	style := m.styles.jobQueued
	switch m.result.Status {
	case daemon.StatusFailed:
		style = m.styles.jobFailed
	case daemon.StatusSucceeded:
		style = m.styles.jobSucceeded
	case daemon.StatusCanceled:
		style = m.styles.jobCanceled
	}

	left := style.Render(status) + "  " + m.result.Name
	if m.result.Error != "" {
		left += ": " + m.result.Error
	}

	elapsed := formatJobElapsed(m.result, now)
	if elapsed == "" {
		return left
	}
	elapsed = m.styles.muted.Render(elapsed)

	width := m.contentWidth()
	left = ansi.Truncate(left, max(0, width-ansi.StringWidth(elapsed)-1), "…")
	spacer := strings.Repeat(" ", max(1, width-ansi.StringWidth(left)-ansi.StringWidth(elapsed)))
	return left + spacer + elapsed
}

func formatJobElapsed(job daemon.Job, now time.Time) string {
	if job.StartedAt.IsZero() {
		return ""
	}
	end := job.EndedAt
	if end.IsZero() {
		end = now
	}
	elapsed := max(end.Sub(job.StartedAt), 0)
	return elapsed.Round(time.Millisecond).String()
}

func (m model) resultView() string {
	var body strings.Builder
	body.WriteString(m.resultHeader(time.Now()))
	if m.result.StorageError != "" {
		body.WriteByte('\n')
		body.WriteString(m.styles.error.Render("Log persistence: " + m.result.StorageError))
	}
	body.WriteString(m.gap())
	body.WriteString(lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.resultViewport.View(),
		m.scrollBar(
			m.resultViewport.Height,
			!m.resultViewport.AtTop() || !m.resultViewport.AtBottom(),
			m.resultViewport.ScrollPercent(),
		),
	))
	return m.renderWorkspacePanel(body.String(), screenResult)
}
