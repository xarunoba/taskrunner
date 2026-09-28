package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/xarunoba/taskrunner/internal/task"
	"github.com/xarunoba/taskrunner/internal/theme"
)

// advertisedFooterKey presses the key displayed immediately before an action.
func advertisedFooterKey(t *testing.T, view, descriptor string) tea.KeyMsg {
	t.Helper()
	lines := strings.Split(ansi.Strip(view), "\n")
	for _, line := range slices.Backward(lines) {
		words := strings.Fields(line)
		for j, word := range words {
			if word != descriptor || j == 0 {
				continue
			}
			switch words[j-1] {
			case "esc":
				return tea.KeyMsg{Type: tea.KeyEsc}
			case "f1":
				return tea.KeyMsg{Type: tea.KeyF1}
			case "f4":
				return tea.KeyMsg{Type: tea.KeyF4}
			default:
				return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(words[j-1])}
			}
		}
	}
	t.Fatalf("no complete shortcut button for %q:\n%s", descriptor, ansi.Strip(view))
	return tea.KeyMsg{}
}

func TestFooterAdvertisedKeysWorkDuringTextEntry(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"settings", "keybinds", "back"} {
		t.Run(action, func(t *testing.T) {
			m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
			m.resize(80, 24)
			m.openTaskForm(task.Task{Name: "Draft", Command: "true"})
			updated, _ := m.Update(advertisedFooterKey(t, m.View(), action))
			m = updated.(model)
			switch action {
			case "settings":
				if !m.settingsOpen || m.taskNameInput.Value() != "Draft" {
					t.Fatal("advertised settings key edits text instead of opening settings")
				}
			case "keybinds":
				if !m.helpOpen || m.taskNameInput.Value() != "Draft" {
					t.Fatal("advertised keybinds key edits text instead of opening help")
				}
			case "back":
				if m.screen != screenList {
					t.Fatal("advertised back key does not return to Tasks")
				}
			}
		})
	}
}

func TestFooterButtonsFitAndMatchMouseActions(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{24, 10}, {32, 14}, {40, 10}, {80, 24}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
			m.resize(size[0], size[1])
			m.openTaskForm(task.Task{Name: "Draft", Command: "true"})
			view := m.View()
			assertFillsTerminal(t, view, size[0], size[1])
			for _, action := range []string{"back", "settings", "keybinds"} {
				keyboard, _ := m.Update(advertisedFooterKey(t, view, action))
				mouse, _ := m.Update(mouseClickOn(t, view, action))
				keyModel, mouseModel := keyboard.(model), mouse.(model)
				if keyModel.screen != mouseModel.screen || keyModel.settingsOpen != mouseModel.settingsOpen || keyModel.helpOpen != mouseModel.helpOpen {
					t.Fatalf("%s: keyboard and mouse perform different actions", action)
				}
				if action == "back" {
					continue
				}
				modalView := mouseModel.View()
				assertFillsTerminal(t, modalView, size[0], size[1])
				closed, _ := mouseModel.Update(mouseClickOn(t, modalView, "esc close"))
				after := closed.(model)
				if after.settingsOpen || after.helpOpen || after.screen != screenTask || after.taskNameInput.Value() != "Draft" {
					t.Fatal("modal close lost the underlying editor or failed to close")
				}
				closed, _ = mouseModel.Update(advertisedFooterKey(t, modalView, "close"))
				after = closed.(model)
				if after.settingsOpen || after.helpOpen || after.screen != screenTask {
					t.Fatal("advertised modal close key did not restore the editor")
				}
			}
		})
	}
}

func TestFieldAffixesKeepShortcutCharactersAsText(t *testing.T) {
	t.Parallel()
	for _, focus := range []int{5, 6} {
		m := newModel(task.NewStore(t.TempDir()), nil, theme.NewStore(t.TempDir()), theme.Default())
		m.resize(40, 10)
		m.openFieldForm(-1)
		m.fieldFocus = focus
		m.focusFieldControl()
		for _, r := range "s?q" {
			updated, _ := m.Update(typeKey(r))
			m = updated.(model)
		}
		if m.fieldInputs[focus-2].Value() != "s?q" || m.helpOpen || m.settingsOpen {
			t.Fatalf("affix focus %d consumed text as a global shortcut", focus)
		}
		updated, _ := m.Update(advertisedFooterKey(t, m.View(), "settings"))
		if !updated.(model).settingsOpen {
			t.Fatalf("affix focus %d: advertised settings shortcut failed", focus)
		}
	}
}
