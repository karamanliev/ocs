## 0.3.0

### New features
- Grouped session view by project path, enabled by default
- `ctrl+g` toggle for grouped mode, `space` and `h/l` for fold control, `ctrl+space` for fold-all

### Changed
- `--grouped` now defaults to `true` and supports explicit boolean values
- `--tmux` now supports explicit boolean values
- `--preview` replaces `--no-preview` and supports explicit boolean values

## 0.2.0

### New features
- `--theme` flag to force light or dark palette (OSC 11 doesn't work inside tmux popups)
- Preview pane scrollable with `J`/`K`, `shift+↓`/`shift+↑`, and mouse wheel
- Preview pane shows first and latest exchange with model name tag
- Tmux windows auto-rename to session title (trimmed to 10 chars)
- Scrollbar in preview pane

### Changed
- Flags use `--` prefix instead of `-` (e.g. `--tmux`, `--preview`, `--theme`)
- Preview pane now shows first and latest message pairs (not just first)
- Preview pane truncation skips blank lines before counting content lines

### Fixed
- Flag parsing rewritten to support `--flag=value` and `--flag value` syntax, including boolean flags

## 0.1.0
- Initial release.
