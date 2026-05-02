package main

import (
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type appState int

const (
	stateShowKeybinds appState = iota
	stateNormal
	stateDeleteMode
	stateRenameInput
	stateForkInput
	stateFilepicker
	stateConfirmingDelete
	stateDeleting
	stateConfirmingNewSession
	stateConfirmingFork
	stateForking
	stateConfirmingCloseTmux
	stateClosingTmux
)

func (m model) appState() appState {
	return m.state
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.appState() {
	case stateShowKeybinds:
		return m.handleKeybindsKeys(msg)
	case stateRenameInput, stateForkInput:
		return m.handleRenameKeys(msg)
	case stateFilepicker:
		return m.handleDirpickerKeys(msg)
	case stateDeleteMode:
		return m.handleNormalKeys(msg)
	case stateConfirmingDelete:
		return m.handleConfirmDeleteKeys(msg)
	case stateDeleting, stateForking, stateClosingTmux:
		return m, nil
	case stateConfirmingNewSession:
		return m.handleConfirmNewSessionKeys(msg)
	case stateConfirmingFork:
		return m.handleConfirmForkKeys(msg)
	case stateConfirmingCloseTmux:
		return m.handleConfirmCloseTmuxKeys(msg)
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
		if m.state == stateForkInput {
			cmd := m.finishFork()
			return m, cmd
		}
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
		m.state = stateDeleting
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
			m.state = stateNormal
			return m, nil
		}
		return m, tea.Batch(m.spinner.Tick, doDeleteCmd(m.agentPath, ids, m.sessions))
	case "n", "N", "esc", "q":
		m.state = stateDeleteMode
		return m, nil
	}
	return m, nil
}

func (m model) handleConfirmNewSessionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.state = stateNormal
		m.actionDir = m.pendingNewSessionDir
		m.actionNewSession = true
		m.actionTmux = m.pendingNewSessionTmux
		m.pendingNewSessionDir = ""
		return m, tea.Quit
	case "n", "N", "esc", "q":
		m.state = stateNormal
		m.pendingNewSessionDir = ""
		return m, nil
	}
	return m, nil
}

func (m model) handleConfirmForkKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.state = stateForking
		return m, tea.Batch(
			m.spinner.Tick,
			doForkCmd(
				m.dbPath,
				m.pendingForkID,
				"!DUP "+m.pendingForkTitle,
				m.pendingForkDir,
				m.mode == "tmux" && m.hasTmux,
			),
		)
	case "n", "N", "esc", "q":
		m.state = stateNormal
		m.pendingForkID = ""
		m.pendingForkTitle = ""
		m.pendingForkDir = ""
		return m, nil
	}
	return m, nil
}

func (m model) handleConfirmCloseTmuxKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.state = stateClosingTmux
		return m, tea.Batch(
			m.spinner.Tick,
			doCloseTmuxCmd(m.dbPath, m.closeTmuxSessionID, m.closeTmuxTitle, m.sessions),
		)
	case "n", "N", "esc", "q":
		m.state = stateNormal
		m.closeTmuxSessionID = ""
		m.closeTmuxTitle = ""
		return m, nil
	}
	return m, nil
}

func (m model) handleKeybindsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	modalMaxH := m.height * 80 / 100
	if modalMaxH < 10 {
		modalMaxH = 10
	}
	const chrome = 8
	maxBodyLines := modalMaxH - chrome
	if maxBodyLines < 4 {
		maxBodyLines = 4
	}
	maxScroll := len(keybindsEntries()) - maxBodyLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "esc", "?", "q":
		m.state = stateNormal
		m.keybindsScroll = 0
		return m, nil
	case "j", "down":
		if m.keybindsScroll < maxScroll {
			m.keybindsScroll++
		}
		return m, nil
	case "k", "up":
		if m.keybindsScroll > 0 {
			m.keybindsScroll--
		}
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
		m.state = stateNormal
		return m, nil
	case "q":
		m.state = stateNormal
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
	m.state = stateNormal
	m.actionDir = dir
	m.actionNewSession = true
	m.actionTmux = m.mode == "tmux" && m.hasTmux
	return m, tea.Quit
}

func (m model) handleNormalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	isFiltering := m.list.SettingFilter()
	isDeleting := m.state == stateDeleteMode

	if isDeleting && !isFiltering {
		switch msg.String() {
		case "esc", "q":
			m.state = stateNormal
			m.selected = make(map[string]struct{})
			cmd := m.rebuildItems()
			return m, cmd
		case "?":
			m.state = stateShowKeybinds
			m.keybindsScroll = 0
			return m, nil
		case " ":
			if m.grouped {
				if item := m.list.SelectedItem(); item != nil {
					if _, ok := item.(groupHeaderItem); ok {
						cmd := m.handleGroupSpace()
						return m, tea.Batch(cmd, needsPreview(m))
					}
				}
			}
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
		case "enter", "d":
			if len(m.selected) == 0 {
				if _, ok := currentSessionID(m); !ok {
					return m, nil
				}
			}
			m.state = stateConfirmingDelete
			return m, nil
		}
	}

	if !isFiltering {
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			m.state = stateShowKeybinds
			m.keybindsScroll = 0
			return m, nil
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
		case "g":
			m.grouped = !m.grouped
			if len(m.groups) == 0 {
				m.syncGroups()
			}
			cmd := m.rebuildItems()
			return m, tea.Batch(cmd, needsPreview(m))
		case "d":
			m.state = stateDeleteMode
			cmd := m.rebuildItems()
			return m, cmd
		case "ctrl+space", "ctrl+@":
			if m.grouped {
				cmd := m.toggleAllGroups()
				return m, tea.Batch(cmd, needsPreview(m))
			}
			return m, nil
		case "t":
			if m.hasTmux {
				m.mode = toggleMode(m)
				m.delegate.mode = m.mode
				m.states = getSessionStates(m.sessions, m.mode)
				m.lastStateRefresh = time.Now()
				cmd := m.rebuildItems()
				return m, tea.Batch(cmd, needsPreview(m))
			}
			return m, nil
		case "r":
			if !isDeleting {
				m.startRename()
			}
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
			if m.grouped && !isDeleting {
				cmd := m.handleGroupSpace()
				return m, tea.Batch(cmd, needsPreview(m))
			}
			return m, nil
		case "alt+enter", "ctrl+o":
			if !isDeleting && m.hasTmux {
				if m.mode == "all" {
					return m.setAction(true)
				}
				return m.setAction(false)
			}
			return m, nil
		case "enter":
			if item := m.list.SelectedItem(); item != nil {
				if header, ok := item.(groupHeaderItem); ok && m.grouped {
					return m.toggleGroupByPath(header.path, header.collapsed)
				}
			}
			return m.setAction(m.mode == "tmux" && m.hasTmux)
		case "n":
			if !isDeleting {
				return m.handleNewSessionKey()
			}
			return m, nil
		case "N":
			if !isDeleting {
				return m.handleNewSessionPickerKey()
			}
			return m, nil
		case "y":
			if !isDeleting {
				return m.handleForkKey()
			}
			return m, nil
		case "Y":
			if !isDeleting {
				m.startFork()
			}
			return m, nil
		case "x":
			if !isDeleting && m.hasTmux && m.mode == "tmux" {
				return m.handleCloseTmuxKey()
			}
			return m, nil
		}
	}

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

	m.state = stateConfirmingNewSession
	m.pendingNewSessionDir = dir
	m.pendingNewSessionTmux = m.mode == "tmux" && m.hasTmux
	return m, nil
}

func (m model) handleNewSessionPickerKey() (tea.Model, tea.Cmd) {
	m.state = stateFilepicker
	return m, m.dirpicker.readDir()
}

func (m model) handleForkKey() (tea.Model, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}
	sess, ok := sessionFromItem(item)
	if !ok {
		return m, nil
	}
	m.state = stateConfirmingFork
	m.pendingForkID = sess.ID
	m.pendingForkTitle = sess.Title
	m.pendingForkDir = sess.Directory
	return m, nil
}

func (m model) handleCloseTmuxKey() (tea.Model, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}
	sess, ok := sessionFromItem(item)
	if !ok {
		return m, nil
	}
	st := m.states[sess.ID]
	if st != stateLinked {
		return m, nil
	}
	m.state = stateConfirmingCloseTmux
	m.closeTmuxSessionID = sess.ID
	m.closeTmuxTitle = sess.Title
	return m, nil
}
