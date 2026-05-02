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

// findTmuxWindow locates a tmux window running opencode for the given session.
//
// Strategy:
//  1. Unique non-truncated pane title wins and can repair stale tags
//  2. Ambiguous titles use exact proc -s first, then exact tag
//  3. Bare panes use exact proc -s first, then exact tag
//  4. /proc fallback handles exact -s matches and cwd guesses
//
// Returns (tmuxSessionName, windowIndex, found).
func findTmuxWindow(tmuxPath, id string, sessions []Session) (string, string, bool) {
	if id == "" {
		return "", "", false
	}

	panes, err := listTmuxPanes(tmuxPath)
	if err != nil {
		return "", "", false
	}

	// Find target session for title + directory matching.
	var targetSess Session
	for _, s := range sessions {
		if s.ID == id {
			targetSess = s
			break
		}
	}

	procPanes := listProcPanes()

	// Detection priority per pane:
	//  1. Non-truncated unique title: authoritative, ignore tag (may be stale)
	//  2. Truncated/duplicate title (ambiguous): prefer exact proc -s, then tag
	//  3. No parseable title (bare "OpenCode"): exact proc -s, then tag
	var procAmbiguousMatch *tmuxPaneInfo
	var tagAmbiguousMatch *tmuxPaneInfo
	var procNoTitleMatch *tmuxPaneInfo
	var tagNoTitleMatch *tmuxPaneInfo
	for i, p := range panes {
		if p.paneCommand != "node" {
			continue
		}
		proc := procPanes[p.paneID]
		title := resolvePaneTitle(p.paneTitle, p.sessName, sessions)

		if matchedID, ok := title.uniqueSessionID(); ok && matchedID == id {
			if p.ocsTag != id {
				_ = exec.Command(tmuxPath, "set-option", "-p", "-t", p.paneID, "@ocs_session_id", id).Run()
			}
			return p.sessName, p.winIdx, true
		}

		if title.parseable {
			if proc.sessionID == id && title.hasSessionID(id) && procAmbiguousMatch == nil {
				procAmbiguousMatch = &panes[i]
			}
			if p.ocsTag == id && title.hasSessionID(id) && tagAmbiguousMatch == nil {
				tagAmbiguousMatch = &panes[i]
			}
			continue
		}

		if proc.sessionID == id && procNoTitleMatch == nil {
			procNoTitleMatch = &panes[i]
		}
		if p.ocsTag == id && tagNoTitleMatch == nil {
			tagNoTitleMatch = &panes[i]
		}
	}
	for _, match := range []*tmuxPaneInfo{procAmbiguousMatch, tagAmbiguousMatch, procNoTitleMatch, tagNoTitleMatch} {
		if match != nil {
			return match.sessName, match.winIdx, true
		}
	}

	// Step 3: /proc scan for opencode -s <id> or cwd fallback
	paneMap := make(map[string]tmuxPaneInfo, len(panes))
	for _, p := range panes {
		paneMap[p.paneID] = p
	}

	// For cwd fallback, only match if the target session is the most recently
	// updated in its directory. This prevents sessions with no indicator from
	// stealing focus from the actually-running session's window.
	isMostRecent := true
	for _, s := range sessions {
		if s.Directory == targetSess.Directory && s.Updated > targetSess.Updated {
			isMostRecent = false
			break
		}
	}

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

		// Check -s <id> match or cwd match
		cmdID := extractSessionIDFromCmdline(cmdline)
		cwdMatch := false
		if cmdID != id {
			// Skip cwd fallback if this session isn't the most recent in its dir.
			if !isMostRecent {
				continue
			}
			// Try cwd fallback: if this process's cwd matches the target session's directory
			procDir, err := os.Readlink(fmt.Sprintf("/proc/%s/cwd", pid))
			if err != nil || procDir != targetSess.Directory {
				continue
			}
			cwdMatch = true
		}

		// Read TMUX_PANE from environment to map back to a tmux window
		environ, err := os.ReadFile(fmt.Sprintf("/proc/%s/environ", pid))
		if err != nil {
			continue
		}
		for _, env := range strings.Split(string(environ), "\x00") {
			if strings.HasPrefix(env, "TMUX_PANE=") {
				paneID := strings.TrimPrefix(env, "TMUX_PANE=")
				if info, ok := paneMap[paneID]; ok {
					// Tag the pane if this was an exact -s match (not cwd guess)
					if !cwdMatch {
						_ = exec.Command(tmuxPath, "set-option", "-p", "-t", paneID, "@ocs_session_id", id).Run()
					}
					return info.sessName, info.winIdx, true
				}
			}
		}
	}

	return "", "", false
}

// sanitizeTmuxSessionName replaces characters that tmux uses as target
// separators (. : / \) with underscores so that session names work safely
// in target specifications like "session:window.pane".
func sanitizeTmuxSessionName(name string) string {
	if name == "" || name == "." || name == "/" {
		return "default"
	}
	r := strings.NewReplacer("/", "_", "\\", "_", ".", "_", ":", "_")
	return r.Replace(name)
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

	// If this exact session is already in a tmux window, focus it.
	if targetSess, winIdx, found := findTmuxWindow(tmuxPath, id, sessions); found {
		if err := exec.Command(tmuxPath, "select-window", "-t", targetSess+":"+winIdx).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "ocs: select-window failed: %v\n", err)
		}
		attachTmux(tmuxPath, targetSess)
		return
	}

	// Not found: create a window in a session named after the directory.
	sessionName := sanitizeTmuxSessionName(filepath.Base(dir))

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
	c := exec.Command(tmuxPath, "new-window", "-t", winTarget, "-n", tmuxWindowName(title), "-c", dir, "--", agentPath, "-s", id)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
		return
	}

	// Tag the new pane so we can find it later without /proc scanning.
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
	c := exec.Command(tmuxPath, "new-window", "-t", winTarget, "-n", "opencode", "-c", dir, "--", agentPath)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating tmux window: %v\n", err)
		return
	}

	attachTmux(tmuxPath, sessionName)
}
