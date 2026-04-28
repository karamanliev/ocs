# ocs

A fast terminal UI for managing [OpenCode](https://opencode.ai) sessions.

`ocs` reads your local OpenCode SQLite database directly and gives you a keyboard-driven interface to resume, attach, rename, preview, and delete sessions. No fzf, no subprocess hacks, just a single static binary.

https://github.com/user-attachments/assets/029e4199-8e3e-4dda-bee4-a6f7ad915ad8

## Features

- **Dual mode** — `ALL` mode resumes sessions in-place; `TMUX` mode attaches them to tmux windows
- **Fuzzy filtering** — type `/` to filter sessions in real time
- **Preview pane** — visible by default, auto-moves right or below based on terminal width
- **Grouped view** — enabled by default, can be toggled on the fly
- **Batch delete** — multi-select and confirm delete
- **Rename** — inline session rename
- **Running indicator** — green dot shows which sessions have active tmux panes
- **Auto theme** — detects light/dark terminal background on startup and live-reloads on focus change
- **Mode-colored UI** — cursor highlight, border title, and footer keys shift between steel blue (ALL) and dusty purple (TMUX)
- **Pure Go** — static binary, zero runtime dependencies beyond `opencode` and optionally `tmux`

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
ocs              # launch in ALL mode, grouped by path
ocs --tmux        # launch directly in TMUX mode
ocs --preview=false # launch with preview pane hidden
ocs --grouped=false # start ungrouped
ocs --theme dark  # force dark theme (light also available)
```

## Keybinds

| Key | Action |
| --- | --- |
| `enter` | Open selected session, group headers do nothing |
| `alt+enter`, `ctrl+o` | Open selected session in tmux |
| `ctrl+g` | Toggle grouped view |
| `space` | Collapse or expand current group |
| `ctrl+space` | Collapse or expand all groups |
| `h`, `l` | Collapse or expand current group |
| `[` / `]` | Jump to previous or next group |
| `ctrl+d` | Enter delete mode |
| `ctrl+r` | Rename selected session |
| `tab` | Toggle preview pane |
| `J`, `K`, `shift+up`, `shift+down` | Scroll preview |
| Mouse wheel | Scroll preview or list |
| Mouse click | Select row, click group header to fold or unfold |

## Modes

- **ALL** — the default. Pressing `enter` resumes the session directly.
- **TMUX** — `enter` opens the session in a tmux window. The title bar shows `[tmux]` and the UI shifts to purple tones.


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
- [ ] Add more agents like `claude code`, `codex`, `gemini-cli`, `pi`. Maybe rename the project.
- [ ] More tmux controls - close windows, create new opencode sessions, duplicate (fork) sessions from the TUI
  - [ ] `<C-x>` closes the currently active/running tmux window
  - [ ] `n` creates a new session in the path of the currently selected item and let the agent name it. `N` lets the user manually name the session. If in tmux mode also attaches/creates a tmux session in that path.
  - [ ] `y` duplicates the currently selected session in the path of the currently selected item with a `#DUP {oldTitle}` name. `Y` lets the user manually name the duplicated session.
- [ ] Add configurable keybinds, maybe a toml/json config file in `~/.config/ocs/config.{toml,json}`
- [ ] Rework CLI flags (add more options, add `true/false` to already existing flags, etc)
- [ ] Add sorting by different criteria to the session list
- [x] Add `--theme` flag to target `light` or `dark` theme

## License

MIT
