package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Color palette
	colorPrimary   = lipgloss.Color("#FF6B35") // orange — ClawChat brand
	colorBlue      = lipgloss.Color("#4A90E2") // border blue
	colorMuted     = lipgloss.Color("#6C757D") // gray
	colorDimmed    = lipgloss.Color("#3A3A3A") // very dim
	colorSuccess   = lipgloss.Color("#6BCF7F") // green
	colorDanger    = lipgloss.Color("#FF6B6B") // red
	colorWhite     = lipgloss.Color("#FFFFFF")
	colorAssistant = lipgloss.Color("#87CEEB") // sky blue

	// Title
	styleAppTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	// Panes
	styleChatBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(0, 1)

	styleInputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(0, 1)

	styleInputBoxFocused = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	// Header bar
	styleHeaderBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#1A1A2E")).
			Foreground(colorWhite).
			Padding(0, 2)

	// Footer / help
	styleHelp = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Message labels
	styleUserLabel = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	styleAssistantLabel = lipgloss.NewStyle().
				Foreground(colorAssistant).
				Bold(true)

	styleSystemMsg = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	styleTimestamp = lipgloss.NewStyle().
			Foreground(colorDimmed)

	// Status badges
	styleBadgeSSH = lipgloss.NewStyle().
			Background(colorBlue).
			Foreground(colorWhite).
			Padding(0, 1).
			Bold(true)

	styleBadgeConnected = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	styleBadgeConnecting = lipgloss.NewStyle().
				Foreground(colorMuted)

	// Session label
	styleSession = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Error
	styleError = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	// Connect screen
	styleConnectTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary).
				MarginBottom(1)

	styleConnectBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(1, 3).
			Width(50)
)
