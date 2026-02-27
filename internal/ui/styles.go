package ui

import "github.com/charmbracelet/lipgloss"

var (
	// ANSI 256-color palette — predictable contrast across all terminals
	colorOrange    = lipgloss.Color("208") // bright orange — brand
	colorCyan      = lipgloss.Color("39")  // bright blue-cyan — assistant
	colorBorder    = lipgloss.Color("63")  // medium purple-blue — borders
	colorGray      = lipgloss.Color("246") // medium gray — readable muted text
	colorSubtle    = lipgloss.Color("240") // dark gray — timestamps, faint info
	colorGreen     = lipgloss.Color("82")  // bright green — connected
	colorRed       = lipgloss.Color("196") // bright red — errors
	colorWhite     = lipgloss.Color("255") // near-white
	colorHeaderBg  = lipgloss.Color("235") // dark gray bg — header bar

	// App title
	styleAppTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorOrange)

	// Panes with rounded borders
	styleChatBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	styleInputBoxFocused = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(colorOrange).
				Padding(0, 1)

	// Header bar — slightly elevated background so it reads as a distinct bar
	styleHeaderBar = lipgloss.NewStyle().
			Background(colorHeaderBg).
			Foreground(colorWhite).
			Padding(0, 2)

	// Help line below input
	styleHelp = lipgloss.NewStyle().
			Foreground(colorGray).
			Padding(0, 1)

	// Message labels
	styleUserLabel = lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true)

	styleAssistantLabel = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)

	styleSystemMsg = lipgloss.NewStyle().
			Foreground(colorGray).
			Italic(true)

	// Timestamps — subtle but actually readable
	styleTimestamp = lipgloss.NewStyle().
			Foreground(colorSubtle)

	// Status badges
	styleBadgeSSH = lipgloss.NewStyle().
			Background(colorBorder).
			Foreground(colorWhite).
			Padding(0, 1).
			Bold(true)

	styleBadgeConnected = lipgloss.NewStyle().
				Foreground(colorGreen).
				Bold(true)

	styleBadgeConnecting = lipgloss.NewStyle().
				Foreground(colorGray)

	// Session key in header
	styleSession = lipgloss.NewStyle().
			Foreground(colorGray)

	// Errors
	styleError = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	// Connect / error screens
	styleConnectTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorOrange)

	styleConnectBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 3).
			Width(50)
)
