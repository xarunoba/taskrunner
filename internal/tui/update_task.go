package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xarunoba/taskrunner/internal/task"
)

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "tab", "shift+tab":
		m.switchTab(screenJobs)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.tasks)-1 {
			m.cursor++
		}
	case "n":
		m.openTaskForm(task.Task{})
	case "e":
		if len(m.tasks) > 0 {
			m.openTaskForm(m.tasks[m.cursor])
		}
	case "d":
		if len(m.tasks) == 0 {
			break
		}
		name := m.tasks[m.cursor].Name
		if err := m.store.Delete(m.tasks[m.cursor]); err != nil {
			m.setError(err)
			break
		}
		if err := m.reload(); err != nil {
			m.setError(err)
			break
		}
		m.setStatus(fmt.Sprintf("Deleted %s", name))
	case "enter":
		if len(m.tasks) == 0 {
			break
		}
		return m.startRun(m.tasks[m.cursor])
	}
	return m, nil
}

func (m model) updateTaskForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			if m.standaloneForm {
				return m, tea.Quit
			}
			m.closeTaskForm()
			return m, nil
		case "ctrl+s", "f2":
			return m.saveTaskForm()
		case "tab":
			m.taskFocus = (m.taskFocus + 1) % 4
			return m, m.focusTaskControl()
		case "shift+tab":
			m.taskFocus = (m.taskFocus + 3) % 4
			return m, m.focusTaskControl()
		case "up":
			if m.taskFocus == 1 {
				break
			}
			if m.taskFocus == 3 && m.fieldCursor > 0 {
				m.fieldCursor--
				return m, nil
			}
			if m.taskFocus > 0 {
				m.taskFocus--
				return m, m.focusTaskControl()
			}
		case "down":
			if m.taskFocus == 1 {
				break
			}
			if m.taskFocus == 3 {
				if m.fieldCursor < len(m.formFields)-1 {
					m.fieldCursor++
				}
				return m, nil
			}
			m.taskFocus++
			return m, m.focusTaskControl()
		case "enter":
			if m.taskFocus == 1 {
				break
			}
			if m.taskFocus < 3 {
				m.taskFocus++
				return m, m.focusTaskControl()
			}
			if len(m.formFields) == 0 {
				m.openFieldForm(-1)
			} else {
				m.openFieldForm(m.fieldCursor)
			}
			return m, nil
		}

		if m.taskFocus == 2 {
			switch key.String() {
			case "left", "h":
				m.moveJobPolicy(-1)
			case "right", "l", " ":
				m.moveJobPolicy(1)
			}
			return m, nil
		}
		if m.taskFocus == 3 {
			switch key.String() {
			case "k":
				if m.fieldCursor > 0 {
					m.fieldCursor--
				} else {
					m.taskFocus = 2
					return m, m.focusTaskControl()
				}
			case "j":
				if m.fieldCursor < len(m.formFields)-1 {
					m.fieldCursor++
				}
			case "a":
				m.openFieldForm(-1)
			case "e":
				if len(m.formFields) > 0 {
					m.openFieldForm(m.fieldCursor)
				}
			case "[":
				if m.fieldCursor > 0 {
					m.formFields[m.fieldCursor-1], m.formFields[m.fieldCursor] = m.formFields[m.fieldCursor], m.formFields[m.fieldCursor-1]
					m.fieldCursor--
				}
			case "]":
				if m.fieldCursor < len(m.formFields)-1 {
					m.formFields[m.fieldCursor], m.formFields[m.fieldCursor+1] = m.formFields[m.fieldCursor+1], m.formFields[m.fieldCursor]
					m.fieldCursor++
				}
			case "d":
				if len(m.formFields) > 0 {
					m.formFields = append(m.formFields[:m.fieldCursor], m.formFields[m.fieldCursor+1:]...)
					if m.fieldCursor >= len(m.formFields) && m.fieldCursor > 0 {
						m.fieldCursor--
					}
				}
			}
			return m, nil
		}
	}

	if m.taskFocus >= 2 {
		return m, nil
	}

	var cmd tea.Cmd
	if m.taskFocus == 0 {
		m.taskNameInput, cmd = m.taskNameInput.Update(msg)
	} else {
		m.taskCommandInput, cmd = m.taskCommandInput.Update(msg)
		m.resizeTaskForm()
	}
	return m, cmd
}

func (m *model) moveJobPolicy(direction int) {
	index := jobPolicyIndex(m.formJobPolicy)
	index = (index + direction + len(jobPolicies)) % len(jobPolicies)
	m.formJobPolicy = jobPolicies[index]
}

func jobPolicyIndex(policy task.JobPolicy) int {
	for i, candidate := range jobPolicies {
		if candidate == policy {
			return i
		}
	}
	return 0
}

func (m *model) openTaskForm(item task.Task) {
	m.showScreen(screenTask)
	m.editingFile = item.File
	m.taskNameInput.SetValue(item.Name)
	m.taskCommandInput.SetValue(item.Command)
	m.resizeTaskForm()
	m.formFields = cloneFields(item.Fields)
	m.formJobPolicy = item.JobPolicy
	m.taskFocus = 0
	m.fieldCursor = 0
	m.focusTaskControl()
}

func (m *model) closeTaskForm() {
	m.showScreen(screenList)
	m.editingFile = ""
	m.formFields = nil
	m.formJobPolicy = task.JobSequential
	m.taskNameInput.Blur()
	m.taskNameInput.SetValue("")
	m.taskCommandInput.Blur()
	m.taskCommandInput.SetValue("")
}

func (m *model) focusTaskControl() tea.Cmd {
	m.taskNameInput.Blur()
	m.taskCommandInput.Blur()
	switch m.taskFocus {
	case 0:
		return m.taskNameInput.Focus()
	case 1:
		return m.taskCommandInput.Focus()
	default:
		return nil
	}
}

func (m model) saveTaskForm() (tea.Model, tea.Cmd) {
	item := task.Task{
		Name:      m.taskNameInput.Value(),
		Command:   m.taskCommandInput.Value(),
		Fields:    cloneFields(m.formFields),
		JobPolicy: m.formJobPolicy,
	}
	saved, err := m.store.Save(item, m.editingFile)
	if err != nil {
		m.setError(err)
		return m, nil
	}
	if err := m.reload(); err != nil {
		m.setError(err)
		return m, nil
	}
	if m.standaloneForm {
		return m, tea.Quit
	}
	m.closeTaskForm()
	m.setStatus(fmt.Sprintf("Saved %s", saved.Name))
	for i := range m.tasks {
		if m.tasks[i].File == saved.File {
			m.cursor = i
			break
		}
	}
	return m, nil
}
