package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

var homeDir string

func init() {
	homeDir, _ = os.UserHomeDir()
}

func displayPath(path string) string {
	if homeDir != "" && strings.HasPrefix(path, homeDir) {
		return "~" + path[len(homeDir):]
	}
	return path
}

func isWorktree(dir, worktree string) bool {
	return worktree != "" && dir != worktree
}

type sessionItem struct {
	session      Session
	state        sessionState
	isSelected   bool
	showCheckbox bool
	groupPath    string
}

func (i sessionItem) FilterValue() string {
	return i.session.Title + " " + i.session.Directory
}

type sessionDelegate struct {
	width, timeW, checkboxW, indicatorW, titleW, dirW int
	showCheckbox                                      bool
	grouped                                           bool
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
	d.width = max(totalWidth, 30)
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
	remain := max(content-d.timeW-d.indicatorW-d.checkboxW, 20)
	if d.grouped {
		d.titleW = remain
		d.dirW = 0
		return
	}
	d.titleW = remain * 50 / 100
	d.dirW = remain - d.titleW
}

func (d *sessionDelegate) Height() int                         { return 1 }
func (d *sessionDelegate) Spacing() int                        { return 0 }
func (d *sessionDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d *sessionDelegate) ShortHelp() []key.Binding            { return nil }
func (d *sessionDelegate) FullHelp() [][]key.Binding           { return nil }

func (d *sessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if _, ok := item.(groupSeparatorItem); ok {
		fmt.Fprint(w, strings.Repeat(" ", d.width))
		return
	}
	if header, ok := item.(groupHeaderItem); ok {
		d.renderGroupHeader(w, m, index, header)
		return
	}

	i, ok := item.(sessionItem)
	if !ok {
		return
	}

	updated := time.Unix(i.session.Updated/1000, (i.session.Updated%1000)*1e6)
	timeText := formatDuration(time.Since(updated))

	filterText := ""
	if m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied {
		filterText = m.FilterInput.Value()
	}

	if index == m.Index() {
		d.renderCursor(w, i, timeText, updated, filterText)
		return
	}
	d.renderItem(w, i, timeText, updated, filterText)
}

func (d *sessionDelegate) formattedTitle(i sessionItem, filter string, cursor bool) string {
	titleW := d.titleW
	if d.grouped {
		titleW -= 2
	}
	titleText := groupedPrefix(truncate.StringWithTail(i.session.Title, uint(max(titleW, 8)), "…"), d.grouped)
	if cursor {
		return padRight(highlightSubstring(titleText, filter, d.theme.filterMatch), d.titleW)
	}
	if filter != "" {
		return padRight(highlightSubstringStyled(titleText, filter, d.theme.textMain, d.theme.filterMatch), d.titleW)
	}
	return lipgloss.NewStyle().Width(d.titleW).Foreground(d.theme.textMain).Render(titleText)
}

func (d *sessionDelegate) formattedDir(sess Session, filter string, cursor bool) string {
	if d.grouped {
		return ""
	}
	pathText := displayPath(sess.Directory)
	arrow := ""
	if isWorktree(sess.Directory, sess.Worktree) {
		arrow = lipgloss.NewStyle().Foreground(d.theme.indicatorRunning).Render(" ↗")
	}
	if cursor {
		return padRight(highlightSubstring(pathText, filter, d.theme.filterMatch)+arrow, d.dirW)
	}
	if filter != "" {
		return padRight(highlightSubstringStyled(pathText, filter, d.theme.dim, d.theme.filterMatch)+arrow, d.dirW)
	}
	if arrow != "" {
		return padRight(lipgloss.NewStyle().Foreground(d.theme.dim).Render(pathText)+arrow, d.dirW)
	}
	return lipgloss.NewStyle().Width(d.dirW).Foreground(d.theme.dim).Italic(false).Render(pathText)
}

func (d *sessionDelegate) renderLine(i sessionItem, timeText string, updated time.Time, filterText string, cursor bool) string {
	var parts []string

	timeStr := padRight(timeText, d.timeW)
	if !cursor {
		timeStr = lipgloss.NewStyle().Width(d.timeW).Foreground(d.theme.colorForDuration(time.Since(updated))).Render(timeText)
	}
	parts = append(parts, "  ", timeStr+" ")

	if d.showCheckbox {
		cb := strings.Repeat(" ", d.checkboxW)
		if i.showCheckbox {
			text := "[ ]"
			if i.isSelected {
				text = "[x]"
			}
			if cursor {
				cb = padRight(text, d.checkboxW)
			} else {
				cb = lipgloss.NewStyle().Width(d.checkboxW).Render(text)
			}
		}
		parts = append(parts, cb+" ")
	}

	ind := indicatorForState(i.state, d.mode)
	if cursor {
		ind = padRight(ind, d.indicatorW)
	} else {
		indColor := d.theme.indicatorActive
		if i.state == stateLinked || (i.state == stateDetected && d.mode != "tmux") {
			indColor = d.theme.indicatorRunning
		}
		ind = lipgloss.NewStyle().Width(d.indicatorW).Foreground(indColor).Render(ind)
	}
	parts = append(parts, ind+" ")

	parts = append(parts, d.formattedTitle(i, filterText, cursor))

	if !d.grouped {
		parts = append(parts, " ", d.formattedDir(i.session, filterText, cursor))
	}

	var line string
	if cursor {
		line = strings.Join(parts, "")
	} else {
		line = lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	}
	if vis := lipgloss.Width(line); vis < d.width {
		line += strings.Repeat(" ", d.width-vis)
	}
	return line
}

func (d *sessionDelegate) renderCursor(w io.Writer, i sessionItem, timeText string, updated time.Time, filterText string) {
	line := d.renderLine(i, timeText, updated, filterText, true)
	line = lipgloss.NewStyle().Background(d.theme.cursorBg(d.mode)).Bold(true).Render(line)
	fmt.Fprint(w, line)
}

func (d *sessionDelegate) renderItem(w io.Writer, i sessionItem, timeText string, updated time.Time, filterText string) {
	fmt.Fprint(w, d.renderLine(i, timeText, updated, filterText, false))
}

func (d *sessionDelegate) renderGroupHeader(w io.Writer, m list.Model, index int, item groupHeaderItem) {
	isCursor := index == m.Index()
	marker := "▶"
	if !item.collapsed {
		marker = "▼"
	}

	pathText := displayPath(item.path)
	var arrowText string
	if item.worktree != "" && item.path != item.worktree {
		arrowText = "↗ "
	}

	base := lipgloss.NewStyle().Bold(true).Foreground(d.theme.dim)
	accent := d.theme.titleColor(d.mode)
	markStyle := lipgloss.NewStyle().Bold(true).Foreground(accent)
	countStyle := lipgloss.NewStyle().Bold(true).Foreground(accent)
	arrowStyle := lipgloss.NewStyle().Bold(true).Foreground(d.theme.indicatorRunning)

	if isCursor {
		bg := d.theme.cursorBg(d.mode)
		base = base.Background(bg).Foreground(d.theme.dim)
		markStyle = markStyle.Background(bg)
		countStyle = countStyle.Background(bg)
		arrowStyle = arrowStyle.Background(bg)
	}

	count := "(" + strconv.Itoa(item.count) + ")"
	baseLine := base.Render("  ") + markStyle.Render(marker) + base.Render(" "+pathText+" ")
	fmt.Fprint(w, baseLine+arrowStyle.Render(arrowText)+countStyle.Render(count))
}

func groupedPrefix(title string, grouped bool) string {
	if !grouped {
		return title
	}
	return "  " + title
}

// indicatorForState returns the visual indicator for a session state.
//
// In tmux mode: stateLinked = "●" (green), stateDetected = "○" (dim).
// In non-tmux mode: any detected state = "●" (green).
func indicatorForState(st sessionState, mode string) string {
	switch {
	case st == stateLinked:
		return "●"
	case st == stateDetected && mode == "tmux":
		return "○"
	case st == stateDetected:
		return "●"
	default:
		return " "
	}
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func highlightSubstring(text, filter string, matchColor lipgloss.Color) string {
	return highlightMatch(text, filter, nil, matchColor)
}

func highlightSubstringStyled(text, filter string, defaultFg, matchColor lipgloss.Color) string {
	defaultStyle := lipgloss.NewStyle().Foreground(defaultFg)
	return highlightMatch(text, filter, &defaultStyle, matchColor)
}

func highlightMatch(text, filter string, defaultStyle *lipgloss.Style, matchColor lipgloss.Color) string {
	if filter == "" || text == "" {
		if defaultStyle != nil {
			return defaultStyle.Render(text)
		}
		return text
	}

	lower := strings.ToLower(text)
	fl := strings.ToLower(filter)
	ms := lipgloss.NewStyle().Foreground(matchColor)

	var buf strings.Builder
	i := 0
	for i < len(text) {
		idx := strings.Index(lower[i:], fl)
		if idx < 0 {
			remaining := text[i:]
			if defaultStyle != nil {
				buf.WriteString(defaultStyle.Render(remaining))
			} else {
				buf.WriteString(remaining)
			}
			break
		}
		before := text[i : i+idx]
		if before != "" {
			if defaultStyle != nil {
				buf.WriteString(defaultStyle.Render(before))
			} else {
				buf.WriteString(before)
			}
		}
		buf.WriteString(ms.Render(text[i+idx : i+idx+len(filter)]))
		i = i + idx + len(filter)
	}
	return buf.String()
}

func withLineBg(line string, bg lipgloss.Color) string {
	hex := string(bg)
	if !strings.HasPrefix(hex, "#") || len(hex) != 7 {
		return line
	}
	r, _ := strconv.ParseUint(hex[1:3], 16, 8)
	g, _ := strconv.ParseUint(hex[3:5], 16, 8)
	b, _ := strconv.ParseUint(hex[5:7], 16, 8)
	bgSeq := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	result := strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+bgSeq)
	return bgSeq + result
}
