package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

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
	d.width = totalWidth
	if d.width < 30 {
		d.width = 30
	}
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
	switch i.state {
	case stateRunning:
		indicatorText = "●"
	case stateActive:
		indicatorText = "○"
	}

	titleWidth := d.titleW
	if d.grouped {
		titleWidth -= 2
		if titleWidth < 8 {
			titleWidth = 8
		}
	}
	titleText := truncate.StringWithTail(i.session.Title, uint(titleWidth), "…")
	dirText := ""
	if !d.grouped {
		dirText = truncate.StringWithTail(i.session.Directory, uint(d.dirW), "…")
	}

	prefix := "  "

	filterText := ""
	if m.FilterState() == list.Filtering || m.FilterState() == list.FilterApplied {
		filterText = m.FilterInput.Value()
	}

	if isCursor {
		if d.grouped {
			titleText = "  " + titleText
		}
		hTitle := padRight(highlightSubstring(titleText, filterText, d.theme.filterMatch), d.titleW)
		hDir := ""
		if !d.grouped {
			hDir = padRight(highlightSubstring(dirText, filterText, d.theme.filterMatch), d.dirW)
		}

		parts := []string{
			prefix,
			padRight(timeText, d.timeW) + " ",
		}
		if d.showCheckbox {
			parts = append(parts, padRight(checkboxText, d.checkboxW)+" ")
		}
		parts = append(parts,
			padRight(indicatorText, d.indicatorW)+" ",
			hTitle,
		)
		if !d.grouped {
			parts = append(parts, " ", hDir)
		}

		line := strings.Join(parts, "")
		vis := lipgloss.Width(line)
		if vis < d.width {
			line += strings.Repeat(" ", d.width-vis)
		}
		line = lipgloss.NewStyle().Background(d.theme.cursorBg(d.mode)).Bold(true).Render(line)
		fmt.Fprint(w, line)
		return
	}

	timeStr := lipgloss.NewStyle().Width(d.timeW).Foreground(d.theme.colorForDuration(dura)).Render(timeText)
	indicatorColor := d.theme.indicatorActive
	if i.state == stateRunning {
		indicatorColor = d.theme.indicatorRunning
	}
	indicatorStr := lipgloss.NewStyle().Width(d.indicatorW).Foreground(indicatorColor).Render(indicatorText)

	var titleStr, dirStr string
	if filterText != "" {
		if d.grouped {
			titleText = "  " + titleText
		}
		titleStr = padRight(highlightSubstringStyled(titleText, filterText, d.theme.textMain, d.theme.filterMatch), d.titleW)
		if !d.grouped {
			dirStr = padRight(highlightSubstringStyled(dirText, filterText, d.theme.dim, d.theme.filterMatch), d.dirW)
		}
	} else {
		if d.grouped {
			titleText = "  " + titleText
		}
		titleStr = lipgloss.NewStyle().Width(d.titleW).Foreground(d.theme.textMain).Render(titleText)
		if !d.grouped {
			dirStr = lipgloss.NewStyle().Width(d.dirW).Foreground(d.theme.dim).Italic(false).Render(dirText)
		}
	}

	parts := []string{
		prefix,
		timeStr + " ",
	}
	if d.showCheckbox {
		checkboxStr := strings.Repeat(" ", d.checkboxW)
		if checkboxText != "" {
			checkboxStr = lipgloss.NewStyle().Width(d.checkboxW).Render(checkboxText)
		}
		parts = append(parts, checkboxStr+" ")
	}
	parts = append(parts,
		indicatorStr+" ",
		titleStr,
	)
	if !d.grouped {
		parts = append(parts, " ", dirStr)
	}

	line := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	visibleWidth := lipgloss.Width(line)
	if visibleWidth < d.width {
		line += strings.Repeat(" ", d.width-visibleWidth)
	}
	fmt.Fprint(w, line)
}

func (d *sessionDelegate) renderGroupHeader(w io.Writer, m list.Model, index int, item groupHeaderItem) {
	isCursor := index == m.Index()
	marker := "▶"
	if !item.collapsed {
		marker = "▼"
	}
	label := "  " + marker + " " + item.path + " "
	count := "(" + strconv.Itoa(item.count) + ")"
	line := label + count
	line = padRight(truncate.StringWithTail(line, uint(max(1, d.width-2)), "…"), d.width)

	base := lipgloss.NewStyle().Bold(true).Foreground(d.theme.dim)
	accent := d.theme.titleColor(d.mode)
	markStyle := lipgloss.NewStyle().Bold(true).Foreground(accent)
	countStyle := lipgloss.NewStyle().Bold(true).Foreground(accent)
	if isCursor {
		base = base.Background(d.theme.cursorBg(d.mode)).Foreground(d.theme.dim)
		markStyle = markStyle.Background(d.theme.cursorBg(d.mode))
		countStyle = countStyle.Background(d.theme.cursorBg(d.mode))
	}
	baseLine := base.Render("  ") + markStyle.Render(marker) + base.Render(" "+item.path+" ")
	fmt.Fprint(w, baseLine+countStyle.Render(count))
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

// Rendering helpers shared between delegate and view

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
