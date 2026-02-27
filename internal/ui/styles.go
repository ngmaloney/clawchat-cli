package ui

import "github.com/charmbracelet/lipgloss"

var (
	orange = lipgloss.Color("#FF6B35")
	gray   = lipgloss.Color("#888888")
	dimGray = lipgloss.Color("#444444")
	white  = lipgloss.Color("#FFFFFF")
	red    = lipgloss.Color("#FF4444")
	green  = lipgloss.Color("#44FF88")
	blue   = lipgloss.Color("#4488FF")

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(orange)

	styleHeader = lipgloss.NewStyle().
			Background(lipgloss.Color("#1A1A1A")).
			Foreground(white).
			Padding(0, 1)

	styleFooter = lipgloss.NewStyle().
			Background(lipgloss.Color("#1A1A1A")).
			Foreground(gray).
			Padding(0, 1)

	styleStatusOK = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	styleStatusErr = lipgloss.NewStyle().
			Foreground(red).
			Bold(true)

	styleStatusWait = lipgloss.NewStyle().
			Foreground(blue)

	styleDivider = lipgloss.NewStyle().
			Foreground(dimGray)

	styleUserLabel = lipgloss.NewStyle().
			Foreground(orange).
			Bold(true)

	styleAssistantLabel = lipgloss.NewStyle().
				Foreground(blue).
				Bold(true)

	styleTimestamp = lipgloss.NewStyle().
			Foreground(dimGray)

	styleError = lipgloss.NewStyle().
			Foreground(red).
			Padding(1, 2)

	styleHelp = lipgloss.NewStyle().
			Foreground(dimGray)

	styleBadgeSSH = lipgloss.NewStyle().
			Background(blue).
			Foreground(white).
			Padding(0, 1).
			Bold(true)

	styleBadgeConnecting = lipgloss.NewStyle().
				Background(lipgloss.Color("#666666")).
				Foreground(white).
				Padding(0, 1)

	styleBadgeConnected = lipgloss.NewStyle().
				Background(green).
				Foreground(lipgloss.Color("#000000")).
				Padding(0, 1).
				Bold(true)
)
