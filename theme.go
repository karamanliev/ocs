package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

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
	filterMatch      lipgloss.Color
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
	filterMatch:      lipgloss.Color("#FF8C00"),
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
	filterMatch:      lipgloss.Color("#E65100"),
	accent:           lipgloss.Color("#2563EB"),
	textMain:         lipgloss.Color("#1F2937"),
	textMuted:        lipgloss.Color("#D1D5DB"),
	footerKeyAll:     lipgloss.Color("#5B7D9F"),
	footerKeyTmux:    lipgloss.Color("#7B6D9F"),
	footerLabel:      lipgloss.Color("#9CA3AF"),
	titleAllFg:       lipgloss.Color("#5B7D9F"),
	titleTmuxFg:      lipgloss.Color("#7B6D9F"),
}

var themeForDark = map[bool]theme{
	true:  darkTheme,
	false: lightTheme,
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

func (t theme) cursorBg(mode string) lipgloss.Color {
	if mode == "tmux" {
		return t.cursorBgTmux
	}
	return t.cursorBgAll
}

func (t theme) footerKeyColor(mode string) lipgloss.Color {
	if mode == "tmux" {
		return t.footerKeyTmux
	}
	return t.footerKeyAll
}

func (t theme) titleColor(mode string) lipgloss.Color {
	if mode == "tmux" {
		return t.titleTmuxFg
	}
	return t.titleAllFg
}

func (t theme) filterStyles() (prompt, cursor lipgloss.Style) {
	base := lipgloss.NewStyle().Foreground(t.filterPrompt)
	return base, base
}