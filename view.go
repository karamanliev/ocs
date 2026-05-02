package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

	switch m.state {
	case stateForking:
		out = m.renderOverlay(out, m.renderForkingBox(), m.theme.modalBg, true)
	case stateClosingTmux:
		out = m.renderOverlay(out, m.renderClosingTmuxBox(), m.theme.modalBg, true)
	case stateDeleting:
		out = m.renderOverlay(out, m.renderDeletingBox(), m.theme.modalBg, true)
	case stateConfirmingDelete:
		out = m.renderOverlay(out, m.renderDeleteBox(), m.theme.modalBg, true)
	case stateConfirmingFork:
		out = m.renderOverlay(out, m.renderConfirmForkBox(), m.theme.modalBg, true)
	case stateConfirmingNewSession:
		out = m.renderOverlay(out, m.renderConfirmNewSessionBox(), m.theme.modalBg, true)
	case stateConfirmingCloseTmux:
		out = m.renderOverlay(out, m.renderConfirmCloseTmuxBox(), m.theme.modalBg, true)
	}

	if m.state == stateRenameInput || m.state == stateForkInput {
		out = m.renderOverlay(out, m.renderRenameBox(), m.theme.modalBg, true)
	}

	if m.state == stateFilepicker {
		out = m.renderOverlay(out, m.renderDirpickerModal(), m.theme.modalBg, true)
	}

	if m.state == stateShowKeybinds {
		out = m.renderOverlay(out, m.renderKeybindsBox(), m.theme.modalBg, true)
	}

	return out
}

func (m model) modalWidth(preferred, min, margin int) int {
	w := preferred
	if m.width < margin+preferred {
		w = m.width - margin
	}
	if w < min {
		w = min
	}
	return w
}

func (m model) layoutMetrics() layoutMetrics {
	width := max(m.width, 20)
	height := max(m.height, 8)

	layout := layoutMetrics{
		showPreview: m.showPreview,
		listWidth:   width,
		listHeight:  max(height-4, 5),
	}
	if !m.showPreview {
		return layout
	}

	const gap = 1
	const minListOuterW = 52
	const minPreviewOuterW = 36
	const sideThreshold = 132

	if width >= sideThreshold && width >= height*2 {
		previewW := max(width*38/100, minPreviewOuterW)
		previewW = min(previewW, width-minListOuterW-gap)
		listW := width - previewW - gap
		if listW >= minListOuterW && previewW >= minPreviewOuterW {
			layout.previewSide = true
			layout.listWidth = listW
			layout.previewW = previewW
			layout.previewH = max(height-1, 6)
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
	previewH = max(previewH, minPreviewH)
	previewH = min(previewH, maxPreviewHLimit)
	maxPreviewH := max(height-10, 4)
	previewH = min(previewH, maxPreviewH)
	listHeight := height - 4 - gap - previewH
	if listHeight < 5 {
		listHeight = 5
		previewH = max(height-10, 4)
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
	usable := max(m.width, 10) - 2

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
	} else if m.state == stateDeleteMode {
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
	{
		var linkedCount, detectedCount int
		for _, st := range m.states {
			switch st {
			case stateLinked:
				linkedCount++
			case stateDetected:
				detectedCount++
			}
		}
		if m.mode == "tmux" {
			var parts []string
			if linkedCount > 0 {
				dot := lipgloss.NewStyle().Foreground(m.theme.indicatorRunning).Render("●")
				parts = append(parts, dot+labelStyle.Render(fmt.Sprintf(" %d in tmux", linkedCount)))
			}
			if detectedCount > 0 {
				dot := lipgloss.NewStyle().Foreground(m.theme.indicatorActive).Render("○")
				parts = append(parts, dot+labelStyle.Render(fmt.Sprintf(" %d unknown", detectedCount)))
			}
			rightStyled = strings.Join(parts, "  ")
		} else {
			total := linkedCount + detectedCount
			if total > 0 {
				dot := lipgloss.NewStyle().Foreground(m.theme.indicatorRunning).Render("●")
				rightStyled = dot + labelStyle.Render(fmt.Sprintf(" %d active", total))
			}
		}
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(rightStyled)
	gap := max(usable-leftW-rightW, 1)

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
	width = max(width, 4)
	innerW := width - 2
	lines := strings.Split(content, "\n")
	if height > 0 {
		innerH := max(height-2, 0)
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

	midW := max(innerW-leftW-rightW, 0)
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
	topPad := max((m.height-boxH)/2, 0)

	boxWidth := 0
	for _, l := range boxLines {
		if w := lipgloss.Width(l); w > boxWidth {
			boxWidth = w
		}
	}

	leftPad := max((m.width-boxWidth)/2, 0)

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
