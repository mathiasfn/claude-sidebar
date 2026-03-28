package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mathiasfn/claude-sidebar/internal/claude"
	gitpkg "github.com/mathiasfn/claude-sidebar/internal/git"
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

	// Status message (e.g. "copied!")
	statusMsg  string
	statusTime time.Time

	// Git mode: unstaged / staged / branch
	gitMode gitpkg.GitMode

	// Git panel scroll (internal to git panel)
	scrollOffset int


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
			case tea.MouseButtonWheelDown:
				m.diffScrollY += 3
			}
			return m, nil
		}
		// Main view
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.scrollOffset > 0 {
				m.scrollOffset -= 3
				if m.scrollOffset < 0 {
					m.scrollOffset = 0
				}
			}
		case tea.MouseButtonWheelDown:
			m.scrollOffset += 3
			files := m.currentFiles()
			maxScroll := len(files) - m.gitScrollAreaHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset > maxScroll {
				m.scrollOffset = maxScroll
			}
		case tea.MouseButtonLeft:
			// Render header+sessions to measure exact Y positions
			w := m.width
			if w < 40 {
				w = 40
			}
			headerLines := strings.Count(m.renderHeader(w), "\n") + 1
			sessionsPanel := m.renderSessionsPanel(w)
			sessionsPanelLines := strings.Count(sessionsPanel, "\n") + 1

			// Session rows: inside sessions panel, after border(1) + title(1) + header(1) + separator(1) = 4 lines
			sessions := m.sortedSessions()
			sessionRowsStart := headerLines + 4
			clickedSession := msg.Y - sessionRowsStart
			if clickedSession >= 0 && clickedSession < len(sessions) {
				sid := sessions[clickedSession].Session.SessionID
				copyToClipboard(sid)
				m.statusMsg = "Copied: " + sid[:8]
				m.statusTime = time.Now()
				return m, nil
			}

			// File rows: after header + sessions panel + git panel chrome
			// git panel chrome: border(1) + title(1) + tabs(1) + blank(1) + summary(1) + blank(1) = 6
			fileRowsStart := headerLines + sessionsPanelLines + 6
			files := m.currentFiles()
			if len(files) > 0 {
				clickedIdx := msg.Y - fileRowsStart + m.scrollOffset
				if clickedIdx >= 0 && clickedIdx < len(files) {
					m.fileCursor = clickedIdx
					m.ensureCursorVisible()
				}
			}
		}
		return m, nil

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

	case editorOpenedMsg:
		return m, nil

	case copiedMsg:
		m.statusMsg = "Copied: " + msg.text
		m.statusTime = time.Now()
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
		files := m.currentFiles()
		if len(files) > 0 {
			m.fileCursor++
			if m.fileCursor >= len(files) {
				m.fileCursor = 0 // wrap to top
				m.scrollOffset = 0
			}
		}
		m.ensureCursorVisible()
		return m, nil
	case "k", "up":
		files := m.currentFiles()
		if len(files) > 0 {
			m.fileCursor--
			if m.fileCursor < 0 {
				m.fileCursor = len(files) - 1 // wrap to bottom
			}
		}
		m.ensureCursorVisible()
		return m, nil
	case "g":
		m.fileCursor = 0
		m.scrollOffset = 0
		return m, nil
	case "G":
		files := m.currentFiles()
		if len(files) > 0 {
			m.fileCursor = len(files) - 1
		}
		m.ensureCursorVisible()
		return m, nil
	case "enter", "l", "right":
		return m, m.loadDiff()
	case "e", "o":
		return m, m.openInEditor()
	case "c":
		return m, m.copySessionID()
	case "1":
		m.gitMode = gitpkg.ModeUnstaged
		m.fileCursor = 0
		return m, nil
	case "2":
		m.gitMode = gitpkg.ModeStaged
		m.fileCursor = 0
		return m, nil
	case "3":
		m.gitMode = gitpkg.ModeBranch
		m.fileCursor = 0
		return m, nil
	case "tab":
		m.gitMode = (m.gitMode + 1) % 3
		m.fileCursor = 0
		return m, nil
	case "shift+tab":
		m.gitMode = (m.gitMode + 2) % 3
		m.fileCursor = 0
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
	case "esc", "h":
		m.viewingDiff = false
		m.diffContent = ""
		m.diffPath = ""
		return m, nil
	case "left":
		files := m.currentFiles()
		if len(files) > 0 {
			m.fileCursor--
			if m.fileCursor < 0 {
				m.fileCursor = len(files) - 1
			}
			return m, m.loadDiff()
		}
		return m, nil
	case "right":
		files := m.currentFiles()
		if len(files) > 0 {
			m.fileCursor++
			if m.fileCursor >= len(files) {
				m.fileCursor = 0
			}
			return m, m.loadDiff()
		}
		return m, nil
	case "e", "o":
		return m, m.openInEditor()
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
		lines := countLines(m.diffContent)
		if lines > m.height-4 {
			m.diffScrollY = lines - m.height + 4
		}
		return m, nil
	case "J", "tab":
		files := m.currentFiles()
		if len(files) > 0 {
			m.fileCursor++
			if m.fileCursor >= len(files) {
				m.fileCursor = 0
			}
			return m, m.loadDiff()
		}
		return m, nil
	case "K", "shift+tab":
		files := m.currentFiles()
		if len(files) > 0 {
			m.fileCursor--
			if m.fileCursor < 0 {
				m.fileCursor = len(files) - 1
			}
			return m, m.loadDiff()
		}
		return m, nil
	case "1":
		m.gitMode = gitpkg.ModeUnstaged
		m.fileCursor = 0
		return m, m.loadDiff()
	case "2":
		m.gitMode = gitpkg.ModeStaged
		m.fileCursor = 0
		return m, m.loadDiff()
	case "3":
		m.gitMode = gitpkg.ModeBranch
		m.fileCursor = 0
		return m, m.loadDiff()
	}
	return m, nil
}

type editorOpenedMsg struct{}
type copiedMsg struct{ text string }

func copyToClipboard(text string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, fall back to xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	default:
		return
	}
	cmd.Stdin = strings.NewReader(text)
	cmd.Run()
}

func (m Model) copySessionID() tea.Cmd {
	sessions := m.sortedSessions()
	if len(sessions) == 0 {
		return nil
	}
	sessionID := sessions[0].Session.SessionID
	return func() tea.Msg {
		copyToClipboard(sessionID)
		return copiedMsg{text: sessionID[:8]}
	}
}

func (m Model) openInEditor() tea.Cmd {
	if m.gitInfo == nil {
		return nil
	}
	files := m.gitInfo.FilesForMode(m.gitMode)
	if m.fileCursor >= len(files) || len(files) == 0 {
		return nil
	}
	filePath := filepath.Join(m.cwd, files[m.fileCursor].Path)
	return func() tea.Msg {
		// Try $EDITOR, then "code", then "open"
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "code"
		}
		cmd := exec.Command(editor, filePath)
		cmd.Start() // fire and forget
		return editorOpenedMsg{}
	}
}

func (m Model) loadDiff() tea.Cmd {
	if m.gitInfo == nil {
		return nil
	}
	files := m.gitInfo.FilesForMode(m.gitMode)
	if m.fileCursor >= len(files) || len(files) == 0 {
		return nil
	}
	path := files[m.fileCursor].Path
	cwd := m.cwd
	mode := m.gitMode
	return func() tea.Msg {
		diff := gitpkg.GetFileDiff(cwd, path, mode)
		return fileDiffMsg{path: path, diff: diff}
	}
}

// ensureCursorVisible adjusts scrollOffset so the selected file is visible
// in the git panel's internal scroll area
func (m *Model) ensureCursorVisible() {
	// scrollOffset is now the git panel's internal scroll position
	// fileCursor maps directly to the scroll content row index
	scrollAreaH := m.gitScrollAreaHeight()
	if scrollAreaH < 1 {
		return
	}

	if m.fileCursor < m.scrollOffset {
		m.scrollOffset = m.fileCursor
	} else if m.fileCursor >= m.scrollOffset+scrollAreaH {
		m.scrollOffset = m.fileCursor - scrollAreaH + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// gitScrollAreaHeight estimates the scrollable area height in the git panel
func (m *Model) gitScrollAreaHeight() int {
	if m.height == 0 {
		return 10
	}
	// header(2) + sessions panel border(2) + rows + separator + status + footer(2)
	sessions := m.sortedSessions()
	fixedAbove := 2 + 4 + len(sessions) + 2 // header, session panel chrome, rows, footer
	// git panel: border(2) + title(1) + tabs(1) + blank(1) + summary(1) + blank(1) = 7 fixed
	gitFixed := 7
	available := m.height - fixedAbove - gitFixed
	if available < 3 {
		available = 3
	}
	return available
}

// computeFileListStartY estimates the Y position where files start
func (m Model) computeFileListStartY() int {
	w := m.width
	if w < 40 {
		w = 40
	}
	headerH := strings.Count(m.renderHeader(w), "\n") + 1
	sessionsH := strings.Count(m.renderSessionsPanel(w), "\n") + 1
	// git panel chrome: border(1) + title(1) + tabs(1) + blank(1) + summary(1) + blank(1) = 6
	return headerH + sessionsH + 6
}

func (m Model) currentFiles() []gitpkg.FileEntry {
	if m.gitInfo == nil {
		return nil
	}
	return m.gitInfo.FilesForMode(m.gitMode)
}

func (m *Model) clampCursor() {
	if m.gitInfo == nil {
		m.fileCursor = 0
		return
	}
	files := m.gitInfo.FilesForMode(m.gitMode)
	if len(files) == 0 {
		m.fileCursor = 0
		return
	}
	if m.fileCursor >= len(files) {
		m.fileCursor = len(files) - 1
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
