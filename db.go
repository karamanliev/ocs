package main

import (
	"crypto/rand"
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

func getPreviewData(dbPath, sessionID string) previewData {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return previewData{}
	}
	defer db.Close()

	queryText := func(role, order string) string {
		var text string
		_ = db.QueryRow(fmt.Sprintf(`
			SELECT json_extract(p.data, '$.text') FROM part p
			JOIN message m ON m.id = p.message_id
			WHERE m.session_id = ?
			  AND json_extract(m.data, '$.role') = ?
			  AND json_extract(p.data, '$.type') = 'text'
			ORDER BY m.time_created %s, p.time_created %s
			LIMIT 1
		`, order, order), sessionID, role).Scan(&text)
		return text
	}

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

	return previewData{
		firstUser:      queryText("user", "ASC"),
		firstAssistant: queryText("assistant", "ASC"),
		lastUser:       queryText("user", "DESC"),
		lastAssistant:  queryText("assistant", "DESC"),
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
// database.  Returns the new session ID.  The title is used verbatim for the
// forked session.
func forkSession(dbPath, sessionID, title string) (string, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()

	// Read the original session row.
	var projectID, slug, directory, version string
	var shareURL, permission sql.NullString
	var summaryAdditions, summaryDeletions, summaryFiles sql.NullInt32
	var summaryDiffs, revert sql.NullString
	var timeCreated, timeUpdated int64
	err = db.QueryRow(`
		SELECT project_id, slug, directory, title, version, share_url,
			summary_additions, summary_deletions, summary_files, summary_diffs,
			revert, permission, time_created, time_updated
		FROM session
		WHERE id = ?
	`, sessionID).Scan(
		&projectID, &slug, &directory, new(string), &version, &shareURL,
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

	// Copy messages.
	rows, err := db.Query(`
		SELECT id, data, time_created, time_updated
		FROM message
		WHERE session_id = ?
	`, sessionID)
	if err != nil {
		return "", fmt.Errorf("reading messages: %w", err)
	}

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
			rows.Close()
			return "", err
		}
		mi.newID = generateID("msg_")
		msgs = append(msgs, mi)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	for _, mi := range msgs {
		_, err := db.Exec(`
			INSERT INTO message (id, session_id, data, time_created, time_updated)
			VALUES (?, ?, ?, ?, ?)
		`, mi.newID, newID, mi.data, mi.timeCreated, mi.timeUpdated)
		if err != nil {
			return "", fmt.Errorf("inserting message: %w", err)
		}
	}

	// Copy parts.
	partRows, err := db.Query(`
		SELECT p.id, p.message_id, p.data, p.time_created, p.time_updated
		FROM part p
		JOIN message m ON m.id = p.message_id
		WHERE m.session_id = ?
	`, sessionID)
	if err != nil {
		return "", fmt.Errorf("reading parts: %w", err)
	}

	type partInfo struct {
		oldID       string
		messageID   string
		newID       string
		data        string
		timeCreated int64
		timeUpdated int64
	}
	var parts []partInfo
	for partRows.Next() {
		var pi partInfo
		if err := partRows.Scan(&pi.oldID, &pi.messageID, &pi.data, &pi.timeCreated, &pi.timeUpdated); err != nil {
			partRows.Close()
			return "", err
		}
		pi.newID = generateID("prt_")
		parts = append(parts, pi)
	}
	partRows.Close()
	if err := partRows.Err(); err != nil {
		return "", err
	}

	// Build a map from old message ID to new message ID.
	msgMap := make(map[string]string, len(msgs))
	for _, mi := range msgs {
		msgMap[mi.oldID] = mi.newID
	}

	for _, pi := range parts {
		newMsgID := msgMap[pi.messageID]
		_, err := db.Exec(`
			INSERT INTO part (id, message_id, session_id, data, time_created, time_updated)
			VALUES (?, ?, ?, ?, ?, ?)
		`, pi.newID, newMsgID, newID, pi.data, pi.timeCreated, pi.timeUpdated)
		if err != nil {
			return "", fmt.Errorf("inserting part: %w", err)
		}
	}

	return newID, nil
}