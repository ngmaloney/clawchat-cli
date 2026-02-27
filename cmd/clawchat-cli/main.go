package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ngmaloney/clawchat-cli/internal/config"
	"github.com/ngmaloney/clawchat-cli/internal/ui"
)

// Set by GoReleaser via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clawchat-cli: config error: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "clawchat-cli: %v\n\n", err)
		fmt.Fprintf(os.Stderr, "Config file: %s\n\n", config.FilePath())
		fmt.Fprintf(os.Stderr, "Example config:\n")
		fmt.Fprintf(os.Stderr, "  gateway_url: ws://pinchy.home.wrox.us:18789\n")
		fmt.Fprintf(os.Stderr, "  token: your-gateway-token\n")
		os.Exit(1)
	}

	app := ui.New(cfg)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "clawchat-cli: %v\n", err)
		os.Exit(1)
	}
}
