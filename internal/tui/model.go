package tui

import (
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mathias/claude-sidebar/internal/claude"
	gitpkg "github.com/mathias/claude-sidebar/internal/git"
)

// Messages
type tickMsg struct{}

type gitRefreshMsg struct {
	info *gitpkg.Info
}

type watcherReadyMsg struct {
	watcher *claude.Watcher
}

type fileDiffMsg struct {
	path string
	diff string
}

type Model struct {
	cwd      string
	watcher  *claude.Watcher
	gitInfo  *gitpkg.Info
	width    int
	height   int
	quitting bool

	// Git file navigation
	fileCursor  int
	viewingDiff bool
	diffPath    string
	diffContent string
	diffScrollY int
}

func NewModel(cwd string) Model {
	return Model{
		cwd: cwd,
	}
}

func (m Model) Init() tea.Cmd {
	cwd := m.cwd
	return tea.Batch(
		startWatcher(cwd),
		refreshGit(cwd),
		tick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Diff view has its own keybindings
		if m.viewingDiff {
			return m.updateDiffView(msg)
		}
		return m.updateMainView(msg)

	case tea.MouseMsg:
		if m.viewingDiff {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.diffScrollY > 0 {
					m.diffScrollY -= 3
					if m.diffScrollY < 0 {
						m.diffScrollY = 0
					}
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				m.diffScrollY += 3
				return m, nil
			case tea.MouseButtonLeft:
				// Click anywhere to go back
				// (could be refined to only close on specific areas)
			}
		} else {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.fileCursor > 0 {
					m.fileCursor--
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				m.fileCursor++
				m.clampCursor()
				return m, nil
			case tea.MouseButtonLeft:
				// Try to figure out which file was clicked based on Y position
				if m.gitInfo != nil && len(m.gitInfo.Status) > 0 {
					return m, m.loadDiff()
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case watcherReadyMsg:
		m.watcher = msg.watcher
		return m, nil

	case gitRefreshMsg:
		m.gitInfo = msg.info
		m.clampCursor()
		return m, nil

	case fileDiffMsg:
		m.diffPath = msg.path
		m.diffContent = msg.diff
		m.viewingDiff = true
		m.diffScrollY = 0
		return m, nil

	case tickMsg:
		return m, tea.Batch(tick(), refreshGit(m.cwd))
	}

	return m, nil
}

func (m *Model) updateMainView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		if m.watcher != nil {
			m.watcher.Stop()
		}
		return m, tea.Quit
	case "r":
		return m, refreshGit(m.cwd)
	case "j", "down":
		m.fileCursor++
		m.clampCursor()
		return m, nil
	case "k", "up":
		if m.fileCursor > 0 {
			m.fileCursor--
		}
		return m, nil
	case "g":
		m.fileCursor = 0
		return m, nil
	case "G":
		if m.gitInfo != nil && len(m.gitInfo.Status) > 0 {
			m.fileCursor = len(m.gitInfo.Status) - 1
		}
		return m, nil
	case "enter", "l", "right":
		if m.gitInfo != nil && len(m.gitInfo.Status) > 0 {
			return m, m.loadDiff()
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) updateDiffView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		if m.watcher != nil {
			m.watcher.Stop()
		}
		return m, tea.Quit
	case "esc", "h", "left":
		m.viewingDiff = false
		m.diffContent = ""
		m.diffPath = ""
		return m, nil
	case "j", "down":
		m.diffScrollY++
		return m, nil
	case "k", "up":
		if m.diffScrollY > 0 {
			m.diffScrollY--
		}
		return m, nil
	case "d", "ctrl+d":
		m.diffScrollY += m.height / 2
		return m, nil
	case "u", "ctrl+u":
		m.diffScrollY -= m.height / 2
		if m.diffScrollY < 0 {
			m.diffScrollY = 0
		}
		return m, nil
	case "g":
		m.diffScrollY = 0
		return m, nil
	case "G":
		// Jump to bottom
		lines := countLines(m.diffContent)
		if lines > m.height-4 {
			m.diffScrollY = lines - m.height + 4
		}
		return m, nil
	// Navigate to prev/next file while in diff view
	case "J", "tab":
		if m.gitInfo != nil && m.fileCursor < len(m.gitInfo.Status)-1 {
			m.fileCursor++
			return m, m.loadDiff()
		}
		return m, nil
	case "K", "shift+tab":
		if m.fileCursor > 0 {
			m.fileCursor--
			return m, m.loadDiff()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) loadDiff() tea.Cmd {
	if m.gitInfo == nil || m.fileCursor >= len(m.gitInfo.Status) {
		return nil
	}
	path := m.gitInfo.Status[m.fileCursor].Path
	cwd := m.cwd
	return func() tea.Msg {
		diff := gitpkg.GetFileDiff(cwd, path)
		return fileDiffMsg{path: path, diff: diff}
	}
}

func (m *Model) clampCursor() {
	if m.gitInfo == nil || len(m.gitInfo.Status) == 0 {
		m.fileCursor = 0
		return
	}
	if m.fileCursor >= len(m.gitInfo.Status) {
		m.fileCursor = len(m.gitInfo.Status) - 1
	}
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 1
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}

func startWatcher(cwd string) tea.Cmd {
	return func() tea.Msg {
		w, err := claude.NewWatcher(cwd, nil)
		if err != nil {
			return nil
		}
		w.Start()
		return watcherReadyMsg{watcher: w}
	}
}

func refreshGit(cwd string) tea.Cmd {
	return func() tea.Msg {
		info := gitpkg.GetInfo(cwd)
		return gitRefreshMsg{info: info}
	}
}

func tick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) sortedSessions() []*claude.SessionState {
	if m.watcher == nil {
		return nil
	}
	sessions := m.watcher.Sessions()
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Session.StartedAt > sessions[j].Session.StartedAt
	})
	return sessions
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "Loading..."
	}
	if m.viewingDiff {
		return m.renderDiffView()
	}
	return m.renderDashboard()
}
