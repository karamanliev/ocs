package main

import (
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

const (
	stateRefreshCooldown = 5 * time.Second

	safetyTickInterval = 2 * time.Minute
)

type dbChangedMsg struct{}

type safetyTickMsg struct{}

type stateRefreshMsg struct {
	sessions []Session
	states   map[string]sessionState
	fromDB   bool
	mode     string
}

type dbWatcher struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
}

func newDBWatcher(dbPath string, send func(tea.Msg)) *dbWatcher {
	walPath := dbPath + "-wal"
	if _, err := os.Stat(walPath); err != nil {
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

func safetyTick() tea.Cmd {
	return tea.Tick(safetyTickInterval, func(time.Time) tea.Msg {
		return safetyTickMsg{}
	})
}

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
			mode:     mode,
		}
	}
}

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
