package main

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.updatePreviewScrollMax()
		return m, nil

	case previewMsg:
		m.firstMsgs[msg.id] = msg.data
		m.updatePreviewScrollMax()
		return m, nil

	case deleteDoneMsg:
		m.deleting = false
		m.confirming = false
		m.deleteMode = false
		m.selected = make(map[string]struct{})
		sessions, err := getSessions(m.dbPath)
		if err != nil {
			m.sessions = nil
			m.list.SetItems(nil)
			return m, nil
		}
		m.sessions = sessions
		m.states = getSessionStates(sessions)
		m.syncGroups()
		cmd := m.rebuildItems()
		return m, cmd

	case spinner.TickMsg:
		if m.deleting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
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
			cmd := m.finishRename()
			return m, cmd
		}
		var cmd tea.Cmd
		m.renameInput, cmd = m.renameInput.Update(msg)
		return m, cmd
	}

	if m.deleting {
		return m, nil
	}

	if m.confirmingDelete() {
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
			if m.grouped {
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

func (m model) passToList(msg tea.Msg) (tea.Model, tea.Cmd) {
	wasFiltering := m.list.SettingFilter()
	oldFilterState := m.list.FilterState()
	oldFilterValue := m.list.FilterValue()
	oldID, _ := currentSessionID(m)

	var cmd tea.Cmd
	oldIndex := m.list.Index()
	m.list, cmd = m.list.Update(msg)
	m.skipSeparatorSelection(oldIndex)

	if wasFiltering != m.list.SettingFilter() {
		m.resize()
	}
	if m.grouped && (oldFilterState != m.list.FilterState() || oldFilterValue != m.list.FilterValue()) {
		cmd = tea.Batch(cmd, m.rebuildItems())
	}

	newID, hasNew := currentSessionID(m)
	if hasNew && newID != oldID {
		m.previewScroll = 0
		m.previewScrollMax = 0
		m.updatePreviewScrollMax()
		cmd = tea.Batch(cmd, needsPreview(m))
	}

	return m, cmd
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.renameID != "" || m.confirmingDelete() || m.deleting {
		return m, nil
	}

	// Wheel over preview pane scrolls preview content
	if m.inPreviewBody(msg.X, msg.Y) {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.previewScroll > 0 {
				m.previewScroll--
			}
			return m, nil
		case tea.MouseButtonWheelDown:
			if m.previewScroll < m.previewScrollMax {
				m.previewScroll++
			}
			return m, nil
		}
		return m, nil
	}

	if !m.inListBody(msg.X, msg.Y) {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		oldIndex := m.list.Index()
		m.list.CursorUp()
		m.skipSeparatorSelection(oldIndex)
		return m.afterMove()
	case tea.MouseButtonWheelDown:
		oldIndex := m.list.Index()
		m.list.CursorDown()
		m.skipSeparatorSelection(oldIndex)
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
	if item := m.list.SelectedItem(); item != nil {
		if header, ok := item.(groupHeaderItem); ok && m.grouped {
			return m.toggleGroupByPath(header.path, header.collapsed)
		}
	}

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
		m.skipSeparatorSelection(oldIx)
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

func (m *model) rebuildItems() tea.Cmd {
	ref := itemRefFromItem(m.list.SelectedItem())
	return m.rebuildItemsFor(ref)
}

func (m *model) rebuildItemsFor(ref itemRef) tea.Cmd {
	m.delegate.showCheckbox = m.deleteMode
	m.delegate.grouped = m.grouped
	m.resize()

	if len(m.groups) == 0 {
		m.syncGroups()
	}

	ordered := orderedSessions(m.sessions, m.states, m.mode, m.grouped)
	items := buildListItems(ordered, m.groups, m.states, m.selected, m.grouped, m.filterActive(), m.matchingGroupPaths())
	for i := range items {
		if sessItem, ok := items[i].(sessionItem); ok {
			sessItem.showCheckbox = m.deleteMode
			items[i] = sessItem
		}
	}

	targetIndex := 0
	fallback := itemRef{groupPath: ref.groupPath, header: ref.groupPath != ""}
	for i, item := range items {
		if itemMatchesRef(item, ref) {
			targetIndex = i
			fallback = itemRef{}
			break
		}
	}
	if fallback.header {
		for i, item := range items {
			if itemMatchesRef(item, fallback) {
				targetIndex = i
				break
			}
		}
	}

	cmd := m.list.SetItems(items)
	if len(items) > 0 {
		m.list.Select(targetIndex)
	}
	return cmd
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

func (m model) groupCollapsed(path string) (bool, bool) {
	for _, g := range m.groups {
		if g.path == path {
			return g.collapsed, true
		}
	}
	return false, false
}

func (m *model) setGroupCollapsed(path string, collapsed bool) tea.Cmd {
	if path == "" {
		return nil
	}
	changed := false
	for i := range m.groups {
		if m.groups[i].path != path {
			continue
		}
		if m.groups[i].collapsed == collapsed {
			return nil
		}
		m.groups[i].collapsed = collapsed
		changed = true
		break
	}
	if !changed {
		return nil
	}
	return m.rebuildItemsFor(itemRef{groupPath: path, header: true})
}

func (m *model) setCurrentGroupCollapsed(collapsed bool) tea.Cmd {
	path, ok := groupPathFromItem(m.list.SelectedItem())
	if !ok {
		return nil
	}
	return m.setGroupCollapsed(path, collapsed)
}

func (m *model) handleGroupSpace() tea.Cmd {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	path, ok := groupPathFromItem(item)
	if !ok {
		return nil
	}
	if header, ok := item.(groupHeaderItem); ok && header.collapsed {
		return m.setGroupCollapsed(path, false)
	}
	return m.setGroupCollapsed(path, true)
}

func (m *model) toggleAllGroups() tea.Cmd {
	if len(m.groups) == 0 {
		return nil
	}
	expandAll := false
	for _, g := range m.groups {
		if g.collapsed {
			expandAll = true
			break
		}
	}

	ref := itemRefFromItem(m.list.SelectedItem())
	if !expandAll {
		if path, ok := groupPathFromItem(m.list.SelectedItem()); ok {
			ref = itemRef{groupPath: path, header: true}
		}
	}

	for i := range m.groups {
		m.groups[i].collapsed = !expandAll
	}
	return m.rebuildItemsFor(ref)
}

func (m *model) jumpGroup(delta int) (tea.Model, tea.Cmd) {
	if !m.grouped || delta == 0 {
		return m, nil
	}
	items := m.list.VisibleItems()
	if len(items) == 0 {
		return m, nil
	}

	headerIndices := make([]int, 0)
	for i, item := range items {
		if _, ok := item.(groupHeaderItem); ok {
			headerIndices = append(headerIndices, i)
		}
	}
	if len(headerIndices) == 0 {
		return m, nil
	}

	current := m.list.GlobalIndex()
	currentHeader := -1
	for i := current; i >= 0; i-- {
		if _, ok := items[i].(groupHeaderItem); ok {
			currentHeader = i
			break
		}
	}
	if currentHeader < 0 {
		currentHeader = headerIndices[0]
	}

	target := headerIndices[0]
	for idx, headerIndex := range headerIndices {
		if headerIndex == currentHeader {
			if delta > 0 {
				target = headerIndices[(idx+1)%len(headerIndices)]
			} else {
				target = headerIndices[(idx-1+len(headerIndices))%len(headerIndices)]
			}
			break
		}
	}

	m.list.Select(target)
	return m.afterMove()
}

func (m *model) toggleGroupByPath(path string, collapsed bool) (tea.Model, tea.Cmd) {
	cmd := m.setGroupCollapsed(path, !collapsed)
	return m, tea.Batch(cmd, needsPreview(*m))
}

func (m *model) skipSeparatorSelection(prevIndex int) {
	for i := 0; i < 3; i++ {
		item := m.list.SelectedItem()
		if item == nil {
			return
		}
		if _, ok := item.(groupSeparatorItem); !ok {
			return
		}
		if m.list.Index() > prevIndex {
			m.list.CursorDown()
		} else if m.list.Index() > 0 {
			m.list.CursorUp()
		} else {
			return
		}
	}
}
