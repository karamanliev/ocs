package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Tmux pane metadata
// ---------------------------------------------------------------------------

// tmuxPane holds parsed data from a single tmux pane including its process ID.
type tmuxPane struct {
	paneID      string
	panePID     int
	sessName    string
	winIdx      string
	ocsTag      string // @ocs_session_id, may be empty or stale
	paneTitle   string
	paneCommand string
}

// listTmuxPanes queries all tmux panes in a single call.
func listTmuxPanes(tmuxPath string) ([]tmuxPane, error) {
	const sep = "\x7f"
	format := strings.Join([]string{
		"#{pane_id}",
		"#{pane_pid}",
		"#{session_name}",
		"#{window_index}",
		"#{@ocs_session_id}",
		"#{pane_title}",
		"#{pane_current_command}",
	}, sep)
	out, err := exec.Command(tmuxPath, "list-panes", "-a", "-F", format).Output()
	if err != nil {
		return nil, err
	}

	var panes []tmuxPane
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, sep)
		if len(fields) < 7 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		panes = append(panes, tmuxPane{
			paneID:      fields[0],
			panePID:     pid,
			sessName:    fields[2],
			winIdx:      fields[3],
			ocsTag:      fields[4],
			paneTitle:   fields[5],
			paneCommand: fields[6],
		})
	}
	return panes, nil
}

// ---------------------------------------------------------------------------
// Process table: built once from /proc, queried per pane via pane_pid
// ---------------------------------------------------------------------------

type procEntry struct {
	pid     int
	ppid    int
	cmdline string // null bytes replaced with spaces
	cwd     string
}

// procTable maps pid -> procEntry and ppid -> list of child pids.
type procTable struct {
	byPID    map[int]procEntry
	children map[int][]int
}

// buildProcTable scans /proc once and builds a complete process table.
func buildProcTable() procTable {
	pt := procTable{
		byPID:    make(map[int]procEntry),
		children: make(map[int][]int),
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return pt
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		pe := procEntry{pid: pid}

		// Read ppid from /proc/pid/stat (field 4).
		if stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			pe.ppid = parsePPID(string(stat))
		}

		if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
			pe.cmdline = strings.ReplaceAll(string(data), "\x00", " ")
		}

		if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid)); err == nil {
			pe.cwd = cwd
		}

		pt.byPID[pid] = pe
		pt.children[pe.ppid] = append(pt.children[pe.ppid], pid)
	}
	return pt
}

// parsePPID extracts the parent PID from /proc/pid/stat content.
// The stat line format is: pid (comm) state ppid ...
// We must handle comm containing spaces/parens by finding the last ')'.
func parsePPID(stat string) int {
	idx := strings.LastIndex(stat, ")")
	if idx < 0 || idx+2 >= len(stat) {
		return 0
	}
	rest := strings.TrimSpace(stat[idx+1:])
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(fields[1])
	return ppid
}

// paneProc holds the opencode process info found for a specific pane.
type paneProc struct {
	sessionID string // from -s <id>, may be empty
	cwd       string
}

// findPaneProc walks descendants of panePID to find a live opencode process.
func (pt procTable) findPaneProc(panePID int) paneProc {
	var result paneProc
	var walk func(pid int)
	walk = func(pid int) {
		pe, ok := pt.byPID[pid]
		if !ok {
			return
		}
		if isOpencodeCmdline(pe.cmdline) {
			if result.sessionID == "" {
				result.sessionID = extractSessionIDFromCmdline(pe.cmdline)
			}
			if result.cwd == "" {
				result.cwd = pe.cwd
			}
		}
		for _, child := range pt.children[pid] {
			walk(child)
		}
	}
	walk(panePID)
	return result
}

// ---------------------------------------------------------------------------
// Title resolution
// ---------------------------------------------------------------------------

const paneTitlePrefix = "OC | "

// parsePaneTitle extracts the session title from a tmux pane title.
func parsePaneTitle(paneTitle string) (string, bool) {
	if !strings.HasPrefix(paneTitle, paneTitlePrefix) {
		return "", false
	}
	title := paneTitle[len(paneTitlePrefix):]
	return title, title != ""
}

// titleMatch holds the result of matching a pane title to known sessions.
type titleMatch struct {
	parseable  bool
	candidates []Session
}

func (m titleMatch) isAuthoritative() bool {
	return m.parseable && len(m.candidates) == 1
}

func (m titleMatch) uniqueID() (string, bool) {
	if m.isAuthoritative() {
		return m.candidates[0].ID, true
	}
	return "", false
}

func (m titleMatch) hasID(id string) bool {
	for _, s := range m.candidates {
		if s.ID == id {
			return true
		}
	}
	return false
}

// resolveTitle matches a tmux pane title against the session list.
// Exactly one candidate (even truncated) is considered authoritative.
// Multiple candidates are narrowed by tmux session basename but remain
// ambiguous if more than one survives.
func resolveTitle(paneTitle, tmuxSessName string, sessions []Session) titleMatch {
	title, ok := parsePaneTitle(paneTitle)
	if !ok {
		return titleMatch{}
	}

	match := titleMatch{parseable: true}
	truncated := strings.HasSuffix(title, "...")
	if truncated {
		title = strings.TrimSuffix(title, "...")
	}

	for _, s := range sessions {
		if truncated {
			if strings.HasPrefix(s.Title, title) {
				match.candidates = append(match.candidates, s)
			}
		} else if s.Title == title {
			match.candidates = append(match.candidates, s)
		}
	}

	if len(match.candidates) <= 1 {
		return match
	}

	// Narrow by tmux session basename.
	var narrowed []Session
	for _, s := range match.candidates {
		if filepath.Base(s.Directory) == tmuxSessName {
			narrowed = append(narrowed, s)
		}
	}
	if len(narrowed) > 0 {
		match.candidates = narrowed
	}
	return match
}

// ---------------------------------------------------------------------------
// Pane-centric resolver: the single source of truth
// ---------------------------------------------------------------------------

// resolvedPane is the output of resolving a single tmux pane to a session.
type resolvedPane struct {
	pane      tmuxPane
	sessionID string
	method    string // "title", "proc", "tag", "cwd", ""
}

// resolvePanes runs the full pane-centric resolution pipeline.
//
// For each pane with pane_current_command == "node":
//  1. Inspect descendants of pane_pid for a live opencode process (proc -s, cwd)
//  2. Resolve title candidates
//  3. Apply priority:
//     a. Authoritative title (exactly one candidate): trust title
//     b. Ambiguous title: trust proc -s if it is one of the candidates
//     c. Ambiguous title: trust tag if it is one of the candidates
//     d. Bare pane (no parseable title): proc -s, then tag, then cwd
func resolvePanes(panes []tmuxPane, pt procTable, sessions []Session) []resolvedPane {
	dirToMostRecent := make(map[string]string, len(sessions))
	dirSessionCount := make(map[string]int, len(sessions))
	for _, s := range sessions {
		dirSessionCount[s.Directory]++
		if _, exists := dirToMostRecent[s.Directory]; !exists {
			dirToMostRecent[s.Directory] = s.ID
		}
	}

	var resolved []resolvedPane
	for _, p := range panes {
		if p.paneCommand != "node" {
			continue
		}

		proc := pt.findPaneProc(p.panePID)
		title := resolveTitle(p.paneTitle, p.sessName, sessions)
		r := resolvedPane{pane: p}

		if id, ok := title.uniqueID(); ok {
			// Case 1: authoritative title (one candidate, truncated or not).
			r.sessionID = id
			r.method = "title"
		} else if title.parseable {
			// Case 2: ambiguous title, use proc -s or tag to narrow.
			if proc.sessionID != "" && title.hasID(proc.sessionID) {
				r.sessionID = proc.sessionID
				r.method = "proc"
			} else if p.ocsTag != "" && title.hasID(p.ocsTag) {
				r.sessionID = p.ocsTag
				r.method = "tag"
			}
		} else {
			// Case 3: bare pane (no parseable title).
			if proc.sessionID != "" {
				r.sessionID = proc.sessionID
				r.method = "proc"
			} else if p.ocsTag != "" {
				r.sessionID = p.ocsTag
				r.method = "tag"
			} else if proc.cwd != "" {
				// cwd fallback: only for bare panes, only most-recent session.
				if id, ok := dirToMostRecent[proc.cwd]; ok {
					r.sessionID = id
					r.method = "cwd"
				}
			}
		}

		if r.sessionID != "" {
			resolved = append(resolved, r)
		}
	}
	return resolved
}

// ---------------------------------------------------------------------------
// Public API used by getSessionStates and findTmuxWindow
// ---------------------------------------------------------------------------

// getTmuxPaneStates populates result with session states detected via tmux.
func getTmuxPaneStates(tmuxPath string, sessions []Session, result map[string]sessionState) {
	panes, err := listTmuxPanes(tmuxPath)
	if err != nil {
		return
	}
	pt := buildProcTable()
	dirSessionCount := make(map[string]int, len(sessions))
	for _, s := range sessions {
		dirSessionCount[s.Directory]++
	}

	for _, r := range resolvePanes(panes, pt, sessions) {
		st := stateLinked
		if r.method == "cwd" {
			dir := ""
			for _, s := range sessions {
				if s.ID == r.sessionID {
					dir = s.Directory
					break
				}
			}
			if dir != "" && dirSessionCount[dir] > 1 {
				st = stateDetected
			}
		}
		upgrade(result, r.sessionID, st)
	}
}

// findTmuxWindow locates a tmux window running opencode for the given session.
//
// Uses the pane-centric resolver, then falls back to a global /proc cwd scan
// for sessions not reachable via any pane.
func findTmuxWindow(tmuxPath, id string, sessions []Session) (string, string, bool) {
	if id == "" {
		return "", "", false
	}

	panes, err := listTmuxPanes(tmuxPath)
	if err != nil {
		return "", "", false
	}
	pt := buildProcTable()

	for _, r := range resolvePanes(panes, pt, sessions) {
		if r.sessionID != id {
			continue
		}
		// Retag the pane when resolution is authoritative or exact proc.
		if r.method == "title" || r.method == "proc" {
			if r.pane.ocsTag != id {
				_ = exec.Command(tmuxPath, "set-option", "-p", "-t",
					r.pane.paneID, "@ocs_session_id", id).Run()
			}
		}
		return r.pane.sessName, r.pane.winIdx, true
	}

	// Final fallback: global /proc cwd match (for bare panes that the
	// resolver missed, e.g., opencode running outside any tmux pane that
	// has TMUX_PANE set).
	var targetSess Session
	for _, s := range sessions {
		if s.ID == id {
			targetSess = s
			break
		}
	}
	isMostRecent := true
	for _, s := range sessions {
		if s.Directory == targetSess.Directory && s.Updated > targetSess.Updated {
			isMostRecent = false
			break
		}
	}
	if !isMostRecent {
		return "", "", false
	}

	paneMap := make(map[string]tmuxPane, len(panes))
	for _, p := range panes {
		paneMap[p.paneID] = p
	}

	for _, pe := range pt.byPID {
		if !isOpencodeCmdline(pe.cmdline) {
			continue
		}
		if pe.cwd != targetSess.Directory {
			continue
		}
		// Read TMUX_PANE from environment.
		environ, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pe.pid))
		if err != nil {
			continue
		}
		for _, env := range strings.Split(string(environ), "\x00") {
			if strings.HasPrefix(env, "TMUX_PANE=") {
				paneID := strings.TrimPrefix(env, "TMUX_PANE=")
				if info, ok := paneMap[paneID]; ok {
					return info.sessName, info.winIdx, true
				}
			}
		}
	}

	return "", "", false
}
