// Package ui implements the ClawChat CLI terminal user interface.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/ngmaloney/clawchat-cli/internal/config"
	"github.com/ngmaloney/clawchat-cli/internal/gateway"
	"github.com/ngmaloney/clawchat-cli/internal/tunnel"
)

// ── State ─────────────────────────────────────────────────────────────────────

type appState int

const (
	stateConnecting appState = iota
	stateChat
	stateError
)

// ── Tea messages ──────────────────────────────────────────────────────────────

type connectDoneMsg struct {
	sessionKey string
	session    gateway.Session
	history    []gateway.Message
	client     *gateway.Client
	tun        *tunnel.Tunnel
}

type connectErrMsg struct{ err error }

type chatEventMsg gateway.ChatEvent

// ── Rendered message ──────────────────────────────────────────────────────────

type renderMsg struct {
	role      string
	content   string
	rendered  string
	timestamp time.Time
}

// ── App ───────────────────────────────────────────────────────────────────────

// App is the top-level Bubble Tea model.
type App struct {
	cfg   *config.Config
	state appState
	err   error

	// Connection
	client *gateway.Client
	tun    *tunnel.Tunnel

	// Session
	sessionKey string
	session    gateway.Session

	// Messages
	messages    []renderMsg
	streamRunID string
	streamBuf   string

	// Events channel (gateway → bubbletea)
	// Created in New(); the gateway.Client's OnEvent writes to this.
	events chan gateway.ChatEvent

	// UI components
	viewport viewport.Model
	input    textinput.Model
	spin     spinner.Model

	// Layout
	width  int
	height int
	ready  bool

	// Markdown renderer
	renderer *glamour.TermRenderer

	// Message counter for idempotency keys
	msgSeq int
}

// New creates the App model.
func New(cfg *config.Config) *App {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleStatusWait

	ti := textinput.New()
	ti.Placeholder = "Type a message… (/help for commands)"
	ti.CharLimit = 4096
	ti.Focus()

	return &App{
		cfg:    cfg,
		state:  stateConnecting,
		spin:   sp,
		input:  ti,
		events: make(chan gateway.ChatEvent, 64),
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.spin.Tick,
		a.connectCmd(),
	)
}

// connectCmd performs SSH + gateway connect + session lookup + history load
// in a background goroutine, then returns a connectDoneMsg or connectErrMsg.
func (a *App) connectCmd() tea.Cmd {
	// Capture the events channel so the gateway can write to it.
	events := a.events

	return func() tea.Msg {
		// 1. SSH tunnel
		var tun *tunnel.Tunnel
		gatewayURL := a.cfg.GatewayURL

		if a.cfg.SSHEnabled() {
			t, err := tunnel.Start(a.cfg.SSH)
			if err != nil {
				return connectErrMsg{fmt.Errorf("SSH tunnel: %w", err)}
			}
			tun = t
			gatewayURL = t.GatewayURL()
		}

		// 2. Gateway connection
		client := gateway.New(gateway.Options{
			URL:   gatewayURL,
			Token: a.cfg.Token,
			OnEvent: func(event string, payload map[string]any) {
				if event == "chat" {
					ev := gateway.ParseChatEvent(payload)
					select {
					case events <- ev:
					default:
					}
				}
			},
		})

		if err := client.Connect(); err != nil {
			if tun != nil {
				tun.Stop()
			}
			return connectErrMsg{fmt.Errorf("gateway: %w", err)}
		}

		// 3. Find session
		sessions, err := client.ListSessions()
		if err != nil {
			client.Close()
			if tun != nil {
				tun.Stop()
			}
			return connectErrMsg{fmt.Errorf("listing sessions: %w", err)}
		}

		var session gateway.Session
		if a.cfg.SessionKey != "" {
			for _, s := range sessions {
				if s.Key == a.cfg.SessionKey {
					session = s
					break
				}
			}
			if session.Key == "" {
				client.Close()
				if tun != nil {
					tun.Stop()
				}
				return connectErrMsg{fmt.Errorf("session %q not found", a.cfg.SessionKey)}
			}
		} else if len(sessions) > 0 {
			session = sessions[0]
		} else {
			client.Close()
			if tun != nil {
				tun.Stop()
			}
			return connectErrMsg{fmt.Errorf("no sessions available on this gateway")}
		}

		// 4. Load history (non-fatal on failure)
		history, _ := client.GetHistory(session.Key, 50)

		return connectDoneMsg{
			sessionKey: session.Key,
			session:    session,
			history:    history,
			client:     client,
			tun:        tun,
		}
	}
}

// waitForEvent bridges the gateway events channel into the Bubble Tea loop.
func waitForEvent(ch <-chan gateway.ChatEvent) tea.Cmd {
	return func() tea.Msg {
		return chatEventMsg(<-ch)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.initViewport()
		a.refreshViewport()

	case tea.KeyMsg:
		switch a.state {
		case stateConnecting:
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				return a, tea.Quit
			}
		case stateChat:
			cmd := a.handleChatKey(msg)
			if cmd != nil {
				return a, cmd
			}
		case stateError:
			return a, tea.Quit
		}

	case spinner.TickMsg:
		if a.state == stateConnecting {
			var cmd tea.Cmd
			a.spin, cmd = a.spin.Update(msg)
			cmds = append(cmds, cmd)
		}

	case connectDoneMsg:
		a.client = msg.client
		a.tun = msg.tun
		a.sessionKey = msg.sessionKey
		a.session = msg.session
		a.messages = make([]renderMsg, 0, len(msg.history))
		for _, m := range msg.history {
			a.messages = append(a.messages, a.renderMessage(m.Role, m.Content, m.Timestamp))
		}
		a.state = stateChat
		a.initViewport()
		a.refreshViewport()
		cmds = append(cmds, waitForEvent(a.events))

	case connectErrMsg:
		a.err = msg.err
		a.state = stateError

	case chatEventMsg:
		a.handleChatEvent(gateway.ChatEvent(msg))
		cmds = append(cmds, waitForEvent(a.events))

	case nil:
		// no-op (e.g. send succeeded)

	}

	// Forward input/viewport events in chat state
	if a.state == stateChat && a.ready {
		var vpCmd, tiCmd tea.Cmd
		a.viewport, vpCmd = a.viewport.Update(msg)
		a.input, tiCmd = a.input.Update(msg)
		cmds = append(cmds, vpCmd, tiCmd)
	}

	return a, tea.Batch(cmds...)
}

func (a *App) handleChatKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		a.cleanup()
		return tea.Quit
	case "enter":
		text := strings.TrimSpace(a.input.Value())
		if text == "" {
			return nil
		}
		a.input.SetValue("")
		if strings.HasPrefix(text, "/") {
			return a.handleSlashCommand(text)
		}
		a.addUserMessage(text)
		return a.sendCmd(text)
	}
	return nil
}

func (a *App) handleSlashCommand(cmd string) tea.Cmd {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "/quit", "/exit":
		a.cleanup()
		return tea.Quit
	case "/clear":
		a.messages = nil
		a.refreshViewport()
	case "/help":
		a.addSystemMessage("Commands: /clear  /quit  /help  |  Scroll: ↑↓ / PgUp PgDn")
		a.refreshViewport()
	default:
		a.addSystemMessage(fmt.Sprintf("Unknown command: %s", parts[0]))
		a.refreshViewport()
	}
	return nil
}

func (a *App) handleChatEvent(ev gateway.ChatEvent) {
	if ev.SessionKey != "" && ev.SessionKey != a.sessionKey {
		return
	}
	switch ev.State {
	case "delta":
		a.streamRunID = ev.RunID
		a.streamBuf = ev.Content
		a.refreshViewport()
	case "final":
		content := ev.Content
		if content == "" {
			content = a.streamBuf
		}
		a.streamBuf = ""
		a.streamRunID = ""
		if content != "" {
			a.messages = append(a.messages, a.renderMessage("assistant", content, time.Now()))
		}
		a.refreshViewport()
	case "error":
		a.streamBuf = ""
		a.streamRunID = ""
		a.addSystemMessage("⚠ " + ev.ErrorMsg)
		a.refreshViewport()
	}
}

func (a *App) sendCmd(text string) tea.Cmd {
	a.msgSeq++
	key := fmt.Sprintf("cli-%d-%d", time.Now().UnixMilli(), a.msgSeq)
	sessionKey := a.sessionKey
	client := a.client

	return func() tea.Msg {
		if err := client.SendMessage(sessionKey, text, key); err != nil {
			return chatEventMsg(gateway.ChatEvent{
				State:    "error",
				ErrorMsg: err.Error(),
			})
		}
		return nil
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

func (a *App) View() string {
	if a.width == 0 {
		return ""
	}
	switch a.state {
	case stateConnecting:
		return a.viewConnecting()
	case stateChat:
		return a.viewChat()
	case stateError:
		return a.viewError()
	}
	return ""
}

func (a *App) viewConnecting() string {
	var line string
	if a.cfg.SSHEnabled() {
		line = fmt.Sprintf("%s Establishing SSH tunnel to %s…", a.spin.View(), a.cfg.SSH.Host)
	} else {
		line = fmt.Sprintf("%s Connecting to %s…", a.spin.View(), a.cfg.GatewayURL)
	}
	return "\n\n  " + line + "\n\n  " + styleHelp.Render("ctrl+c to quit")
}

func (a *App) viewError() string {
	return styleError.Render(fmt.Sprintf("Error: %v\n\nPress any key to quit.", a.err))
}

func (a *App) viewChat() string {
	if !a.ready {
		return ""
	}
	divider := styleDivider.Render(strings.Repeat("─", a.width))
	return strings.Join([]string{
		a.renderHeader(),
		a.viewport.View(),
		divider,
		a.input.View(),
		a.renderFooter(),
	}, "\n")
}

func (a *App) renderHeader() string {
	left := styleTitle.Render("🦀 ClawChat CLI")

	var badges []string
	if a.tun != nil {
		badges = append(badges, styleBadgeSSH.Render("SSH"))
	}
	if a.client != nil && a.client.Status() == gateway.StatusConnected {
		badges = append(badges, styleBadgeConnected.Render("●"))
	} else {
		badges = append(badges, styleBadgeConnecting.Render("○"))
	}
	right := styleHelp.Render(a.sessionKey) + " " + strings.Join(badges, " ")

	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return styleHeader.Width(a.width).Render(left + strings.Repeat(" ", gap) + right)
}

func (a *App) renderFooter() string {
	model := styleHelp.Render(a.session.Model)
	help := styleHelp.Render("enter: send  ctrl+c: quit  /help")
	gap := a.width - lipgloss.Width(model) - lipgloss.Width(help)
	if gap < 2 {
		gap = 2
	}
	return styleFooter.Width(a.width).Render(model + strings.Repeat(" ", gap) + help)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (a *App) initViewport() {
	if a.width == 0 || a.height == 0 {
		return
	}
	// Reserve: header(1) + divider(1) + input(1) + footer(1) = 4 lines
	vpHeight := a.height - 4
	if vpHeight < 3 {
		vpHeight = 3
	}
	if !a.ready {
		a.viewport = viewport.New(a.width, vpHeight)
	} else {
		a.viewport.Width = a.width
		a.viewport.Height = vpHeight
	}
	a.ready = true

	if r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(a.width-4),
	); err == nil {
		a.renderer = r
	}
}

func (a *App) refreshViewport() {
	if !a.ready {
		return
	}
	var sb strings.Builder
	for _, m := range a.messages {
		sb.WriteString(m.rendered)
	}
	if a.streamBuf != "" {
		label := styleAssistantLabel.Render("assistant")
		sb.WriteString(fmt.Sprintf("\n%s\n%s▌\n", label, a.streamBuf))
	}
	a.viewport.SetContent(sb.String())
	a.viewport.GotoBottom()
}

func (a *App) renderMessage(role, content string, ts time.Time) renderMsg {
	tsStr := ""
	if !ts.IsZero() {
		tsStr = " " + styleTimestamp.Render(ts.Format("15:04"))
	}
	var rendered string
	switch role {
	case "user":
		label := styleUserLabel.Render("you") + tsStr
		rendered = fmt.Sprintf("\n%s\n%s\n", label, content)
	case "assistant":
		label := styleAssistantLabel.Render("assistant") + tsStr
		md := content
		if a.renderer != nil {
			if r, err := a.renderer.Render(content); err == nil {
				md = r
			}
		}
		rendered = fmt.Sprintf("\n%s\n%s", label, md)
	default:
		rendered = styleHelp.Render(fmt.Sprintf("\n[%s]\n", content))
	}
	return renderMsg{role: role, content: content, rendered: rendered, timestamp: ts}
}

func (a *App) addUserMessage(text string) {
	a.messages = append(a.messages, a.renderMessage("user", text, time.Now()))
	a.refreshViewport()
}

func (a *App) addSystemMessage(text string) {
	a.messages = append(a.messages, renderMsg{
		role:     "system",
		content:  text,
		rendered: styleHelp.Render(fmt.Sprintf("\n[%s]\n", text)),
	})
}

func (a *App) cleanup() {
	if a.client != nil {
		a.client.Close()
	}
	if a.tun != nil {
		a.tun.Stop()
	}
}
