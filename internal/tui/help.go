package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
)

type helpBinding struct {
	key         string
	description string
}

type helpGroup struct {
	title    string
	bindings []helpBinding
}

func (m model) helpGroups() []helpGroup {
	general := helpGroup{
		title: "General",
		bindings: []helpBinding{
			{key: "tab / f6", description: "switch Tasks or Jobs"},
			{key: "q / ctrl+c", description: "quit"},
			{key: "? / f1 / esc", description: "close keybinds"},
		},
	}
	switch m.screen {
	case screenTask:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "tab / shift+tab", description: "move between sections"},
					{key: "↑ / ↓", description: "move or select"},
					{key: "enter", description: "insert command line or edit field"},
					{key: "← / →", description: "change job policy"},
					{key: "a / e / d", description: "add, edit, or delete field"},
					{key: "[ / ]", description: "reorder selected field"},
					{key: "f2 / ctrl+s", description: "save task"},
					{key: "esc", description: "cancel"},
				},
			},
			general,
		}
	case screenField:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "tab / shift+tab", description: "move between controls"},
					{key: "↑ / ↓", description: "move between controls"},
					{key: "← / → / space", description: "change selected option"},
					{key: "enter / ctrl+s", description: "save field"},
					{key: "esc", description: "cancel"},
				},
			},
			general,
		}
	case screenRun:
		bindings := []helpBinding{
			{key: "enter", description: "accept and continue"},
			{key: "shift+tab / ctrl+←", description: "return to previous field"},
			{key: "esc", description: "cancel run"},
		}
		if len(m.runTask.Fields) > 0 {
			switch m.runTask.Fields[m.runIndex].Type {
			case task.FieldText, task.FieldChoice:
				bindings = append(bindings, helpBinding{key: "↑ / ↓", description: "history or choice"})
			case task.FieldFile:
				bindings = append(bindings, helpBinding{key: "↑ / ↓", description: "browse files"})
			case task.FieldConfirm:
				bindings = append(bindings, helpBinding{key: "← / →", description: "choose No or Yes"})
			}
		}
		return []helpGroup{{bindings: bindings}, general}
	case screenJobs:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "↑ / k", description: "select previous job"},
					{key: "↓ / j", description: "select next job"},
					{key: "enter", description: "open selected log"},
					{key: "c", description: "cancel selected job"},
					{key: "r", description: "rerun selected job"},
					{key: "d", description: "delete selected completed job"},
				},
			},
			general,
		}
	case screenResult:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "↑ / ↓", description: "scroll one line"},
					{key: "pgup / pgdown", description: "scroll one page"},
					{key: "home / g", description: "jump to top"},
					{key: "end / G", description: "jump to bottom"},
					{key: "c / r / d", description: "cancel, rerun, or delete job"},
					{key: "esc", description: "return to Jobs"},
				},
			},
			general,
		}
	default:
		return []helpGroup{
			{
				bindings: []helpBinding{
					{key: "↑ / k", description: "select previous task"},
					{key: "↓ / j", description: "select next task"},
					{key: "enter", description: "run selected task"},
					{key: "n / e / d", description: "new, edit, or delete task"},
				},
			},
			general,
		}
	}
}

func (m model) helpContent() string {
	var body strings.Builder
	for _, group := range m.helpGroups() {
		if body.Len() > 0 {
			body.WriteString("\n\n")
		}
		if group.title != "" {
			body.WriteString(accentStyle.Render(group.title))
			body.WriteByte('\n')
		}
		keyWidth := 0
		for _, binding := range group.bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(binding.key))
		}
		for i, binding := range group.bindings {
			if i > 0 {
				body.WriteByte('\n')
			}
			body.WriteString("  ")
			body.WriteString(mutedStyle.Render(binding.key))
			body.WriteString(strings.Repeat(" ", keyWidth-ansi.StringWidth(binding.key)+2))
			body.WriteString(binding.description)
		}
	}
	return body.String()
}

func (m model) helpScreenName() string {
	switch m.screen {
	case screenTask:
		return "Task editor"
	case screenField:
		return "Field editor"
	case screenRun:
		return "Run task"
	case screenJobs:
		return "Jobs"
	case screenResult:
		return "Job log"
	default:
		return "Tasks"
	}
}

func (m model) helpTitle() string {
	return "Keybinds - " + m.helpScreenName()
}

func (m model) compactHelp() bool {
	return m.width < 60 || m.height < 18
}

func (m *model) syncHelpViewport() {
	content := m.helpContent()
	lineCount := strings.Count(content, "\n") + 1
	if m.compactHelp() {
		m.helpViewport.Width = max(1, m.contentWidth()-1)
		m.helpViewport.Height = m.contentHeight()
	} else {
		modalWidth := min(72, max(1, m.width-8))
		m.helpViewport.Width = max(1, modalWidth-helpModalStyle.GetHorizontalFrameSize())
		maxHeight := max(1, m.height-6-helpModalStyle.GetVerticalFrameSize())
		m.helpViewport.Height = min(lineCount, maxHeight)
	}
	m.helpViewport.SetContent(content)
}

func (m *model) openHelp() {
	m.helpOpen = true
	m.syncHelpViewport()
	m.helpViewport.GotoTop()
}

func (m model) updateHelp(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "?", "f1", "esc":
			m.helpOpen = false
			return m, nil
		case "q":
			return m, tea.Quit
		case "home", "g":
			m.helpViewport.GotoTop()
			return m, nil
		case "end", "G":
			m.helpViewport.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.helpViewport, cmd = m.helpViewport.Update(msg)
	return m, cmd
}
