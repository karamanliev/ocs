package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	list        list.Model
	delegate    *sessionDelegate
	sessions    []Session
	running     map[string]struct{}
	firstMsgs   map[string]previewData
	selected    map[string]struct{}
	deleteMode  bool
	confirming  bool
	showPreview bool
	mode        string
	hasTmux     bool
	isDark      bool
	agentPath   string
	dbPath      string
	width       int
	height      int
	actionID    string
	actionDir   string
	actionTmux  bool
	theme       theme
	renameID    string
	renameInput textinput.Model
	lastClickAt time.Time
	lastClickIx int
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
		return item.(sessionItem).session.ID, true
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

func newModel(startTmux bool, noPreview bool) (*model, error) {
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

	running := getRunningSessionIDs()

	isDark := detectDarkMode()
	theme := themeForDark[isDark]

	delegate := newSessionDelegate(theme)
	initialMode := "all"
	if startTmux && hasTmux {
		initialMode = "tmux"
	}
	delegate.mode = initialMode

	var ordered []Session
	if initialMode == "tmux" {
		var run, other []Session
		for _, s := range sessions {
			if _, ok := running[s.ID]; ok {
				run = append(run, s)
			} else {
				other = append(other, s)
			}
		}
		ordered = append(run, other...)
	} else {
		ordered = sessions
	}

	items := make([]list.Item, 0, len(sessions))
	for _, s := range ordered {
		_, isRunning := running[s.ID]
		items = append(items, sessionItem{session: s, isRunning: isRunning})
	}

	l := list.New(items, delegate, 80, 20)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(false)
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

	return &model{
		list:        l,
		delegate:    delegate,
		sessions:    sessions,
		running:     running,
		firstMsgs:   make(map[string]previewData),
		selected:    make(map[string]struct{}),
		mode:        initialMode,
		hasTmux:     hasTmux,
		agentPath:   agentPath,
		dbPath:      dbPath,
		showPreview: !noPreview,
		theme:       theme,
		isDark:      isDark,
		renameInput: ti,
		lastClickIx: -1,
	}, nil
}

func (m model) Init() tea.Cmd {
	return needsPreview(m)
}

func (m *model) applyTheme() {
	t := m.theme
	m.delegate.theme = t
	m.delegate.mode = m.mode
	promptStyle, cursorStyle := t.filterStyles()
	m.list.FilterInput.PromptStyle = promptStyle
	m.list.FilterInput.Cursor.Style = cursorStyle
	m.list.Styles.FilterPrompt = promptStyle
	m.list.Styles.FilterCursor = cursorStyle
	m.renameInput.PromptStyle = lipgloss.NewStyle().Foreground(t.filterPrompt)
	m.renameInput.Cursor.Style = lipgloss.NewStyle().Foreground(t.filterPrompt)
	m.renameInput.TextStyle = lipgloss.NewStyle().Foreground(t.modalPromptFg)
	m.renameInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(t.textMuted)
}

func (m model) confirmingDelete() bool {
	return m.confirming && m.deleteMode
}

func (m model) setAction(useTmux bool) (tea.Model, tea.Cmd) {
	item := m.list.SelectedItem()
	if item == nil {
		return m, nil
	}
	sess := item.(sessionItem).session
	m.actionID = sess.ID
	m.actionDir = sess.Directory
	m.actionTmux = useTmux
	return m, tea.Quit
}

func (m *model) startRename() {
	item := m.list.SelectedItem()
	if item == nil {
		return
	}
	sess := item.(sessionItem).session
	m.renameID = sess.ID
	m.renameInput.SetValue(sess.Title)
	m.renameInput.Focus()
}

func (m *model) finishRename() {
	if m.renameID == "" {
		return
	}
	newTitle := strings.TrimSpace(m.renameInput.Value())
	if newTitle != "" {
		if err := renameSession(m.dbPath, m.renameID, newTitle); err != nil {
			m.renameID = ""
			m.renameInput.Blur()
			m.renameInput.SetValue("")
			return
		}
		sessions, err := getSessions(m.dbPath)
		if err == nil {
			m.sessions = sessions
			m.running = getRunningSessionIDs()
			m.rebuildItems()
		}
	}
	m.renameID = ""
	m.renameInput.Blur()
	m.renameInput.SetValue("")
}

func (m *model) cancelRename() {
	m.renameID = ""
	m.renameInput.Blur()
	m.renameInput.SetValue("")
}

func (m model) executeDelete() (tea.Model, tea.Cmd) {
	m.confirming = false
	m.deleteMode = false

	var ids []string
	for id := range m.selected {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		if item := m.list.SelectedItem(); item != nil {
			ids = append(ids, item.(sessionItem).session.ID)
		}
	}

	for _, id := range ids {
		deleteSession(m.agentPath, id)
	}

	m.selected = make(map[string]struct{})

	sessions, err := getSessions(m.dbPath)
	if err != nil {
		m.sessions = nil
		m.list.SetItems(nil)
		return m, nil
	}
	m.sessions = sessions
	m.running = getRunningSessionIDs()
	m.rebuildItems()

	return m, nil
}

func (m *model) afterMove() (tea.Model, tea.Cmd) {
	return m, needsPreview(*m)
}

func toggleMode(m model) string {
	if m.mode == "all" {
		return "tmux"
	}
	return "all"
}