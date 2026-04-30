package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

func (m model) previewDivider(label string, contentW int, fg lipgloss.Color) string {
	prefix := "── " + label + " "
	dashes := max(contentW-len(prefix), 2)
	return "  " + lipgloss.NewStyle().Foreground(fg).Render(prefix+strings.Repeat("─", dashes))
}

func (m model) previewScrollbarChar(lineIdx, viewH, scroll, maxScroll, totalLines int) string {
	if totalLines <= 0 {
		return " "
	}
	thumbH := max(viewH*viewH/totalLines, 1)
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

func (m model) previewContent(item any, contentW int) (header []string, body []string) {
	padLeft := "  "
	header = append(header, "")
	if item == nil {
		header = append(header, padLeft+lipgloss.NewStyle().Foreground(m.theme.dim).Render("No session selected."))
		return
	}
	if sess, ok := sessionFromItem(item); ok {
		header = append(header, padLeft+lipgloss.NewStyle().Bold(true).Foreground(m.theme.previewTitleFg).Render(
			truncate.StringWithTail(sess.Title, uint(contentW), "...")))
		pathStr := displayPath(sess.Directory)
		if isWorktree(sess.Directory, sess.Worktree) {
			wtName := filepath.Base(sess.Worktree)
			arrow := lipgloss.NewStyle().Foreground(m.theme.indicatorRunning).Render("↗")
			pathStr += " [" + arrow + " " + wtName + "]"
		}
		header = append(header, padLeft+lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Render(
			truncate.StringWithTail(pathStr, uint(contentW), "...")))
		header = append(header, "")

		cached, ok := m.firstMsgs[sess.ID]
		if !ok {
			body = append(body, padLeft+lipgloss.NewStyle().Foreground(m.theme.dim).Render("Loading..."))
			return
		}
		body = m.buildPreviewLines(cached, contentW)
		return
	}
	if groupHeader, ok := item.(groupHeaderItem); ok {
		header = append(header, padLeft+lipgloss.NewStyle().Bold(true).Foreground(m.theme.previewTitleFg).Render(
			truncate.StringWithTail(displayPath(groupHeader.path), uint(contentW), "...")))
		header = append(header, "")
		body = append(body,
			padLeft+lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Render(fmt.Sprintf("%d sessions in group", groupHeader.count)),
			"",
			padLeft+lipgloss.NewStyle().Foreground(m.theme.dim).Render("Press space to fold or unfold."),
		)
		return
	}
	return
}

func (m model) renderPreviewPane(width int, height int) string {
	innerW := max(width-2, 10)
	innerH := max(height-2, 2)
	// Reserve 1 col for scrollbar (always, to avoid reflow when it appears)
	contentW := max(innerW-5, 6)

	header, body := m.previewContent(m.list.SelectedItem(), contentW)
	allLines := append(header, body...)

	// Scroll logic
	totalLines := len(allLines)
	canScroll := totalLines > innerH
	maxScroll := 0
	if canScroll {
		maxScroll = totalLines - innerH
	}
	scroll := max(m.previewScroll, 0)
	scroll = min(scroll, maxScroll)

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
