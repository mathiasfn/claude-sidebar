# claude-sidebar

A read-only TUI companion for [Claude Code](https://docs.anthropic.com/en/docs/claude-code). Run it in a terminal pane next to your Claude Code sessions to get a live overview of session activity, token usage, and git changes.

Built for multi-pane workflows — if you run 4 Claude Code sessions in Ghostty/tmux, `claude-sidebar` gives you one place to see what's happening across all of them.

## Install

```bash
go install github.com/mathiasfn/claude-sidebar/cmd/claude-sidebar@latest
```

Requires Go 1.21+. The binary has no other dependencies.

## Usage

```bash
claude-sidebar              # sessions in current directory
claude-sidebar --cwd /path  # specific directory
claude-sidebar --all        # all active sessions on machine
claude-sidebar --json       # dump session info as JSON
```

## What it shows

### Sessions

- Active sessions for the current directory with model, context usage (tokens/limit with %), and age
- Recent dead sessions greyed out below for context on previous work
- Click a session ID to copy it to clipboard (for `claude --resume <id>`)

### Git (3 modes, switch with `1` `2` `3` or `tab`)

- **Unstaged** — working tree changes + untracked files
- **Staged** — files staged for commit
- **Branch** — all changes vs `origin/master` or `origin/main`, with ahead/behind count and commit list

Each file shows line-level changes (`+42 -7`). Navigate files with `j`/`k` or mouse, press `enter` to view the full diff.

## Keybindings

### Main view

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate files (wraps around) |
| `enter` | View diff for selected file |
| `e` | Open file in `$EDITOR` (falls back to `code`) |
| `c` | Copy first session ID to clipboard |
| `1` `2` `3` | Switch git mode (unstaged/staged/branch) |
| `tab` | Cycle git mode |
| `g` / `G` | Jump to top / bottom |
| `r` | Refresh |
| `q` | Quit |

### Diff view

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll |
| `d` / `u` | Half-page scroll |
| `left` / `right` | Previous / next file |
| `g` / `G` | Top / bottom |
| `e` | Open in editor |
| `1` `2` `3` | Switch git mode |
| `esc` | Back to file list |

Mouse scroll and click-to-select work in both views.

## How it works

`claude-sidebar` reads Claude Code's local session data from `~/.claude/`:

- **Session discovery**: scans `~/.claude/sessions/*.json`, filters by working directory, checks PID liveness to show only active sessions
- **Token tracking**: parses session JSONL files with streaming dedup (last message per UUID), tracks context window usage from the most recent completed message
- **Live updates**: watches JSONL files via fsnotify, refreshes git every 5s, rechecks session liveness every 10s
- **Git info**: runs standard git commands (`status`, `diff`, `log`) against the target directory

All data is read-only. The sidebar never modifies your sessions or git state.

## Tech stack

- [Bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) — Styling
- [fsnotify](https://github.com/fsnotify/fsnotify) — File watching

## License

MIT
