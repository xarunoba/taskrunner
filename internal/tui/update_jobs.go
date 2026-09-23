package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/daemon"
)

func (m *model) applyDaemonPoll(msg daemonPollMsg) {
	if msg.err != nil {
		m.setError(fmt.Errorf("poll daemon: %w", msg.err))
		return
	}
	m.setJobs(msg.jobs)

	for _, job := range m.jobs {
		detail, ok := msg.details[job.ID]
		if !ok {
			continue
		}
		if m.result.ID == job.ID {
			m.updateOpenResult(detail)
		}
		if _, tracked := m.trackedJobs[job.ID]; tracked && job.Done() {
			delete(m.trackedJobs, job.ID)
			if m.screen == screenList {
				m.openResult(detail)
			}
		}
	}
}

func (m *model) setJobs(jobs []daemon.Job) {
	m.jobs = jobs
	clear(m.running)
	clear(m.queued)
	clear(m.latest)
	for _, job := range m.jobs {
		m.latest[job.TaskID] = job.Status
		switch job.Status {
		case daemon.StatusRunning:
			m.running[job.TaskID]++
		case daemon.StatusQueued:
			m.queued[job.TaskID]++
		}
	}
	if m.jobCursor >= len(m.jobs) && m.jobCursor > 0 {
		m.jobCursor = len(m.jobs) - 1
	}
}

func (m *model) updateOpenResult(job daemon.Job) {
	atBottom := m.resultViewport.AtBottom()
	if len(job.Output) == job.OutputSize {
		m.result.Output = job.Output
	} else {
		m.result.Output += job.Output
	}
	job.Output = m.result.Output
	m.result = job
	m.setResultContent()
	if atBottom {
		m.resultViewport.GotoBottom()
	}
}

func (m *model) openResult(job daemon.Job) {
	m.result = job
	m.setResultContent()
	m.resultViewport.GotoBottom()
	m.showScreen(screenResult)
}

func (m *model) setResultContent() {
	output := ansi.Strip(m.result.Output)
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	output = strings.TrimRight(output, "\n")
	if output == "" {
		output = "(no output)"
	}

	var content strings.Builder
	if m.result.Command != "" {
		script := ansi.Hardwrap("$ "+m.result.Command, m.resultViewport.Width, true)
		content.WriteString(mutedStyle.Render(script))
		content.WriteString("\n\n")
	}
	content.WriteString(output)
	m.resultViewport.SetContent(content.String())
	m.resizeResultViewport()
}

func (m model) openJob(id string) tea.Cmd {
	client := m.daemon
	return func() tea.Msg {
		job, err := client.Job(id, 0)
		if err != nil {
			err = fmt.Errorf("open job: %w", err)
		}
		return jobOpenedMsg{job: job, err: err}
	}
}

func (m model) runJobAction(action jobAction, id string) tea.Cmd {
	client := m.daemon
	return func() tea.Msg {
		var (
			job daemon.Job
			err error
		)
		switch action {
		case jobActionCancel:
			job, err = client.Cancel(id)
		case jobActionRerun:
			job, err = client.Rerun(id)
		case jobActionDelete:
			job, err = client.Remove(id)
		}
		if err != nil {
			err = fmt.Errorf("%s job: %w", action, err)
		}
		return jobActionMsg{action: action, job: job, err: err}
	}
}

func jobActionForKey(key string) (jobAction, bool) {
	switch key {
	case "c":
		return jobActionCancel, true
	case "r":
		return jobActionRerun, true
	case "d":
		return jobActionDelete, true
	default:
		return "", false
	}
}

func (m *model) applyJobAction(msg jobActionMsg) {
	switch msg.action {
	case jobActionRerun:
		m.jobs = append(m.jobs, msg.job)
		m.trackedJobs[msg.job.ID] = struct{}{}
		m.jobCursor = 0
		m.showScreen(screenJobs)
		m.setStatus(fmt.Sprintf("Started new job for %s", msg.job.Name))
	case jobActionDelete:
		for i := range m.jobs {
			if m.jobs[i].ID != msg.job.ID {
				continue
			}
			last := len(m.jobs) - 1
			copy(m.jobs[i:], m.jobs[i+1:])
			m.jobs[last] = daemon.Job{}
			m.setJobs(m.jobs[:last])
			break
		}
		delete(m.trackedJobs, msg.job.ID)
		if m.result.ID == msg.job.ID {
			m.result = daemon.Job{}
			m.showScreen(screenJobs)
			m.jobScreen = screenJobs
		}
		m.setStatus(fmt.Sprintf("Deleted job %s %s", msg.job.Name, msg.job.ShortID()))
	default:
		for i := range m.jobs {
			if m.jobs[i].ID == msg.job.ID {
				m.jobs[i] = msg.job
				break
			}
		}
		m.setStatus(fmt.Sprintf("Cancel requested for %s %s", msg.job.Name, msg.job.ShortID()))
	}
}

func (m model) updateJobs(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.switchTab(screenList)
	case "up", "k":
		if m.jobCursor > 0 {
			m.jobCursor--
		}
	case "down", "j":
		if m.jobCursor < len(m.jobs)-1 {
			m.jobCursor++
		}
	case "enter":
		job, ok := m.selectedJob()
		if ok {
			return m, m.openJob(job.ID)
		}
	case "c", "r", "d":
		job, ok := m.selectedJob()
		if !ok {
			break
		}
		action, _ := jobActionForKey(key.String())
		return m, m.runJobAction(action, job.ID)
	}
	return m, nil
}

func (m model) selectedJob() (daemon.Job, bool) {
	if len(m.jobs) == 0 || m.jobCursor < 0 || m.jobCursor >= len(m.jobs) {
		return daemon.Job{}, false
	}
	return m.jobs[len(m.jobs)-1-m.jobCursor], true
}

func (m model) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.showScreen(screenJobs)
			return m, nil
		case "home", "g":
			m.resultViewport.GotoTop()
			return m, nil
		case "end", "G":
			m.resultViewport.GotoBottom()
			return m, nil
		case "c", "r", "d":
			if m.result.ID == "" {
				return m, nil
			}
			action, _ := jobActionForKey(key.String())
			return m, m.runJobAction(action, m.result.ID)
		}
	}
	var cmd tea.Cmd
	m.resultViewport, cmd = m.resultViewport.Update(msg)
	return m, cmd
}
