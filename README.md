# ocs

A fast terminal UI for managing [OpenCode](https://opencode.ai) sessions.

`ocs` reads your local OpenCode SQLite database directly and gives you a keyboard-driven interface to resume, attach, rename, preview, and delete sessions. No fzf, no subprocess hacks, just a single static binary.

## Features

- **Dual mode** — `ALL` mode resumes sessions in-place; `TMUX` mode attaches them to tmux windows
- **Fuzzy filtering** — type `/` to filter sessions in real time
- **Preview** — `tab` to peek the first user message of any session
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
```

## Modes

- **ALL** — the default. Pressing `enter` resumes the session directly. Press `alt+enter` or `ctrl+o` to open it in tmux instead.
- **TMUX** — `enter` always opens the session in a tmux window. The title bar shows `[tmux]` and the UI shifts to purple tones.

Use `-tmux` to start directly in TMUX mode.

## Theme

`ocs` detects your terminal background (light or dark) on startup and live-reloads when the window regains focus. No manual toggle needed.

The UI adapts its palette to the current mode:

| Element | ALL | TMUX |
|---|---|---|
| Border title | steel blue | dusty purple |
| Cursor highlight | blue-tinted | purple-tinted |
| Footer keys | steel blue | dusty purple |
| Running indicator | green | green |

## Requirements

- `opencode` must be installed and in your `PATH`
- `tmux` is optional; without it `ocs` works in ALL mode only
- SQLite database is read from `$XDG_DATA_HOME/opencode/opencode.db`

## License

MIT
