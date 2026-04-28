package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func parseArgs() (startTmux, noPreview bool, theme string) {
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--tmux":
			startTmux = true
		case arg == "--no-preview":
			noPreview = true
		case arg == "--theme":
			i++
			if i >= len(os.Args) {
				fmt.Fprintln(os.Stderr, "ocs: --theme requires a value")
				os.Exit(1)
			}
			theme = os.Args[i]
		case strings.HasPrefix(arg, "--theme="):
			theme = strings.SplitN(arg, "=", 2)[1]
		case arg == "--help":
			fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
			fmt.Fprintln(os.Stderr, "  --tmux         start in tmux mode")
			fmt.Fprintln(os.Stderr, "  --no-preview   start with preview pane hidden")
			fmt.Fprintln(os.Stderr, "  --theme value  force theme: dark or light (default auto-detect)")
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "ocs: unknown flag %s\n", arg)
			os.Exit(1)
		}
	}
	return
}

func main() {
	startTmux, noPreview, theme := parseArgs()

	m, err := newModel(startTmux, noPreview, theme)
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
