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

func findTmuxWindowWithSession(tmuxPath, sessionName, id string) (string, bool) {
	out, err := exec.Command(tmuxPath, "list-panes", "-t", sessionName, "-F", "#{window_index} #{pane_pid}").Output()
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		winIdx, pid := fields[0], fields[1]
		data, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", pid))
		if err != nil {
			continue
		}
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		if strings.Contains(cmdline, "opencode") && strings.Contains(cmdline, id) {
			return winIdx, true
		}
	}
	return "", false
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

	if winIdx, found := findTmuxWindowWithSession(tmuxPath, sessionName, id); found {
		c := exec.Command(tmuxPath, "select-window", "-t", sessionName+":"+winIdx)
		c.Stderr = os.Stderr
		_ = c.Run()
	} else {
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
		c := exec.Command(tmuxPath, "new-window", "-t", fmt.Sprintf("%s:%d", sessionName, maxIdx+1), "-c", dir, "--", agentPath, "-s", id)
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
			return
		}
	}

	if os.Getenv("TMUX") != "" {
		c := exec.Command(tmuxPath, "switch-client", "-t", sessionName)
		c.Stderr = os.Stderr
		_ = c.Run()
	} else {
		syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", sessionName}, os.Environ())
	}
}