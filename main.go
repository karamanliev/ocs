package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	var startTmux bool
	var noPreview bool
	flag.BoolVar(&startTmux, "tmux", false, "start in tmux mode")
	flag.BoolVar(&noPreview, "no-preview", false, "start with preview pane hidden")
	flag.Parse()

	m, err := newModel(startTmux, noPreview)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocs: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocs: %v\n", err)
		os.Exit(1)
	}

	var fm model
	switch v := finalModel.(type) {
	case model:
		fm = v
	case *model:
		fm = *v
	default:
		os.Exit(0)
	}

	if fm.actionID != "" {
		if fm.actionTmux {
			ctrlTmux(fm.agentPath, fm.actionID, fm.actionDir, fm.actionTitle)
		} else {
			resumeSession(fm.agentPath, fm.actionID, fm.actionDir)
		}
	}
}