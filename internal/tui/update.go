package tui

import tea "github.com/charmbracelet/bubbletea"

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		if m.helpOpen {
			m.syncHelpViewport()
		}
		return m, nil
	case settingsLoadedMsg:
		m.applySettingsLoaded(msg)
		return m, nil
	case settingSelectedMsg:
		m.applySettingSelected(msg)
		return m, nil
	case daemonPollMsg:
		m.applyDaemonPoll(msg)
		return m, m.pollDaemon()
	case taskStartedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.trackedJobs[msg.job.ID] = struct{}{}
		return m, nil
	case jobOpenedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.openResult(msg.job)
		return m, nil
	case jobActionMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.applyJobAction(msg)
		return m, nil
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
		if m.settingsOpen {
			return m.updateSettings(msg)
		}
		if m.helpOpen {
			return m.updateHelp(msg)
		}
		switch msg.String() {
		case "f1":
			m.openHelp()
			return m, nil
		case "?":
			if !m.acceptsTextInput() {
				m.openHelp()
				return m, nil
			}
		case "s":
			if !m.acceptsTextInput() {
				return m.openSettings()
			}
		case "q":
			if !m.acceptsTextInput() {
				return m, tea.Quit
			}
		case "f6":
			m.switchTab(otherTopLevelTab(m.screen))
			return m, nil
		}
	}

	switch m.screen {
	case screenTask:
		return m.updateTaskForm(msg)
	case screenField:
		return m.updateFieldForm(msg)
	case screenRun:
		return m.updateRunForm(msg)
	case screenJobs:
		return m.updateJobs(msg)
	case screenResult:
		return m.updateResult(msg)
	default:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			return m, nil
		}
		return m.updateList(key)
	}
}
