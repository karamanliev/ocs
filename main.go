package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Updated   int64  `json:"updated"`
	Created   int64  `json:"created"`
	ProjectID string `json:"projectId"`
	Directory string `json:"directory"`
}

func main() {
	cmd := os.Getenv("AGENT_CMD")
	if cmd == "" {
		cmd = "ocv"
	}

	for {
		sessions, err := getSessions(cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching sessions: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Fprintln(os.Stderr, "No sessions found.")
			os.Exit(1)
		}

		lines := formatSessions(sessions)
		item, key, err := runFzf(lines)
		if err != nil {
			os.Exit(0)
		}
		if item == "" {
			os.Exit(0)
		}

		parts := strings.Split(item, "\t")
		if len(parts) < 4 {
			continue
		}
		id := parts[1]
		title := parts[2]

		switch key {
		case "ctrl-d":
			if askDelete(title) {
				deleteSession(cmd, id)
			}
		case "ctrl-t":
			dir := parts[3]
			ctrlTmux(agentPath, id, dir)
			os.Exit(0)
		default:
			resumeSession(cmd, id)
			os.Exit(0)
		}
	}
}

func getSessions(cmd string) ([]Session, error) {
	c := exec.Command(cmd, "session", "list", "--format", "json")
	out, err := c.Output()
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func formatSessions(sessions []Session) []string {
	now := time.Now()
	lines := make([]string, 0, len(sessions))
	for _, s := range sessions {
		updated := time.Unix(s.Updated/1000, (s.Updated%1000)*1e6)
		d := now.Sub(updated)
		ago := formatDuration(d)
		color := colorForDuration(d)
		lines = append(lines, fmt.Sprintf("%s%s\033[0m\t%s\t%s\t\033[90m%s\033[0m", color, ago, s.ID, s.Title, s.Directory))
	}
	return lines
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

func colorForDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "\033[1;31m" // bright red
	case d < time.Hour:
		return "\033[1;33m" // yellow
	case d < 24*time.Hour:
		return "\033[1;36m" // cyan
	default:
		return "\033[38;5;163m" // muted blue
	}
}

func runFzf(lines []string) (string, string, error) {
	args := []string{
		"--ansi",
		"--height", "40%",
		"--reverse",
		"--cycle",
		"--border-label", " opencode sessions ",
		"--border",
		"--prompt", "⚡  ",
		"--header", "\033[90m  ^d\033[0m delete  •  \033[90m^t\033[0m tmux  •  \033[90menter\033[0m resume  ",
		"--delimiter", "\t",
		"--with-nth", "1,3,4",
		"--expect=ctrl-d,ctrl-t",
	}

	cmd := exec.Command("fzf", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", "", err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", "", err
	}

	go func() {
		defer stdin.Close()
		w := bufio.NewWriter(stdin)
		for _, line := range lines {
			fmt.Fprintln(w, line)
		}
		w.Flush()
	}()

	out, err := io.ReadAll(stdout)
	if err != nil {
		return "", "", err
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return "", "", fmt.Errorf("cancelled")
		}
		return "", "", err
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return "", "", nil
	}

	idx := strings.Index(output, "\n")
	if idx >= 0 {
		return output[idx+1:], output[:idx], nil
	}
	return output, "", nil
}

func askDelete(title string) bool {
	fmt.Fprintf(os.Stderr, "\n\033[1;31m  ⚠  DELETE\033[0m  \033[1m%s\033[0m\n", title)
	fmt.Fprint(os.Stderr, "\033[90m     Confirm? \033[0m\033[1m[y/N]\033[0m \033[90m\033[0m")

	tty, err := os.Open("/dev/tty")
	if err != nil {
		tty = os.Stdin
	}
	defer tty.Close()

	fd := int(tty.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return false
	}
	defer term.Restore(fd, oldState)

	var b [1]byte
	_, _ = tty.Read(b[:])
	ch := b[0]

	confirmed := ch == 'y' || ch == 'Y'

	term.Restore(fd, oldState)
	fmt.Fprintln(os.Stderr)
	if confirmed {
		fmt.Fprintln(os.Stderr, "\033[1;31m     Deleted.\033[0m")
	} else {
		fmt.Fprintln(os.Stderr, "\033[90m     Cancelled.\033[0m")
	}
	fmt.Fprintln(os.Stderr)

	return confirmed
}

func ctrlTmux(agentPath, id, dir string) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: tmux not found in PATH")
		return
	}

	sessionName := filepath.Base(dir)
	if sessionName == "" || sessionName == "." || sessionName == "/" {
		sessionName = "default"
	}
	sessionName = strings.ReplaceAll(sessionName, "/", "-")
	sessionName = strings.ReplaceAll(sessionName, "\\", "-")

	exists := exec.Command(tmuxPath, "has-session", "-t", sessionName).Run() == nil
	if !exists {
		c := exec.Command(tmuxPath, "new-session", "-ds", sessionName, "-c", dir)
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating tmux session: %v\n", err)
			return
		}
	}

	c := exec.Command(tmuxPath, "new-window", "-t", sessionName, "-c", dir, agentPath, "-s", id)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
		return
	}

	if os.Getenv("TMUX") != "" {
		c := exec.Command(tmuxPath, "switch-client", "-t", sessionName)
		c.Stderr = os.Stderr
		_ = c.Run()
	} else {
		syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", sessionName}, os.Environ())
	}
}

func deleteSession(cmd, id string) {
	c := exec.Command(cmd, "session", "delete", id)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
}

func resumeSession(cmd, id string) {
	c := exec.Command(cmd, "-s", id)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
}
