package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	list                  list.Model
	delegate              *sessionDelegate
	sessions              []Session
	groups                []groupInfo
	states                map[string]sessionState
	firstMsgs             map[string]previewData
	selected              map[string]struct{}
	showPreview           bool
	grouped               bool
	mode                  string
	hasTmux               bool
	isDark                bool
	agentPath             string
	dbPath                string
	width                 int
	height                int
	actionID              string
	actionDir             string
	actionTitle           string
	actionTmux            bool
	actionNewSession      bool
	theme                 theme
	renameID              string
	renameInput           textinput.Model
	forkSessionID         string
	pendingNewSessionDir  string
	pendingNewSessionTmux bool
	pendingForkID         string
	pendingForkTitle      string
	pendingForkDir        string
	closeTmuxSessionID    string
	closeTmuxTitle        string
	lastClickAt           time.Time
	lastClickIx           int
	state                 appState
	spinner               spinner.Model
	previewScroll         int
	previewScrollMax      int
	pendingSelectRef      *itemRef
	dirpicker             dirpicker
	keybindsScroll        int
}

type deleteDoneMsg struct{}

func doDeleteCmd(agentPath string, ids []string) tea.Cmd {
	return func() tea.Msg {
		for _, id := range ids {
			deleteSession(agentPath, id)
		}
		return deleteDoneMsg{}
	}
}

type forkDoneMsg struct {
	newID string
	dir   string
	title string
	tmux  bool
	err   error
}

func doForkCmd(dbPath, sessionID, title, dir string, useTmux bool) tea.Cmd {
	return func() tea.Msg {
		newID, err := forkSession(dbPath, sessionID, title)
		return forkDoneMsg{
			newID: newID,
			dir:   dir,
			title: title,
			tmux:  useTmux,
			err:   err,
		}
	}
}

type closeTmuxDoneMsg struct {
	id  string
	err error
}

func doCloseTmuxCmd(dbPath, sessionID, title string) tea.Cmd {
	return func() tea.Msg {
		tmuxPath, err := exec.LookPath("tmux")
		if err != nil {
			return closeTmuxDoneMsg{id: sessionID, err: err}
		}
		sessName, winIdx, found := findTmuxWindow(tmuxPath, sessionID)
		if !found {
			return closeTmuxDoneMsg{id: sessionID, err: fmt.Errorf("tmux window not found")}
		}
		err = killTmuxWindow(tmuxPath, sessName, winIdx)
		return closeTmuxDoneMsg{id: sessionID, err: err}
	}
}

type previewMsg struct {
	id   string
	data previewData
}

func fetchPreview(dbPath, sessionID string) tea.Cmd {
	return func() tea.Msg {
		return previewMsg{id: sessionID, data: getPreviewData(dbPath, sessionID)}
	}
}

func currentSessionID(m model) (string, bool) {
	if item := m.list.SelectedItem(); item != nil {
		if sess, ok := sessionFromItem(item); ok {
			return sess.ID, true
		}
	}
	return "", false
}

func needsPreview(m model) tea.Cmd {
	if !m.showPreview {
		return nil
	}
	id, ok := currentSessionID(m)
	if !ok {
		return nil
	}
	if _, cached := m.firstMsgs[id]; cached {
		return nil
	}
	return fetchPreview(m.dbPath, id)
}

func sessionByID(sessions []Session, id string) (Session, bool) {
	for _, s := range sessions {
		if s.ID == id {
			return s, true
		}
	}
	return Session{}, false
}

func dirFromItem(item any) (string, bool) {
	switch v := item.(type) {
	case sessionItem:
		return v.session.Directory, true
	case groupHeaderItem:
		return v.path, true
	default:
		return "", false
	}
}

func newModel(startTmux bool, noPreview bool, grouped bool, themeOverride string) (*model, error) {
	agentPath, err := exec.LookPath("opencode")
	if err != nil {
		return nil, fmt.Errorf("opencode not found in PATH")
	}

	_, tmuxErr := exec.LookPath("tmux")
	hasTmux := tmuxErr == nil

	dbPath := getDBPath()
	sessions, err := getSessions(dbPath)
	if err != nil {
		return nil, fmt.Errorf("fetching sessions: %w", err)
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	states := getSessionStates(sessions)

	isDark := detectDarkMode()
	if themeOverride == "dark" {
		isDark = true
	} else if themeOverride == "light" {
		isDark = false
	}
	theme := themeForDark[isDark]

	delegate := newSessionDelegate(theme)
	initialMode := "all"
	if startTmux && hasTmux {
		initialMode = "tmux"
	}
	delegate.mode = initialMode
	delegate.grouped = grouped

	ordered := orderedSessions(sessions, states, initialMode, grouped)
	groups := buildGroups(sessions, nil)
	items := buildListItems(ordered, groups, states, nil, grouped, false, nil, initialMode)

	l := list.New(items, delegate, 80, 20)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(false)
	l.Filter = list.UnsortedFilter
	l.FilterInput.Prompt = "> "
	promptStyle, cursorStyle := theme.filterStyles()
	l.FilterInput.PromptStyle = promptStyle
	l.FilterInput.Cursor.Style = cursorStyle
	l.Styles.FilterPrompt = promptStyle
	l.Styles.FilterCursor = cursorStyle

	ti := textinput.New()
	ti.Placeholder = "New name"
	ti.CharLimit = 100
	ti.Width = 40
	ti.PromptStyle = lipgloss.NewStyle().Foreground(theme.filterPrompt)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(theme.filterPrompt)
	ti.TextStyle = lipgloss.NewStyle().Foreground(theme.modalPromptFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.textMuted)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.filterPrompt)

	dp := newDirpicker("")

	return &model{
		list:        l,
		delegate:    delegate,
		sessions:    sessions,
		groups:      groups,
		states:      states,
		firstMsgs:   make(map[string]previewData),
		selected:    make(map[string]struct{}),
		grouped:     grouped,
		mode:        initialMode,
		hasTmux:     hasTmux,
		agentPath:   agentPath,
		dbPath:      dbPath,
		showPreview: !noPreview,
		theme:       theme,
		isDark:      isDark,
		renameInput: ti,
		lastClickIx: -1,
		spinner:     s,
		dirpicker:   dp,
		state:       stateNormal,
	}, nil
}

func (m model) Init() tea.Cmd {
	return needsPreview(m)
}

func (m *model) applyTheme() {
	t := m.theme
	m.delegate.theme = t
	m.delegate.mode = m.mode
	m.delegate.grouped = m.grouped
	promptStyle, cursorStyle := t.filterStyles()
	m.list.FilterInput.PromptStyle = promptStyle
	m.list.FilterInput.Cursor.Style = cursorStyle
	m.list.Styles.FilterPrompt = promptStyle
	m.list.Styles.FilterCursor = cursorStyle
	m.renameInput.PromptStyle = lipgloss.NewStyle().Foreground(t.filterPrompt)
	m.renameInput.Cursor.Style = lipgloss.NewStyle().Foreground(t.filterPrompt)
	m.renameInput.TextStyle = lipgloss.NewStyle().Foreground(t.modalPromptFg)
	m.renameInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(t.textMuted)
	m.spinner.Style = lipgloss.NewStyle().Foreground(t.filterPrompt)
}

func (m model) confirmingDelete() bool {
	return m.state == stateConfirmingDelete
}

func (m *model) selectedSession() (Session, bool) {
	item := m.list.SelectedItem()
	if item == nil {
		return Session{}, false
	}
	return sessionFromItem(item)
}

func (m model) setAction(useTmux bool) (tea.Model, tea.Cmd) {
	sess, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	m.actionID = sess.ID
	m.actionDir = sess.Directory
	m.actionTitle = sess.Title
	m.actionTmux = useTmux
	return m, tea.Quit
}

func (m *model) startRename() {
	sess, ok := m.selectedSession()
	if !ok {
		return
	}
	m.renameID = sess.ID
	m.renameInput.SetValue("")
	m.renameInput.Focus()
	m.state = stateRenameInput
}

func (m *model) startFork() {
	sess, ok := m.selectedSession()
	if !ok {
		return
	}
	m.forkSessionID = sess.ID
	m.renameInput.SetValue("")
	m.renameInput.Focus()
	m.state = stateForkInput
}

func (m *model) finishRename() tea.Cmd {
	if m.renameID == "" || m.state != stateRenameInput {
		return nil
	}
	newTitle := strings.TrimSpace(m.renameInput.Value())
	var cmd tea.Cmd
	if newTitle != "" {
		if err := renameSession(m.dbPath, m.renameID, newTitle); err != nil {
			m.cancelRename()
			return nil
		}
		sessions, err := getSessions(m.dbPath)
		if err == nil {
			m.sessions = sessions
			m.states = getSessionStates(m.sessions)
			m.syncGroups()
			cmd = m.rebuildItems()
		}
	}
	m.cancelRename()
	return cmd
}

func (m *model) finishFork() tea.Cmd {
	if m.forkSessionID == "" || m.state != stateForkInput {
		return nil
	}
	title := strings.TrimSpace(m.renameInput.Value())
	if title == "" {
		m.cancelRename()
		return nil
	}
	sess, ok := sessionByID(m.sessions, m.forkSessionID)
	if !ok {
		m.cancelRename()
		return nil
	}
	m.state = stateForking
	useTmux := m.mode == "tmux" && m.hasTmux
	return tea.Batch(
		m.spinner.Tick,
		doForkCmd(m.dbPath, m.forkSessionID, title, sess.Directory, useTmux),
	)
}

func (m *model) cancelRename() {
	m.renameID = ""
	m.forkSessionID = ""
	m.pendingNewSessionDir = ""
	m.pendingNewSessionTmux = false
	m.pendingForkID = ""
	m.pendingForkTitle = ""
	m.pendingForkDir = ""
	m.closeTmuxSessionID = ""
	m.closeTmuxTitle = ""
	m.keybindsScroll = 0
	m.renameInput.Blur()
	m.renameInput.SetValue("")
	m.state = stateNormal
}

func (m *model) updatePreviewScrollMax() {
	layout := m.layoutMetrics()
	if !layout.showPreview {
		m.previewScrollMax = 0
		return
	}
	innerH := layout.previewH - 2
	if innerH < 1 {
		m.previewScrollMax = 0
		return
	}
	item := m.list.SelectedItem()
	if item == nil {
		m.previewScrollMax = 0
		return
	}
	sess, ok := sessionFromItem(item)
	if !ok {
		m.previewScrollMax = 0
		return
	}
	cached, ok := m.firstMsgs[sess.ID]
	if !ok {
		m.previewScrollMax = 0
		return
	}
	contentW := (layout.previewW - 2) - 5
	if contentW < 6 {
		contentW = 6
	}
	total := 4 + len(m.buildPreviewLines(cached, contentW))
	max := total - innerH
	if max < 0 {
		max = 0
	}
	m.previewScrollMax = max
}

func (m *model) afterMove() (tea.Model, tea.Cmd) {
	m.previewScroll = 0
	m.previewScrollMax = 0
	m.updatePreviewScrollMax() // instant if data already cached
	return m, needsPreview(*m)
}

func toggleMode(m model) string {
	if m.mode == "all" {
		return "tmux"
	}
	return "all"
}

func orderedSessions(sessions []Session, states map[string]sessionState, mode string, grouped bool) []Session {
	if grouped || mode != "tmux" {
		return sessions
	}

	var run, other []Session
	for _, s := range sessions {
		if states[s.ID] > stateNone {
			run = append(run, s)
		} else {
			other = append(other, s)
		}
	}
	return append(run, other...)
}

func (m *model) syncGroups() {
	collapsedByPath := make(map[string]bool, len(m.groups))
	for _, g := range m.groups {
		collapsedByPath[g.path] = g.collapsed
	}
	m.groups = buildGroups(m.sessions, collapsedByPath)
}

func (m model) filterActive() bool {
	return m.list.FilterState() != list.Unfiltered && strings.TrimSpace(m.list.FilterValue()) != ""
}

func (m model) matchingGroupPaths() map[string]struct{} {
	if !m.filterActive() {
		return nil
	}

	term := m.list.FilterValue()
	targets := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		targets[i] = s.Title + " " + s.Directory
	}

	matches := make(map[string]struct{})
	for _, rank := range m.list.Filter(term, targets) {
		if rank.Index >= 0 && rank.Index < len(m.sessions) {
			matches[m.sessions[rank.Index].Directory] = struct{}{}
		}
	}
	return matches
}

func buildListItems(ordered []Session, groups []groupInfo, states map[string]sessionState, selected map[string]struct{}, grouped bool, filterActive bool, matchingGroups map[string]struct{}, mode string) []list.Item {
	if !grouped {
		items := make([]list.Item, 0, len(ordered))
		for _, s := range ordered {
			_, isSelected := selected[s.ID]
			items = append(items, sessionItem{
				session:    s,
				state:      states[s.ID],
				isSelected: isSelected,
			})
		}
		return items
	}

	sessionByID := make(map[string]Session, len(ordered))
	for _, s := range ordered {
		sessionByID[s.ID] = s
	}

	items := make([]list.Item, 0, len(ordered)+len(groups))
	for i, g := range groups {
		expanded := !g.collapsed
		if filterActive {
			_, expanded = matchingGroups[g.path]
		}
		items = append(items, groupHeaderItem{
			path:        g.path,
			worktree:    g.worktree,
			count:       len(g.sessionIDs),
			collapsed:   !expanded,
			filterValue: g.filterValue,
		})
		if !expanded {
			continue
		}
		ids := g.sessionIDs
		if mode == "tmux" {
			var runIDs, otherIDs []string
			for _, id := range g.sessionIDs {
				if states[id] > stateNone {
					runIDs = append(runIDs, id)
				} else {
					otherIDs = append(otherIDs, id)
				}
			}
			ids = append(runIDs, otherIDs...)
		}

		for _, id := range ids {
			s := sessionByID[id]
			_, isSelected := selected[s.ID]
			items = append(items, sessionItem{
				session:    s,
				state:      states[s.ID],
				isSelected: isSelected,
				groupPath:  g.path,
			})
		}
		if i < len(groups)-1 {
			items = append(items, groupSeparatorItem{})
		}
	}
	return items
}
