package main

import "testing"

// ---------------------------------------------------------------------------
// parsePaneTitle
// ---------------------------------------------------------------------------

func TestParsePaneTitle(t *testing.T) {
	tests := []struct {
		input     string
		wantTitle string
		wantOK    bool
	}{
		{"OC | hello", "hello", true},
		{"OC | hello...", "hello...", true},
		{"OpenCode", "", false},
		{"karamanliev", "", false},
		{"OC | ", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		title, ok := parsePaneTitle(tt.input)
		if ok != tt.wantOK || title != tt.wantTitle {
			t.Errorf("parsePaneTitle(%q) = (%q, %v), want (%q, %v)",
				tt.input, title, ok, tt.wantTitle, tt.wantOK)
		}
	}
}

// ---------------------------------------------------------------------------
// resolveTitle
// ---------------------------------------------------------------------------

func sessions(ss ...Session) []Session { return ss }

func ses(id, title, dir string) Session {
	return Session{ID: id, Title: title, Directory: dir}
}

func TestResolveTitle_UniqueExact(t *testing.T) {
	ss := sessions(
		ses("s1", "hello", "/a"),
		ses("s2", "world", "/b"),
	)
	m := resolveTitle("OC | hello", "a", ss)
	if !m.isAuthoritative() {
		t.Fatal("expected authoritative")
	}
	if id, ok := m.uniqueID(); !ok || id != "s1" {
		t.Fatalf("uniqueID = (%q, %v), want (s1, true)", id, ok)
	}
}

func TestResolveTitle_UniqueTruncated(t *testing.T) {
	ss := sessions(
		ses("s1", "Review staged changes with bubbletea & golang-patterns", "/a"),
		ses("s2", "Something else entirely", "/b"),
	)
	m := resolveTitle("OC | Review staged changes with bubbletea ...", "a", ss)
	if !m.isAuthoritative() {
		t.Fatal("expected authoritative for sole truncated candidate")
	}
	if id, ok := m.uniqueID(); !ok || id != "s1" {
		t.Fatalf("uniqueID = (%q, %v), want (s1, true)", id, ok)
	}
}

func TestResolveTitle_DuplicateExact(t *testing.T) {
	ss := sessions(
		ses("s1", "Greeting", "/a"),
		ses("s2", "Greeting", "/a"),
	)
	m := resolveTitle("OC | Greeting", "a", ss)
	if m.isAuthoritative() {
		t.Fatal("should not be authoritative with duplicates")
	}
	if !m.parseable {
		t.Fatal("should be parseable")
	}
	if !m.hasID("s1") || !m.hasID("s2") {
		t.Fatal("should have both candidates")
	}
}

func TestResolveTitle_DuplicateNarrowedByDir(t *testing.T) {
	ss := sessions(
		ses("s1", "Greeting", "/a/hypr"),
		ses("s2", "Greeting", "/b/dotfiles"),
	)
	m := resolveTitle("OC | Greeting", "hypr", ss)
	if !m.isAuthoritative() {
		t.Fatal("expected authoritative after basename narrowing")
	}
	if id, _ := m.uniqueID(); id != "s1" {
		t.Fatalf("uniqueID = %q, want s1", id)
	}
}

func TestResolveTitle_TruncatedMultipleCandidates(t *testing.T) {
	ss := sessions(
		ses("s1", "Full repository refactor using golang-patterns", "/a"),
		ses("s2", "Full repository refactor using golang-patterns (fork #1)", "/a"),
	)
	m := resolveTitle("OC | Full repository refactor using gol...", "a", ss)
	if m.isAuthoritative() {
		t.Fatal("should not be authoritative with multiple truncated candidates")
	}
	if !m.parseable {
		t.Fatal("should be parseable")
	}
	if !m.hasID("s1") || !m.hasID("s2") {
		t.Fatal("should have both candidates")
	}
}

func TestResolveTitle_BarePaneTitle(t *testing.T) {
	ss := sessions(ses("s1", "hello", "/a"))
	m := resolveTitle("OpenCode", "a", ss)
	if m.parseable {
		t.Fatal("bare pane should not be parseable")
	}
}

func TestResolveTitle_NoCandidates(t *testing.T) {
	ss := sessions(ses("s1", "hello", "/a"))
	m := resolveTitle("OC | deleted session", "a", ss)
	if m.isAuthoritative() {
		t.Fatal("no candidates should not be authoritative")
	}
	if len(m.candidates) != 0 {
		t.Fatal("expected zero candidates")
	}
}

// ---------------------------------------------------------------------------
// parsePPID
// ---------------------------------------------------------------------------

func TestParsePPID(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"123 (bash) S 456 0 0", 456},
		{"123 (node (v20)) S 789 0 0", 789},
		{"bad input", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parsePPID(tt.input)
		if got != tt.want {
			t.Errorf("parsePPID(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// resolvePanes
// ---------------------------------------------------------------------------

func TestResolvePanes_AuthoritativeTitle(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			paneTitle: "OC | unique title", paneCommand: "node"},
	}
	pt := procTable{byPID: map[int]procEntry{}, children: map[int][]int{}}
	ss := sessions(ses("s1", "unique title", "/a"))

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
	if resolved[0].sessionID != "s1" {
		t.Fatalf("sessionID = %q, want s1", resolved[0].sessionID)
	}
	if resolved[0].method != "title" {
		t.Fatalf("method = %q, want title", resolved[0].method)
	}
}

func TestResolvePanes_AuthoritativeTruncatedSoleCandidate(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			paneTitle: "OC | Review staged changes with bubbletea ...", paneCommand: "node"},
	}
	pt := procTable{byPID: map[int]procEntry{}, children: map[int][]int{}}
	ss := sessions(ses("s1", "Review staged changes with bubbletea & golang-patterns", "/a"))

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
	if resolved[0].sessionID != "s1" || resolved[0].method != "title" {
		t.Fatalf("got (%q, %q), want (s1, title)", resolved[0].sessionID, resolved[0].method)
	}
}

func TestResolvePanes_AmbiguousTitleProcWins(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			ocsTag: "s1", paneTitle: "OC | Greeting", paneCommand: "node"},
		{paneID: "%2", panePID: 200, sessName: "ocs", winIdx: "2",
			ocsTag: "s1", paneTitle: "OC | Greeting", paneCommand: "node"},
	}
	ss := sessions(
		ses("s1", "Greeting", "/a"),
		ses("s2", "Greeting", "/a"),
	)
	// Simulate: pane %1 runs s1 via -s, pane %2 runs s2 via -s
	pt := procTable{
		byPID: map[int]procEntry{
			100: {pid: 100, ppid: 0},
			101: {pid: 101, ppid: 100, cmdline: "node /usr/bin/opencode -s s1"},
			200: {pid: 200, ppid: 0},
			201: {pid: 201, ppid: 200, cmdline: "node /usr/bin/opencode -s s2"},
		},
		children: map[int][]int{
			100: {101},
			200: {201},
		},
	}

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved, got %d", len(resolved))
	}

	byPane := make(map[string]resolvedPane)
	for _, r := range resolved {
		byPane[r.pane.paneID] = r
	}
	if r := byPane["%1"]; r.sessionID != "s1" || r.method != "proc" {
		t.Errorf("pane %%1: got (%q, %q), want (s1, proc)", r.sessionID, r.method)
	}
	if r := byPane["%2"]; r.sessionID != "s2" || r.method != "proc" {
		t.Errorf("pane %%2: got (%q, %q), want (s2, proc)", r.sessionID, r.method)
	}
}

func TestResolvePanes_AmbiguousTitleTagFallback(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			ocsTag: "s2", paneTitle: "OC | Greeting", paneCommand: "node"},
	}
	ss := sessions(
		ses("s1", "Greeting", "/a"),
		ses("s2", "Greeting", "/a"),
	)
	pt := procTable{byPID: map[int]procEntry{}, children: map[int][]int{}}

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
	if resolved[0].sessionID != "s2" || resolved[0].method != "tag" {
		t.Fatalf("got (%q, %q), want (s2, tag)", resolved[0].sessionID, resolved[0].method)
	}
}

func TestResolvePanes_StaleTagOverriddenByTitle(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			ocsTag: "s_old", paneTitle: "OC | bla", paneCommand: "node"},
	}
	ss := sessions(
		ses("s_old", "original hello", "/a"),
		ses("s_new", "bla", "/a"),
	)
	pt := procTable{byPID: map[int]procEntry{}, children: map[int][]int{}}

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
	if resolved[0].sessionID != "s_new" || resolved[0].method != "title" {
		t.Fatalf("got (%q, %q), want (s_new, title)", resolved[0].sessionID, resolved[0].method)
	}
}

func TestResolvePanes_BarePaneProcID(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			paneTitle: "OpenCode", paneCommand: "node"},
	}
	ss := sessions(ses("s1", "hello", "/a"))
	pt := procTable{
		byPID: map[int]procEntry{
			100: {pid: 100, ppid: 0},
			101: {pid: 101, ppid: 100, cmdline: "node /usr/bin/opencode -s s1"},
		},
		children: map[int][]int{100: {101}},
	}

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
	if resolved[0].sessionID != "s1" || resolved[0].method != "proc" {
		t.Fatalf("got (%q, %q), want (s1, proc)", resolved[0].sessionID, resolved[0].method)
	}
}

func TestResolvePanes_BarePaneCWDFallback(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			paneTitle: "OpenCode", paneCommand: "node"},
	}
	ss := sessions(ses("s1", "hello", "/a"))
	pt := procTable{
		byPID: map[int]procEntry{
			100: {pid: 100, ppid: 0},
			101: {pid: 101, ppid: 100, cmdline: "node /usr/bin/opencode", cwd: "/a"},
		},
		children: map[int][]int{100: {101}},
	}

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved, got %d", len(resolved))
	}
	if resolved[0].sessionID != "s1" || resolved[0].method != "cwd" {
		t.Fatalf("got (%q, %q), want (s1, cwd)", resolved[0].sessionID, resolved[0].method)
	}
}

func TestResolvePanes_TitledPaneNoCWDFallback(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			paneTitle: "OC | unknown deleted session", paneCommand: "node"},
	}
	ss := sessions(ses("s1", "hello", "/a"))
	pt := procTable{
		byPID: map[int]procEntry{
			100: {pid: 100, ppid: 0},
			101: {pid: 101, ppid: 100, cmdline: "node /usr/bin/opencode", cwd: "/a"},
		},
		children: map[int][]int{100: {101}},
	}

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 0 {
		t.Fatalf("titled pane should not use cwd fallback, got %d resolved", len(resolved))
	}
}

func TestResolvePanes_SkipsNonNodePanes(t *testing.T) {
	panes := []tmuxPane{
		{paneID: "%1", panePID: 100, sessName: "ocs", winIdx: "1",
			paneTitle: "OC | hello", paneCommand: "zsh"},
	}
	ss := sessions(ses("s1", "hello", "/a"))
	pt := procTable{byPID: map[int]procEntry{}, children: map[int][]int{}}

	resolved := resolvePanes(panes, pt, ss)
	if len(resolved) != 0 {
		t.Fatalf("expected 0 resolved for non-node pane, got %d", len(resolved))
	}
}
