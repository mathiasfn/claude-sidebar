package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors — Claude aesthetic
	purple    = lipgloss.Color("#A855F7")
	dimPurple = lipgloss.Color("#7C3AED")
	blue      = lipgloss.Color("#60A5FA")
	dimBlue   = lipgloss.Color("#3B82F6")
	green     = lipgloss.Color("#34D399")
	yellow    = lipgloss.Color("#FBBF24")
	red       = lipgloss.Color("#F87171")
	cyan      = lipgloss.Color("#22D3EE")
	dimWhite  = lipgloss.Color("#9CA3AF")
	white     = lipgloss.Color("#F3F4F6")
	bg        = lipgloss.Color("#111111")
	darkGreen = lipgloss.Color("#064E3B")
	darkRed   = lipgloss.Color("#7F1D1D")

	// Panel border
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(dimPurple).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true)

	// Header
	headerStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true).
			Padding(0, 1)

	// Labels and values
	labelStyle = lipgloss.NewStyle().
			Foreground(dimWhite).
			Width(12)

	valueStyle = lipgloss.NewStyle().
			Foreground(white)

	// Session row
	sessionActiveStyle = lipgloss.NewStyle().
				Foreground(green)

	sessionModelStyle = lipgloss.NewStyle().
				Foreground(blue)

	sessionCostStyle = lipgloss.NewStyle().
				Foreground(yellow)

	// Git file list
	gitAddedStyle = lipgloss.NewStyle().
			Foreground(green)

	gitModifiedStyle = lipgloss.NewStyle().
				Foreground(yellow)

	gitDeletedStyle = lipgloss.NewStyle().
				Foreground(red)

	gitUntrackedStyle = lipgloss.NewStyle().
				Foreground(dimWhite)

	// File cursor and selection
	cursorStyle = lipgloss.NewStyle().
			Foreground(purple).
			Bold(true)

	selectedFileStyle = lipgloss.NewStyle().
				Foreground(white).
				Bold(true)

	selectedIndicatorStyle = lipgloss.NewStyle().
				Foreground(purple).
				Bold(true)

	// Diff view
	diffAddedStyle = lipgloss.NewStyle().
			Foreground(green)

	diffRemovedStyle = lipgloss.NewStyle().
				Foreground(red)

	diffHunkStyle = lipgloss.NewStyle().
			Foreground(cyan).
			Bold(true)

	diffFileHeaderStyle = lipgloss.NewStyle().
				Foreground(white).
				Bold(true)

	// Footer
	footerStyle = lipgloss.NewStyle().
			Foreground(dimWhite).
			Padding(0, 1)

	// Dim text
	dimStyle = lipgloss.NewStyle().
			Foreground(dimWhite)

	// Bold value
	boldValueStyle = lipgloss.NewStyle().
			Foreground(white).
			Bold(true)

	// Cost highlight
	costStyle = lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true)
)
