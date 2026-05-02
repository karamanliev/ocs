package main

import (
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

const (
	// stateRefreshCooldown is the minimum time between keypress-triggered
	// /proc scans. Prevents unnecessary rescans on rapid key presses.
	stateRefreshCooldown = 5 * time.Second

	// safetyTickInterval is the fallback refresh interval for edge cases
	// where fsnotify misses events (e.g., WAL checkpoint replaces file).
	safetyTickInterval = 2 * time.Minute
)

// dbChangedMsg is sent by the filesystem watcher when opencode.db-wal changes.
type dbChangedMsg struct{}

// safetyTickMsg is sent by the 2-minute fallback ticker.
type safetyTickMsg struct{}

// stateRefreshMsg is the result of an async state refresh.
type stateRefreshMsg struct {
	sessions []Session
	states   map[string]sessionState
	fromDB   bool // true if sessions were re-read from DB
}

// dbWatcher watches the opencode database WAL file for changes and sends
// dbChangedMsg to the bubbletea program when a write occurs.
type dbWatcher struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
}

// newDBWatcher creates a filesystem watcher on the opencode.db-wal file.
// Returns nil if the file does not exist or watching fails.
func newDBWatcher(dbPath string, send func(tea.Msg)) *dbWatcher {
	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); err != nil {
		// Also try watching the parent directory (WAL may not exist yet)
		walPath = filepath.Dir(dbPath)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	if err := w.Add(walPath); err != nil {
		w.Close()
		return nil
	}

	dw := &dbWatcher{
		watcher: w,
		done:    make(chan struct{}),
	}

	go func() {
		// Debounce: collapse rapid writes into a single notification.
		var debounce *time.Timer
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(500*time.Millisecond, func() {
					send(dbChangedMsg{})
				})
			case <-w.Errors:
				// Ignore errors, the safety tick will catch staleness.
			case <-dw.done:
				if debounce != nil {
					debounce.Stop()
				}
				return
			}
		}
	}()

	return dw
}

func (dw *dbWatcher) close() {
	if dw == nil {
		return
	}
	close(dw.done)
	dw.watcher.Close()
}

// safetyTick returns a command that sends safetyTickMsg after the safety interval.
func safetyTick() tea.Cmd {
	return tea.Tick(safetyTickInterval, func(time.Time) tea.Msg {
		return safetyTickMsg{}
	})
}

// refreshStatesAsync performs an async state refresh. If fromDB is true,
// sessions are re-read from the database first.
func refreshStatesAsync(dbPath, mode string, currentSessions []Session, fromDB bool) tea.Cmd {
	return func() tea.Msg {
		sessions := currentSessions
		if fromDB {
			newSessions, err := getSessions(dbPath)
			if err == nil {
				sessions = newSessions
			}
		}
		states := getSessionStates(sessions, mode)
		return stateRefreshMsg{
			sessions: sessions,
			states:   states,
			fromDB:   fromDB,
		}
	}
}

// statesEqual compares two state maps for equality.
func statesEqual(a, b map[string]sessionState) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// sessionsEqual compares two session slices by ID and Updated timestamp.
func sessionsEqual(a, b []Session) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Updated != b[i].Updated || a[i].Title != b[i].Title {
			return false
		}
	}
	return true
}
