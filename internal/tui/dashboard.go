package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mathias/claude-sidebar/internal/claude"
	"github.com/mathias/claude-sidebar/internal/tokens"
)

func (m Model) renderDashboard() string {
	w := m.width
	if w < 40 {
		w = 40
	}

	contentWidth := w

	var sections []string

	// Header
	sections = append(sections, m.renderHeader(contentWidth))

	// Sessions panel
	sections = append(sections, m.renderSessionsPanel(contentWidth))

	// Git panel
	sections = append(sections, m.renderGitPanel(contentWidth))

	// Footer
	sections = append(sections, m.renderFooter(contentWidth))

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Trim to terminal height
	lines := strings.Split(content, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderHeader(width int) string {
	sessions := m.sortedSessions()

	branch := ""
	if m.gitInfo != nil && m.gitInfo.Branch != "" {
		branch = m.gitInfo.Branch
	}

	// Dir name (last component of cwd)
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

	// Total cost
	if m.watcher != nil {
		total := m.watcher.TotalTokens()
		model := "claude-opus-4-6"
		if len(sessions) > 0 && sessions[0].Data != nil && sessions[0].Data.Model != "" {
			model = sessions[0].Data.Model
		}
		cost := tokens.EstimateCost(total, model)
		if cost > 0 {
			info += "  " + costStyle.Render(fmt.Sprintf("Σ $%.2f", cost))
		}
	}

	gap := width - lipgloss.Width(title) - lipgloss.Width(info) - 2
	if gap < 1 {
		gap = 1
	}

	return title + strings.Repeat(" ", gap) + info + "\n"
}

func (m Model) renderSessionsPanel(width int) string {
	sessions := m.sortedSessions()

	innerWidth := width - 4 // border + padding
	if innerWidth < 30 {
		innerWidth = 30
	}

	var rows []string

	if len(sessions) == 0 {
		rows = append(rows, dimStyle.Render("No active sessions in this directory"))
	} else {
		// Table header
		headerFmt := "  %-10s %-14s %7s %7s %9s %8s %6s  %6s"
		headerArgs := []interface{}{"ID", "MODEL", "IN", "OUT", "CACHE R", "COST", "TURNS", "AGE"}
		if m.cwd == "" {
			headerFmt += "  %s"
			headerArgs = append(headerArgs, "DIR")
		}
		header := fmt.Sprintf(headerFmt, headerArgs...)
		rows = append(rows, dimStyle.Render(header))
		rows = append(rows, dimStyle.Render("  "+strings.Repeat("─", min(innerWidth-2, 70))))

		for _, s := range sessions {
			rows = append(rows, m.renderSessionRow(s, innerWidth))
		}

		// Total row
		if len(sessions) > 1 {
			total := m.watcher.TotalTokens()
			model := "claude-opus-4-6"
			if sessions[0].Data != nil && sessions[0].Data.Model != "" {
				model = sessions[0].Data.Model
			}
			cost := tokens.EstimateCost(total, model)

			totalTurns := 0
			for _, s := range sessions {
				if s.Data != nil {
					totalTurns += s.Data.Turns
				}
			}

			rows = append(rows, dimStyle.Render("  "+strings.Repeat("─", min(innerWidth-2, 70))))
			rows = append(rows, fmt.Sprintf("  %-10s %-14s %7s %7s %9s %s %6d",
				boldValueStyle.Render("TOTAL"),
				"",
				boldValueStyle.Render(tokens.FormatTokens(total.InputTokens)),
				boldValueStyle.Render(tokens.FormatTokens(total.OutputTokens)),
				boldValueStyle.Render(tokens.FormatTokens(total.CacheReadInputTokens)),
				costStyle.Render(fmt.Sprintf("$%6.2f", cost)),
				totalTurns,
			))
		}
	}

	content := strings.Join(rows, "\n")
	panel := panelStyle.Width(width - 2).Render(
		panelTitleStyle.Render("Sessions") + "\n" + content,
	)
	return panel
}

func (m Model) renderSessionRow(s *claude.SessionState, width int) string {
	sess := s.Session
	data := s.Data

	id := sessionActiveStyle.Render(sess.ShortID())
	age := dimStyle.Render(fmt.Sprintf("%6s", claude.FormatAge(sess.Age())))

	if data == nil || data.Model == "" {
		row := fmt.Sprintf("  %s  %s  %s",
			id, dimStyle.Render(fmt.Sprintf("pid:%d", sess.PID)), age)
		if m.cwd == "" {
			dirParts := strings.Split(sess.Cwd, "/")
			row += "  " + dimStyle.Render(dirParts[len(dirParts)-1])
		}
		return row
	}

	// Short model name
	short := data.Model
	short = strings.TrimPrefix(short, "claude-")
	if len(short) > 12 {
		short = short[:12]
	}
	model := sessionModelStyle.Render(short)

	cost := tokens.EstimateCost(data.Tokens, data.Model)

	row := fmt.Sprintf("  %-10s %-14s %7s %7s %9s %s %6d  %s",
		id,
		model,
		valueStyle.Render(tokens.FormatTokens(data.Tokens.InputTokens)),
		valueStyle.Render(tokens.FormatTokens(data.Tokens.OutputTokens)),
		valueStyle.Render(tokens.FormatTokens(data.Tokens.CacheReadInputTokens)),
		sessionCostStyle.Render(fmt.Sprintf("$%6.2f", cost)),
		data.Turns,
		age,
	)

	if m.cwd == "" {
		dirParts := strings.Split(sess.Cwd, "/")
		row += "  " + dimStyle.Render(dirParts[len(dirParts)-1])
	}

	return row
}

func (m Model) renderGitPanel(width int) string {
	if m.gitInfo == nil {
		return panelStyle.Width(width - 2).Render(
			panelTitleStyle.Render("Git") + "\n" + dimStyle.Render("Loading..."),
		)
	}

	innerWidth := width - 4
	var rows []string

	// Summary line
	status := m.gitInfo.Status
	staged := 0
	unstaged := 0
	untracked := 0
	for _, e := range status {
		if e.Staging != ' ' && e.Staging != '?' {
			staged++
		}
		if e.Working != ' ' && e.Working != '?' {
			unstaged++
		}
		if e.Staging == '?' {
			untracked++
		}
	}

	var summaryParts []string
	if staged > 0 {
		summaryParts = append(summaryParts, gitAddedStyle.Render(fmt.Sprintf("+%d staged", staged)))
	}
	if unstaged > 0 {
		summaryParts = append(summaryParts, gitModifiedStyle.Render(fmt.Sprintf("~%d modified", unstaged)))
	}
	if untracked > 0 {
		summaryParts = append(summaryParts, gitUntrackedStyle.Render(fmt.Sprintf("?%d untracked", untracked)))
	}
	if len(summaryParts) == 0 {
		summaryParts = append(summaryParts, dimStyle.Render("working tree clean"))
	}
	rows = append(rows, strings.Join(summaryParts, "  "))

	// File list with cursor
	if len(status) > 0 {
		rows = append(rows, "")
		maxFiles := 30
		for i, entry := range status {
			if i >= maxFiles {
				rows = append(rows, dimStyle.Render(fmt.Sprintf("  … and %d more", len(status)-maxFiles)))
				break
			}

			style := gitModifiedStyle
			switch {
			case entry.Staging == '?' || entry.Working == '?':
				style = gitUntrackedStyle
			case entry.Staging == 'A':
				style = gitAddedStyle
			case entry.Staging == 'D' || entry.Working == 'D':
				style = gitDeletedStyle
			}

			indicator := string(entry.Staging) + string(entry.Working)

			// Highlight selected file
			cursor := "  "
			if i == m.fileCursor {
				cursor = cursorStyle.Render("▸ ")
				// Highlight the whole row
				rows = append(rows, fmt.Sprintf("%s%s %s",
					cursor,
					selectedIndicatorStyle.Render(indicator),
					selectedFileStyle.Render(truncate(entry.Path, innerWidth-6)),
				))
			} else {
				rows = append(rows, fmt.Sprintf("%s%s %s",
					cursor,
					dimStyle.Render(indicator),
					style.Render(truncate(entry.Path, innerWidth-6)),
				))
			}
		}
	}

	content := strings.Join(rows, "\n")
	panel := panelStyle.Width(width - 2).Render(
		panelTitleStyle.Render("Git") + "\n" + content,
	)
	return panel
}

// renderDiffView shows a full-screen diff for the selected file
func (m Model) renderDiffView() string {
	w := m.width
	if w < 40 {
		w = 40
	}

	var sections []string

	// Header bar with file path and navigation hints
	fileNum := fmt.Sprintf("[%d/%d]", m.fileCursor+1, len(m.gitInfo.Status))
	headerLeft := panelTitleStyle.Render("  " + m.diffPath)
	headerRight := dimStyle.Render(fileNum)
	gap := w - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight) - 2
	if gap < 1 {
		gap = 1
	}
	sections = append(sections, headerLeft+strings.Repeat(" ", gap)+headerRight)

	// Navigation bar
	navBar := dimStyle.Render("  esc back  j/k scroll  d/u half-page  J/K prev/next file  g/G top/bottom")
	sections = append(sections, navBar)
	sections = append(sections, "")

	// Diff content
	if m.diffContent == "" {
		sections = append(sections, dimStyle.Render("  No diff available for this file"))
	} else {
		diffLines := strings.Split(m.diffContent, "\n")

		// Apply scroll
		start := m.diffScrollY
		if start >= len(diffLines) {
			start = len(diffLines) - 1
		}
		if start < 0 {
			start = 0
		}

		viewHeight := m.height - 4 // header + nav + padding
		end := start + viewHeight
		if end > len(diffLines) {
			end = len(diffLines)
		}

		visible := diffLines[start:end]

		for _, line := range visible {
			sections = append(sections, colorizeDiffLine(line, w-2))
		}

		// Scroll indicator
		if len(diffLines) > viewHeight {
			pct := 0
			if len(diffLines)-viewHeight > 0 {
				pct = start * 100 / (len(diffLines) - viewHeight)
			}
			scrollInfo := dimStyle.Render(fmt.Sprintf("  ─── %d%% (%d/%d lines) ───", pct, start+len(visible), len(diffLines)))
			sections = append(sections, scrollInfo)
		}
	}

	return strings.Join(sections, "\n")
}

func colorizeDiffLine(line string, maxWidth int) string {
	if len(line) == 0 {
		return ""
	}

	// Truncate very long lines
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

func colorizeDiffStat(line string) string {
	idx := strings.LastIndex(line, "|")
	if idx < 0 {
		return dimStyle.Render(line)
	}

	filePart := line[:idx]
	statPart := line[idx:]

	var result strings.Builder
	result.WriteString(dimStyle.Render(filePart))

	for _, ch := range statPart {
		switch ch {
		case '+':
			result.WriteString(gitAddedStyle.Render("+"))
		case '-':
			result.WriteString(gitDeletedStyle.Render("-"))
		default:
			result.WriteString(dimStyle.Render(string(ch)))
		}
	}
	return result.String()
}

func (m Model) renderFooter(width int) string {
	return "\n" + footerStyle.Render("j/k navigate files  enter view diff  r refresh  q quit")
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
