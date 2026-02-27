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
	events chan gateway.ChatEvent

	// UI components
	viewport viewport.Model
	input    textinput.Model
	spin     spinner.Model

	// Layout
	width  int
	height int
	ready  bool

	// Message counter for idempotency keys
	msgSeq int
}

// New creates the App model.
func New(cfg *config.Config) *App {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleBadgeConnecting

	ti := textinput.New()
	ti.Placeholder = "Type a message…"
	ti.CharLimit = 4096
	ti.Width = 80
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

func (a *App) connectCmd() tea.Cmd {
	events := a.events
	return func() tea.Msg {
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
			return connectErrMsg{fmt.Errorf("no sessions available")}
		}

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
		a.rebuildLayout()

	case tea.KeyMsg:
		switch a.state {
		case stateConnecting:
			if msg.String() == "ctrl+c" {
				return a, tea.Quit
			}
		case stateChat:
			if cmd := a.handleKey(msg); cmd != nil {
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
			a.messages = append(a.messages, a.renderMsg(m.Role, m.Content, m.Timestamp))
		}
		a.state = stateChat
		a.rebuildLayout()
		a.flushViewport()
		cmds = append(cmds, waitForEvent(a.events))

	case connectErrMsg:
		a.err = msg.err
		a.state = stateError

	case chatEventMsg:
		a.handleChatEvent(gateway.ChatEvent(msg))
		cmds = append(cmds, waitForEvent(a.events))

	case nil:
		// no-op

	}

	if a.state == stateChat && a.ready {
		var vpCmd, tiCmd tea.Cmd
		a.viewport, vpCmd = a.viewport.Update(msg)
		a.input, tiCmd = a.input.Update(msg)
		cmds = append(cmds, vpCmd, tiCmd)
	}

	return a, tea.Batch(cmds...)
}

func (a *App) handleKey(msg tea.KeyMsg) tea.Cmd {
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
			return a.handleSlash(text)
		}
		a.appendMsg(a.renderMsg("user", text, time.Now()))
		return a.sendCmd(text)
	}
	return nil
}

func (a *App) handleSlash(cmd string) tea.Cmd {
	switch strings.Fields(cmd)[0] {
	case "/quit", "/exit":
		a.cleanup()
		return tea.Quit
	case "/clear":
		a.messages = nil
		a.flushViewport()
	case "/help":
		a.appendMsg(renderMsg{
			role:     "system",
			rendered: styleSystemMsg.Render("  Commands: /clear  /quit  /help  │  Scroll: ↑↓ PgUp PgDn"),
		})
	default:
		a.appendMsg(renderMsg{
			role:     "system",
			rendered: styleSystemMsg.Render(fmt.Sprintf("  Unknown command: %s", cmd)),
		})
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
		a.flushViewport()
	case "final":
		content := ev.Content
		if content == "" {
			content = a.streamBuf
		}
		a.streamBuf = ""
		a.streamRunID = ""
		if content != "" {
			a.appendMsg(a.renderMsg("assistant", content, time.Now()))
		}
	case "error":
		a.streamBuf = ""
		a.streamRunID = ""
		a.appendMsg(renderMsg{
			role:     "system",
			rendered: styleError.Render("  ⚠ " + ev.ErrorMsg),
		})
		a.flushViewport()
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
	var statusLine string
	if a.cfg.SSHEnabled() {
		statusLine = fmt.Sprintf("%s Establishing SSH tunnel to %s…", a.spin.View(), a.cfg.SSH.Host)
	} else {
		statusLine = fmt.Sprintf("%s Connecting to %s…", a.spin.View(), a.cfg.GatewayURL)
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		styleConnectTitle.Render("🦀 ClawChat CLI"),
		"",
		statusLine,
		"",
		styleHelp.Render("ctrl+c to quit"),
	)

	box := styleConnectBox.Render(content)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, box)
}

func (a *App) viewError() string {
	content := lipgloss.JoinVertical(lipgloss.Left,
		styleError.Render("Connection Error"),
		"",
		fmt.Sprintf("%v", a.err),
		"",
		styleHelp.Render("Press any key to quit."),
	)
	box := styleConnectBox.Width(60).Render(content)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, box)
}

func (a *App) viewChat() string {
	if !a.ready {
		return ""
	}

	// Header bar — full width
	header := a.renderHeaderBar()

	// Chat box — viewport inside rounded border
	chatContent := a.viewport.View()
	chatBox := styleChatBox.
		Width(a.width - 2). // -2 for border chars
		Render(chatContent)

	// Input box
	inputContent := a.input.View()
	inputBox := styleInputBoxFocused.
		Width(a.width - 2).
		Render(inputContent)

	// Help line
	help := styleHelp.Render("  enter: send   ctrl+c: quit   /help for commands   ↑↓: scroll")

	return strings.Join([]string{header, chatBox, inputBox, help}, "\n")
}

func (a *App) renderHeaderBar() string {
	left := styleAppTitle.Render("🦀 ClawChat CLI")

	var right string
	sessionPart := styleSession.Render(a.sessionKey)

	var badges []string
	if a.tun != nil {
		badges = append(badges, styleBadgeSSH.Render(" SSH "))
	}
	if a.client != nil && a.client.Status() == gateway.StatusConnected {
		badges = append(badges, styleBadgeConnected.Render("● connected"))
	} else {
		badges = append(badges, styleBadgeConnecting.Render("○ connecting"))
	}

	right = sessionPart + "  " + strings.Join(badges, "  ")

	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 4 // 4 for padding
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return styleHeaderBar.Width(a.width).Render(line)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (a *App) rebuildLayout() {
	if a.width == 0 || a.height == 0 {
		return
	}

	// Layout: header(1) + chatBox(border=2 + viewport) + inputBox(border=2 + 1) + help(1)
	// chatBox border = 2, inputBox border+content = 3, help = 1, header = 1
	// viewport height = totalHeight - 1 - 2 - 3 - 1 = totalHeight - 7
	vpHeight := a.height - 7
	if vpHeight < 3 {
		vpHeight = 3
	}
	// viewport width = totalWidth - 2 (border) - 2 (padding)
	vpWidth := a.width - 4
	if vpWidth < 20 {
		vpWidth = 20
	}

	if !a.ready {
		a.viewport = viewport.New(vpWidth, vpHeight)
	} else {
		a.viewport.Width = vpWidth
		a.viewport.Height = vpHeight
	}
	a.ready = true

	// Update input width
	a.input.Width = a.width - 6 // border(2) + padding(2) + cursor space(2)

	a.flushViewport()
}

func (a *App) flushViewport() {
	if !a.ready {
		return
	}
	var sb strings.Builder
	for _, m := range a.messages {
		sb.WriteString(m.rendered)
		sb.WriteString("\n")
	}
	if a.streamBuf != "" {
		label := styleAssistantLabel.Render("assistant")
		// Word-wrap the streaming content
		wrapped := wordWrap(a.streamBuf, a.viewport.Width-2)
		sb.WriteString(fmt.Sprintf("\n  %s\n%s▌\n", label, wrapped))
	}
	a.viewport.SetContent(sb.String())
	a.viewport.GotoBottom()
}

func (a *App) renderMsg(role, content string, ts time.Time) renderMsg {
	tsStr := ""
	if !ts.IsZero() {
		tsStr = "  " + styleTimestamp.Render(ts.Format("15:04"))
	}

	var rendered string
	wrapped := wordWrap(content, a.viewport.Width-4)

	switch role {
	case "user":
		label := styleUserLabel.Render("  you") + tsStr
		rendered = fmt.Sprintf("\n%s\n%s", label, indentBlock(wrapped, "  "))
	case "assistant":
		label := styleAssistantLabel.Render("  assistant") + tsStr
		rendered = fmt.Sprintf("\n%s\n%s", label, indentBlock(wrapped, "  "))
	default:
		rendered = styleSystemMsg.Render("  " + content)
	}

	return renderMsg{role: role, content: content, rendered: rendered, timestamp: ts}
}

func (a *App) appendMsg(m renderMsg) {
	a.messages = append(a.messages, m)
	a.flushViewport()
}

func (a *App) cleanup() {
	if a.client != nil {
		a.client.Close()
	}
	if a.tun != nil {
		a.tun.Stop()
	}
}

// wordWrap wraps text at word boundaries for a given width.
func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}
	var lines []string
	var line string
	for _, w := range words {
		if line == "" {
			line = w
		} else if len(line)+1+len(w) <= width {
			line += " " + w
		} else {
			lines = append(lines, line)
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// indentBlock adds a prefix to every line.
func indentBlock(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
