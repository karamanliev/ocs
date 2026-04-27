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
	_ = c.Run()
}

func resumeSession(cmd, id, dir string) {
	c := exec.Command(cmd, "-s", id)
	if dir != "" {
		c.Dir = dir
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	_ = c.Run()
}

// findTmuxWindow locates a tmux window running opencode for the given session.
//
// Strategy (exact ID match only, never falls back to cwd):
//  1. Check panes tagged with @ocs_session_id by a previous ocs launch
//  2. Scan /proc for opencode processes launched with -s <id>, read TMUX_PANE
//     from their environment to map back to a tmux window
//
// Returns (tmuxSessionName, windowIndex, found).
func findTmuxWindow(tmuxPath, id string) (string, string, bool) {
	if id == "" {
		return "", "", false
	}

	// Step 1: check ocs pane tags
	out, err := exec.Command(tmuxPath, "list-panes", "-a",
		"-F", "#{pane_id} #{session_name} #{window_index} #{@ocs_session_id}").Output()
	if err != nil {
		return "", "", false
	}
	type paneInfo struct{ sessName, winIdx string }
	panes := make(map[string]paneInfo)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		panes[fields[0]] = paneInfo{fields[1], fields[2]}
		// Tagged pane with matching session ID
		if len(fields) == 4 && fields[3] == id {
			return fields[1], fields[2], true
		}
	}

	// Step 2: scan /proc for opencode -s <id>
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return "", "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		pid := e.Name()
		data, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", pid))
		if err != nil {
			continue
		}
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		if !isOpencodeCmdline(cmdline) {
			continue
		}
		if extractSessionIDFromCmdline(cmdline) != id {
			continue
		}
		environ, err := os.ReadFile(fmt.Sprintf("/proc/%s/environ", pid))
		if err != nil {
			continue
		}
		for _, env := range strings.Split(string(environ), "\x00") {
			if strings.HasPrefix(env, "TMUX_PANE=") {
				paneID := strings.TrimPrefix(env, "TMUX_PANE=")
				if info, ok := panes[paneID]; ok {
					return info.sessName, info.winIdx, true
				}
			}
		}
	}
	return "", "", false
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

func ctrlTmux(agentPath, id, dir string) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: tmux not found in PATH")
		return
	}

	// If this exact session is already in a tmux window, focus it.
	if targetSess, winIdx, found := findTmuxWindow(tmuxPath, id); found {
		_ = exec.Command(tmuxPath, "select-window", "-t", targetSess+":"+winIdx).Run()
		attachTmux(tmuxPath, targetSess)
		return
	}

	// Not found: create a window in a session named after the directory.
	sessionName := filepath.Base(dir)
	if sessionName == "" || sessionName == "." || sessionName == "/" {
		sessionName = "default"
	}
	sessionName = strings.ReplaceAll(sessionName, "/", "-")
	sessionName = strings.ReplaceAll(sessionName, "\\", "-")

	if exec.Command(tmuxPath, "has-session", "-t", sessionName).Run() != nil {
		c := exec.Command(tmuxPath, "new-session", "-ds", sessionName, "-c", dir)
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating tmux session: %v\n", err)
			return
		}
	}

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
	winTarget := fmt.Sprintf("%s:%d", sessionName, maxIdx+1)
	c := exec.Command(tmuxPath, "new-window", "-t", winTarget, "-c", dir, "--", agentPath, "-s", id)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
		return
	}

	// Tag the new pane so we can find it later without /proc scanning.
	paneOut, _ := exec.Command(tmuxPath, "list-panes", "-t", winTarget, "-F", "#{pane_id}").Output()
	if paneID := strings.TrimSpace(string(paneOut)); paneID != "" {
		_ = exec.Command(tmuxPath, "set-option", "-p", "-t", paneID, "@ocs_session_id", id).Run()
	}

	attachTmux(tmuxPath, sessionName)
}