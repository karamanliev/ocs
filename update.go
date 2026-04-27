package main

import (
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case previewMsg:
		m.firstMsgs[msg.id] = msg.data
		return m, nil

	case tea.FocusMsg:
		return m, func() tea.Msg {
			dark, err := queryTerminalBackground()
			return checkThemeMsg{dark: dark, err: err}
		}

	case checkThemeMsg:
		if msg.err == nil && msg.dark != m.isDark {
			m.isDark = msg.dark
			m.theme = themeForDark[m.isDark]
			m.applyTheme()
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m.passToList(msg)
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.renameID != "" {
		switch msg.String() {
		case "esc":
			m.cancelRename()
			return m, nil
		case "enter":
			m.finishRename()
			return m, nil
		}
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	}

	if m.confirmingDelete() {
		switch msg.String() {
		case "y", "Y":
			return m.executeDelete()
		case "n", "N", "esc", "q":
			m.confirming = false
			return m, nil
		}
		return m, nil
	}

	isFiltering := m.list.SettingFilter()

	if m.deleteMode && !isFiltering {
		switch msg.String() {
		case "esc":
			m.deleteMode = false
			m.selected = make(map[string]struct{})
			m.rebuildItems()
			return m, nil
		case " ":
			if item := m.list.SelectedItem(); item != nil {
				id := item.(sessionItem).session.ID
				if _, ok := m.selected[id]; ok {
					delete(m.selected, id)
				} else {
					m.selected[id] = struct{}{}
				}
				m.rebuildItems()
			}
			return m, nil
		case "enter", "ctrl+d":
			m.confirming = true
			return m, nil
		}
	}

	if !isFiltering {
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.showPreview = !m.showPreview
			m.resize()
			return m, needsPreview(m)
		case "ctrl+d":
			m.deleteMode = true
			m.rebuildItems()
			return m, nil
		case "ctrl+t":
			if m.hasTmux {
				m.mode = toggleMode(m)
				m.delegate.mode = m.mode
				m.rebuildItems()
				return m, needsPreview(m)
			}
			return m, nil
		case "ctrl+r":
			m.startRename()
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
		}
	}

	if !m.deleteMode {
		switch msg.String() {
		case "ctrl+n":
			m.list.CursorDown()
			return m.afterMove()
		case "ctrl+p":
			m.list.CursorUp()
			return m.afterMove()
		}
	}

	return m.passToList(msg)
}

func (m model) passToList(msg tea.Msg) (tea.Model, tea.Cmd) {
	wasFiltering := m.list.SettingFilter()
	oldID, _ := currentSessionID(m)

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	if wasFiltering != m.list.SettingFilter() {
		m.resize()
	}

	newID, hasNew := currentSessionID(m)
	if hasNew && newID != oldID {
		cmd = tea.Batch(cmd, needsPreview(m))
	}

	return m, cmd
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.renameID != "" || m.confirmingDelete() {
		return m, nil
	}

	if !m.inListBody(msg.X, msg.Y) {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.list.CursorUp()
		return m.afterMove()
	case tea.MouseButtonWheelDown:
		m.list.CursorDown()
		return m.afterMove()
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	ix, ok := m.listIndexAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	oldIx := m.list.Index()
	m.list.Select(ix)

	now := time.Now()
	doubleClick := m.lastClickIx == ix && !m.lastClickAt.IsZero() && now.Sub(m.lastClickAt) <= 400*time.Millisecond
	m.lastClickIx = ix
	m.lastClickAt = now

	if doubleClick && !m.deleteMode {
		m.lastClickIx = -1
		m.lastClickAt = time.Time{}
		return m.setAction(m.mode == "tmux" && m.hasTmux)
	}

	if ix != oldIx {
		return m.afterMove()
	}

	return m, nil
}

func (m model) inListBody(x int, y int) bool {
	layout := m.layoutMetrics()
	if x < 1 || x >= layout.listWidth-1 {
		return false
	}
	if y < 2 || y >= 2+m.list.Height() {
		return false
	}
	return true
}

func (m model) listIndexAt(x int, y int) (int, bool) {
	if !m.inListBody(x, y) {
		return 0, false
	}
	row := y - 2
	visible := m.list.VisibleItems()
	start, end := m.list.Paginator.GetSliceBounds(len(visible))
	ix := start + row
	if ix < start || ix >= end {
		return 0, false
	}
	return ix, true
}

func (m *model) rebuildItems() {
	m.delegate.showCheckbox = m.deleteMode
	m.resize()

	var ordered []Session
	if m.mode == "tmux" {
		var run, other []Session
		for _, s := range m.sessions {
			if _, ok := m.running[s.ID]; ok {
				run = append(run, s)
			} else {
				other = append(other, s)
			}
		}
		ordered = append(run, other...)
	} else {
		ordered = m.sessions
	}

	items := make([]list.Item, 0, len(m.sessions))
	for _, s := range ordered {
		_, isRunning := m.running[s.ID]
		_, isSelected := m.selected[s.ID]
		items = append(items, sessionItem{
			session:      s,
			isRunning:    isRunning,
			isSelected:   isSelected,
			showCheckbox: m.deleteMode,
		})
	}
	m.list.SetItems(items)
}

func (m *model) resize() {
	layout := m.layoutMetrics()
	innerListWidth := layout.listWidth - 2
	if innerListWidth < 20 {
		innerListWidth = 20
	}
	m.delegate.updateWidths(innerListWidth)
	m.list.SetSize(innerListWidth, layout.listHeight)
}