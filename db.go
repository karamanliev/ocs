package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// NOTE: tmux pane-centric detection is in tmux_resolve.go.
// This file keeps: session types, DB helpers, sessionState, cmdline helpers,
// getSessionStates (orchestrator), getProcStates (non-tmux fallback), and
// DB mutation functions (fork, copy, etc.).

type Session struct {
	ID        string
	Title     string
	Updated   int64
	Directory string
	Worktree  string
}

type previewData struct {
	firstUser      string
	firstAssistant string
	lastUser       string
	lastAssistant  string
	modelName      string
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
		SELECT s.id, s.title, s.time_updated, s.directory, p.worktree
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
		if err := rows.Scan(&s.ID, &s.Title, &s.Updated, &s.Directory, &s.Worktree); err != nil {
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

// sessionState represents the detection confidence for a running session.
//
//   - stateNone: no process found
//   - stateLinked: confirmed match (pane tag, -s <id>, or unambiguous title match)
//   - stateDetected: cwd fallback with multiple sessions in the same directory
type sessionState int

const (
	stateNone     sessionState = iota
	stateDetected              // process alive, but focus is a guess (cwd fallback, multi-session dir)
	stateLinked                // confirmed pane mapping, will focus correctly in tmux
)

// isOpencodeCmdline checks whether a cmdline belongs to an opencode process.
// opencode is a Node script, so cmdline may look like:
//
//	node /path/to/opencode [flags]
//
// We match known launcher basenames used locally.
func isOpencodeCmdline(cmdline string) bool {
	for _, f := range strings.Fields(cmdline) {
		switch filepath.Base(f) {
		case "opencode", ".ocv":
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



// getSessionStates detects which sessions have a live opencode process.
//
// In tmux mode, uses the pane-centric resolver (see tmux_resolve.go) plus
// a /proc cwd fallback for sessions not reachable via any tmux pane.
//
// In non-tmux mode, only /proc scan is used.
func getSessionStates(sessions []Session, mode string) map[string]sessionState {
	result := make(map[string]sessionState)

	if mode == "tmux" {
		tmuxPath, err := exec.LookPath("tmux")
		if err == nil {
			getTmuxPaneStates(tmuxPath, sessions, result)
		}
	}

	// /proc fallback: catches sessions not detected via tmux
	getProcStates(sessions, mode, result)

	return result
}

// getProcStates scans /proc for opencode processes and populates result.
func getProcStates(sessions []Session, mode string, result map[string]sessionState) {
	dirToMostRecent := make(map[string]string, len(sessions))
	dirSessionCount := make(map[string]int, len(sessions))
	for _, s := range sessions {
		dirSessionCount[s.Directory]++
		if _, exists := dirToMostRecent[s.Directory]; !exists {
			dirToMostRecent[s.Directory] = s.ID
		}
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
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
		// Explicit -s <id> match
		if id := extractSessionIDFromCmdline(cmdline); id != "" {
			upgrade(result, id, stateLinked)
		}
		// cwd fallback: mark the most recently updated session in this directory
		procDir, err := os.Readlink(fmt.Sprintf("/proc/%s/cwd", pid))
		if err != nil {
			continue
		}
		if id, ok := dirToMostRecent[procDir]; ok {
			// In tmux mode: if multiple sessions share this dir, it is a guess
			st := stateLinked
			if mode == "tmux" && dirSessionCount[procDir] > 1 {
				st = stateDetected
			}
			upgrade(result, id, st)
		}
	}
}

// upgrade sets the state for id only if the new state is higher (more confident).
func upgrade(result map[string]sessionState, id string, st sessionState) {
	if id == "" {
		return
	}
	if st > result[id] {
		result[id] = st
	}
}

func timeNowMs() int64 {
	return time.Now().UnixMilli()
}

func queryPreviewText(db *sql.DB, sessionID, role, order string) (string, error) {
	var text string
	err := db.QueryRow(fmt.Sprintf(`
		SELECT json_extract(p.data, '$.text') FROM part p
		JOIN message m ON m.id = p.message_id
		WHERE m.session_id = ?
		  AND json_extract(m.data, '$.role') = ?
		  AND json_extract(p.data, '$.type') = 'text'
		ORDER BY m.time_created %s, p.time_created %s
		LIMIT 1
	`, order, order), sessionID, role).Scan(&text)
	return text, err
}

func getPreviewData(dbPath, sessionID string) previewData {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return previewData{}
	}
	defer db.Close()

	var modelName string
	_ = db.QueryRow(`
		SELECT json_extract(data, '$.modelID')
		FROM message
		WHERE session_id = ?
		  AND json_extract(data, '$.role') = 'assistant'
		  AND json_extract(data, '$.modelID') IS NOT NULL
		ORDER BY time_created DESC
		LIMIT 1
	`, sessionID).Scan(&modelName)

	firstUser, _ := queryPreviewText(db, sessionID, "user", "ASC")
	firstAssistant, _ := queryPreviewText(db, sessionID, "assistant", "ASC")
	lastUser, _ := queryPreviewText(db, sessionID, "user", "DESC")
	lastAssistant, _ := queryPreviewText(db, sessionID, "assistant", "DESC")

	return previewData{
		firstUser:      firstUser,
		firstAssistant: firstAssistant,
		lastUser:       lastUser,
		lastAssistant:  lastAssistant,
		modelName:      modelName,
	}
}

// generateID creates a nanoid-style random identifier with the given prefix.
func generateID(prefix string) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 26)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = alphabet[time.Now().UnixNano()%int64(len(alphabet))]
		}
	} else {
		for i := range b {
			b[i] = alphabet[int(b[i])%len(alphabet)]
		}
	}
	return prefix + string(b)
}

// forkSession duplicates a session (including all messages and parts) in the
// database. Returns the new session ID. The title is used verbatim for the
// forked session.
func forkSession(dbPath, sessionID, title string) (string, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()

	var projectID, slug, directory, version string
	var shareURL, permission sql.NullString
	var summaryAdditions, summaryDeletions, summaryFiles sql.NullInt32
	var summaryDiffs, revert sql.NullString
	var timeCreated, timeUpdated int64
	var discardTitle string

	err = db.QueryRow(`
		SELECT project_id, slug, directory, title, version, share_url,
			summary_additions, summary_deletions, summary_files, summary_diffs,
			revert, permission, time_created, time_updated
		FROM session
		WHERE id = ?
	`, sessionID).Scan(
		&projectID, &slug, &directory, &discardTitle, &version, &shareURL,
		&summaryAdditions, &summaryDeletions, &summaryFiles, &summaryDiffs,
		&revert, &permission, &timeCreated, &timeUpdated,
	)
	if err != nil {
		return "", fmt.Errorf("reading session: %w", err)
	}

	newID := generateID("ses_")
	now := timeNowMs()

	_, err = db.Exec(`
		INSERT INTO session
			(id, project_id, slug, directory, title, version, share_url,
			 summary_additions, summary_deletions, summary_files, summary_diffs,
			 revert, permission, time_created, time_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newID, projectID, slug, directory, title, version, shareURL,
		summaryAdditions, summaryDeletions, summaryFiles, summaryDiffs,
		revert, permission, now, now)
	if err != nil {
		return "", fmt.Errorf("inserting session: %w", err)
	}

	msgMap, err := copyMessages(db, sessionID, newID)
	if err != nil {
		return "", err
	}

	if err := copyParts(db, sessionID, newID, msgMap); err != nil {
		return "", err
	}

	return newID, nil
}

func copyMessages(db *sql.DB, oldSessionID, newSessionID string) (map[string]string, error) {
	rows, err := db.Query(`
		SELECT id, data, time_created, time_updated
		FROM message
		WHERE session_id = ?
	`, oldSessionID)
	if err != nil {
		return nil, fmt.Errorf("reading messages: %w", err)
	}
	defer rows.Close()

	type msgInfo struct {
		oldID       string
		newID       string
		data        string
		timeCreated int64
		timeUpdated int64
	}
	var msgs []msgInfo
	for rows.Next() {
		var mi msgInfo
		if err := rows.Scan(&mi.oldID, &mi.data, &mi.timeCreated, &mi.timeUpdated); err != nil {
			return nil, err
		}
		mi.newID = generateID("msg_")
		msgs = append(msgs, mi)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, mi := range msgs {
		_, err := db.Exec(`
			INSERT INTO message (id, session_id, data, time_created, time_updated)
			VALUES (?, ?, ?, ?, ?)
		`, mi.newID, newSessionID, mi.data, mi.timeCreated, mi.timeUpdated)
		if err != nil {
			return nil, fmt.Errorf("inserting message: %w", err)
		}
	}

	msgMap := make(map[string]string, len(msgs))
	for _, mi := range msgs {
		msgMap[mi.oldID] = mi.newID
	}
	return msgMap, nil
}

func copyParts(db *sql.DB, oldSessionID, newSessionID string, msgMap map[string]string) error {
	rows, err := db.Query(`
		SELECT p.id, p.message_id, p.data, p.time_created, p.time_updated
		FROM part p
		JOIN message m ON m.id = p.message_id
		WHERE m.session_id = ?
	`, oldSessionID)
	if err != nil {
		return fmt.Errorf("reading parts: %w", err)
	}
	defer rows.Close()

	type partInfo struct {
		oldID       string
		messageID   string
		newID       string
		data        string
		timeCreated int64
		timeUpdated int64
	}
	var parts []partInfo
	for rows.Next() {
		var pi partInfo
		if err := rows.Scan(&pi.oldID, &pi.messageID, &pi.data, &pi.timeCreated, &pi.timeUpdated); err != nil {
			return err
		}
		pi.newID = generateID("prt_")
		parts = append(parts, pi)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, pi := range parts {
		newMsgID, ok := msgMap[pi.messageID]
		if !ok {
			return fmt.Errorf("message id %s not found in msgMap", pi.messageID)
		}
		_, err := db.Exec(`
			INSERT INTO part (id, message_id, session_id, data, time_created, time_updated)
			VALUES (?, ?, ?, ?, ?, ?)
		`, pi.newID, newMsgID, newSessionID, pi.data, pi.timeCreated, pi.timeUpdated)
		if err != nil {
			return fmt.Errorf("inserting part: %w", err)
		}
	}
	return nil
}
