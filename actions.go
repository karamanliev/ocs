package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func deleteSession(cmd, id string) {
	c := exec.Command(cmd, "session", "delete", id)
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ocs: failed to delete session %s: %v\n", id, err)
	}
}

func resumeSession(cmd, id, dir string) {
	c := exec.Command(cmd, "-s", id)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ocs: failed to resume session %s: %v\n", id, err)
	}
}

func newSessionInDir(cmd, dir string) {
	c := exec.Command(cmd)
	c.Dir = dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ocs: failed to start new session: %v\n", err)
	}
}

func sanitizeTmuxSessionName(name string) string {
	if name == "" || name == "." || name == "/" {
		return "default"
	}
	r := strings.NewReplacer("/", "_", "\\", "_", ".", "_", ":", "_")
	return r.Replace(name)
}

func ensureTmuxSession(tmuxPath, sessionName, dir string) error {
	if exec.Command(tmuxPath, "has-session", "-t", sessionName).Run() == nil {
		return nil
	}
	c := exec.Command(tmuxPath, "new-session", "-ds", sessionName, "-c", dir)
	c.Stderr = os.Stderr
	return c.Run()
}

func nextTmuxWindowTarget(tmuxPath, sessionName string) string {
	out, _ := exec.Command(tmuxPath, "list-windows", "-t", sessionName, "-F", "#{window_index}").Output()
	maxIdx := -1
	for _, s := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if n, err := strconv.Atoi(s); err == nil && n > maxIdx {
			maxIdx = n
		}
	}
	return fmt.Sprintf("%s:%d", sessionName, maxIdx+1)
}

func attachTmux(tmuxPath, sessionName string) {
	if os.Getenv("TMUX") != "" {
		c := exec.Command(tmuxPath, "switch-client", "-t", sessionName)
		c.Stderr = os.Stderr
		_ = c.Run()
	} else {
		syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", sessionName}, os.Environ())
	}
}

func killTmuxWindow(tmuxPath, sessionName, winIdx string) error {
	return exec.Command(tmuxPath, "kill-window", "-t", sessionName+":"+winIdx).Run()
}

func tmuxWindowName(title string) string {
	const max = 10
	if len(title) > max {
		title = strings.TrimRight(title[:max], " \t") + "..."
	}
	return title
}

func ctrlTmux(agentPath, id, dir, title string, sessions []Session) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: tmux not found in PATH")
		return
	}

	if targetSess, winIdx, found := findTmuxWindow(tmuxPath, id, sessions); found {
		if err := exec.Command(tmuxPath, "select-window", "-t", targetSess+":"+winIdx).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "ocs: select-window failed: %v\n", err)
		}
		attachTmux(tmuxPath, targetSess)
		return
	}

	sessionName := sanitizeTmuxSessionName(filepath.Base(dir))
	if err := ensureTmuxSession(tmuxPath, sessionName, dir); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux session: %v\n", err)
		return
	}

	winTarget := nextTmuxWindowTarget(tmuxPath, sessionName)
	c := exec.Command(tmuxPath, "new-window", "-t", winTarget, "-n", tmuxWindowName(title), "-c", dir, "--", agentPath, "-s", id)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
		return
	}

	paneOut, _ := exec.Command(tmuxPath, "list-panes", "-t", winTarget, "-F", "#{pane_id}").Output()
	if paneID := strings.TrimSpace(string(paneOut)); paneID != "" {
		if err := exec.Command(tmuxPath, "set-option", "-p", "-t", paneID, "@ocs_session_id", id).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "ocs: failed to tag pane: %v\n", err)
		}
	}

	attachTmux(tmuxPath, sessionName)
}

func ctrlTmuxNew(agentPath, dir string) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: tmux not found in PATH")
		return
	}

	sessionName := sanitizeTmuxSessionName(filepath.Base(dir))
	if err := ensureTmuxSession(tmuxPath, sessionName, dir); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux session: %v\n", err)
		return
	}

	winTarget := nextTmuxWindowTarget(tmuxPath, sessionName)
	c := exec.Command(tmuxPath, "new-window", "-t", winTarget, "-n", "opencode", "-c", dir, "--", agentPath)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
		return
	}

	attachTmux(tmuxPath, sessionName)
}
