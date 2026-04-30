package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

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

func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		var line []rune
		var lineLen int
		for _, word := range strings.Fields(para) {
			wordRunes := []rune(word)
			wordLen := len(wordRunes)
			if lineLen > 0 {
				if lineLen+1+wordLen > width {
					lines = append(lines, string(line))
					line = nil
					lineLen = 0
				} else {
					line = append(line, ' ')
					lineLen++
				}
			}
			for wordLen > width {
				lines = append(lines, string(wordRunes[:width]))
				wordRunes = wordRunes[width:]
				wordLen = len(wordRunes)
			}
			line = append(line, wordRunes...)
			lineLen += wordLen
		}
		if lineLen > 0 {
			lines = append(lines, string(line))
		}
	}
	return strings.Join(lines, "\n")
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
