package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func parseBoolArg(name string, value string) bool {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocs: %s must be true or false\n", name)
		os.Exit(1)
	}
	return parsed
}

func parseOptionalBoolFlag(args []string, i *int, name string) bool {
	arg := args[*i]
	if arg == name {
		if *i+1 < len(args) {
			next := args[*i+1]
			if next == "true" || next == "false" {
				*i++
				return parseBoolArg(name, next)
			}
		}
		return true
	}
	if strings.HasPrefix(arg, name+"=") {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			return parseBoolArg(name, parts[1])
		}
	}
	return false
}

func parseArgs() (startTmux, noPreview, grouped bool, theme string) {
	preview := true
	grouped = true
	tmuxExplicit := false
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--tmux" || strings.HasPrefix(arg, "--tmux="):
			startTmux = parseOptionalBoolFlag(os.Args, &i, "--tmux")
			tmuxExplicit = true
		case arg == "--preview" || strings.HasPrefix(arg, "--preview="):
			preview = parseOptionalBoolFlag(os.Args, &i, "--preview")
		case arg == "--grouped" || strings.HasPrefix(arg, "--grouped="):
			grouped = parseOptionalBoolFlag(os.Args, &i, "--grouped")
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
			fmt.Fprintln(os.Stderr, "  --tmux=bool     start in tmux mode, auto-detected when inside tmux")
			fmt.Fprintln(os.Stderr, "  --preview=bool  show preview pane, default true")
			fmt.Fprintln(os.Stderr, "  --grouped=bool  group by path, default true")
			fmt.Fprintln(os.Stderr, "  --theme value  force theme: dark or light (default auto-detect)")
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "ocs: unknown flag %s\n", arg)
			os.Exit(1)
		}
	}
	if !tmuxExplicit && os.Getenv("TMUX") != "" {
		startTmux = true
	}
	noPreview = !preview
	return
}

func main() {
	startTmux, noPreview, grouped, theme := parseArgs()

	m, err := newModel(startTmux, noPreview, grouped, theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ocs: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus(), tea.WithMouseCellMotion())

	m.dbWatcher = newDBWatcher(m.dbPath, func(msg tea.Msg) { p.Send(msg) })
	defer m.dbWatcher.close()

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

	if fm.actionNewSession {
		if fm.actionTmux {
			ctrlTmuxNew(fm.agentPath, fm.actionDir)
		} else {
			newSessionInDir(fm.agentPath, fm.actionDir)
		}
	} else if fm.actionID != "" {
		if fm.actionTmux {
			ctrlTmux(fm.agentPath, fm.actionID, fm.actionDir, fm.actionTitle, fm.sessions)
		} else {
			resumeSession(fm.agentPath, fm.actionID, fm.actionDir)
		}
	}
}
