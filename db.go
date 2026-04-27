package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

func getRunningSessionIDs() map[string]struct{} {
	result := make(map[string]struct{})
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return result
	}
	out, err := exec.Command(tmuxPath, "list-panes", "-a", "-F", "#{pane_pid}").Output()
	if err != nil {
		return result
	}
	for _, pid := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		data, err := os.ReadFile(fmt.Sprintf("/proc/%s/cmdline", pid))
		if err != nil {
			continue
		}
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		if !strings.Contains(cmdline, "opencode") {
			continue
		}
		fields := strings.Fields(cmdline)
		for i, f := range fields {
			if f == "-s" && i+1 < len(fields) {
				result[fields[i+1]] = struct{}{}
				break
			}
		}
	}
	return result
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