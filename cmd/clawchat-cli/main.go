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
		if cfg.IsOllama() {
			fmt.Fprintf(os.Stderr, "Example (Ollama):\n")
			fmt.Fprintf(os.Stderr, "  backend: ollama\n")
			fmt.Fprintf(os.Stderr, "  ollama:\n")
			fmt.Fprintf(os.Stderr, "    url: http://llama.home.wrox.us:11434\n")
			fmt.Fprintf(os.Stderr, "    model: qwen2.5:14b\n")
		} else {
			fmt.Fprintf(os.Stderr, "Example (OpenClaw):\n")
			fmt.Fprintf(os.Stderr, "  gateway_url: ws://pinchy.home.wrox.us:18789\n")
			fmt.Fprintf(os.Stderr, "  token: your-gateway-token\n")
		}
		os.Exit(1)
	}

	var model tea.Model
	if cfg.IsOllama() {
		model = ui.NewOllama(cfg)
	} else {
		model = ui.New(cfg)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "clawchat-cli: %v\n", err)
		os.Exit(1)
	}
}
