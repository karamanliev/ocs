# ocs

A fast terminal UI for managing [OpenCode](https://opencode.ai) sessions.

`ocs` reads your local OpenCode SQLite database directly and gives you a keyboard-driven interface to resume, attach, rename, preview, and delete sessions. No fzf, no subprocess hacks, just a single static binary.

https://github.com/user-attachments/assets/029e4199-8e3e-4dda-bee4-a6f7ad915ad8

## Features

- **Dual mode** — `ALL` mode resumes sessions in-place; `TMUX` mode attaches them to tmux windows
- **Fuzzy filtering** — type `/` to filter sessions in real time
- **Preview pane** — visible by default, toggle with `tab`, auto-moves right or below based on terminal width
- **Batch delete** — `ctrl+d` to toggle checkboxes, `enter` to confirm
- **Rename** — `ctrl+r` to rename a session inline
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
ocs              # launch in ALL mode
ocs -tmux        # launch directly in TMUX mode
ocs -no-preview  # launch with preview pane hidden
```

## Modes

- **ALL** — the default. Pressing `enter` resumes the session directly. Press `alt+enter` or `ctrl+o` to open it in tmux instead.
- **TMUX** — `enter` always opens the session in a tmux window. The title bar shows `[tmux]` and the UI shifts to purple tones.


## Theme

`ocs` detects your terminal background (light or dark) on startup and live-reloads when the window regains focus. No manual toggle needed.

## Preview

`ocs` shows a bordered preview pane by default. The pane displays the first user message for the selected session.

- Press `tab` to toggle it on or off
- On wider terminals it appears to the right of the session list
- On narrower terminals it moves below the session list
- The pane header shows `Preview` on the left and `<Tab> Toggle` on the right

## Requirements

- `opencode` must be installed and in your `PATH`
- `tmux` is optional; without it `ocs` works in ALL mode only
- SQLite database is read from `$XDG_DATA_HOME/opencode/opencode.db`

## TODO

- [ ] Add `<C-g>` and `-grouped` flag which toggles grouping by path
- [ ] More tmux controls - close windows, create new opencode sessions, duplicate (fork) sessions from the TUI
- [ ] Rework CLI flags
- [ ] Add configurable keybinds
- [ ] Add `-popup` flag which opens the TUI in a tmux popup

## License

MIT
