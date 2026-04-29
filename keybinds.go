package main

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

type appState int

const (
	stateNormal appState = iota
	stateRenaming
	stateFilepicker
	stateConfirmingDelete
	stateDeleting
)

func (m model) appState() appState {
	switch {
	case m.renameID != "":
		return stateRenaming
	case m.dirpickerOpen:
		return stateFilepicker
	case m.deleting:
		return stateDeleting
	case m.confirmingDelete():
		return stateConfirmingDelete
	default:
		return stateNormal
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.appState() {
	case stateRenaming:
		return m.handleRenameKeys(msg)
	case stateFilepicker:
		return m.handleDirpickerKeys(msg)
	case stateConfirmingDelete:
		return m.handleConfirmDeleteKeys(msg)
	case stateDeleting:
		return m, nil
	default:
		return m.handleNormalKeys(msg)
	}
}

func (m model) handleRenameKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelRename()
		return m, nil
	case "enter":
		cmd := m.finishRename()
		return m, cmd
	}
	var cmd tea.Cmd
	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
}

func (m model) handleConfirmDeleteKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.deleting = true
		var ids []string
		for id := range m.selected {
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			if item := m.list.SelectedItem(); item != nil {
				if sess, ok := sessionFromItem(item); ok {
					ids = append(ids, sess.ID)
				}
			}
		}
		if len(ids) == 0 {
			m.deleting = false
			m.confirming = false
			return m, nil
		}
		return m, tea.Batch(m.spinner.Tick, doDeleteCmd(m.agentPath, ids))
	case "n", "N", "esc", "q":
		m.confirming = false
		return m, nil
	}
	return m, nil
}

func (m model) handleDirpickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if m.dirpicker.filtering {
		switch msg.String() {
		case "esc":
			m.dirpicker.filtering = false
			m.dirpicker.filterText = ""
			m.dirpicker.applyFilter()
			return m, nil
		case "backspace":
			if m.dirpicker.filterText == "" {
				m.dirpicker.filtering = false
			} else {
				m.dirpicker.filterText = m.dirpicker.filterText[:len(m.dirpicker.filterText)-1]
			}
			m.dirpicker.applyFilter()
			return m, nil
		case "enter":
			m.dirpicker.filtering = false
			return m, nil
		}

		s := msg.String()
		if len(s) == 1 && s[0] >= ' ' && s[0] <= '~' {
			m.dirpicker.filterText += s
			m.dirpicker.applyFilter()
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "j", "k", "up", "down", "ctrl+n", "ctrl+p",
		"g", "G", "pgup", "pgdown",
		"h", "left", "backspace",
		"l", "right", ".":
		var cmd tea.Cmd
		cmd = m.dirpicker.Update(msg)
		return m, cmd
	case "esc":
		if m.dirpicker.filterText != "" {
			m.dirpicker.filterText = ""
			m.dirpicker.applyFilter()
			return m, nil
		}
		m.dirpickerOpen = false
		return m, nil
	case "q":
		m.dirpickerOpen = false
		return m, nil
	case "enter":
		return m.handleDirpickerEnter()
	case "/":
		m.dirpicker.filtering = true
		m.dirpicker.filterText = ""
		m.dirpicker.applyFilter()
		return m, nil
	}
	return m, nil
}

func (m model) handleDirpickerEnter() (tea.Model, tea.Cmd) {
	var dir string
	if m.dirpicker.selected < len(m.dirpicker.entries) {
		dir = filepath.Join(m.dirpicker.currentDir, m.dirpicker.entries[m.dirpicker.selected].Name())
	} else {
		dir = m.dirpicker.currentDir
	}
	return m.confirmDirpickerDir(dir)
}

func (m model) confirmDirpickerDir(dir string) (tea.Model, tea.Cmd) {
	m.dirpickerOpen = false
	m.actionDir = dir
	m.actionNewSession = true
	m.actionTmux = m.mode == "tmux" && m.hasTmux
	return m, tea.Quit
}

func (m model) handleNormalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	isFiltering := m.list.SettingFilter()

	if m.deleteMode && !isFiltering {
		switch msg.String() {
		case "esc":
			m.deleteMode = false
			m.selected = make(map[string]struct{})
			cmd := m.rebuildItems()
			return m, cmd
		case " ":
			if item := m.list.SelectedItem(); item != nil {
				sess, ok := sessionFromItem(item)
				if !ok {
					return m, nil
				}
				id := sess.ID
				if _, ok := m.selected[id]; ok {
					delete(m.selected, id)
				} else {
					m.selected[id] = struct{}{}
				}
				cmd := m.rebuildItems()
				return m, cmd
			}
			return m, nil
		case "enter", "ctrl+d":
			if len(m.selected) == 0 {
				if _, ok := currentSessionID(m); !ok {
					return m, nil
				}
			}
			m.confirming = true
			return m, nil
		}
	}

	if !isFiltering {
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "shift+down", "J":
			if m.showPreview && m.previewScroll < m.previewScrollMax {
				m.previewScroll += 18
				if m.previewScroll > m.previewScrollMax {
					m.previewScroll = m.previewScrollMax
				}
				return m, nil
			}
		case "shift+up", "K":
			if m.showPreview && m.previewScroll > 0 {
				m.previewScroll -= 18
				if m.previewScroll < 0 {
					m.previewScroll = 0
				}
				return m, nil
			}
		case "tab":
			m.showPreview = !m.showPreview
			m.resize()
			return m, needsPreview(m)
		case "ctrl+g":
			m.grouped = !m.grouped
			if len(m.groups) == 0 {
				m.syncGroups()
			}
			cmd := m.rebuildItems()
			return m, tea.Batch(cmd, needsPreview(m))
		case "ctrl+d":
			m.deleteMode = true
			cmd := m.rebuildItems()
			return m, cmd
		case "ctrl+space", "ctrl+@":
			if m.grouped {
				cmd := m.toggleAllGroups()
				return m, tea.Batch(cmd, needsPreview(m))
			}
			return m, nil
		case "ctrl+t":
			if m.hasTmux {
				m.mode = toggleMode(m)
				m.delegate.mode = m.mode
				cmd := m.rebuildItems()
				return m, tea.Batch(cmd, needsPreview(m))
			}
			return m, nil
		case "ctrl+r":
			m.startRename()
			return m, nil
		case "[":
			if m.grouped {
				return m.jumpGroup(-1)
			}
		case "]":
			if m.grouped {
				return m.jumpGroup(1)
			}
		case "h":
			if m.grouped {
				cmd := m.setCurrentGroupCollapsed(true)
				return m, tea.Batch(cmd, needsPreview(m))
			}
		case "l":
			if m.grouped {
				cmd := m.setCurrentGroupCollapsed(false)
				return m, tea.Batch(cmd, needsPreview(m))
			}
		case " ":
			if m.grouped && !m.deleteMode {
				cmd := m.handleGroupSpace()
				return m, tea.Batch(cmd, needsPreview(m))
			}
			return m, nil
		case "alt+enter", "ctrl+o":
			if m.hasTmux {
				if m.mode == "all" {
					return m.setAction(true)
				}
				return m.setAction(false)
			}
			return m, nil
		case "enter":
			return m.setAction(m.mode == "tmux" && m.hasTmux)
		case "n":
			return m.handleNewSessionKey()
		case "N":
			return m.handleNewSessionPickerKey()
		}
	}

	if !m.deleteMode {
		switch msg.String() {
		case "ctrl+n":
			oldIndex := m.list.Index()
			m.list.CursorDown()
			m.skipSeparatorSelection(oldIndex)
			return m.afterMove()
		case "ctrl+p":
			oldIndex := m.list.Index()
			m.list.CursorUp()
			m.skipSeparatorSelection(oldIndex)
			return m.afterMove()
		}
	}

	return m.passToList(msg)
}

func (m model) handleNewSessionKey() (tea.Model, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}

	var dir string
	switch v := item.(type) {
	case sessionItem:
		dir = v.session.Directory
	case groupHeaderItem:
		dir = v.path
	default:
		return m, nil
	}

	if dir == "" {
		return m, nil
	}

	m.actionDir = dir
	m.actionNewSession = true
	m.actionTmux = m.mode == "tmux" && m.hasTmux
	return m, tea.Quit
}

func (m model) handleNewSessionPickerKey() (tea.Model, tea.Cmd) {
	m.dirpickerOpen = true
	return m, m.dirpicker.readDir()
}
