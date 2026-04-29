# ocs

[![GitHub Releases](https://img.shields.io/github/downloads/karamanliev/ocs/total)](https://github.com/karamanliev/ocs/releases) [![GitHub tag](https://img.shields.io/github/v/tag/karamanliev/ocs?color=blue)](https://github.com/karamanliev/ocs/releases/latest) [![build](https://github.com/karamanliev/ocs/actions/workflows/build.yml/badge.svg)](https://github.com/karamanliev/ocs/actions/workflows/build.yml) [![test](https://github.com/karamanliev/ocs/actions/workflows/test.yml/badge.svg)](https://github.com/karamanliev/ocs/actions/workflows/test.yml)

A fast terminal UI for managing [OpenCode](https://opencode.ai) sessions.

`ocs` reads your local OpenCode SQLite database directly and gives you a keyboard-driven interface to resume, attach, rename, preview, and delete sessions. No fzf, no subprocess hacks, just a single static binary.

https://github.com/user-attachments/assets/49dd22b3-77b5-4223-aea4-5e11f714eafa

## Features

- **Pure Go** - static binary, zero runtime dependencies beyond `opencode` and optionally `tmux`
- **Dual mode** - `ALL` mode resumes sessions in-place; `TMUX` mode attaches them to tmux windows
- **Fuzzy filtering** - type `/` to filter sessions in real time
- **Preview pane** - visible by default, auto-moves right or below based on terminal width
- **Grouped view** - enabled by default, can be toggled on the fly
- **Batch delete** - multi-select and confirm delete
- **Rename** - inline session rename
- **Running indicator** - green dot shows which sessions have active tmux panes
- **Auto theme** - detects light/dark terminal background on startup and live-reloads on focus change
- **Mouse support** - scroll lists and preview pane, select rows, expand/collapse groups

## Installation

```bash
go install github.com/karamanliev/ocs@latest
```

Or grab a prebuilt binary from the [releases page](https://github.com/karamanliev/ocs/releases).

Or build from source:

```bash
cd ~/Projects/personal/ocs
go build -o ~/.local/bin/ocs .
```

Requires Go 1.22+.

## Usage

```bash
ocs                 # launch in ALL mode, grouped by path
ocs --tmux          # launch directly in TMUX mode
ocs --preview=false # launch with preview pane hidden
ocs --grouped=false # start ungrouped
ocs --theme dark    # force dark theme (light also available)
```

## Keybinds

| Key | Action |
| --- | --- |
| `?` | Show keybinds help |
| `enter` | Open selected session, mode dependent: ALL resumes, TMUX opens in tmux; group headers do nothing |
| `alt+enter`, `ctrl+o` | Open in tmux when in ALL mode, resume when in TMUX mode |
| `t` | Toggle ALL and TMUX mode |
| `g` | Toggle grouped view |
| `/` | Start filtering sessions |
| `space` | Collapse or expand current group |
| `ctrl+space` | Collapse or expand all groups |
| `h`, `l` | Collapse or expand current group |
| `[` / `]` | Jump to previous or next group |
| `d` | Enter delete mode |
| `r` | Rename selected session |
| `n` | Create new session in selected item's directory (confirmation modal) |
| `N` | Open directory picker to choose directory for new session |
| `y` | Duplicate selected session with `!DUP` prefix (confirmation modal) |
| `Y` | Duplicate selected session with a custom title (rename modal) |
| `x` | Close running tmux window for selected session (tmux mode only, confirmation modal) |
| `tab` | Toggle preview pane |
| `J`, `K`, `shift+up`, `shift+down` | Scroll preview |
| Mouse wheel | Scroll preview or list |
| Mouse click | Select row, click group header to fold or unfold |

## Modes

- **ALL** - the default. Pressing `enter` resumes the session directly.
- **TMUX** - `enter` opens the session in a tmux window. The title bar shows `[tmux]` and the UI shifts to purple tones.


## Theme

`ocs` detects your terminal background (light or dark) on startup and live-reloads when the window regains focus. No manual toggle needed.

When running inside a tmux popup, OSC 11 detection does not work because tmux intercepts the query. Use `--theme` to force the correct palette, or detect it before launching the popup:

```bash
# Example wrapper that detects theme outside tmux, then opens a popup
ocs-popup() {
  local theme
  theme=$(detect-theme-here) # your own OSC 11 or $COLORFGBG check
  tmux display-popup -E "ocs --theme=$theme"
}
```

I personally use [darkman](https://gitlab.com/WhyNotHugo/darkman) and have this in my `~/.tmux.conf`, works perfect:

```bash
bind-key -n M-o run-shell 'theme=$(darkman get); tmux display-popup -w 80% -h 80% -E "ocs --tmux --theme=$theme"'
```

## Preview

`ocs` shows a bordered preview pane by default. The pane displays the first user message for the selected session.

- On wider terminals it appears to the right of the session list
- On narrower terminals it moves below the session list
- Can be scrolled with the mouse or the preview scroll keys in the table above

## Requirements

- `opencode` must be installed and in your `PATH`
- `tmux` is optional; without it `ocs` works in ALL mode only
- SQLite database is read from `$XDG_DATA_HOME/opencode/opencode.db`

## Managing sessions in every tmux window

In **TMUX** mode `ocs` keeps track of which opencode sessions are already running in tmux windows. When you select a session that already has a window, `ocs` switches to that window instead of creating a duplicate. Panes are tagged automatically so the mapping survives pane respawns and window renames.

This means you can open `ocs` from any tmux window, pick a session, and end up exactly where that session lives, or create a fresh window if it is not running yet.

## TODO

- [x] Add `<C-g>` and `--grouped` flag which toggles grouping by path. Groups can be expanded/collapsed with `space`, `<C-space>`, and `h/l`.
- [x] Add `--theme` flag to target `light` or `dark` theme
- [x] More tmux controls - close windows, create new opencode sessions, duplicate (fork) sessions from the TUI
  - [x] `x` closes the window of the currently running/active tmux session. Refreshes the list to update the indicator. Works only in tmux mode.
  - [x] `n` creates a new session in the path of the currently selected item/group and let the agent name it. `N` lets the user choose the filepath of the session. If in tmux mode also attaches/creates a tmux session in that path.
  - [x] `y` duplicates the currently selected session in the path of the currently selected item with a `#DUP {oldTitle}` name. `Y` lets the user manually name the duplicated session.
- [x] Add a popup with the keybinds, list only the most commonly used ones in the footer
- [ ] Add configurable keybinds, maybe a toml/json config file in `~/.config/ocs/config.{toml,json}`
- [ ] Rework CLI flags (add more options, add `true/false` to already existing flags, etc)
- [ ] Add sorting by different criteria to the session list
- [ ] Add more agents like `claude code`, `codex`, `gemini-cli`, `pi`. Maybe rename the project.

## License

MIT
