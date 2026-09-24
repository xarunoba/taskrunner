package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/xarunoba/taskrunner/internal/daemon"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

const violetThemeJSON = `{
	"colors": {
		"accent": "63",
		"muted": "241",
		"cursor": "212",
		"error": "196",
		"selection": {"foreground": "230", "background": "57"},
		"inactive_tab": {"foreground": "250", "background": "236"},
		"status": {"foreground": "250", "background": "236"},
		"error_badge": {"foreground": "230", "background": "196"},
		"queued": {"foreground": "230", "background": "63"},
		"succeeded": {"foreground": "0", "background": "42"},
		"canceled": {"foreground": "255", "background": "241"}
	}
}`

func writeThemeFile(t *testing.T, root, name, raw string) {
	t.Helper()
	dir := filepath.Join(root, "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readConfigFile(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(raw)
}

func typeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func wheelMsg(x, y int, up bool) tea.MouseMsg {
	button := tea.MouseButtonWheelDown
	if up {
		button = tea.MouseButtonWheelUp
	}
	return tea.MouseMsg(tea.MouseEvent{X: x, Y: y, Action: tea.MouseActionPress, Button: button})
}

func openSettingsWithThemes(t *testing.T, m model, names ...string) model {
	t.Helper()
	updated, cmd := m.Update(typeKey('s'))
	m = updated.(model)
	if !m.settingsOpen || cmd == nil {
		t.Fatal("s did not open settings with an enumeration command")
	}
	loaded, ok := cmd().(settingsLoadedMsg)
	if !ok {
		t.Fatalf("settings command produced %#v", cmd())
	}
	loaded.names = names
	updated, _ = m.Update(loaded)
	m = updated.(model)
	if m.settingsLoading || len(m.settings) != 1 {
		t.Fatalf("settings load produced rows %#v", m.settings)
	}
	return m
}

func TestSettingsKeyboardCyclesPersistsAndCloses(t *testing.T) {
	t.Parallel()

	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 40, height: 10},
		{width: 80, height: 24},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			themes := theme.NewStore(root)
			writeThemeFile(t, root, "violet", violetThemeJSON)
			m := newModel(task.NewStore(t.TempDir()), []task.Task{{Name: "Build", Command: "true"}}, themes, theme.Default())
			m.resize(size.width, size.height)

			m = openSettingsWithThemes(t, m, "default", "violet")
			if m.settings[0].selected != 0 {
				t.Fatalf("theme row selected = %d, want 0", m.settings[0].selected)
			}
			assertFillsTerminal(t, m.View(), size.width, size.height)

			// Cycling applies immediately and persists.
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
			m = updated.(model)
			if !m.settingsSaving || cmd == nil {
				t.Fatalf("cycling set saving=%t cmd=%v", m.settingsSaving, cmd)
			}
			if m.settings[0].selected != 1 {
				t.Fatalf("theme row selected = %d, want 1", m.settings[0].selected)
			}

			// Input is rejected while a save is in flight.
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
			m = updated.(model)
			if m.settings[0].selected != 1 || !m.settingsSaving {
				t.Fatalf("saving state accepted input: selected=%d saving=%t", m.settings[0].selected, m.settingsSaving)
			}

			selected, ok := cmd().(settingSelectedMsg)
			if !ok || selected.err != nil {
				t.Fatalf("save command produced %#v", cmd())
			}
			updated, _ = m.Update(selected)
			m = updated.(model)
			if m.theme.Name != "violet" || m.settingsSaving {
				t.Fatalf("applied theme %q saving=%t", m.theme.Name, m.settingsSaving)
			}
			if m.styles.selected.Render("x") == newStyles(theme.Default().Palette).selected.Render("x") {
				t.Fatal("custom theme did not change the selection style")
			}
			if config := readConfigFile(t, root); !strings.Contains(config, `"theme": "violet"`) {
				t.Fatalf("config does not persist the theme:\n%s", config)
			}
			assertFillsTerminal(t, m.View(), size.width, size.height)

			// Closing changes nothing further.
			configBefore := readConfigFile(t, root)
			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = updated.(model)
			if m.settingsOpen {
				t.Fatal("esc did not close settings")
			}
			if readConfigFile(t, root) != configBefore {
				t.Fatal("closing settings rewrote the configuration")
			}

			// Reopening picks up theme files added meanwhile.
			writeThemeFile(t, root, "later", violetThemeJSON)
			m = openSettingsWithThemes(t, m, "default", "violet", "later")
			if got := m.settings[0].options; strings.Join(got, ",") != "default,violet,later" {
				t.Fatalf("reopened options = %#v", got)
			}
			if m.settings[0].selected != 1 {
				t.Fatalf("reopened selection = %d, want 1", m.settings[0].selected)
			}
		})
	}
}

func TestSettingsTruncatesLongThemeNames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	themes := theme.NewStore(root)
	longName := "a-very-long-custom-theme-name-that-exceeds-the-narrow-modal-width"
	writeThemeFile(t, root, longName, violetThemeJSON)
	selected, err := themes.Select(longName)
	if err != nil {
		t.Fatalf("select long theme: %v", err)
	}
	m := newModel(task.NewStore(t.TempDir()), nil, themes, selected)
	m.resize(40, 10)

	updated, cmd := m.Update(typeKey('s'))
	m = updated.(model)
	if cmd == nil {
		t.Fatal("s did not open settings")
	}
	loaded, _ := cmd().(settingsLoadedMsg)
	loaded.names = []string{"default", longName}
	updated, _ = m.Update(loaded)
	m = updated.(model)
	view := ansi.Strip(m.View())
	if strings.Contains(view, longName) {
		t.Fatalf("full long theme name rendered without truncation:\n%s", view)
	}
	if !strings.Contains(view, "…") {
		t.Fatalf("long theme name is not truncated:\n%s", view)
	}
	row := ansi.Strip(m.settingsRow(0, m.settings[0]))
	if got := ansi.StringWidth(row); got > m.contentWidth() {
		t.Fatalf("theme row = %d columns, want at most %d", got, m.contentWidth())
	}
	assertFillsTerminal(t, m.View(), 40, 10)
}

func TestCustomThemePaletteReachesEveryStyle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	themes := theme.NewStore(root)
	writeThemeFile(t, root, "violet", violetThemeJSON)
	selected, err := themes.Select("violet")
	if err != nil {
		t.Fatalf("select violet: %v", err)
	}

	m := newModel(task.NewStore(t.TempDir()), []task.Task{{Name: "Deploy", Command: "true"}}, themes, selected)
	m.resize(80, 24)
	fallback := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
	fallback.resize(80, 24)

	if m.styles.selected.Render("x") == fallback.styles.selected.Render("x") {
		t.Fatal("custom palette did not change the selection style")
	}

	view := m.View()
	if !strings.Contains(view, m.styles.selected.Render("› Deploy")) {
		t.Fatal("selected task does not use the custom selection style")
	}
	header, _, _ := strings.Cut(view, "\n")
	if !strings.Contains(header, m.styles.activeTab.Render("Tasks")) ||
		!strings.Contains(header, m.styles.inactiveTab.Render("Jobs")) {
		t.Fatal("tabs do not use the custom tab styles")
	}
	footer := strings.Split(view, "\n")[m.height-1]
	if !strings.Contains(footer, m.styles.helpChip.Render("? keybinds")) {
		t.Fatal("footer chip does not use the custom selection style")
	}

	m.openResult(daemon.Job{ID: "job-12345678", Name: "Deploy", Status: daemon.StatusFailed})
	if !strings.Contains(m.View(), m.styles.jobFailed.Render("FAILED")) {
		t.Fatal("failed badge does not use the custom error badge style")
	}

	fileTask := task.Task{
		Name:    "Upload",
		Command: "upload {{file}}",
		Fields:  []task.Field{{Key: "file", Label: "File", Type: task.FieldFile}},
	}
	pickerModel := newModel(task.NewStore(t.TempDir()), []task.Task{fileTask}, themes, selected)
	started, _ := pickerModel.startRun(fileTask)
	pickerModel = started.(model)
	if got := pickerModel.filePicker.Styles.Cursor.Render("f"); got != pickerModel.styles.cursor.Render("f") {
		t.Fatal("file picker cursor does not use the model cursor style")
	}
	if got := pickerModel.filePicker.Styles.Selected.Render("f"); got != pickerModel.styles.selected.Render("f") {
		t.Fatal("file picker selection does not use the model selection style")
	}
}

func TestSettingsMouseOpensCyclesAndBlocksUnderlyingScreen(t *testing.T) {
	t.Parallel()

	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 40, height: 10},
		{width: 80, height: 24},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			t.Parallel()

			for _, jobsScreen := range []bool{false, true} {
				root := t.TempDir()
				themes := theme.NewStore(root)
				writeThemeFile(t, root, "violet", violetThemeJSON)
				tasks := []task.Task{
					{
						Name:    "Build",
						Command: "build {{dir}}",
						Fields:  []task.Field{{Key: "dir", Label: "Dir", Type: task.FieldText}},
					},
					{
						Name:    "Deploy",
						Command: "deploy {{env}}",
						Fields:  []task.Field{{Key: "env", Label: "Env", Type: task.FieldText}},
					},
				}
				m := newModel(task.NewStore(t.TempDir()), tasks, themes, theme.Default())
				m.resize(size.width, size.height)
				target := "Build"
				if jobsScreen {
					m.applyDaemonPoll(daemonPollMsg{
						jobs:    []daemon.Job{{ID: "job-12345678", Name: "Deploy", Status: daemon.StatusRunning}},
						details: map[string]daemon.Job{},
					})
					m.showScreen(screenJobs)
					target = "Deploy"
				}

				// The footer chip opens settings.
				updated, cmd := m.Update(mouseClickOn(t, m.View(), "s settings"))
				m = updated.(model)
				if !m.settingsOpen || cmd == nil {
					t.Fatalf("%dx%d: settings chip click did not open settings", size.width, size.height)
				}
				loaded, _ := cmd().(settingsLoadedMsg)
				loaded.names = []string{"default", "violet"}
				updated, _ = m.Update(loaded)
				m = updated.(model)

				// Wheel and clicks never reach the covered screen. The click
				// lands on modal dead space so the modal state stays put.
				updated, _ = m.Update(wheelMsg(4, 5, true))
				m = updated.(model)
				if !m.settingsOpen {
					t.Fatal("wheel closed settings")
				}
				if m.cursor != 0 || m.jobCursor != 0 {
					t.Fatalf("wheel moved the hidden cursor: task=%d job=%d", m.cursor, m.jobCursor)
				}
				updated, blockedCmd := m.Update(mouseClickAt(4, 5))
				m = updated.(model)
				if blockedCmd != nil {
					t.Fatal("click while settings open produced a command")
				}
				if m.cursor != 0 || m.jobCursor != 0 {
					t.Fatalf("click reached the hidden screen: task=%d job=%d", m.cursor, m.jobCursor)
				}

				// Help remains blocked while settings is open.
				updated, _ = m.Update(typeKey('?'))
				m = updated.(model)
				if m.helpOpen || !m.settingsOpen {
					t.Fatalf("help key while settings open produced help=%t settings=%t", m.helpOpen, m.settingsOpen)
				}

				// Clicking the value side cycles to the next theme.
				before := m.settings[0].selected
				current := m.settings[0].options[before]
				updated, cmd = m.Update(mouseClickOn(t, m.View(), current))
				m = updated.(model)
				after := (before + 1) % len(m.settings[0].options)
				if cmd == nil || m.settings[0].selected != after {
					t.Fatalf("value click selected=%d cmd=%v", m.settings[0].selected, cmd)
				}
				selectedMsg, _ := cmd().(settingSelectedMsg)
				updated, _ = m.Update(selectedMsg)
				m = updated.(model)
				if m.theme.Name != m.settings[0].options[after] {
					t.Fatalf("value click applied %q", m.theme.Name)
				}
				if config := readConfigFile(t, root); !strings.Contains(config, `"theme": "`+m.theme.Name+`"`) {
					t.Fatalf("value click did not persist the theme:\n%s", config)
				}

				// Clicking the label side cycles to the previous theme.
				before = m.settings[0].selected
				updated, cmd = m.Update(mouseClickOn(t, m.View(), "Theme"))
				m = updated.(model)
				after = (before + 1) % len(m.settings[0].options)
				if cmd == nil || m.settings[0].selected != after {
					t.Fatalf("label click selected=%d cmd=%v", m.settings[0].selected, cmd)
				}
				selectedMsg, _ = cmd().(settingSelectedMsg)
				updated, _ = m.Update(selectedMsg)
				m = updated.(model)
				if m.theme.Name != m.settings[0].options[after] {
					t.Fatalf("label click applied %q", m.theme.Name)
				}

				// The footer chip closes settings; the screen works again.
				updated, _ = m.Update(mouseClickOn(t, m.View(), "esc close"))
				m = updated.(model)
				if m.settingsOpen {
					t.Fatal("settings chip click did not close settings")
				}
				if jobsScreen {
					updated, cmd = m.Update(mouseClickOn(t, m.View(), target))
					m = updated.(model)
					if cmd == nil {
						t.Fatal("job click after close did not open the job")
					}
					return
				}
				// Select the second task, then click it to start its run form.
				updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
				m = updated.(model)
				updated, cmd = m.Update(mouseClickOn(t, m.View(), "Deploy"))
				m = updated.(model)
				if m.screen != screenRun {
					t.Fatalf("task click after close produced screen %d, want run screen", m.screen)
				}
			}
		})
	}
}
