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

	case dirpickerRefreshMsg:
		m.dirpicker.allEntries = msg.entries
		m.dirpicker.setListHeight(m.dirpickerHeight())
		m.dirpicker.applyFilter()
		return m, nil

	case previewMsg:
		m.firstMsgs[msg.id] = msg.data
		m.updatePreviewScrollMax()
		return m, nil

	case deleteDoneMsg:
		m.state = stateNormal
		m.selected = make(map[string]struct{})
		sessions, err := getSessions(m.dbPath)
		if err != nil {
			m.sessions = nil
			m.list.SetItems(nil)
			return m, nil
		}
		m.sessions = sessions
		m.states = getSessionStates(sessions, m.mode)
		m.syncGroups()
		cmd := m.rebuildItems()
		return m, cmd

	case spinner.TickMsg:
		if m.state == stateDeleting || m.state == stateForking || m.state == stateClosingTmux {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case forkDoneMsg:
		m.state = stateNormal
		m.pendingForkID = ""
		m.pendingForkTitle = ""
		m.pendingForkDir = ""
		if msg.err != nil {
			return m, nil
		}
		m.actionID = msg.newID
		m.actionDir = msg.dir
		m.actionTitle = msg.title
		m.actionTmux = msg.tmux
		return m, tea.Quit

	case closeTmuxDoneMsg:
		m.state = stateNormal
		m.closeTmuxSessionID = ""
		m.closeTmuxTitle = ""
		m.states = getSessionStates(m.sessions, m.mode)
		cmd := m.rebuildItems()
		return m, cmd

	case tea.FocusMsg:
		m.lastStateRefresh = time.Now()
		return m, tea.Batch(
			func() tea.Msg {
				dark, err := queryTerminalBackground()
				return checkThemeMsg{dark: dark, err: err}
			},
			refreshStatesAsync(m.dbPath, m.mode, m.sessions, true),
		)

	case checkThemeMsg:
		if msg.err == nil && msg.dark != m.isDark {
			m.isDark = msg.dark
			m.theme = themeForDark[m.isDark]
			m.applyTheme()
		}
		return m, nil

	case dbChangedMsg:
		m.lastStateRefresh = time.Now()
		return m, refreshStatesAsync(m.dbPath, m.mode, m.sessions, true)

	case safetyTickMsg:
		m.lastStateRefresh = time.Now()
		return m, tea.Batch(
			refreshStatesAsync(m.dbPath, m.mode, m.sessions, true),
			safetyTick(),
		)

	case stateRefreshMsg:
		if msg.mode != m.mode {
			if msg.fromDB && !sessionsEqual(m.sessions, msg.sessions) {
				m.sessions = msg.sessions
				m.syncGroups()
				m.states = getSessionStates(m.sessions, m.mode)
				cmd := m.rebuildItems()
				return m, cmd
			}
			return m, nil
		}

		changed := false
		if msg.fromDB && !sessionsEqual(m.sessions, msg.sessions) {
			m.sessions = msg.sessions
			m.syncGroups()
			changed = true
		}
		if !statesEqual(m.states, msg.states) {
			m.states = msg.states
			changed = true
		}
		if changed {
			cmd := m.rebuildItems()
			return m, cmd
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		var refreshCmd tea.Cmd
		if time.Since(m.lastStateRefresh) > stateRefreshCooldown {
			m.lastStateRefresh = time.Now()
			refreshCmd = refreshStatesAsync(m.dbPath, m.mode, m.sessions, false)
		}
		mdl, cmd := m.handleKey(msg)
		if refreshCmd != nil {
			cmd = tea.Batch(cmd, refreshCmd)
		}
		return mdl, cmd
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

	if m.pendingSelectRef != nil {
		visible := m.list.VisibleItems()
		if len(visible) > 0 {
			for i, item := range visible {
				if itemMatchesRef(item, *m.pendingSelectRef) {
					m.list.Select(i)
					break
				}
			}
			m.pendingSelectRef = nil
		}
	}

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
	if m.state == stateRenameInput || m.state == stateForkInput || m.state == stateConfirmingDelete || m.state == stateDeleting || m.state == stateFilepicker {
		if m.state == stateFilepicker {
			return m.handleDirpickerMouse(msg)
		}
		return m, nil
	}

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

	if doubleClick && m.state != stateDeleteMode {
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

func (m model) handleDirpickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.dirpicker.setListHeight(m.dirpickerHeight())
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		cmd := m.dirpicker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		return m, cmd
	case tea.MouseButtonWheelDown:
		cmd := m.dirpicker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		return m, cmd
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
	m.delegate.showCheckbox = m.state == stateDeleteMode
	m.delegate.grouped = m.grouped
	m.resize()

	if len(m.groups) == 0 {
		m.syncGroups()
	}

	ordered := orderedSessions(m.sessions, m.states, m.mode, m.grouped)
	items := buildListItems(ordered, m.groups, m.states, m.selected, m.grouped, m.filterActive(), m.matchingGroupPaths(), m.mode)
	for i := range items {
		if sessItem, ok := items[i].(sessionItem); ok {
			sessItem.showCheckbox = m.state == stateDeleteMode
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
		if m.filterActive() {
			// SetItems clears filteredItems and re-filters async. The cursor
			// set here would be based on the full list, but VisibleItems will
			// be a smaller filtered subset once FilterMatchesMsg arrives,
			// causing a slice-bounds panic. Use 0 as a safe fallback and
			// defer the real selection until the filter results come back.
			m.list.Select(0)
			m.pendingSelectRef = &ref
		} else {
			m.list.Select(targetIndex)
		}
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

	current := m.list.Index()
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
