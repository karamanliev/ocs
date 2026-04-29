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

	if m.forking {
		out = m.renderOverlay(out, m.renderForkingBox(), m.theme.modalBg, true)
	} else if m.closingTmux {
		out = m.renderOverlay(out, m.renderClosingTmuxBox(), m.theme.modalBg, true)
	} else if m.deleting {
		out = m.renderOverlay(out, m.renderDeletingBox(), m.theme.modalBg, true)
	} else if m.confirmingDelete() {
		out = m.renderOverlay(out, m.renderDeleteBox(), m.theme.modalBg, true)
	} else if m.confirmingFork {
		out = m.renderOverlay(out, m.renderConfirmForkBox(), m.theme.modalBg, true)
	} else if m.confirmingNewSession {
		out = m.renderOverlay(out, m.renderConfirmNewSessionBox(), m.theme.modalBg, true)
	} else if m.confirmingCloseTmux {
		out = m.renderOverlay(out, m.renderConfirmCloseTmuxBox(), m.theme.modalBg, true)
	}

	if m.renameID != "" || m.forkMode {
		out = m.renderOverlay(out, m.renderRenameBox(), m.theme.modalBg, true)
	}

	if m.dirpickerOpen {
		out = m.renderOverlay(out, m.renderDirpickerModal(), m.theme.modalBg, true)
	}

	if m.keybindsOpen {
		out = m.renderOverlay(out, m.renderKeybindsBox(), m.theme.modalBg, true)
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

	parts := []string{pad, timeH, " "}
	if m.delegate.showCheckbox {
		cbH := lipgloss.NewStyle().Width(m.delegate.checkboxW).Render("")
		parts = append(parts, cbH, " ")
	}
	parts = append(parts, indH, " ", titleH)
	if !m.grouped {
		dirH := lipgloss.NewStyle().Width(m.delegate.dirW).Bold(true).Foreground(m.theme.colHeaderFg).Render("PATH")
		parts = append(parts, " ", dirH)
	}

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
	if m.list.SettingFilter() {
		prompt := "> " + m.list.FilterInput.Value()
		line := lipgloss.NewStyle().Foreground(m.theme.filterPrompt).Render(prompt)
		vis := lipgloss.Width(line)
		if vis < usable {
			line += strings.Repeat(" ", usable-vis)
		}
		return " " + line + " "
	} else if m.deleteMode {
		left = build("space", " toggle") + sepStyle.Render(" · ") +
			build("d/<CR>", " delete") + sepStyle.Render(" · ") +
			build("esc", " cancel")
	} else {
		var parts []string
		parts = append(parts, build("/", " filter"))
		parts = append(parts, sepStyle.Render(" · ")+build("<CR>", " open"))
		if m.hasTmux {
			toggleLabel := " tmux"
			if m.mode == "tmux" {
				toggleLabel = " normal"
			}
			parts = append(parts, sepStyle.Render(" · ")+build("t", toggleLabel))
		}
		groupLabel := " group"
		if m.grouped {
			groupLabel = " ungroup"
		}
		parts = append(parts, sepStyle.Render(" · ")+build("g", groupLabel))
		parts = append(parts, sepStyle.Render(" · ")+build("?", " keys"))
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

type hintPart struct {
	key   string
	label string
}

func (m model) buildHint(parts []hintPart) string {
	keyStyle := lipgloss.NewStyle().Foreground(m.theme.accent).Background(m.theme.modalBg)
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Background(m.theme.modalBg)
	sepStyle := lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Background(m.theme.modalBg)
	var result string
	for i, p := range parts {
		if i > 0 {
			result += sepStyle.Render(" · ")
		}
		result += keyStyle.Render(p.key) + labelStyle.Render(p.label)
	}
	return result
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
		Background(m.theme.modalBg).
		Render(body)

	hintView := lipgloss.NewStyle().
		Background(m.theme.modalBg).
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
	if limit < 1 {
		return append([]string(nil), lines...)
	}
	nonEmpty := 0
	var keep []string
	for _, l := range lines {
		if nonEmpty >= limit {
			break
		}
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
		keep = append(keep, l)
	}
	if len(keep) == len(lines) {
		return keep
	}
	last := keep[len(keep)-1]
	if lipgloss.Width(last) > 3 {
		last = truncate.StringWithTail(last, uint(lipgloss.Width(last)), "...")
	} else {
		last = "..."
	}
	keep[len(keep)-1] = last
	return keep
}

func (m model) previewDivider(label string, contentW int, fg lipgloss.Color) string {
	prefix := "── " + label + " "
	dashes := contentW - len(prefix)
	if dashes < 2 {
		dashes = 2
	}
	return "  " + lipgloss.NewStyle().Foreground(fg).Render(prefix+strings.Repeat("─", dashes))
}

func (m model) previewScrollbarChar(lineIdx, viewH, scroll, maxScroll, totalLines int) string {
	thumbH := viewH * viewH / totalLines
	if thumbH < 1 {
		thumbH = 1
	}
	thumbStart := 0
	if maxScroll > 0 {
		thumbStart = scroll * (viewH - thumbH) / maxScroll
	}
	if lineIdx >= thumbStart && lineIdx < thumbStart+thumbH {
		return lipgloss.NewStyle().Foreground(m.theme.scrollbarThumb).Render("█")
	}
	return lipgloss.NewStyle().Foreground(m.theme.scrollbarTrack).Render("│")
}

func (m model) buildPreviewLines(data previewData, contentW int) []string {
	padLeft := "  "
	youStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.previewTitleFg)
	agentStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalHintFg)
	userMsgStyle := lipgloss.NewStyle().Italic(true).Foreground(m.theme.previewContentFg)
	agentMsgStyle := lipgloss.NewStyle().Italic(true).Foreground(m.theme.modalHintFg)

	agentLabel := "Agent"
	if data.modelName != "" {
		agentLabel = "Agent [" + data.modelName + "]"
	}

	var lines []string

	// ── First exchange ──
	lines = append(lines, m.previewDivider("First exchange", contentW, m.theme.timeDay))
	lines = append(lines, "")

	addLines := func(text string, style lipgloss.Style) {
		for _, l := range truncatePreviewLines(strings.Split(wrapText(text, contentW), "\n"), 8) {
			lines = append(lines, padLeft+style.Render(l))
		}
	}

	if data.firstUser != "" {
		lines = append(lines, padLeft+youStyle.Render("You"))
		addLines(data.firstUser, userMsgStyle)
	} else {
		lines = append(lines, padLeft+userMsgStyle.Render("(no messages)"))
	}

	if data.firstAssistant != "" {
		lines = append(lines, "")
		lines = append(lines, padLeft+agentStyle.Render(agentLabel))
		addLines(data.firstAssistant, agentMsgStyle)
	}

	// ── Latest exchange ── only if different from first
	lastUserDiff := data.lastUser != "" && data.lastUser != data.firstUser
	lastAsstDiff := data.lastAssistant != "" && data.lastAssistant != data.firstAssistant
	if lastUserDiff || lastAsstDiff {
		lines = append(lines, "")
		lines = append(lines, m.previewDivider("Latest exchange", contentW, m.theme.timeOld))
		lines = append(lines, "")

		if lastUserDiff {
			lines = append(lines, padLeft+youStyle.Render("You"))
			addLines(data.lastUser, userMsgStyle)
		}

		if lastAsstDiff {
			if lastUserDiff {
				lines = append(lines, "")
			}
			lines = append(lines, padLeft+agentStyle.Render(agentLabel))
			addLines(data.lastAssistant, agentMsgStyle)
		}
	}

	// Bottom padding so content doesn't stick to the border
	lines = append(lines, "", "")

	return lines
}

func (m model) renderPreviewPane(width int, height int) string {
	item := m.list.SelectedItem()

	innerW := width - 2
	if innerW < 10 {
		innerW = 10
	}
	innerH := height - 2
	if innerH < 2 {
		innerH = 2
	}
	// Reserve 1 col for scrollbar (always, to avoid reflow when it appears)
	contentW := innerW - 5
	if contentW < 6 {
		contentW = 6
	}

	padLeft := "  "
	var header []string
	header = append(header, "")
	var bodyLines []string

	var data previewData
	loading := false

	if item == nil {
		header = append(header, padLeft+lipgloss.NewStyle().Foreground(m.theme.dim).Render("No session selected."))
	} else {
		if sess, ok := sessionFromItem(item); ok {
			header = append(header, padLeft+lipgloss.NewStyle().Bold(true).Foreground(m.theme.previewTitleFg).Render(
				truncate.StringWithTail(sess.Title, uint(contentW), "...")))
			header = append(header, padLeft+lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Render(
				truncate.StringWithTail(sess.Directory, uint(contentW), "...")))
			header = append(header, "")

			cached, ok := m.firstMsgs[sess.ID]
			if !ok {
				loading = true
			} else {
				data = cached
			}
		} else if groupHeader, ok := item.(groupHeaderItem); ok {
			header = append(header, padLeft+lipgloss.NewStyle().Bold(true).Foreground(m.theme.previewTitleFg).Render(
				truncate.StringWithTail(groupHeader.path, uint(contentW), "...")))
			header = append(header, "")
			bodyLines = append(bodyLines,
				padLeft+lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Render(fmt.Sprintf("%d sessions in group", groupHeader.count)),
				"",
				padLeft+lipgloss.NewStyle().Foreground(m.theme.dim).Render("Press space to fold or unfold."),
			)
		}
	}
	if loading {
		bodyLines = append(bodyLines, padLeft+lipgloss.NewStyle().Foreground(m.theme.dim).Render("Loading..."))
	} else if item != nil && len(bodyLines) == 0 {
		bodyLines = m.buildPreviewLines(data, contentW)
	}

	allLines := append(header, bodyLines...)

	// Scroll logic
	totalLines := len(allLines)
	canScroll := totalLines > innerH
	maxScroll := 0
	if canScroll {
		maxScroll = totalLines - innerH
	}
	scroll := m.previewScroll
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}

	// Pad to at least innerH
	for len(allLines) < innerH {
		allLines = append(allLines, "")
	}

	visible := allLines[scroll:]
	if len(visible) > innerH {
		visible = visible[:innerH]
	}

	// Render each line with scrollbar in the last column
	sbColW := innerW - 1
	rendered := make([]string, len(visible))
	for i, line := range visible {
		vis := lipgloss.Width(line)
		if vis < sbColW {
			line += strings.Repeat(" ", sbColW-vis)
		}
		var sb string
		if canScroll {
			sb = m.previewScrollbarChar(i, innerH, scroll, maxScroll, totalLines)
		} else {
			sb = " "
		}
		rendered[i] = line + sb
	}

	rightTitle := "<Tab> toggle"
	if canScroll {
		rightTitle = "<Tab> toggle · <J/K> scroll"
	}

	return m.renderPanel(strings.Join(rendered, "\n"), width, height,
		"Preview", rightTitle, m.theme.previewBorderColor(m.mode), m.theme.modalHintFg, m.theme.previewBg)
}

// inPreviewBody reports whether the terminal cell (x,y) is inside the preview
// pane's inner content area (i.e. not on the border).
func (m model) inPreviewBody(x, y int) bool {
	if !m.showPreview {
		return false
	}
	layout := m.layoutMetrics()
	if !layout.showPreview {
		return false
	}
	if layout.previewSide {
		left := layout.listWidth + 1 + 1 // gap + left border
		right := layout.listWidth + 1 + layout.previewW - 1
		return x >= left && x < right && y >= 1 && y < layout.previewH-1
	}
	// Bottom layout: listBox has listHeight+3 rows (top border + header + listHeight + bottom border)
	previewTop := layout.listHeight + 3 + 1 // 1 for "\n" separator + top border
	return x >= 1 && x < layout.previewW-1 &&
		y >= previewTop && y < previewTop+layout.previewH-1
}

func (m model) renderDeleteBox() string {
	var prompt string
	if len(m.selected) > 0 {
		prompt = fmt.Sprintf("Delete %d sessions?", len(m.selected))
	} else {
		if item := m.list.SelectedItem(); item != nil {
			if sess, ok := sessionFromItem(item); ok {
				prompt = fmt.Sprintf("Delete \"%s\"?", sess.Title)
			}
		}
		if prompt == "" {
			prompt = "Delete session?"
		} else {
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
		Width(width - 6).
		Align(lipgloss.Left).
		Render(wrapText(prompt, width-6))

	hint := m.buildHint([]hintPart{
		{"y", " confirm"},
		{"n", " cancel"},
	})
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
	body = lipgloss.NewStyle().Width(width - 6).Align(lipgloss.Center).Render(body)

	return m.renderModalBox(width, m.theme.modalBorder, "Delete", m.theme.modalBorder, body, "")
}

func (m model) renderConfirmNewSessionBox() string {
	prompt := "Create new session?"
	if m.pendingNewSessionDir != "" {
		prompt = fmt.Sprintf("Create new session in %s?", m.pendingNewSessionDir)
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
		Width(width - 6).
		Align(lipgloss.Left).
		Render(wrapText(prompt, width-6))

	hint := m.buildHint([]hintPart{
		{"y", " confirm"},
		{"n", " cancel"},
	})
	return m.renderModalBox(width, m.theme.accent, "New Session", m.theme.accent, body, hint)
}

func (m model) renderConfirmForkBox() string {
	prompt := "Duplicate session?"
	if m.pendingForkTitle != "" {
		prompt = fmt.Sprintf("Duplicate \"%s\"?", m.pendingForkTitle)
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
		Width(width - 6).
		Align(lipgloss.Left).
		Render(wrapText(prompt, width-6))

	hint := m.buildHint([]hintPart{
		{"y", " confirm"},
		{"n", " cancel"},
	})
	return m.renderModalBox(width, m.theme.accent, "Fork", m.theme.accent, body, hint)
}

func (m model) renderForkingBox() string {
	width := 48
	if m.width < 64 {
		width = m.width - 16
	}
	if width < 30 {
		width = 30
	}

	spin := m.spinner.View()
	body := lipgloss.JoinHorizontal(lipgloss.Center, spin, "  ", lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalPromptFg).Render("Duplicating..."))
	body = lipgloss.NewStyle().Width(width - 6).Align(lipgloss.Center).Render(body)

	return m.renderModalBox(width, m.theme.accent, "Fork", m.theme.accent, body, "")
}

func (m model) renderConfirmCloseTmuxBox() string {
	prompt := "Close tmux window?"
	if m.closeTmuxTitle != "" {
		prompt = fmt.Sprintf("Close tmux window for \"%s\"?", m.closeTmuxTitle)
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
		Width(width - 6).
		Align(lipgloss.Left).
		Render(wrapText(prompt, width-6))

	hint := m.buildHint([]hintPart{
		{"y", " confirm"},
		{"n", " cancel"},
	})
	return m.renderModalBox(width, m.theme.accent, "Close", m.theme.accent, body, hint)
}

func (m model) renderClosingTmuxBox() string {
	width := 48
	if m.width < 64 {
		width = m.width - 16
	}
	if width < 30 {
		width = 30
	}

	spin := m.spinner.View()
	body := lipgloss.JoinHorizontal(lipgloss.Center, spin, "  ", lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalPromptFg).Render("Closing window..."))
	body = lipgloss.NewStyle().Width(width - 6).Align(lipgloss.Center).Render(body)

	return m.renderModalBox(width, m.theme.accent, "Close", m.theme.accent, body, "")
}

func (m model) renderKeybindsBox() string {
	width := 52
	if m.width < 68 {
		width = m.width - 16
	}
	if width < 40 {
		width = 40
	}

	// inner width after border(2) + horizontal padding(4)
	innerW := width - 6
	keyColW := 14
	sepW := 1
	descMaxW := innerW - keyColW - sepW
	if descMaxW < 10 {
		descMaxW = 10
	}

	bg := m.theme.modalBg
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.accent).Background(bg).Width(keyColW)
	descStyle := lipgloss.NewStyle().Foreground(m.theme.modalPromptFg).Background(bg)
	sepStyle := lipgloss.NewStyle().Foreground(m.theme.textMuted).Background(bg)

	entries := keybindsEntries()

	var bodyLines []string
	for _, e := range entries {
		if e.key == "" && e.desc == "" {
			bodyLines = append(bodyLines, "")
			continue
		}
		if e.key == "" {
			bodyLines = append(bodyLines, sepStyle.Render(e.desc))
			continue
		}
		keyStr := e.key
		if len(keyStr) > keyColW {
			keyStr = keyStr[:keyColW]
		}
		descStr := e.desc
		if len(descStr) > descMaxW {
			descStr = descStr[:descMaxW-3] + "..."
		}

		keyBlock := keyStyle.Render(keyStr)
		line := keyBlock + descStyle.Render(" "+descStr)
		bodyLines = append(bodyLines, line)
	}

	// Modal at 80% of terminal height
	modalMaxH := m.height * 80 / 100
	if modalMaxH < 10 {
		modalMaxH = 10
	}
	// chrome = border(2) + pad(2) + badge(1) + hint(1) + blank spacer(2) = 8
	const chrome = 8
	maxBodyLines := modalMaxH - chrome
	if maxBodyLines < 4 {
		maxBodyLines = 4
	}

	scroll := m.keybindsScroll
	if scroll > len(bodyLines)-maxBodyLines {
		scroll = len(bodyLines) - maxBodyLines
	}
	if scroll < 0 {
		scroll = 0
	}
	if len(bodyLines) > maxBodyLines {
		bodyLines = bodyLines[scroll : scroll+maxBodyLines]
	}

	badgeView := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.modalBg).
		Background(m.theme.accent).
		Padding(0, 1).
		Render("KEYS")

	bodyStr := strings.Join(bodyLines, "\n")
	bodyView := lipgloss.NewStyle().
		Foreground(m.theme.textMain).
		Background(bg).
		Render(bodyStr)

	hintView := m.buildHint([]hintPart{
		{"j/k", " scroll"},
		{"esc", " close"},
	})

	content := badgeView + "\n\n" + bodyView + "\n\n" + hintView

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.accent).
		Background(bg).
		Padding(1, 2).
		Width(width).
		Render(content)
}

func (m model) renderRenameBox() string {
	item := m.list.SelectedItem()
	var title string
	if item != nil {
		if sess, ok := sessionFromItem(item); ok {
			title = sess.Title
		}
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

	badge := "Rename"
	prompt := "Set new session title"
	if m.forkMode {
		badge = "Fork"
		prompt = "Set forked session title"
	}

	body := lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalPromptFg).Render(prompt)
	if sub != "" {
		body += "\n" + sub
	}
	body += "\n\n" + field

	hint := m.buildHint([]hintPart{
		{"<CR>", " save"},
		{"esc", " cancel"},
	})
	return m.renderModalBox(boxWidth, m.theme.accent, badge, m.theme.accent, body, hint)
}

func (m model) renderDirpickerModal() string {
	width := m.dirpickerWidth()
	dpHeight := m.dirpickerHeight()

	dpView := m.dirpicker.View(m.theme, width, dpHeight)
	body := lipgloss.NewStyle().
		Width(width).
		Render(dpView)

	hint := m.buildHint([]hintPart{
		{"<CR>", " confirm"},
		{"/", " filter"},
		{"esc", " clear/close"},
		{".", " hidden"},
	})

	return m.renderModalBox(width, m.theme.accent, "New Session", m.theme.accent, body, hint)
}

func (m model) dirpickerHeight() int {
	h := m.height * 4 / 5
	if h < 12 {
		h = 12
	}
	if h > 26 {
		h = 26
	}
	return h
}

func (m model) dirpickerWidth() int {
	w := m.width * 3 / 5
	if w < 44 {
		w = 44
	}
	if w > 80 {
		w = 80
	}
	return w
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

func keybindsEntries() []struct{ key, desc string } {
	return []struct{ key, desc string }{
		{"/", "Filter"},
		{"<CR>", "Open session"},
		{"<M-CR>", "Open alternate mode"},
		{"t", "Toggle mode"},
		{"g", "Toggle groups"},
		{"space", "Fold/unfold group"},
		{"<C-space>", "Fold/unfold all groups"},
		{"h / l", "Collapse / expand group"},
		{"[ / ]", "Prev / next group"},
		{"n", "New session"},
		{"N", "New session (pick dir)"},
		{"r", "Rename"},
		{"y", "Duplicate session"},
		{"Y", "Duplicate (custom name)"},
		{"x", "Close tmux window (tmux only)"},
		{"d", "Delete mode"},
		{"tab", "Toggle preview"},
		{"J / K", "Scroll preview"},
		{"<C-n/p>", "Navigate lists"},
		{"q", "Quit"},
		{"?", "Keys"},
		{"", ""},
		{"", "--- delete mode ---"},
		{"space", "Toggle selection"},
		{"<CR>", "Confirm delete"},
		{"esc", "Cancel"},
		{"", ""},
		{"", "--- dir picker ---"},
		{"j / k", "Navigate"},
		{"h / l", "Parent / child directory"},
		{".", "Toggle hidden"},
		{"/", "Filter directories"},
		{"<CR>", "Select directory"},
		{"esc", "Back / close"},
		{"q", "Close"},
	}
}
