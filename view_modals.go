package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

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

func (m model) renderConfirmBox(badge string, borderColor lipgloss.Color, prompt string) string {
	width := m.modalWidth(48, 30, 16)
	body := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.modalPromptFg).
		Background(m.theme.modalBg).
		Width(width - 6).
		Align(lipgloss.Left).
		Render(wrapText(prompt, width-6))

	hint := m.buildHint([]hintPart{
		{"y", " confirm"},
		{"n", " cancel"},
	})
	return m.renderModalBox(width, borderColor, badge, borderColor, body, hint)
}

func (m model) renderSpinnerBox(badge string, borderColor lipgloss.Color, label string) string {
	width := m.modalWidth(48, 30, 16)
	spin := m.spinner.View()
	text := lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalPromptFg).Background(m.theme.modalBg).Render(label)
	body := lipgloss.JoinHorizontal(lipgloss.Center, spin, "  ", text)
	body = lipgloss.NewStyle().Background(m.theme.modalBg).Width(width - 6).Align(lipgloss.Center).Render(body)
	return m.renderModalBox(width, borderColor, badge, borderColor, body, "")
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
		}
	}
	return m.renderConfirmBox("Delete", m.theme.modalBorder, prompt)
}

func (m model) renderDeletingBox() string {
	return m.renderSpinnerBox("Delete", m.theme.modalBorder, "Deleting...")
}

func (m model) renderConfirmNewSessionBox() string {
	prompt := "Create new session?"
	if m.pendingNewSessionDir != "" {
		prompt = fmt.Sprintf("Create new session in %s?", m.pendingNewSessionDir)
	}
	return m.renderConfirmBox("New Session", m.theme.accent, prompt)
}

func (m model) renderConfirmForkBox() string {
	prompt := "Duplicate session?"
	if m.pendingForkTitle != "" {
		prompt = fmt.Sprintf("Duplicate \"%s\"?", m.pendingForkTitle)
	}
	return m.renderConfirmBox("Fork", m.theme.accent, prompt)
}

func (m model) renderForkingBox() string {
	return m.renderSpinnerBox("Fork", m.theme.accent, "Duplicating...")
}

func (m model) renderConfirmCloseTmuxBox() string {
	prompt := "Close tmux window?"
	if m.closeTmuxTitle != "" {
		prompt = fmt.Sprintf("Close tmux window for \"%s\"?", m.closeTmuxTitle)
	}
	return m.renderConfirmBox("Close", m.theme.accent, prompt)
}

func (m model) renderClosingTmuxBox() string {
	return m.renderSpinnerBox("Close", m.theme.accent, "Closing window...")
}

func (m model) renderKeybindsBox() string {
	width := m.modalWidth(52, 40, 16)

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
	modalMaxH := max(m.height*80/100, 10)
	// chrome = border(2) + pad(2) + badge(1) + hint(1) + blank spacer(2) = 8
	const chrome = 8
	maxBodyLines := max(modalMaxH-chrome, 4)

	scroll := max(0, min(m.keybindsScroll, len(bodyLines)-maxBodyLines))
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

	boxWidth := m.modalWidth(54, 34, 14)
	innerWidth := boxWidth - 8

	sub := ""
	if title != "" {
		sub = lipgloss.NewStyle().Foreground(m.theme.modalHintFg).Background(m.theme.modalBg).Render(truncate.StringWithTail(title, uint(innerWidth), "..."))
	}

	input := m.renameInput
	input.Width = innerWidth - 4
	inputView := input.View()
	field := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(m.theme.border).
		Padding(0, 1).
		Width(innerWidth).
		BorderRightBackground(m.theme.modalBg).
		Render(inputView)

	badge := "Rename"
	prompt := "Set new session title"
	if m.state == stateForkInput {
		badge = "Fork"
		prompt = "Set forked session title"
	}

	body := lipgloss.NewStyle().Bold(true).Foreground(m.theme.modalPromptFg).Background(m.theme.modalBg).Render(prompt)
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
	return min(max(m.height*4/5, 12), 26)
}

func (m model) dirpickerWidth() int {
	return min(max(m.width*3/5, 44), 80)
}
