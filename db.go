package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Session struct {
	ID        string
	Title     string
	Updated   int64
	Directory string
}

type previewData struct {
	user      string
	assistant string
}

func getDBPath() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode", "opencode.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

func getSessions(dbPath string) ([]Session, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT s.id, s.title, s.time_updated, p.worktree
		FROM session s
		JOIN project p ON p.id = s.project_id
		WHERE s.parent_id IS NULL
		ORDER BY s.time_updated DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.Updated, &s.Directory); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func renameSession(dbPath, id, newTitle string) error {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("UPDATE session SET title = ? WHERE id = ?", newTitle, id)
	return err
}

// sessionState represents whether a session is actively being used or just
// has an opencode process sitting idle.
type sessionState int

const (
	stateNone    sessionState = iota
	stateActive               // opencode process alive, but idle (time_updated > 60s)
	stateRunning              // time_updated within last 60s
)

const runningThreshold = 60 // seconds

// isOpencodeCmdline checks whether a cmdline belongs to an opencode process.
// opencode is a Node script, so cmdline may look like:
//
//	node /path/to/opencode [flags]
//
// We match any field whose basename is "opencode".
func isOpencodeCmdline(cmdline string) bool {
	for _, f := range strings.Fields(cmdline) {
		if filepath.Base(f) == "opencode" {
			return true
		}
	}
	return false
}

func extractSessionIDFromCmdline(cmdline string) string {
	fields := strings.Fields(cmdline)
	for i, f := range fields {
		if f == "-s" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// getSessionStates detects which sessions have a live opencode process and
// whether they are actively being used (time_updated within last 60s).
//
// Detection:
//  1. Scan /proc for opencode processes
//  2. If -s <id> flag present: mark that session
//  3. Also read process cwd and mark the most recently updated session in that
//     directory (handles in-opencode session switching and bare launches)
//  4. Cross-reference with time_updated to distinguish running vs active
func getSessionStates(sessions []Session) map[string]sessionState {
	result := make(map[string]sessionState)
	nowMs := timeNowMs()

	// sessions are sorted by time_updated DESC, so first per-dir is most recent
	dirToMostRecent := make(map[string]string, len(sessions))
	sessionUpdated := make(map[string]int64, len(sessions))
	for _, s := range sessions {
		sessionUpdated[s.ID] = s.Updated
		if _, exists := dirToMostRecent[s.Directory]; !exists {
			dirToMostRecent[s.Directory] = s.ID
		}
	}

	markSession := func(id string) {
		if id == "" {
			return
		}
		ts, ok := sessionUpdated[id]
		if !ok {
			return
		}
		var st sessionState
		if (nowMs - ts) <= runningThreshold*1000 {
			st = stateRunning
		} else {
			st = stateActive
		}
		// Only upgrade, never downgrade
		if st > result[id] {
			result[id] = st
		}
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return result
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
		// Mark the explicitly launched session
		if id := extractSessionIDFromCmdline(cmdline); id != "" {
			markSession(id)
		}
		// Mark the most recently active session in this process's cwd
		procDir, err := os.Readlink(fmt.Sprintf("/proc/%s/cwd", pid))
		if err != nil {
			continue
		}
		if id, ok := dirToMostRecent[procDir]; ok {
			markSession(id)
		}
	}
	return result
}

func timeNowMs() int64 {
	return time.Now().UnixMilli()
}

func getFirstMessageByRole(dbPath, sessionID, role string) string {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()

	var text string
	err = db.QueryRow(`
		SELECT json_extract(p.data, '$.text') FROM part p
		JOIN message m ON m.id = p.message_id
		WHERE m.session_id = ?
		  AND json_extract(m.data, '$.role') = ?
		  AND json_extract(p.data, '$.type') = 'text'
		ORDER BY m.time_created ASC, p.time_created ASC
		LIMIT 1
	`, sessionID, role).Scan(&text)
	if err != nil {
		return ""
	}
	return text
}

func getPreviewData(dbPath, sessionID string) previewData {
	return previewData{
		user:      getFirstMessageByRole(dbPath, sessionID, "user"),
		assistant: getFirstMessageByRole(dbPath, sessionID, "assistant"),
	}
}