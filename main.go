package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/termenv"
	_ "modernc.org/sqlite"
)

// ─── Data Layer ─────────────────────────────────────────────────────────────

type Session struct {
	ID        string
	Title     string
	Updated   int64
	Directory string
}

func getDBPath() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode", "opencode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

func getSessions(dbPath string) ([]Session, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT s.id, s.title, s.time_updated, p.worktree
		FROM session s
		JOIN project p ON p.id = s.project_id
		WHERE s.parent_id IS NULL
		ORDER BY s.time_updated DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.Updated, &s.Directory); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func renameSession(dbPath, id, newTitle string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("UPDATE session SET title = ? WHERE id = ?", newTitle, id)
	return err
}

func getRunningSessionIDs() map[string]struct{} {
	result := make(map[string]struct{})
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return result
	}
	out, err := exec.Command(tmuxPath, "list-panes", "-a", "-F", "#{pane_pid}").Output()
	if err != nil {
		return result
	}
	for _, pid := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", pid))
		if err != nil {
			continue
		}
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		if !strings.Contains(cmdline, "opencode") {
			continue
		}
		fields := strings.Fields(cmdline)
		for i, f := range fields {
			if f == "-s" && i+1 < len(fields) {
				result[fields[i+1]] = struct{}{}
				break
			}
		}
	}
	return result
}

func getFirstMessage(dbPath, sessionID string) string {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()

	var partData string
	// modernc.org/sqlite includes JSON1 extension
	err = db.QueryRow(`
		SELECT p.data FROM part p
		JOIN message m ON m.id = p.message_id
		WHERE m.session_id = ? AND json_extract(m.data, '$.role') = 'user'
		ORDER BY m.time_created ASC, p.time_created ASC
		LIMIT 1
	`, sessionID).Scan(&partData)
	if err != nil {
		return ""
	}
	var part struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(partData), &part); err != nil {
		return ""
	}
	if part.Type == "text" {
		return part.Text
	}
	return ""
}

// ─── Theme ──────────────────────────────────────────────────────────────────

type theme struct {
	timeFresh        lipgloss.Color
	timeHour         lipgloss.Color
	timeDay          lipgloss.Color
	timeOld          lipgloss.Color
	indicator        lipgloss.Color
	dim              lipgloss.Color
	cursorBgAll      lipgloss.Color
	cursorBgTmux     lipgloss.Color
	border           lipgloss.Color
	colHeaderFg      lipgloss.Color
	modalBorder      lipgloss.Color
	modalBg          lipgloss.Color
	modalPromptFg    lipgloss.Color
	modalHintFg      lipgloss.Color
	previewBorder    lipgloss.Color
	previewBg        lipgloss.Color
	previewTitleFg   lipgloss.Color
	previewContentFg lipgloss.Color
	filterPrompt     lipgloss.Color
	accent           lipgloss.Color
	textMain         lipgloss.Color
	textMuted        lipgloss.Color
	footerKeyAll     lipgloss.Color
	footerKeyTmux    lipgloss.Color
	footerLabel      lipgloss.Color
	titleAllFg       lipgloss.Color
	titleTmuxFg      lipgloss.Color
}

var darkTheme = theme{
	timeFresh:        lipgloss.Color("#FF6B6B"),
	timeHour:         lipgloss.Color("#F9A825"),
	timeDay:          lipgloss.Color("#4ECDC4"),
	timeOld:          lipgloss.Color("#B983FF"),
	indicator:        lipgloss.Color("#69F0AE"),
	dim:              lipgloss.Color("#6B7280"),
	cursorBgAll:      lipgloss.Color("#2A3A4A"),
	cursorBgTmux:     lipgloss.Color("#3A2A4A"),
	border:           lipgloss.Color("#4B5563"),
	colHeaderFg:      lipgloss.Color("#9CA3AF"),
	modalBorder:      lipgloss.Color("#FF6B6B"),
	modalBg:          lipgloss.Color("#1A1A2E"),
	modalPromptFg:    lipgloss.Color("#FFFFFF"),
	modalHintFg:      lipgloss.Color("#9CA3AF"),
	previewBorder:    lipgloss.Color("#4ECDC4"),
	previewBg:        lipgloss.Color("#1A1A2E"),
	previewTitleFg:   lipgloss.Color("#FFFFFF"),
	previewContentFg: lipgloss.Color("#D1D5DB"),
	filterPrompt:     lipgloss.Color("#F9A825"),
	accent:           lipgloss.Color("#60A5FA"),
	textMain:         lipgloss.Color("#E5E7EB"),
	textMuted:        lipgloss.Color("#4B5563"),
	footerKeyAll:     lipgloss.Color("#6B8DB5"),
	footerKeyTmux:    lipgloss.Color("#8B7DB0"),
	footerLabel:      lipgloss.Color("#6B7280"),
	titleAllFg:       lipgloss.Color("#6B8DB5"),
	titleTmuxFg:      lipgloss.Color("#8B7DB0"),
}

var lightTheme = theme{
	timeFresh:        lipgloss.Color("#DC2626"),
	timeHour:         lipgloss.Color("#D97706"),
	timeDay:          lipgloss.Color("#0891B2"),
	timeOld:          lipgloss.Color("#7C3AED"),
	indicator:        lipgloss.Color("#16A34A"),
	dim:              lipgloss.Color("#9CA3AF"),
	cursorBgAll:      lipgloss.Color("#D0E0F0"),
	cursorBgTmux:     lipgloss.Color("#E0D0F0"),
	border:           lipgloss.Color("#D1D5DB"),
	colHeaderFg:      lipgloss.Color("#6B7280"),
	modalBorder:      lipgloss.Color("#DC2626"),
	modalBg:          lipgloss.Color("#F9FAFB"),
	modalPromptFg:    lipgloss.Color("#111827"),
	modalHintFg:      lipgloss.Color("#6B7280"),
	previewBorder:    lipgloss.Color("#0891B2"),
	previewBg:        lipgloss.Color("#F9FAFB"),
	previewTitleFg:   lipgloss.Color("#111827"),
	previewContentFg: lipgloss.Color("#374151"),
	filterPrompt:     lipgloss.Color("#D97706"),
	accent:           lipgloss.Color("#2563EB"),
	textMain:         lipgloss.Color("#1F2937"),
	textMuted:        lipgloss.Color("#D1D5DB"),
	footerKeyAll:     lipgloss.Color("#5B7D9F"),
	footerKeyTmux:    lipgloss.Color("#7B6D9F"),
	footerLabel:      lipgloss.Color("#9CA3AF"),
	titleAllFg:       lipgloss.Color("#5B7D9F"),
	titleTmuxFg:      lipgloss.Color("#7B6D9F"),
}

func detectDarkMode() bool {
	out := termenv.NewOutput(os.Stderr)
	return out.HasDarkBackground()
}

func (t theme) colorForDuration(d time.Duration) lipgloss.Color {
	switch {
	case d < time.Minute:
		return t.timeFresh
	case d < time.Hour:
		return t.timeHour
	case d < 24*time.Hour:
		return t.timeDay
	default:
		return t.timeOld
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ─── List Item & Delegate ───────────────────────────────────────────────────

type sessionItem struct {
	session      Session
	isRunning    bool
	isSelected   bool
	showCheckbox bool
}

func (i sessionItem) FilterValue() string {
	return i.session.Title + " " + i.session.Directory
}

type sessionDelegate struct {
	width, timeW, checkboxW, indicatorW, titleW, dirW int
	showCheckbox                                      bool
	mode                                              string
	theme                                             theme
}

func newSessionDelegate(t theme) *sessionDelegate {
	return &sessionDelegate{
		timeW:      10,
		checkboxW:  3,
		indicatorW: 1,
		mode:       "all",
		theme:      t,
	}
}

func (d *sessionDelegate) updateWidths(totalWidth int) {
	d.width = totalWidth
	if d.width < 30 {
		d.width = 30
	}
	// cursorPrefix(2) + time + sep(1) + [checkbox + sep(1)] + indicator + sep(1) + title + sep(1) + dir
	padding := 5
	if d.showCheckbox {
		padding = 6
	}
	content := d.width - padding
	d.timeW = 10
	d.indicatorW = 1
	if d.showCheckbox {
		d.checkboxW = 3
	} else {
		d.checkboxW = 0
	}
	remain := content - d.timeW - d.indicatorW - d.checkboxW
	if remain < 20 {
		remain = 20
	}
	d.titleW = remain * 50 / 100
	d.dirW = remain - d.titleW
}

func (d *sessionDelegate) Height() int                         { return 1 }
func (d *sessionDelegate) Spacing() int                        { return 0 }
func (d *sessionDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d *sessionDelegate) ShortHelp() []key.Binding            { return nil }
func (d *sessionDelegate) FullHelp() [][]key.Binding           { return nil }

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func (d *sessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(sessionItem)
	if !ok {
		return
	}
	isCursor := index == m.Index()

	updated := time.Unix(i.session.Updated/1000, (i.session.Updated%1000)*1e6)
	dura := time.Since(updated)

	timeText := formatDuration(dura)
	checkboxText := ""
	if i.showCheckbox {
		checkboxText = "[ ]"
		if i.isSelected {
			checkboxText = "[x]"
		}
	}
	indicatorText := " "
	if i.isRunning {
		indicatorText = "●"
	}
	titleText := truncate.StringWithTail(i.session.Title, uint(d.titleW), "…")
	dirText := truncate.StringWithTail(i.session.Directory, uint(d.dirW), "…")

	prefix := "  "

	if isCursor {
		// Build plain line, pad to full width, then apply background
		var parts []string
		parts = append(parts, prefix)
		parts = append(parts, padRight(timeText, d.timeW)+" ")
		if d.showCheckbox {
			parts = append(parts, padRight(checkboxText, d.checkboxW)+" ")
		}
		parts = append(parts, padRight(indicatorText, d.indicatorW)+" ")
		parts = append(parts, padRight(titleText, d.titleW)+" ")
		parts = append(parts, padRight(dirText, d.dirW))

		line := strings.Join(parts, "")
		vis := lipgloss.Width(line)
		if vis < d.width {
			line += strings.Repeat(" ", d.width-vis)
		}
		cursorBg := d.theme.cursorBgAll
		if d.mode == "tmux" {
			cursorBg = d.theme.cursorBgTmux
		}
		line = lipgloss.NewStyle().Background(cursorBg).Bold(true).Render(line)
		fmt.Fprint(w, line)
		return
	}

	// Non-cursor: individual column colors
	timeStr := lipgloss.NewStyle().Width(d.timeW).Foreground(d.theme.colorForDuration(dura)).Render(timeText)
	indicatorStr := lipgloss.NewStyle().Width(d.indicatorW).Foreground(d.theme.indicator).Render(indicatorText)
	titleStr := lipgloss.NewStyle().Width(d.titleW).Foreground(d.theme.textMain).Render(titleText)
	dirStr := lipgloss.NewStyle().Width(d.dirW).Foreground(d.theme.dim).Italic(true).Render(dirText)

	var parts []string
	parts = append(parts, prefix)
	parts = append(parts, timeStr+" ")
	if d.showCheckbox {
		checkboxStr := strings.Repeat(" ", d.checkboxW)
		if checkboxText != "" {
			checkboxStr = lipgloss.NewStyle().Width(d.checkboxW).Render(checkboxText)
		}
		parts = append(parts, checkboxStr+" ")
	}
	parts = append(parts, indicatorStr+" ")
	parts = append(parts, titleStr+" ")
	parts = append(parts, dirStr)

	line := lipgloss.JoinHorizontal(lipgloss.Left, parts...)

	visibleWidth := lipgloss.Width(line)
	if visibleWidth < d.width {
		line += strings.Repeat(" ", d.width-visibleWidth)
	}
	fmt.Fprint(w, line)
}

// ─── Bubble Tea Model ───────────────────────────────────────────────────────

type model struct {
	list        list.Model
	delegate    *sessionDelegate
	sessions    []Session
	running     map[string]struct{}
	firstMsgs   map[string]string
	selected    map[string]struct{}
	deleteMode  bool
	confirming  bool
	showPreview bool
	mode        string
	hasTmux     bool
	agentPath   string
	dbPath      string
	width       int
	height      int
	actionID    string
	actionDir   string
	actionTmux  bool
	theme       theme
	isDark      bool
	renameID    string
	renameInput textinput.Model
}

type previewMsg struct {
	id      string
	content string
}

type checkThemeMsg struct {
	dark bool
	err  error
}

func fetchPreview(dbPath, sessionID string) tea.Cmd {
	return func() tea.Msg {
		return previewMsg{id: sessionID, content: getFirstMessage(dbPath, sessionID)}
	}
}

func queryTerminalBackground() (bool, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return true, err
	}
	defer tty.Close()

	if err := tty.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return true, err
	}

	if _, err := tty.WriteString("\x1b]11;?\x07"); err != nil {
		return true, err
	}

	buf := make([]byte, 128)
	n, err := tty.Read(buf)
	if err != nil {
		return true, err
	}

	return parseBackgroundResponse(string(buf[:n]))
}

func parseBackgroundResponse(s string) (bool, error) {
	idx := strings.Index(s, "rgb:")
	if idx < 0 {
		return true, fmt.Errorf("no rgb in response")
	}

	rgb := s[idx+4:]
	parts := strings.SplitN(rgb, "/", 3)
	if len(parts) < 3 {
		return true, fmt.Errorf("invalid rgb")
	}

	parseHex := func(s string) (int, error) {
		s = strings.TrimRight(s, "\x07\x1b\\")
		s = strings.TrimSpace(s)
		var v int
		_, err := fmt.Sscanf(s, "%x", &v)
		return v, err
	}

	r, err := parseHex(parts[0])
	if err != nil {
		return true, err
	}
	g, err := parseHex(parts[1])
	if err != nil {
		return true, err
	}
	b, err := parseHex(parts[2])
	if err != nil {
		return true, err
	}

	if r > 255 || g > 255 || b > 255 {
		r >>= 8
		g >>= 8
		b >>= 8
	}

	brightness := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	return brightness < 128, nil
}

func newModel() (*model, error) {
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
	theme := darkTheme
	if !isDark {
		theme = lightTheme
	}

	delegate := newSessionDelegate(theme)

	items := make([]list.Item, 0, len(sessions))
	for _, s := range sessions {
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
	l.FilterInput.PromptStyle = lipgloss.NewStyle().Foreground(theme.filterPrompt)
	l.FilterInput.Cursor.Style = lipgloss.NewStyle().Foreground(theme.filterPrompt)
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(theme.filterPrompt)
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(theme.filterPrompt)

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
		firstMsgs:   make(map[string]string),
		selected:    make(map[string]struct{}),
		mode:        "all",
		hasTmux:     hasTmux,
		agentPath:   agentPath,
		dbPath:      dbPath,
		showPreview: false,
		theme:       theme,
		isDark:      isDark,
		renameInput: ti,
	}, nil
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case previewMsg:
		m.firstMsgs[msg.id] = msg.content
		return m, nil

	case tea.FocusMsg:
		return m, func() tea.Msg {
			dark, err := queryTerminalBackground()
			return checkThemeMsg{dark: dark, err: err}
		}

	case checkThemeMsg:
		if msg.err == nil && msg.dark != m.isDark {
			m.isDark = msg.dark
			if m.isDark {
				m.theme = darkTheme
			} else {
				m.theme = lightTheme
			}
			m.delegate.theme = m.theme
			m.delegate.mode = m.mode
			m.list.FilterInput.PromptStyle = lipgloss.NewStyle().Foreground(m.theme.filterPrompt)
			m.list.FilterInput.Cursor.Style = lipgloss.NewStyle().Foreground(m.theme.filterPrompt)
			m.list.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(m.theme.filterPrompt)
			m.list.Styles.FilterCursor = lipgloss.NewStyle().Foreground(m.theme.filterPrompt)
			m.renameInput.PromptStyle = lipgloss.NewStyle().Foreground(m.theme.filterPrompt)
			m.renameInput.Cursor.Style = lipgloss.NewStyle().Foreground(m.theme.filterPrompt)
			m.renameInput.TextStyle = lipgloss.NewStyle().Foreground(m.theme.modalPromptFg)
			m.renameInput.PlaceholderStyle = lipgloss.NewStyle().Foreground(m.theme.textMuted)
		}
		return m, nil

	case tea.KeyMsg:
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
			case "esc":
				if m.showPreview {
					m.showPreview = false
					m.resize()
					return m, nil
				}
				return m, nil
			case "tab":
				m.showPreview = !m.showPreview
				m.resize()
				if m.showPreview {
					if item := m.list.SelectedItem(); item != nil {
						id := item.(sessionItem).session.ID
						if _, ok := m.firstMsgs[id]; !ok {
							return m, fetchPreview(m.dbPath, id)
						}
					}
				}
				return m, nil
			case "ctrl+d":
				m.deleteMode = true
				m.rebuildItems()
				return m, nil
			case "ctrl+t":
				if m.hasTmux {
					if m.mode == "all" {
						m.mode = "tmux"
					} else {
						m.mode = "all"
					}
					m.delegate.mode = m.mode
					m.rebuildItems()
				}
				return m, nil
			case "ctrl+r":
				m.startRename()
				return m, nil
			case "alt+enter", "ctrl+o":
				if m.mode == "all" && m.hasTmux {
					return m.setAction(true)
				}
				return m, nil
			case "enter":
				return m.setAction(m.mode == "tmux" && m.hasTmux)
			}
		}

		// Navigation works in both normal and filter modes
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
	}

	// Pass to list for navigation / filtering
	wasFiltering := m.list.SettingFilter()
	oldID := ""
	if item := m.list.SelectedItem(); item != nil {
		oldID = item.(sessionItem).session.ID
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	if wasFiltering != m.list.SettingFilter() {
		m.resize()
	}

	newID := ""
	if item := m.list.SelectedItem(); item != nil {
		newID = item.(sessionItem).session.ID
	}

	if newID != "" && newID != oldID && m.showPreview {
		if _, ok := m.firstMsgs[newID]; !ok {
			cmd = tea.Batch(cmd, fetchPreview(m.dbPath, newID))
		}
	}

	return m, cmd
}

func (m model) confirmingDelete() bool {
	return m.confirming && m.deleteMode
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
		_ = renameSession(m.dbPath, m.renameID, newTitle)
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

func (m model) executeDelete() (tea.Model, tea.Cmd) {
	m.confirming = false
	m.deleteMode = false

	var ids []string
	if len(m.selected) > 0 {
		for id := range m.selected {
			ids = append(ids, id)
		}
	} else {
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
		// If DB error, just clear list
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
	if m.showPreview {
		if item := m.list.SelectedItem(); item != nil {
			id := item.(sessionItem).session.ID
			if _, ok := m.firstMsgs[id]; !ok {
				return m, fetchPreview(m.dbPath, id)
			}
		}
	}
	return m, nil
}

func (m *model) rebuildItems() {
	m.delegate.showCheckbox = m.deleteMode
	m.resize()

	items := make([]list.Item, 0, len(m.sessions))

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
	// Account for left/right borders
	listWidth := m.width - 2
	if listWidth < 20 {
		listWidth = 20
	}

	// colHeader(1) + topBorder(1) + bottomBorder(1) + footer(1)
	listHeight := m.height - 4
	if listHeight < 5 {
		listHeight = 5
	}

	m.delegate.updateWidths(listWidth)
	m.list.SetSize(listWidth, listHeight)
}

// ─── View ───────────────────────────────────────────────────────────────────

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	colHeader := m.renderColumnHeader()
	listView := m.list.View()

	content := colHeader + "\n" + listView
	bordered := m.renderBox(content)

	footer := m.renderFooter()
	out := bordered + "\n" + footer

	if m.showPreview {
		out = m.renderOverlay(out, m.renderPreviewBox(), m.theme.previewBg, false)
	}

	if m.confirmingDelete() {
		out = m.renderOverlay(out, m.renderDeleteBox(), m.theme.modalBg, true)
	}

	if m.renameID != "" {
		out = m.renderOverlay(out, m.renderRenameBox(), m.theme.modalBg, true)
	}

	return out
}

func (m model) renderColumnHeader() string {
	w := m.delegate.width
	if w == 0 {
		w = m.width
	}

	pad := "  "
	timeH := lipgloss.NewStyle().Width(m.delegate.timeW).Bold(true).Foreground(m.theme.colHeaderFg).Render("TIME")
	indH := lipgloss.NewStyle().Width(m.delegate.indicatorW).Render("")
	titleH := lipgloss.NewStyle().Width(m.delegate.titleW).Bold(true).Foreground(m.theme.colHeaderFg).Render("TITLE")
	dirH := lipgloss.NewStyle().Width(m.delegate.dirW).Bold(true).Foreground(m.theme.colHeaderFg).Render("PATH")

	var parts []string
	parts = append(parts, pad, timeH, " ")
	if m.delegate.showCheckbox {
		cbH := lipgloss.NewStyle().Width(m.delegate.checkboxW).Render("")
		parts = append(parts, cbH, " ")
	}
	parts = append(parts, indH, " ", titleH, " ", dirH)

	line := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	visible := lipgloss.Width(line)
	if visible < w {
		line += strings.Repeat(" ", w-visible)
	}

	return lipgloss.NewStyle().Foreground(m.theme.colHeaderFg).Render(line)
}

func (m model) renderFooter() string {
	w := m.width
	if w < 10 {
		w = 10
	}
	usable := w - 2 // 1 left pad + 1 right pad

	footerKey := m.theme.footerKeyAll
	if m.mode == "tmux" {
		footerKey = m.theme.footerKeyTmux
	}
	keyStyle := lipgloss.NewStyle().Foreground(footerKey)
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.footerLabel)
	sepStyle := lipgloss.NewStyle().Foreground(m.theme.footerLabel)

	build := func(key, label string) string {
		return keyStyle.Render(key) + labelStyle.Render(label)
	}

	var left string
	if m.deleteMode {
		left = build("space", " toggle") + sepStyle.Render(" · ") +
			build("enter", " delete") + sepStyle.Render(" · ") +
			build("esc", " cancel")
	} else if m.list.SettingFilter() {
		prompt := "> " + m.list.FilterInput.Value()
		line := lipgloss.NewStyle().Foreground(m.theme.filterPrompt).Render(prompt)
		vis := lipgloss.Width(line)
		if vis < usable {
			line += strings.Repeat(" ", usable-vis)
		}
		return " " + line + " "
	} else {
		var parts []string
		parts = append(parts, build("/", " filter"))
		parts = append(parts, sepStyle.Render(" · ")+build("<CR>", " resume"))
		if m.mode == "all" && m.hasTmux {
			parts = append(parts, sepStyle.Render(" · ")+build("<M-CR>", " resume tmux"))
		}
		if m.hasTmux {
			toggleLabel := " toggle tmux"
			if m.mode == "tmux" {
				toggleLabel = " toggle all"
			}
			parts = append(parts, sepStyle.Render(" · ")+build("<C-t>", toggleLabel))
		}
		parts = append(parts, sepStyle.Render(" · ")+build("<C-r>", " rename"))
		parts = append(parts, sepStyle.Render(" · ")+build("<C-d>", " delete"))
		left = strings.Join(parts, "")
	}

	var rightStyled string
	if m.hasTmux {
		count := len(m.running)
		if count > 0 {
			dot := lipgloss.NewStyle().Foreground(m.theme.indicator).Render("●")
			rightStyled = dot + labelStyle.Render(fmt.Sprintf(" %d attached", count))
		} else {
			rightStyled = labelStyle.Render("○")
		}
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(rightStyled)
	gap := usable - leftW - rightW
	if gap < 1 {
		gap = 1
	}

	return " " + left + strings.Repeat(" ", gap) + rightStyled + " "
}

func (m model) renderBox(content string) string {
	lines := strings.Split(content, "\n")
	innerW := m.width - 2

	titleColor := m.theme.titleAllFg
	if m.mode == "tmux" {
		titleColor = m.theme.titleTmuxFg
	}

	titlePlain := " OpenCode Sessions "
	titleStyled := lipgloss.NewStyle().Bold(true).Foreground(titleColor).Render(titlePlain)
	titleW := lipgloss.Width(titleStyled)

	var tagStyled string
	var tagW int
	if m.mode == "tmux" {
		tagPlain := " [tmux] "
		tagStyled = lipgloss.NewStyle().Bold(true).Foreground(m.theme.titleTmuxFg).Render(tagPlain)
		tagW = lipgloss.Width(tagStyled)
	}

	border := lipgloss.NewStyle().Foreground(m.theme.border)
	left := border.Render("┌")
	right := border.Render("┐")

	midW := innerW - titleW - tagW
	if midW < 0 {
		midW = 0
	}
	mid := border.Render(strings.Repeat("─", midW))
	top := left + titleStyled + mid + tagStyled + right

	var body []string
	for _, line := range lines {
		vis := lipgloss.Width(line)
		if vis < innerW {
			line += strings.Repeat(" ", innerW-vis)
		}
		body = append(body, border.Render("│")+line+border.Render("│"))
	}

	bottom := border.Render("└") + border.Render(strings.Repeat("─", innerW)) + border.Render("┘")

	return top + "\n" + strings.Join(body, "\n") + "\n" + bottom
}

func (m model) renderOverlay(background string, box string, _ lipgloss.Color, dim bool) string {
	boxLines := strings.Split(box, "\n")
	boxH := len(boxLines)
	topPad := (m.height - boxH) / 2
	if topPad < 0 {
		topPad = 0
	}

	boxWidth := 0
	for _, l := range boxLines {
		if w := lipgloss.Width(l); w > boxWidth {
			boxWidth = w
		}
	}

	leftPad := (m.width - boxWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	rightPad := m.width - boxWidth - leftPad
	if rightPad < 0 {
		rightPad = 0
	}

	bgLines := strings.Split(background, "\n")
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.dim)

	var result []string
	for y := 0; y < m.height; y++ {
		line := ""
		if y < len(bgLines) {
			line = padRight(bgLines[y], m.width)
		} else {
			line = strings.Repeat(" ", m.width)
		}
		if dim {
			line = dimStyle.Render(line)
		}

		inBox := y >= topPad && y < topPad+boxH
		if inBox {
			boxIdx := y - topPad
			if boxIdx < len(boxLines) {
				prefix := ansi.Cut(line, 0, leftPad)
				suffix := ansi.Cut(line, leftPad+boxWidth, m.width)
				result = append(result, prefix+boxLines[boxIdx]+suffix)
				continue
			}
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func (m model) renderModalBox(width int, borderColor lipgloss.Color, badge string, badgeColor lipgloss.Color, body string, hint string) string {
	badgeView := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.modalBg).
		Background(badgeColor).
		Padding(0, 1).
		Render(strings.ToUpper(badge))

	bodyView := lipgloss.NewStyle().
		Foreground(m.theme.textMain).
		Width(width).
		Render(body)

	hintView := lipgloss.NewStyle().
		Foreground(m.theme.modalHintFg).
		Width(width).
		Render(hint)

	content := badgeView + "\n\n" + bodyView + "\n\n" + hintView

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(m.theme.modalBg).
		Padding(1, 2).
		Width(width).
		Render(content)

	return card
}

func (m model) renderPreviewBox() string {
	item := m.list.SelectedItem()
	if item == nil {
		return ""
	}
	sess := item.(sessionItem).session

	content, ok := m.firstMsgs[sess.ID]
	if !ok {
		content = "Loading..."
	} else if content == "" {
		content = "No preview available."
	}

	boxWidth := m.width - 14
	if boxWidth > 88 {
		boxWidth = 88
	}
	if boxWidth < 28 {
		boxWidth = 28
	}

	innerWidth := boxWidth - 8 // border(2) + padding(4) + content inset(2)

	maxLines := m.height - 14
	if maxLines < 4 {
		maxLines = 4
	}

	wrapped := wrapText(content, innerWidth)
	lines := strings.Split(wrapped, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	for i, l := range lines {
		w := lipgloss.Width(l)
		if w < innerWidth {
			lines[i] = l + strings.Repeat(" ", innerWidth-w)
		}
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.previewTitleFg).
		Render(truncate.StringWithTail(sess.Title, uint(innerWidth), "..."))

	contentBlock := lipgloss.NewStyle().
		Foreground(m.theme.previewContentFg).
		Background(m.theme.previewBg).
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.border).
		Padding(1, 1).
		Width(innerWidth + 2).
		Render(strings.Join(lines, "\n"))

	body := title + "\n" + lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Render(sess.Directory) + "\n\n" + contentBlock

	return m.renderModalBox(boxWidth, m.theme.previewBorder, "Preview", m.theme.previewBorder, body, "tab close, esc back")
}

func (m model) renderDeleteBox() string {
	var prompt string
	if len(m.selected) > 0 {
		prompt = fmt.Sprintf("Delete %d sessions?", len(m.selected))
	} else {
		if item := m.list.SelectedItem(); item != nil {
			prompt = fmt.Sprintf("Delete \"%s\"?", item.(sessionItem).session.Title)
		} else {
			prompt = "Delete session?"
		}
	}

	width := 48
	if m.width < 64 {
		width = m.width - 16
	}
	if width < 30 {
		width = 30
	}

	body := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.modalPromptFg).
		Width(width).
		Align(lipgloss.Left).
		Render(wrapText(prompt, width))

	hint := "y confirm, n cancel"
	return m.renderModalBox(width, m.theme.modalBorder, "Delete", m.theme.modalBorder, body, hint)
}

func (m model) renderRenameBox() string {
	item := m.list.SelectedItem()
	var title string
	if item != nil {
		title = item.(sessionItem).session.Title
	}

	boxWidth := 54
	if m.width < 70 {
		boxWidth = m.width - 14
	}
	if boxWidth < 34 {
		boxWidth = 34
	}
	innerWidth := boxWidth - 8

	sub := ""
	if title != "" {
		sub = lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Render(truncate.StringWithTail(title, uint(innerWidth), "..."))
	}

	input := m.renameInput
	input.Width = innerWidth - 4
	inputView := input.View()
	field := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.border).
		Background(m.theme.previewBg).
		Padding(0, 1).
		Width(innerWidth).
		Render(inputView)

	body := lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalPromptFg).Render("Set new session title")
	if sub != "" {
		body += "\n" + sub
	}
	body += "\n\n" + field

	return m.renderModalBox(boxWidth, m.theme.accent, "Rename", m.theme.accent, body, "enter save, esc cancel")
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var result []string
	var line []rune
	var lineLen int

	for _, r := range s {
		if r == '\n' {
			result = append(result, string(line))
			line = nil
			lineLen = 0
			continue
		}
		if lineLen >= width && r == ' ' {
			result = append(result, string(line))
			line = nil
			lineLen = 0
			continue
		}
		if lineLen >= width {
			// find last space
			lastSpace := -1
			for i := len(line) - 1; i >= 0; i-- {
				if line[i] == ' ' {
					lastSpace = i
					break
				}
			}
			if lastSpace > 0 {
				result = append(result, string(line[:lastSpace]))
				line = line[lastSpace+1:]
				lineLen = len(line)
			} else {
				result = append(result, string(line))
				line = nil
				lineLen = 0
			}
		}
		line = append(line, r)
		lineLen++
	}
	if len(line) > 0 {
		result = append(result, string(line))
	}
	return strings.Join(result, "\n")
}

// ─── Actions ────────────────────────────────────────────────────────────────

func deleteSession(cmd, id string) {
	c := exec.Command(cmd, "session", "delete", id)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
}

func resumeSession(cmd, id string) {
	c := exec.Command(cmd, "-s", id)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
}

func findTmuxWindowWithSession(tmuxPath, sessionName, id string) (string, bool) {
	out, err := exec.Command(tmuxPath, "list-panes", "-t", sessionName, "-F", "#{window_index} #{pane_pid}").Output()
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		winIdx, pid := fields[0], fields[1]
		data, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", pid))
		if err != nil {
			continue
		}
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		if strings.Contains(cmdline, "opencode") && strings.Contains(cmdline, id) {
			return winIdx, true
		}
	}
	return "", false
}

func ctrlTmux(agentPath, id, dir string) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: tmux not found in PATH")
		return
	}

	sessionName := filepath.Base(dir)
	if sessionName == "" || sessionName == "." || sessionName == "/" {
		sessionName = "default"
	}
	sessionName = strings.ReplaceAll(sessionName, "/", "-")
	sessionName = strings.ReplaceAll(sessionName, "\\", "-")

	exists := exec.Command(tmuxPath, "has-session", "-t", sessionName).Run() == nil
	if !exists {
		c := exec.Command(tmuxPath, "new-session", "-ds", sessionName, "-c", dir)
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating tmux session: %v\n", err)
			return
		}
	}

	if winIdx, found := findTmuxWindowWithSession(tmuxPath, sessionName, id); found {
		c := exec.Command(tmuxPath, "select-window", "-t", sessionName+":"+winIdx)
		c.Stderr = os.Stderr
		_ = c.Run()
	} else {
		c := exec.Command(tmuxPath, "new-window", "-t", sessionName, "-c", dir, agentPath, "-s", id)
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
			return
		}
	}

	if os.Getenv("TMUX") != "" {
		c := exec.Command(tmuxPath, "switch-client", "-t", sessionName)
		c.Stderr = os.Stderr
		_ = c.Run()
	} else {
		syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", sessionName}, os.Environ())
	}
}

// ─── Main ───────────────────────────────────────────────────────────────────

func main() {
	m, err := newModel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocs: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocs: %v\n", err)
		os.Exit(1)
	}

	var fm model
	switch v := finalModel.(type) {
	case model:
		fm = v
	case *model:
		fm = *v
	default:
		os.Exit(0)
	}

	if fm.actionID != "" {
		if fm.actionTmux {
			ctrlTmux(fm.agentPath, fm.actionID, fm.actionDir)
		} else {
			resumeSession(fm.agentPath, fm.actionID)
		}
	}
}
