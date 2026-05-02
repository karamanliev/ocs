package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type tmuxPane struct {
	paneID      string
	panePID     int
	sessName    string
	winIdx      string
	ocsTag      string
	paneTitle   string
	paneCommand string
}

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

type procEntry struct {
	pid     int
	ppid    int
	cmdline string
	cwd     string
}

type procTable struct {
	byPID    map[int]procEntry
	children map[int][]int
}

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

type paneProc struct {
	sessionID string
	cwd       string
}

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

const paneTitlePrefix = "OC | "

func parsePaneTitle(paneTitle string) (string, bool) {
	if !strings.HasPrefix(paneTitle, paneTitlePrefix) {
		return "", false
	}
	title := paneTitle[len(paneTitlePrefix):]
	return title, title != ""
}

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

type resolvedPane struct {
	pane      tmuxPane
	sessionID string
	method    string
}

func sessionDirInfo(sessions []Session) (map[string]string, map[string]int) {
	dirToMostRecent := make(map[string]string, len(sessions))
	dirSessionCount := make(map[string]int, len(sessions))
	for _, s := range sessions {
		dirSessionCount[s.Directory]++
		if _, exists := dirToMostRecent[s.Directory]; !exists {
			dirToMostRecent[s.Directory] = s.ID
		}
	}
	return dirToMostRecent, dirSessionCount
}

func isOpencodePaneCommand(command string) bool {
	switch filepath.Base(command) {
	case "node", "opencode", ".ocv":
		return true
	default:
		return false
	}
}

func resolvePanes(panes []tmuxPane, pt procTable, sessions []Session) []resolvedPane {
	dirToMostRecent, _ := sessionDirInfo(sessions)

	var resolved []resolvedPane
	for _, p := range panes {
		if !isOpencodePaneCommand(p.paneCommand) {
			continue
		}

		proc := pt.findPaneProc(p.panePID)
		title := resolveTitle(p.paneTitle, p.sessName, sessions)
		r := resolvedPane{pane: p}

		if id, ok := title.uniqueID(); ok {
			r.sessionID = id
			r.method = "title"
		} else if title.parseable {
			if proc.sessionID != "" && title.hasID(proc.sessionID) {
				r.sessionID = proc.sessionID
				r.method = "proc"
			} else if p.ocsTag != "" && title.hasID(p.ocsTag) {
				r.sessionID = p.ocsTag
				r.method = "tag"
			}
		} else {
			if proc.sessionID != "" {
				r.sessionID = proc.sessionID
				r.method = "proc"
			} else if p.ocsTag != "" {
				r.sessionID = p.ocsTag
				r.method = "tag"
			} else if proc.cwd != "" {
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

func getTmuxPaneStates(tmuxPath string, sessions []Session, result map[string]sessionState) {
	panes, err := listTmuxPanes(tmuxPath)
	if err != nil {
		return
	}
	pt := buildProcTable()
	_, dirSessionCount := sessionDirInfo(sessions)

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
		if r.method == "title" || r.method == "proc" {
			if r.pane.ocsTag != id {
				_ = exec.Command(tmuxPath, "set-option", "-p", "-t",
					r.pane.paneID, "@ocs_session_id", id).Run()
			}
		}
		return r.pane.sessName, r.pane.winIdx, true
	}

	targetSess, ok := sessionByID(sessions, id)
	if !ok {
		return "", "", false
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
