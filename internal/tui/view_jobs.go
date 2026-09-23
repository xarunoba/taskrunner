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
		body.WriteString(mutedStyle.Render("No jobs yet."))
	} else {
		start, end := visibleRange(len(m.jobs), m.jobCursor, max(1, m.contentHeight()-4))
		for i := start; i < end; i++ {
			job := m.jobs[len(m.jobs)-1-i]
			line := fmt.Sprintf("%-9s %s  %s", strings.ToUpper(string(job.Status)), job.Name, job.ShortID())
			if i == m.jobCursor {
				line = selectedStyle.Render("› " + line)
			} else {
				line = "  " + line
			}
			body.WriteString(line)
			body.WriteByte('\n')
			if i == m.jobCursor {
				body.WriteString("  ")
				body.WriteString(mutedStyle.Render(fmt.Sprintf(
					"%s • %d bytes output • %s",
					job.CreatedAt.Local().Format("2006-01-02 15:04:05"),
					job.OutputSize,
					job.Command,
				)))
				body.WriteByte('\n')
			}
		}
	}
	return m.renderWorkspacePanel(body.String(), screenJobs)
}

func (m model) resultHeader(now time.Time) string {
	status := strings.ToUpper(string(m.result.Status))
	style := jobQueuedStyle
	switch m.result.Status {
	case daemon.StatusFailed:
		style = jobFailedStyle
	case daemon.StatusSucceeded:
		style = jobSucceededStyle
	case daemon.StatusCanceled:
		style = jobCanceledStyle
	}

	left := style.Render(status) + "  " + m.result.Name
	if m.result.Error != "" {
		left += ": " + m.result.Error
	}

	elapsed := formatJobElapsed(m.result, now)
	if elapsed == "" {
		return left
	}
	elapsed = mutedStyle.Render(elapsed)

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
		body.WriteString(errorStyle.Render("Log persistence: " + m.result.StorageError))
	}
	body.WriteString(m.gap())
	body.WriteString(lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.resultViewport.View(),
		scrollBar(
			m.resultViewport.Height,
			!m.resultViewport.AtTop() || !m.resultViewport.AtBottom(),
			m.resultViewport.ScrollPercent(),
		),
	))
	return m.renderWorkspacePanel(body.String(), screenResult)
}
