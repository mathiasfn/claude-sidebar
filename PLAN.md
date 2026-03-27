# claude-sidebar — Technical Plan

## Vision

A read-only TUI companion for Claude Code. Run `claude-sidebar` in a terminal pane next to your Claude Code session to see live session info, token usage, git context, and conversation activity.

Think `gitui` but for Claude Code sessions.

## Data Sources (all in `~/.claude/`)

### Session Discovery

**Active sessions:** `~/.claude/sessions/{pid}.json`
```json
{
  "pid": 82079,
  "sessionId": "1268a317-32c5-4a32-b76c-6262d1cd6d35",
  "cwd": "/Users/mathias/code/eindom-inspections",
  "startedAt": 1774623975517,
  "kind": "interactive",
  "entrypoint": "cli"
}
```
- Filename is the PID — check `kill -0 {pid}` to verify session is still alive
- Filter by `cwd` to find sessions matching current directory
- Multiple sessions can exist for the same cwd

**Project dir naming:** cwd with `/` replaced by `-`, e.g.:
- `/Users/mathias/code/eindom-app` → `-Users-mathias-code-eindom-app`

### Conversation Data

**Session JSONL:** `~/.claude/projects/{project-dir}/{sessionId}.jsonl`

Line types:
| Type | Keys | Use |
|------|------|-----|
| `assistant` | `message.usage`, `message.model`, `message.stop_reason`, `message.content[].type`, `uuid`, `timestamp`, `gitBranch`, `cwd` | Token counting, model info, tool use tracking |
| `user` | `message.content`, `timestamp`, `isMeta` | Turn counting (skip `isMeta: true`) |
| `progress` | `data.type`, `data.hookName`, `toolUseID` | Tool call activity |
| `system` | `subtype`, `content`, `durationMs` | System events |
| `file-history-snapshot` | `snapshot`, `messageId` | File change tracking |

**Token usage** (on every assistant message):
```json
{
  "input_tokens": 961,
  "cache_creation_input_tokens": 2839,
  "cache_read_input_tokens": 19373,
  "output_tokens": 308,
  "service_tier": "standard"
}
```

**Deduplication:** Each assistant message has a unique `uuid`. In the tested data, each uuid appears exactly once (1:1), so no dedup needed. But streaming may produce multiple lines per uuid with incremental usage — take the **last** line per uuid for final counts.

### Git Info

Standard git CLI commands from the session's `cwd`:
- `git branch --show-current`
- `git status --porcelain`
- `git diff --stat`
- `git log --oneline -5`

## Tech Stack: Go + Bubbletea

**Why Go:**
- Single binary distribution (no runtime deps)
- Bubbletea is the gold standard for Go TUI (lazygit, gum, etc.)
- Lipgloss for styling
- fsnotify for file watching
- Fast startup, low memory

**Dependencies:**
- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/lipgloss` — Styling
- `github.com/charmbracelet/bubbles` — Table, viewport, spinner components
- `github.com/fsnotify/fsnotify` — File watching for live updates

**Install:** `brew install go` (not currently installed, trivial to add)

## Architecture

```
claude-sidebar
├── cmd/
│   └── main.go                  # Entry point, arg parsing
├── internal/
│   ├── claude/
│   │   ├── sessions.go          # Discover & filter active sessions
│   │   ├── parser.go            # Parse JSONL conversation data
│   │   └── watcher.go           # Tail JSONL for live updates (fsnotify + seek)
│   ├── git/
│   │   └── info.go              # Git branch, status, diff, log
│   ├── tokens/
│   │   └── counter.go           # Aggregate token usage, compute cost estimates
│   └── tui/
│       ├── model.go             # Bubbletea model (state + Update + View)
│       ├── sessions_view.go     # Session picker panel
│       ├── dashboard_view.go    # Main dashboard layout
│       ├── panels/
│       │   ├── info.go          # Session info panel (model, pid, started, branch)
│       │   ├── tokens.go        # Token usage panel (in/out/cache/cost)
│       │   ├── git.go           # Git panel (branch, status, diff stat)
│       │   ├── activity.go      # Recent activity feed (last N messages/tool calls)
│       │   └── files.go         # Changed files panel (from file-history-snapshot)
│       └── styles.go            # Lipgloss styles
├── go.mod
├── go.sum
└── Makefile
```

## Screens

### 1. Session Picker (startup if multiple sessions in cwd)

```
╭─ claude-sidebar ─────────────────────────────╮
│                                               │
│  Active sessions in /Users/mathias/code/foo   │
│                                               │
│  > 1268a317  pid:82079  2h ago  inspections   │
│    a0fdabf5  pid:64675  5h ago  inspections   │
│    6412a283  pid:89362  17h ago inspections    │
│                                               │
│  ↑↓ select  enter confirm  q quit             │
╰───────────────────────────────────────────────╯
```

If only one session matches, skip straight to dashboard.

### 2. Dashboard (main view)

```
╭─ Session 1268a317 ─ claude-opus-4-6 ─ inspections ──────────────╮
│                                                                    │
│ ┌─ Info ──────────────────────┐ ┌─ Tokens ────────────────────┐   │
│ │ PID:     82079              │ │ Input:    42,381             │   │
│ │ Model:   claude-opus-4-6    │ │ Output:   3,892             │   │
│ │ Branch:  inspections        │ │ Cache R:  189,204           │   │
│ │ Started: 2h 14m ago        │ │ Cache W:  28,441            │   │
│ │ Turns:   12                 │ │ Est cost: $2.84             │   │
│ └─────────────────────────────┘ └─────────────────────────────┘   │
│                                                                    │
│ ┌─ Git ──────────────────────────────────────────────────────┐    │
│ │ branch: inspections  (+3 -1 staged, 2 unstaged)            │    │
│ │                                                             │    │
│ │  app/Models/Inspection.php        | 42 ++++++----           │    │
│ │  app/Services/InspectionService.php | 18 ++++             │    │
│ │  tests/Feature/InspectionTest.php | 87 ++++++++++++        │    │
│ └─────────────────────────────────────────────────────────────┘    │
│                                                                    │
│ ┌─ Activity (last 5) ────────────────────────────────────────┐    │
│ │ 14:32  assistant  → Read app/Models/Inspection.php         │    │
│ │ 14:32  assistant  → Edit app/Models/Inspection.php         │    │
│ │ 14:31  user       "tilføj defect import"                   │    │
│ │ 14:30  assistant  → Bash git status                        │    │
│ │ 14:29  assistant  → Grep "InspectionDefect"                │    │
│ └─────────────────────────────────────────────────────────────┘    │
│                                                                    │
│  tab: switch panel  r: refresh  s: sessions  q: quit              │
╰──────────────────────────────────────────────────────────────────╯
```

## Implementation Phases

### Phase 1: Core data layer (~2h)
1. Session discovery: scan `~/.claude/sessions/`, filter by cwd, check PID alive
2. JSONL parser: read session file, extract assistant/user/progress messages
3. Token aggregator: sum usage across all assistant messages
4. Git info: branch, status, diff stat

### Phase 2: TUI shell (~2h)
1. Bubbletea app skeleton with session picker → dashboard flow
2. Dashboard layout with Lipgloss (4 panels)
3. Info panel, tokens panel, git panel, activity panel
4. Keyboard navigation (tab between panels, q to quit)

### Phase 3: Live updates (~1h)
1. fsnotify watcher on the session JSONL file
2. Incremental parsing (seek to last position, read new lines)
3. Periodic git refresh (every 5s)
4. Session liveness check (every 10s, kill -0)

### Phase 4: Polish (~1h)
1. Auto-detect cwd from `$PWD` (default) or `--cwd` flag
2. `--session` flag to skip picker and connect directly
3. Responsive layout (adapt to terminal width)
4. Color theme (match Claude Code aesthetic — purple/blue accent)
5. Cost estimation with configurable pricing

## CLI Interface

```bash
claude-sidebar              # Auto-detect sessions in $PWD
claude-sidebar --cwd /path  # Specify directory
claude-sidebar --session ID # Connect to specific session
claude-sidebar --all        # Show all active sessions (any cwd)
claude-sidebar --json       # Dump session info as JSON (scriptable)
```

## Token Cost Estimation

Default pricing (configurable via `~/.claude-sidebar.toml`):
```toml
[pricing.claude-opus-4-6]
input_per_1m = 15.00
output_per_1m = 75.00
cache_read_per_1m = 1.50
cache_write_per_1m = 18.75

[pricing.claude-sonnet-4-6]
input_per_1m = 3.00
output_per_1m = 15.00
cache_read_per_1m = 0.30
cache_write_per_1m = 3.75
```

## Edge Cases & Gotchas

1. **Streaming chunks**: Assistant messages may appear multiple times in JSONL with incremental usage. Always take the last occurrence per `uuid` for token counts.
2. **Stale sessions**: PID file exists but process is dead → mark as "dead" and offer to clean up.
3. **Subagent JSONL**: Sessions can have `subagents/agent-*.jsonl` — include in token totals.
4. **Large JSONL files**: Some sessions can be very large. Use seek-based tailing, never read entire file on refresh.
5. **Permission**: Session files are mode 600 — sidebar must run as same user.
6. **Project dir collision**: Different worktrees of same repo may share a project dir prefix. Match on exact cwd, not prefix.
7. **Multiple project dirs per cwd**: Worktrees get suffixed names (e.g., `--claude-worktrees-elastic-allen`). The session's `sessionId` is the canonical link to the JSONL file.

## Future Ideas (not MVP)

- Sparkline chart of tokens over time
- Subagent tree view (parent → child sessions)
- Task/plan display (read from `.claude/tasks/` and `.claude/plans/`)
- Notification when session completes (terminal bell or desktop notification)
- tmux integration (auto-open in split pane)
- WebSocket mode for remote sessions
