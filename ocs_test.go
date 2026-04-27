package main

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{0 * time.Second, "0s ago"},
		{30 * time.Second, "30s ago"},
		{59 * time.Second, "59s ago"},
		{1 * time.Minute, "1m ago"},
		{30 * time.Minute, "30m ago"},
		{59 * time.Minute, "59m ago"},
		{1 * time.Hour, "1h ago"},
		{23 * time.Hour, "23h ago"},
		{24 * time.Hour, "1d ago"},
		{48 * time.Hour, "2d ago"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.expected)
		}
	}
}

func TestColorForDuration(t *testing.T) {
	th := darkTheme
	tests := []struct {
		d     time.Duration
		field lipgloss.Color
	}{
		{0 * time.Second, th.timeFresh},
		{1 * time.Minute, th.timeHour},
		{1 * time.Hour, th.timeDay},
		{24 * time.Hour, th.timeOld},
	}
	for _, tt := range tests {
		got := th.colorForDuration(tt.d)
		if got != tt.field {
			t.Errorf("colorForDuration(%v) = %v, want %v", tt.d, got, tt.field)
		}
	}
}

func TestParseBackgroundResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		dark    bool
		wantErr bool
	}{
		{"black bg", "\x1b]11;rgb:0000/0000/0000\x07", true, false},
		{"white bg", "\x1b]11;rgb:ffff/ffff/ffff\x07", false, false},
		{"dark gray", "\x1b]11;rgb:3333/3333/3333\x07", true, false},
		{"boundary 0x80", "\x1b]11;rgb:80/80/80\x07", false, false},
		{"no rgb", "no rgb here", true, true},
		{"invalid rgb parts", "\x1b]11;rgb:ff/00\x07", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dark, err := parseBackgroundResponse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBackgroundResponse(%q) err = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if !tt.wantErr && dark != tt.dark {
				t.Errorf("parseBackgroundResponse(%q) dark = %v, want %v", tt.name, dark, tt.dark)
			}
		})
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{"short line", "hello", 20, "hello"},
		{"exact width", "12345", 5, "12345"},
		{"word wrap", "hello world", 5, "hello\nworld"},
		{"zero width passthrough", "hello", 0, "hello"},
		{"negative width passthrough", "hello", -1, "hello"},
		{"preserve newlines", "hello\nworld", 20, "hello\nworld"},
		{"long word", "abbreviation", 5, "abbre\nviati\non"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.input, tt.width)
			if got != tt.want {
				t.Errorf("wrapText(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
			}
		})
	}
}

func TestTruncatePreviewLines(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}

	t.Run("under limit returns same", func(t *testing.T) {
		got := truncatePreviewLines([]string{"a", "b"}, 5)
		if len(got) != 2 {
			t.Errorf("got %d lines, want 2", len(got))
		}
	})

	t.Run("truncates and appends ellipsis", func(t *testing.T) {
		got := truncatePreviewLines(lines, 3)
		if len(got) != 3 {
			t.Fatalf("got %d lines, want 3", len(got))
		}
		if got[2] != "..." {
			t.Errorf("last line = %q, want ...", got[2])
		}
	})

	t.Run("does not mutate original", func(t *testing.T) {
		orig := []string{"a", "b", "c", "d"}
		_ = truncatePreviewLines(orig, 2)
		if len(orig) != 4 {
			t.Error("original slice was mutated")
		}
	})

	t.Run("zero limit returns input", func(t *testing.T) {
		got := truncatePreviewLines([]string{"a", "b"}, 0)
		if len(got) != 2 {
			t.Errorf("got %d lines, want 2", len(got))
		}
	})
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"},
		{"", 4, "    "},
	}
	for _, tt := range tests {
		got := padRight(tt.input, tt.width)
		if got != tt.want {
			t.Errorf("padRight(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
		}
	}
}

func TestHighlightMatch(t *testing.T) {
	mc := lipgloss.Color("#FF0000")

	t.Run("empty filter returns text", func(t *testing.T) {
		got := highlightMatch("hello world", "", nil, mc)
		if got != "hello world" {
			t.Errorf("got %q, want %q", got, "hello world")
		}
	})

	t.Run("empty text returns empty", func(t *testing.T) {
		got := highlightMatch("", "test", nil, mc)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("no match returns original text", func(t *testing.T) {
		got := highlightMatch("hello", "xyz", nil, mc)
		if got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("case insensitive match produces output", func(t *testing.T) {
		got := highlightMatch("Hello World", "WORLD", nil, mc)
		if got == "" {
			t.Error("expected non-empty output for case-insensitive match")
		}
	})

	t.Run("multiple matches", func(t *testing.T) {
		got := highlightMatch("abc abc abc", "abc", nil, mc)
		if got == "" {
			t.Error("expected non-empty output for multiple matches")
		}
	})
}

func TestWithLineBg(t *testing.T) {
	t.Run("invalid hex returns line unchanged", func(t *testing.T) {
		got := withLineBg("test", "invalid")
		if got != "test" {
			t.Errorf("got %q, want %q", got, "test")
		}
	})

	t.Run("empty bg returns line unchanged", func(t *testing.T) {
		got := withLineBg("test", "")
		if got != "test" {
			t.Errorf("got %q, want %q", got, "test")
		}
	})

	t.Run("valid hex prepends escape sequence", func(t *testing.T) {
		got := withLineBg("text", "#1A2B3C")
		if got[:4] != "\x1b[48" {
			t.Errorf("expected bg escape sequence prefix, got %q", got[:4])
		}
	})
}

func TestGetDBPath(t *testing.T) {
	t.Run("XDG_DATA_HOME overrides default", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/tmp/xdg-test")
		path := getDBPath()
		want := "/tmp/xdg-test/opencode/opencode.db"
		if path != want {
			t.Errorf("getDBPath() = %q, want %q", path, want)
		}
	})

	t.Run("falls back to home dir", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")
		path := getDBPath()
		if path == "" {
			t.Error("getDBPath() returned empty string")
		}
	})
}

func TestToggleMode(t *testing.T) {
	if toggleMode(model{mode: "all"}) != "tmux" {
		t.Error("toggleMode from all should return tmux")
	}
	if toggleMode(model{mode: "tmux"}) != "all" {
		t.Error("toggleMode from tmux should return all")
	}
}