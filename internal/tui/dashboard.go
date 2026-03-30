package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathiasfn/claude-sidebar/internal/claude"
	gitpkg "github.com/mathiasfn/claude-sidebar/internal/git"
	"github.com/mathiasfn/claude-sidebar/internal/tokens"
)

func (m Model) renderDashboard() string {
	w := m.width
	if w < 40 {
		w = 40
	}
	h := m.height
	if h < 10 {
		h = 10
	}

	// Render fixed sections first to measure them
	header := m.renderHeader(w)
	sessions := m.renderSessionsPanel(w)
	dockerPanel := m.renderDockerPanel(w)
	usageBar := m.renderUsageBar(w)
	footer := m.renderFooter(w)

	headerH := strings.Count(header, "\n") + 1
	sessionsH := strings.Count(sessions, "\n") + 1
	dockerH := strings.Count(dockerPanel, "\n") + 1
	usageH := strings.Count(usageBar, "\n") + 1
	footerH := strings.Count(footer, "\n") + 1

	// Git panel gets all remaining height
	gitH := h - headerH - sessionsH - dockerH - usageH - footerH
	if gitH < 5 {
		gitH = 5
	}

	gitPanel := m.renderGitPanel(w, gitH)

	full := lipgloss.JoinVertical(lipgloss.Left, header, sessions, dockerPanel, gitPanel, usageBar, footer)

	// Hard-trim to terminal height so header never scrolls away
	lines := strings.Split(full, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderHeader(width int) string {
	sessions := m.sortedSessions()

	branch := ""
	if m.gitInfo != nil && m.gitInfo.Branch != "" {
		branch = m.gitInfo.Branch
	}

	dirName := "all sessions"
	if m.cwd != "" {
		parts := strings.Split(m.cwd, "/")
		dirName = parts[len(parts)-1]
	}

	title := headerStyle.Render("⬡ claude-sidebar")

	info := dimStyle.Render(dirName)
	if branch != "" {
		info += "  " + lipgloss.NewStyle().Foreground(blue).Render(" "+branch)
	}
	info += "  " + dimStyle.Render(fmt.Sprintf("%d sessions", len(sessions)))

	gap := width - lipgloss.Width(title) - lipgloss.Width(info) - 2
	if gap < 1 {
		gap = 1
	}

	return "\n" + title + strings.Repeat(" ", gap) + info + "\n"
}

// padRight pads a styled string to a visual width, accounting for ANSI escape codes
func padRight(s string, width int) string {
	visual := lipgloss.Width(s)
	if visual >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visual)
}

// padLeft pads a styled string to a visual width with left-alignment of padding
func padLeft(s string, width int) string {
	visual := lipgloss.Width(s)
	if visual >= width {
		return s
	}
	return strings.Repeat(" ", width-visual) + s
}

// Fixed column widths
const (
	colID    = 10
	colModel = 12
	colCtx   = 20
	colAge   = 7
)

func (m Model) renderSessionsPanel(width int) string {
	sessions := m.sortedSessions()
	innerWidth := width - 6 // border + padding

	var rows []string

	if len(sessions) == 0 {
		rows = append(rows, dimStyle.Render("No active sessions"))
	} else {
		// Header: left-align ID+MODEL, right-align CONTEXT+AGE
		leftH := padRight(dimStyle.Render("ID"), colID) + " " + padRight(dimStyle.Render("MODEL"), colModel)
		rightH := padLeft(dimStyle.Render("CONTEXT"), colCtx) + " " + padLeft(dimStyle.Render("AGE"), colAge)
		gap := innerWidth - lipgloss.Width(leftH) - lipgloss.Width(rightH) - 2
		if gap < 1 {
			gap = 1
		}
		rows = append(rows, "  "+leftH+strings.Repeat(" ", gap)+rightH)
		rows = append(rows, dimStyle.Render("  "+strings.Repeat("─", innerWidth-2)))

		for _, s := range sessions {
			rows = append(rows, m.renderSessionRow(s, innerWidth))
		}
	}

	// Recent dead sessions (greyed out)
	if m.watcher != nil {
		dead := m.watcher.RecentDead()
		if len(dead) > 0 {
			rows = append(rows, dimStyle.Render("  "+strings.Repeat("┈", min(innerWidth-2, colID+colModel+colCtx+colAge+5))))
			for _, s := range dead {
				rows = append(rows, m.renderDeadSessionRow(s, innerWidth))
			}
		}
	}

	// Status message (e.g. "Copied!")
	if m.statusMsg != "" && time.Since(m.statusTime) < 3*time.Second {
		rows = append(rows, "")
		rows = append(rows, "  "+lipgloss.NewStyle().Foreground(green).Render(m.statusMsg))
	}

	content := strings.Join(rows, "\n")
	return panelStyle.Width(width - 2).Render(
		panelTitleStyle.Render("Sessions") + "\n" + content,
	)
}

func (m Model) renderSessionRow(s *claude.SessionState, innerWidth int) string {
	sess := s.Session
	data := s.Data

	id := sessionActiveStyle.Render(sess.ShortID())

	modelStr := "starting…"
	ctxStr := ""

	if data != nil && data.Model != "" {
		short := strings.TrimPrefix(data.Model, "claude-")
		if idx := strings.LastIndex(short, "-20"); idx > 0 {
			short = short[:idx]
		}
		modelStr = short

		ctxTokens := data.LastUsage.ContextTokens()
		if ctxTokens > 0 {
			ctxPct := tokens.ContextPercent(data.LastUsage, data.Model)
			ctxStr = fmt.Sprintf("%s/%s (%d%%)",
				tokens.FormatTokens(ctxTokens),
				tokens.FormatTokens(tokens.ContextLimit(data.Model)),
				ctxPct,
			)
		}
	}

	var ctxStyled string
	if data != nil && data.Model != "" {
		ctxPct := tokens.ContextPercent(data.LastUsage, data.Model)
		switch {
		case ctxPct >= 90:
			ctxStyled = lipgloss.NewStyle().Foreground(red).Render(ctxStr)
		case ctxPct >= 70:
			ctxStyled = lipgloss.NewStyle().Foreground(yellow).Render(ctxStr)
		default:
			ctxStyled = lipgloss.NewStyle().Foreground(green).Render(ctxStr)
		}
	} else {
		ctxStyled = dimStyle.Render(ctxStr)
	}

	ageStr := dimStyle.Render(claude.FormatAge(sess.Age()))

	left := padRight(id, colID) + " " + padRight(sessionModelStyle.Render(modelStr), colModel)
	right := padLeft(ctxStyled, colCtx) + " " + padLeft(ageStr, colAge)
	gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	return "  " + left + strings.Repeat(" ", gap) + right
}

func (m Model) renderDeadSessionRow(s *claude.SessionState, innerWidth int) string {
	sess := s.Session
	data := s.Data

	id := dimStyle.Render(sess.ShortID())

	modelStr := "—"
	ctxStr := ""

	if data != nil && data.Model != "" {
		short := strings.TrimPrefix(data.Model, "claude-")
		if idx := strings.LastIndex(short, "-20"); idx > 0 {
			short = short[:idx]
		}
		modelStr = short

		ctxTokens := data.LastUsage.ContextTokens()
		if ctxTokens > 0 {
			ctxPct := tokens.ContextPercent(data.LastUsage, data.Model)
			ctxStr = fmt.Sprintf("%s/%s (%d%%)",
				tokens.FormatTokens(ctxTokens),
				tokens.FormatTokens(tokens.ContextLimit(data.Model)),
				ctxPct,
			)
		}
	}

	ageStr := claude.FormatAge(sess.Age())

	left := padRight(id, colID) + " " + padRight(dimStyle.Render(modelStr), colModel)
	right := padLeft(dimStyle.Render(ctxStr), colCtx) + " " + padLeft(dimStyle.Render(ageStr), colAge)
	gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	return "  " + left + strings.Repeat(" ", gap) + right
}

func (m Model) renderGitPanel(width int, maxHeight int) string {
	if m.gitInfo == nil {
		return panelStyle.Width(width - 2).Height(maxHeight - 2).Render(
			panelTitleStyle.Render("Git") + "\n" + dimStyle.Render("Loading..."),
		)
	}

	innerWidth := width - 4
	// Content height inside the border (border takes 2 lines, title takes 1)
	contentHeight := maxHeight - 3

	// Fixed header: title + tabs + mode summary
	var headerRows []string
	headerRows = append(headerRows, m.renderModeTabs())
	headerRows = append(headerRows, "")

	switch m.gitMode {
	case gitpkg.ModeUnstaged:
		count := m.gitInfo.UnstagedCount + m.gitInfo.UntrackedCount
		if count == 0 {
			headerRows = append(headerRows, dimStyle.Render("  No unstaged changes"))
		} else {
			parts := []string{}
			if m.gitInfo.UnstagedCount > 0 {
				parts = append(parts, gitModifiedStyle.Render(fmt.Sprintf("~%d modified", m.gitInfo.UnstagedCount)))
			}
			if m.gitInfo.UntrackedCount > 0 {
				parts = append(parts, gitUntrackedStyle.Render(fmt.Sprintf("?%d untracked", m.gitInfo.UntrackedCount)))
			}
			headerRows = append(headerRows, "  "+strings.Join(parts, "  "))
		}
	case gitpkg.ModeStaged:
		if m.gitInfo.StagedCount == 0 {
			headerRows = append(headerRows, dimStyle.Render("  No staged changes"))
		} else {
			headerRows = append(headerRows, "  "+gitAddedStyle.Render(fmt.Sprintf("+%d staged for commit", m.gitInfo.StagedCount)))
		}
	case gitpkg.ModeBranch:
		if m.gitInfo.BranchBase == "" {
			headerRows = append(headerRows, dimStyle.Render("  No remote base branch found"))
		} else {
			branchInfo := fmt.Sprintf("  %s vs %s",
				lipgloss.NewStyle().Foreground(blue).Render(m.gitInfo.Branch),
				dimStyle.Render(m.gitInfo.BranchBase),
			)
			if m.gitInfo.AheadCount > 0 {
				branchInfo += "  " + gitAddedStyle.Render(fmt.Sprintf("↑%d", m.gitInfo.AheadCount))
			}
			if m.gitInfo.BehindCount > 0 {
				branchInfo += "  " + gitDeletedStyle.Render(fmt.Sprintf("↓%d", m.gitInfo.BehindCount))
			}
			headerRows = append(headerRows, branchInfo)
		}
	}

	headerRows = append(headerRows, "") // blank line before files

	// Scrollable content: files + commits
	var scrollRows []string

	files := m.gitInfo.FilesForMode(m.gitMode)
	for i, entry := range files {
		scrollRows = append(scrollRows, m.renderFileRow(i, entry, innerWidth))
	}

	if m.gitMode == gitpkg.ModeBranch && len(m.gitInfo.BranchCommits) > 0 {
		scrollRows = append(scrollRows, "")
		scrollRows = append(scrollRows, dimStyle.Render("  commits:"))
		maxCommits := 10
		for i, c := range m.gitInfo.BranchCommits {
			if i >= maxCommits {
				scrollRows = append(scrollRows, dimStyle.Render(fmt.Sprintf("    … +%d more", len(m.gitInfo.BranchCommits)-maxCommits)))
				break
			}
			scrollRows = append(scrollRows, "    "+commitStyle.Render(truncate(c, innerWidth-6)))
		}
	}

	// Calculate visible scroll area
	headerH := len(headerRows)
	scrollAreaH := contentHeight - headerH
	if scrollAreaH < 1 {
		scrollAreaH = 1
	}

	// Apply scroll offset to scrollable content
	offset := m.scrollOffset
	if offset > len(scrollRows)-scrollAreaH {
		offset = len(scrollRows) - scrollAreaH
	}
	if offset < 0 {
		offset = 0
	}

	end := offset + scrollAreaH
	if end > len(scrollRows) {
		end = len(scrollRows)
	}

	visibleRows := scrollRows[offset:end]

	// Scroll indicator
	if len(scrollRows) > scrollAreaH {
		pct := 0
		if len(scrollRows)-scrollAreaH > 0 {
			pct = offset * 100 / (len(scrollRows) - scrollAreaH)
		}
		indicator := dimStyle.Render(fmt.Sprintf("  ── %d/%d (%d%%) ──", end, len(scrollRows), pct))
		visibleRows = append(visibleRows, indicator)
	}

	// Combine header + visible scroll area
	allRows := append(headerRows, visibleRows...)
	content := strings.Join(allRows, "\n")

	return panelStyle.Width(width - 2).Height(maxHeight - 2).Render(
		panelTitleStyle.Render("Git") + "\n" + content,
	)
}

func (m Model) renderFileRow(idx int, entry gitpkg.FileEntry, maxWidth int) string {
	style := fileStyle(entry.Status)
	indicator := string(entry.Status)

	// Line change stats
	stats := ""
	if entry.Additions > 0 || entry.Deletions > 0 {
		parts := []string{}
		if entry.Additions > 0 {
			parts = append(parts, gitAddedStyle.Render(fmt.Sprintf("+%d", entry.Additions)))
		}
		if entry.Deletions > 0 {
			parts = append(parts, gitDeletedStyle.Render(fmt.Sprintf("-%d", entry.Deletions)))
		}
		stats = " " + strings.Join(parts, " ")
	}

	pathWidth := maxWidth - 6 - lipgloss.Width(stats)
	if pathWidth < 10 {
		pathWidth = 10
	}

	if idx == m.fileCursor {
		return fmt.Sprintf("%s%s %s%s",
			cursorStyle.Render("▸ "),
			selectedIndicatorStyle.Render(indicator),
			selectedFileStyle.Render(truncate(entry.Path, pathWidth)),
			stats,
		)
	}
	return fmt.Sprintf("  %s %s%s",
		dimStyle.Render(indicator),
		style.Render(truncate(entry.Path, pathWidth)),
		stats,
	)
}

func (m Model) renderModeTabs() string {
	modes := []struct {
		mode  gitpkg.GitMode
		label string
		count int
	}{
		{gitpkg.ModeUnstaged, "Unstaged", 0},
		{gitpkg.ModeStaged, "Staged", 0},
		{gitpkg.ModeBranch, "Branch", 0},
	}

	if m.gitInfo != nil {
		modes[0].count = m.gitInfo.UnstagedCount + m.gitInfo.UntrackedCount
		modes[1].count = m.gitInfo.StagedCount
		modes[2].count = len(m.gitInfo.BranchFiles)
	}

	var tabs []string
	for i, mode := range modes {
		num := fmt.Sprintf("%d", i+1)
		label := mode.label
		if mode.count > 0 {
			label += fmt.Sprintf(" (%d)", mode.count)
		}

		if mode.mode == m.gitMode {
			tabs = append(tabs, activeTabStyle.Render(fmt.Sprintf(" %s %s ", num, label)))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(fmt.Sprintf(" %s %s ", num, label)))
		}
	}

	return "  " + strings.Join(tabs, " ")
}

func fileStyle(status byte) lipgloss.Style {
	switch status {
	case 'A':
		return gitAddedStyle
	case 'D':
		return gitDeletedStyle
	case '?':
		return gitUntrackedStyle
	default:
		return gitModifiedStyle
	}
}

// renderDiffView shows a full-screen diff for the selected file
func (m Model) renderDiffView() string {
	w := m.width
	if w < 40 {
		w = 40
	}

	var sections []string

	files := m.gitInfo.FilesForMode(m.gitMode)
	fileNum := fmt.Sprintf("[%d/%d]", m.fileCursor+1, len(files))
	modeLabel := modeIndicatorStyle.Render(" " + m.gitMode.String() + " ")
	headerLeft := modeLabel + "  " + panelTitleStyle.Render(m.diffPath)
	headerRight := dimStyle.Render(fileNum)
	gap := w - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight) - 2
	if gap < 1 {
		gap = 1
	}
	sections = append(sections, headerLeft+strings.Repeat(" ", gap)+headerRight)

	navBar := dimStyle.Render("  esc back  j/k scroll  d/u page  J/K file  1/2/3 mode  e open")
	sections = append(sections, navBar)
	sections = append(sections, "")

	if m.diffContent == "" {
		sections = append(sections, dimStyle.Render("  No diff available for this file"))
	} else {
		diffLines := strings.Split(m.diffContent, "\n")

		start := m.diffScrollY
		if start >= len(diffLines) {
			start = len(diffLines) - 1
		}
		if start < 0 {
			start = 0
		}

		viewHeight := m.height - 4
		end := start + viewHeight
		if end > len(diffLines) {
			end = len(diffLines)
		}

		for _, line := range diffLines[start:end] {
			sections = append(sections, colorizeDiffLine(line, w-2))
		}

		if len(diffLines) > viewHeight {
			pct := 0
			if len(diffLines)-viewHeight > 0 {
				pct = start * 100 / (len(diffLines) - viewHeight)
			}
			sections = append(sections, dimStyle.Render(fmt.Sprintf("  ─── %d%% (%d/%d) ───", pct, min(end, len(diffLines)), len(diffLines))))
		}
	}

	return strings.Join(sections, "\n")
}

func colorizeDiffLine(line string, maxWidth int) string {
	if len(line) == 0 {
		return ""
	}

	display := line
	if len(display) > maxWidth {
		display = display[:maxWidth-1] + "…"
	}

	switch {
	case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
		return diffFileHeaderStyle.Render("  " + display)
	case strings.HasPrefix(line, "@@"):
		return diffHunkStyle.Render("  " + display)
	case strings.HasPrefix(line, "+"):
		return diffAddedStyle.Render("  " + display)
	case strings.HasPrefix(line, "-"):
		return diffRemovedStyle.Render("  " + display)
	case strings.HasPrefix(line, "diff "):
		return diffFileHeaderStyle.Render("  " + display)
	case strings.HasPrefix(line, "index "):
		return dimStyle.Render("  " + display)
	case strings.HasPrefix(line, "new file"):
		return gitAddedStyle.Render("  " + display)
	case strings.HasPrefix(line, "deleted file"):
		return gitDeletedStyle.Render("  " + display)
	default:
		return dimStyle.Render("  " + display)
	}
}

func (m Model) renderDockerPanel(width int) string {
	if len(m.containers) == 0 {
		return ""
	}

	innerWidth := width - 4
	var rows []string

	for _, c := range m.containers {
		name := truncate(c.Name, 35)

		left := "  " + sessionActiveStyle.Render(name)
		right := ""
		if c.Ports != "" {
			right = lipgloss.NewStyle().Foreground(blue).Render(c.Ports)
		}

		if right != "" {
			gap := innerWidth - lipgloss.Width(left) - lipgloss.Width(right)
			if gap < 2 {
				gap = 2
			}
			rows = append(rows, left+strings.Repeat(" ", gap)+right)
		} else {
			rows = append(rows, left+"  "+dimStyle.Render(c.Status))
		}
	}

	content := strings.Join(rows, "\n")
	return panelStyle.Width(width - 2).Render(
		panelTitleStyle.Render("Docker") + "  " + dimStyle.Render(fmt.Sprintf("%d containers", len(m.containers))) + "\n" + content,
	)
}

func (m Model) renderUsageBar(width int) string {
	if m.usage == nil {
		return ""
	}

	u := m.usage
	parts := []string{}

	if u.TodayMessages > 0 || u.TodayTools > 0 {
		today := fmt.Sprintf("today: %d msgs, %d tools", u.TodayMessages, u.TodayTools)
		parts = append(parts, today)
	}

	if u.WeekMessages > 0 {
		week := fmt.Sprintf("week: %d msgs, %d sessions", u.WeekMessages, u.WeekSessions)
		parts = append(parts, week)
	}

	if len(parts) == 0 {
		return ""
	}

	return "  " + dimStyle.Render("Usage  "+strings.Join(parts, "  │  "))
}

func (m Model) renderFooter(width int) string {
	return "\n" + footerStyle.Render("j/k navigate  enter diff  e open  c copy id  1/2/3 mode  tab cycle  q quit")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen || maxLen < 4 {
		return s
	}
	return s[:maxLen-1] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
