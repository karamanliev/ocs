package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dirpicker struct {
	currentDir string
	homeDir    string
	allEntries []os.DirEntry
	entries    []os.DirEntry
	selected   int
	showHidden bool
	filtering  bool
	filterText string
	scroll     int
	height     int
}

type dirpickerRefreshMsg struct {
	entries []os.DirEntry
}

func newDirpicker(startDir string) dirpicker {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	if startDir == "" {
		startDir = home
	}
	return dirpicker{
		currentDir: startDir,
		homeDir:    home,
		showHidden: false,
	}
}

func (d *dirpicker) setListHeight(totalHeight int) {
	d.height = totalHeight - 3 // reserve 3 lines for path header and padding
	if d.height < 1 {
		d.height = 1
	}
}

func (d *dirpicker) readDir() tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(d.currentDir)
		if err != nil {
			return dirpickerRefreshMsg{entries: nil}
		}

		var dirs []os.DirEntry
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if !d.showHidden && strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dirs = append(dirs, e)
		}

		sort.Slice(dirs, func(i, j int) bool {
			return dirs[i].Name() < dirs[j].Name()
		})

		return dirpickerRefreshMsg{entries: dirs}
	}
}

func (d *dirpicker) applyFilter() {
	if d.filterText == "" {
		d.entries = d.allEntries
	} else {
		term := strings.ToLower(d.filterText)
		d.entries = nil
		for _, e := range d.allEntries {
			if strings.Contains(strings.ToLower(e.Name()), term) {
				d.entries = append(d.entries, e)
			}
		}
	}
	d.selected = 0
	d.scroll = 0
	d.ensureScroll()
}

func (d *dirpicker) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down", "ctrl+n":
			if len(d.entries) == 0 {
				break
			}
			d.selected++
			if d.selected >= len(d.entries) {
				d.selected = len(d.entries) - 1
			}
			d.ensureScroll()
		case "k", "up", "ctrl+p":
			if len(d.entries) == 0 {
				break
			}
			d.selected--
			if d.selected < 0 {
				d.selected = 0
			}
			d.ensureScroll()
		case "pgup":
			if len(d.entries) == 0 {
				break
			}
			d.selected -= d.height
			if d.selected < 0 {
				d.selected = 0
			}
			d.ensureScroll()
		case "pgdown":
			if len(d.entries) == 0 {
				break
			}
			d.selected += d.height
			if d.selected >= len(d.entries) {
				d.selected = len(d.entries) - 1
			}
			d.ensureScroll()
		case "g":
			if len(d.entries) == 0 {
				break
			}
			d.selected = 0
			d.scroll = 0
		case "G":
			if len(d.entries) == 0 {
				break
			}
			d.selected = len(d.entries) - 1
			d.ensureScroll()
		case "h", "left", "backspace":
			parent := filepath.Clean(filepath.Dir(d.currentDir))
			home := filepath.Clean(d.homeDir)
			if parent == d.currentDir {
				break // at filesystem root
			}
			rel, err := filepath.Rel(home, parent)
			if err != nil || strings.HasPrefix(rel, "..") {
				break // would go above home
			}
			d.currentDir = parent
			d.selected = 0
			d.scroll = 0
			d.filtering = false
			d.filterText = ""
			return d.readDir()
		case "l", "right":
			if d.selected < len(d.entries) {
				d.currentDir = filepath.Join(d.currentDir, d.entries[d.selected].Name())
				d.selected = 0
				d.scroll = 0
				d.filtering = false
				d.filterText = ""
				return d.readDir()
			}
		case ".":
			d.showHidden = !d.showHidden
			return d.readDir()
		}
	}
	return nil
}

func (d *dirpicker) ensureScroll() {
	if len(d.entries) == 0 {
		d.selected = 0
		d.scroll = 0
		return
	}
	if d.selected < 0 {
		d.selected = 0
	}
	if d.selected >= len(d.entries) {
		d.selected = len(d.entries) - 1
	}

	maxScroll := len(d.entries) - d.height
	if maxScroll < 0 {
		maxScroll = 0
	}

	if d.scroll > maxScroll {
		d.scroll = maxScroll
	}
	if d.selected < d.scroll {
		d.scroll = d.selected
	}
	if d.selected >= d.scroll+d.height {
		d.scroll = d.selected - d.height + 1
	}
	if d.scroll > maxScroll {
		d.scroll = maxScroll
	}
	if d.scroll < 0 {
		d.scroll = 0
	}
}

func (d *dirpicker) View(theme theme, width int, totalHeight int) string {
	d.setListHeight(totalHeight)
	listHeight := d.height

	var s strings.Builder

	pathStyle := lipgloss.NewStyle().Foreground(theme.dim).Bold(true)
	filterStyle := lipgloss.NewStyle().Foreground(theme.filterPrompt)

	s.WriteString("  " + pathStyle.Render(d.currentDir))
	s.WriteString("\n")

	if d.filtering {
		filterLine := "> " + d.filterText
		if d.filterText == "" {
			filterLine = "> "
		}
		s.WriteString("  " + filterStyle.Render(filterLine))
		s.WriteString("\n")
	} else {
		s.WriteString("\n")
	}

	if len(d.entries) == 0 {
		var msg string
		if len(d.allEntries) == 0 {
			msg = "(empty directory)"
		} else {
			msg = "(no matches)"
		}
		emptyStyle := lipgloss.NewStyle().Foreground(theme.dim).PaddingLeft(2)
		s.WriteString(emptyStyle.Render(msg))
		s.WriteString("\n")
		for i := 1; i < listHeight; i++ {
			s.WriteString("\n")
		}
		return s.String()
	}

	visibleEnd := d.scroll + listHeight
	if visibleEnd > len(d.entries) {
		visibleEnd = len(d.entries)
	}

	cursorStyle := lipgloss.NewStyle().Foreground(theme.accent).Bold(true)
	dirStyle := lipgloss.NewStyle().Foreground(theme.textMain)
	selDirStyle := lipgloss.NewStyle().Foreground(theme.textMain).Bold(true)
	matchStyle := lipgloss.NewStyle().Foreground(theme.filterMatch).Bold(true)

	for i := d.scroll; i < visibleEnd; i++ {
		name := d.entries[i].Name()
		prefix := "   "
		style := dirStyle

		if i == d.selected {
			prefix = cursorStyle.Render(" > ") + " "
			style = selDirStyle
		}

		// Highlight filter matches in directory names
		var nameRendered string
		if d.filterText != "" {
			nameRendered = highlightMatchDirName(name, d.filterText, style, matchStyle)
		} else {
			nameRendered = style.Render(name + "/")
		}

		line := prefix + nameRendered
		vis := lipgloss.Width(line)
		if vis < width {
			line += strings.Repeat(" ", width-vis)
		}
		s.WriteString(line)
		s.WriteString("\n")
	}

	for i := visibleEnd - d.scroll; i < listHeight; i++ {
		s.WriteString(strings.Repeat(" ", width))
		s.WriteString("\n")
	}

	return s.String()
}

func highlightMatchDirName(name, filter string, baseStyle, matchStyle lipgloss.Style) string {
	if filter == "" {
		return baseStyle.Render(name + "/")
	}
	lowerName := strings.ToLower(name)
	lowerFilter := strings.ToLower(filter)

	var parts []string
	i := 0
	for i < len(name) {
		idx := strings.Index(lowerName[i:], lowerFilter)
		if idx < 0 {
			parts = append(parts, baseStyle.Render(name[i:]))
			break
		}
		before := name[i : i+idx]
		if before != "" {
			parts = append(parts, baseStyle.Render(before))
		}
		matched := name[i+idx : i+idx+len(filter)]
		parts = append(parts, matchStyle.Render(matched))
		i = i + idx + len(filter)
	}
	parts = append(parts, baseStyle.Render("/"))
	return strings.Join(parts, "")
}
