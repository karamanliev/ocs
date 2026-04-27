package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/truncate"
)

type layoutMetrics struct {
	showPreview bool
	previewSide bool
	listWidth   int
	listHeight  int
	previewW    int
	previewH    int
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	layout := m.layoutMetrics()
	colHeader := m.renderColumnHeader()
	listView := m.list.View()
	listBox := m.renderBox(colHeader+"\n"+listView, layout.listWidth)

	footer := m.renderFooter()
	out := listBox
	if layout.showPreview {
		previewBox := m.renderPreviewPane(layout.previewW, layout.previewH)
		if layout.previewSide {
			out = joinHorizontalPanels(listBox, previewBox, layout.listWidth, layout.previewW)
		} else {
			out = listBox + "\n" + previewBox
		}
	}
	out += "\n" + footer

	if m.deleting {
		out = m.renderOverlay(out, m.renderDeletingBox(), m.theme.modalBg, true)
	} else if m.confirmingDelete() {
		out = m.renderOverlay(out, m.renderDeleteBox(), m.theme.modalBg, true)
	}

	if m.renameID != "" {
		out = m.renderOverlay(out, m.renderRenameBox(), m.theme.modalBg, true)
	}

	return out
}

func (m model) layoutMetrics() layoutMetrics {
	width := m.width
	if width < 20 {
		width = 20
	}
	height := m.height
	if height < 8 {
		height = 8
	}

	layout := layoutMetrics{
		showPreview: m.showPreview,
		listWidth:   width,
		listHeight:  height - 4,
	}
	if layout.listHeight < 5 {
		layout.listHeight = 5
	}
	if !m.showPreview {
		return layout
	}

	const gap = 1
	const minListOuterW = 52
	const minPreviewOuterW = 36
	const sideThreshold = 132

	if width >= sideThreshold && width >= height*2 {
		previewW := width * 38 / 100
		if previewW < minPreviewOuterW {
			previewW = minPreviewOuterW
		}
		if previewW > width-minListOuterW-gap {
			previewW = width - minListOuterW - gap
		}
		listW := width - previewW - gap
		if listW >= minListOuterW && previewW >= minPreviewOuterW {
			layout.previewSide = true
			layout.listWidth = listW
			layout.previewW = previewW
			layout.previewH = height - 1
			if layout.previewH < 6 {
				layout.previewH = 6
			}
			return layout
		}
	}

	previewH := height * 42 / 100
	minPreviewH := 10
	maxPreviewHLimit := 18
	if width < height*2 {
		previewH = height * 48 / 100
		minPreviewH = 12
		maxPreviewHLimit = 22
	}
	if previewH < minPreviewH {
		previewH = minPreviewH
	}
	if previewH > maxPreviewHLimit {
		previewH = maxPreviewHLimit
	}
	maxPreviewH := height - 10
	if maxPreviewH < 4 {
		maxPreviewH = 4
	}
	if previewH > maxPreviewH {
		previewH = maxPreviewH
	}
	listHeight := height - 4 - gap - previewH
	if listHeight < 5 {
		listHeight = 5
		previewH = height - 10
		if previewH < 4 {
			previewH = 4
		}
	}

	layout.listHeight = listHeight
	layout.previewW = width
	layout.previewH = previewH
	return layout
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

	parts := []string{pad, timeH, " "}
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
	usable := w - 2

	keyStyle := lipgloss.NewStyle().Foreground(m.theme.footerKeyColor(m.mode))
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
		crLabel := " resume"
		mcrLabel := ""
		if m.mode == "tmux" {
			crLabel = " resume tmux"
			if m.hasTmux {
				mcrLabel = " resume"
			}
		} else if m.hasTmux {
			mcrLabel = " resume tmux"
		}
		parts = append(parts, sepStyle.Render(" · ")+build("<CR>", crLabel))
		if mcrLabel != "" {
			parts = append(parts, sepStyle.Render(" · ")+build("<M-CR>", mcrLabel))
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
		var runCount, activeCount int
		for _, st := range m.states {
			switch st {
			case stateRunning:
				runCount++
			case stateActive:
				activeCount++
			}
		}
		total := runCount + activeCount
		if total > 0 {
			var parts []string
			if runCount > 0 {
				dot := lipgloss.NewStyle().Foreground(m.theme.indicatorRunning).Render("●")
				parts = append(parts, dot+labelStyle.Render(fmt.Sprintf(" %d running", runCount)))
			}
			if activeCount > 0 {
				dot := lipgloss.NewStyle().Foreground(m.theme.indicatorActive).Render("○")
				parts = append(parts, dot+labelStyle.Render(fmt.Sprintf(" %d active", activeCount)))
			}
			rightStyled = strings.Join(parts, "  ")
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

func (m model) renderBox(content string, width int) string {
	titleColor := m.theme.titleColor(m.mode)
	rightTitle := ""
	if m.mode == "tmux" {
		rightTitle = "[tmux]"
	}
	return m.renderPanel(content, width, 0, "OpenCode Sessions", rightTitle, titleColor, m.theme.titleTmuxFg, "")
}

func (m model) renderPanel(content string, width int, height int, leftTitle string, rightTitle string, leftColor lipgloss.Color, rightColor lipgloss.Color, bg lipgloss.Color) string {
	if width < 4 {
		width = 4
	}
	innerW := width - 2
	lines := strings.Split(content, "\n")
	if height > 0 {
		innerH := height - 2
		if innerH < 0 {
			innerH = 0
		}
		if len(lines) > innerH {
			lines = lines[:innerH]
		}
		for len(lines) < innerH {
			lines = append(lines, "")
		}
	}

	leftStyled := lipgloss.NewStyle().Bold(true).Foreground(leftColor).Render(" " + leftTitle + " ")
	leftW := lipgloss.Width(leftStyled)
	rightStyled := ""
	rightW := 0
	if rightTitle != "" {
		rightStyled = lipgloss.NewStyle().Bold(true).Foreground(rightColor).Render(" " + rightTitle + " ")
		rightW = lipgloss.Width(rightStyled)
	}

	border := lipgloss.NewStyle().Foreground(m.theme.border)
	left := border.Render("┌")
	right := border.Render("┐")

	midW := innerW - leftW - rightW
	if midW < 0 {
		midW = 0
	}
	mid := border.Render(strings.Repeat("─", midW))
	top := left + leftStyled + mid + rightStyled + right

	var body []string
	for _, line := range lines {
		vis := lipgloss.Width(line)
		if vis < innerW {
			line += strings.Repeat(" ", innerW-vis)
		}
		if bg != "" {
			line = withLineBg(line, bg)
		}
		body = append(body, border.Render("│")+line+"\x1b[0m"+border.Render("│"))
	}

	bottom := border.Render("└") + border.Render(strings.Repeat("─", innerW)) + border.Render("┘")

	return top + "\n" + strings.Join(body, "\n") + "\n" + bottom
}

func joinHorizontalPanels(left string, right string, leftW int, rightW int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	count := len(leftLines)
	if len(rightLines) > count {
		count = len(rightLines)
	}
	var out []string
	for i := 0; i < count; i++ {
		leftLine := strings.Repeat(" ", leftW)
		if i < len(leftLines) {
			leftLine = padRight(leftLines[i], leftW)
		}
		rightLine := strings.Repeat(" ", rightW)
		if i < len(rightLines) {
			rightLine = padRight(rightLines[i], rightW)
		}
		out = append(out, leftLine+" "+rightLine)
	}
	return strings.Join(out, "\n")
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
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Background(m.theme.modalBg).
		Padding(1, 2).
		Width(width).
		Render(content)

	return card
}

func truncatePreviewLines(lines []string, limit int) []string {
	if limit < 1 || len(lines) <= limit {
		return lines
	}
	lines = append([]string{}, lines[:limit]...)
	last := lines[len(lines)-1]
	if lipgloss.Width(last) > 3 {
		last = truncate.StringWithTail(last, uint(lipgloss.Width(last)), "...")
	} else {
		last = "..."
	}
	lines[len(lines)-1] = last
	return lines
}

func (m model) renderPreviewPane(width int, height int) string {
	item := m.list.SelectedItem()
	userContent := "No session selected."
	assistantContent := ""
	titleText := ""
	dirText := ""
	if item != nil {
		sess := item.(sessionItem).session
		titleText = sess.Title
		dirText = sess.Directory
		if cached, ok := m.firstMsgs[sess.ID]; !ok {
			userContent = "Loading..."
		} else {
			if cached.user == "" {
				userContent = "No user preview available."
			} else {
				userContent = cached.user
			}
			assistantContent = cached.assistant
		}
	}

	innerW := width - 2
	if innerW < 10 {
		innerW = 10
	}
	innerH := height - 2
	if innerH < 2 {
		innerH = 2
	}
	contentW := innerW - 4
	if contentW < 6 {
		contentW = 6
	}

	padLeft := "  "
	var lines []string
	lines = append(lines, "")

	if titleText != "" {
		lines = append(lines, padLeft+lipgloss.NewStyle().Bold(true).Foreground(m.theme.previewTitleFg).Render(truncate.StringWithTail(titleText, uint(contentW), "...")))
	}
	if dirText != "" {
		lines = append(lines, padLeft+lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Render(truncate.StringWithTail(dirText, uint(contentW), "...")))
		lines = append(lines, "")
	}

	maxContentLines := innerH - len(lines) - 2
	if maxContentLines < 1 {
		maxContentLines = 1
		if innerH > 1 && len(lines) >= innerH {
			lines = lines[:innerH-1]
		}
	}

	lines = append(lines, padLeft+lipgloss.NewStyle().Bold(true).Foreground(m.theme.previewTitleFg).Render("You"))
	userLines := truncatePreviewLines(strings.Split(wrapText(userContent, contentW), "\n"), 4)
	for _, line := range userLines {
		lines = append(lines, padLeft+lipgloss.NewStyle().Italic(true).Foreground(m.theme.previewContentFg).Render(line))
	}

	if assistantContent != "" && len(lines) < innerH-1 {
		lines = append(lines, "")
		lines = append(lines, padLeft+lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalHintFg).Render("Agent"))
		assistantLines := truncatePreviewLines(strings.Split(wrapText(assistantContent, contentW), "\n"), 5)
		free := innerH - len(lines)
		if free > 0 && len(assistantLines) > free {
			assistantLines = truncatePreviewLines(assistantLines, free)
		}
		for _, line := range assistantLines {
			lines = append(lines, padLeft+lipgloss.NewStyle().Italic(true).Foreground(m.theme.modalHintFg).Render(line))
		}
	}

	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}

	contentView := strings.Join(lines, "\n")

	return m.renderPanel(contentView, width, height, "Preview", "<Tab> Toggle", m.theme.previewBorder, m.theme.modalHintFg, m.theme.previewBg)
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

func (m model) renderDeletingBox() string {
	width := 48
	if m.width < 64 {
		width = m.width - 16
	}
	if width < 30 {
		width = 30
	}

	spin := m.spinner.View()
	body := lipgloss.JoinHorizontal(lipgloss.Center, spin, "  ", lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalPromptFg).Render("Deleting..."))
	body = lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(body)

	return m.renderModalBox(width, m.theme.modalBorder, "Delete", m.theme.modalBorder, body, "")
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