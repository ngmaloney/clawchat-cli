// Package ui implements the ClawChat CLI terminal user interface.
package ui

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
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
type sendDoneMsg struct{ runID string }
type historyReloadMsg []gateway.Message

// ── Rendered message ──────────────────────────────────────────────────────────

type renderMsg struct {
	role      string
	content   string
	rendered  string
	timestamp time.Time
}

// ── App ───────────────────────────────────────────────────────────────────────

type App struct {
	cfg   *config.Config
	state appState
	err   error

	client *gateway.Client
	tun    *tunnel.Tunnel

	sessionKey string
	session    gateway.Session

	messages    []renderMsg
	streamRunID string
	streamBuf   string
	localRunID  string // run ID of the most recent locally-initiated send
	isWaiting   bool   // true between send and first assistant token — shows "thinking" indicator

	events chan gateway.ChatEvent

	viewport viewport.Model
	input    textarea.Model
	spin     spinner.Model

	width  int
	height int
	ready  bool
	msgSeq int
}

func New(cfg *config.Config) *App {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleBadgeConnecting

	ti := textarea.New()
	ti.Placeholder = "Type a message…"
	ti.CharLimit = 4096
	ti.ShowLineNumbers = false
	ti.SetHeight(3)
	ti.Prompt = ""
	// Remove textarea's own border and cursor-line highlight — we style the box ourselves
	noBorder := lipgloss.NewStyle()
	ti.FocusedStyle.Base = noBorder
	ti.BlurredStyle.Base = noBorder
	ti.FocusedStyle.CursorLine = noBorder
	ti.BlurredStyle.CursorLine = noBorder
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
	return tea.Batch(a.spin.Tick, a.connectCmd())
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
	return func() tea.Msg { return chatEventMsg(<-ch) }
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
			a.messages = append(a.messages, a.renderMessage(m.Role, m.Content, m.Timestamp))
		}
		a.state = stateChat
		a.rebuildLayout()
		a.flushViewport()
		cmds = append(cmds, waitForEvent(a.events))

	case connectErrMsg:
		a.err = msg.err
		a.state = stateError

	case chatEventMsg:
		if cmd := a.handleChatEvent(gateway.ChatEvent(msg)); cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, waitForEvent(a.events))

	case sendDoneMsg:
		a.localRunID = msg.runID

	case historyReloadMsg:
		a.messages = make([]renderMsg, 0, len(msg))
		for _, m := range msg {
			a.messages = append(a.messages, a.renderMessage(m.Role, m.Content, m.Timestamp))
		}
		a.flushViewport()

	case nil:
		// no-op

	}

	if a.state == stateChat && a.ready {
		var vpCmd, tiCmd tea.Cmd
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			// Route scroll keys to viewport only — prevents typed chars from scrolling
			switch keyMsg.String() {
			case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d", "home", "end":
				a.viewport, vpCmd = a.viewport.Update(msg)
			default:
				a.input, tiCmd = a.input.Update(msg)
			}
		} else {
			a.viewport, vpCmd = a.viewport.Update(msg)
			a.input, tiCmd = a.input.Update(msg)
		}
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
		a.input.Reset()
		if strings.HasPrefix(text, "/") {
			return a.handleSlash(text)
		}
		a.isWaiting = true
		a.appendMsg(a.renderMessage("user", text, time.Now()))
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
		return nil
	case "/help":
		a.appendMsg(renderMsg{
			rendered: styleSystemMsg.Render(
				"Client: /clear  /quit\n" +
					"Gateway: /model  /models  /status  /stop  /thinking  /verbose  /compact  /reset  /new\n" +
					"Scroll: ↑↓ PgUp PgDn",
			),
		})
		return nil
	default:
		// Forward to gateway — it handles /model, /stop, /thinking, /status, etc.
		a.isWaiting = true
		a.appendMsg(a.renderMessage("user", cmd, time.Now()))
		return a.sendCmd(cmd)
	}
}

func (a *App) handleChatEvent(ev gateway.ChatEvent) tea.Cmd {
	if ev.SessionKey != "" && ev.SessionKey != a.sessionKey {
		return nil
	}
	switch ev.State {
	case "delta":
		a.isWaiting = false
		a.streamRunID = ev.RunID
		a.streamBuf = ev.Content
		a.flushViewport()
	case "final":
		a.isWaiting = false
		content := ev.Content
		if content == "" {
			content = a.streamBuf
		}
		a.streamBuf = ""
		a.streamRunID = ""
		if content != "" {
			a.appendMsg(a.renderMessage("assistant", content, time.Now()))
		}
		// If this run was triggered by another client, reload history to show their message
		if ev.RunID != "" && ev.RunID != a.localRunID {
			a.localRunID = "" // clear so next external run also triggers reload
			return a.reloadHistoryCmd()
		}
		a.localRunID = ""
	case "error":
		a.isWaiting = false
		a.streamBuf = ""
		a.streamRunID = ""
		a.appendMsg(renderMsg{
			rendered: styleError.Render("⚠ " + ev.ErrorMsg),
		})
	}
	return nil
}

func (a *App) sendCmd(text string) tea.Cmd {
	a.msgSeq++
	key := fmt.Sprintf("cli-%d-%d", time.Now().UnixMilli(), a.msgSeq)
	sessionKey := a.sessionKey
	client := a.client
	return func() tea.Msg {
		runID, err := client.SendMessage(sessionKey, text, key)
		if err != nil {
			return chatEventMsg(gateway.ChatEvent{State: "error", ErrorMsg: err.Error()})
		}
		return sendDoneMsg{runID: runID}
	}
}

func (a *App) reloadHistoryCmd() tea.Cmd {
	sessionKey := a.sessionKey
	client := a.client
	return func() tea.Msg {
		history, err := client.GetHistory(sessionKey, 50)
		if err != nil {
			return nil
		}
		return historyReloadMsg(history)
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
		styleAppTitle.Render("🦀 ClawChat CLI"),
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

	header := a.renderHeader()
	chatBox := styleChatBox.Width(a.width - 2).Render(a.viewport.View())
	inputBox := styleInputBoxFocused.Width(a.width - 2).Render(a.input.View())
	help := styleHelp.Padding(0, 1).Render("enter: send   ctrl+c: quit   /help   ↑↓: scroll")

	return lipgloss.JoinVertical(lipgloss.Left, header, chatBox, inputBox, help)
}

func (a *App) renderHeader() string {
	left := styleAppTitle.Render("🦀 ClawChat CLI")

	var badges []string
	if a.tun != nil {
		badges = append(badges, styleBadgeSSH.Render(" SSH "))
	}
	if a.client != nil && a.client.Status() == gateway.StatusConnected {
		badges = append(badges, styleBadgeConnected.Render("● connected"))
	} else {
		badges = append(badges, styleBadgeConnecting.Render("○ connecting"))
	}

	host := gatewayHost(a.cfg.GatewayURL)

	right := lipgloss.JoinHorizontal(lipgloss.Center,
		styleSession.Render(host),
		"  ",
		styleSession.Render(a.sessionKey),
		"  ",
		strings.Join(badges, "  "),
	)

	// Fill the gap between left and right
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right

	return styleHeaderBar.Width(a.width).Render(line)
}

// gatewayHost extracts the host (host:port) from a WebSocket URL.
func gatewayHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}

// ── Layout helpers ────────────────────────────────────────────────────────────

func (a *App) rebuildLayout() {
	if a.width == 0 || a.height == 0 {
		return
	}
	// header(1) + chatBox(border 2 + viewport) + inputBox(border 2 + content 3) + help(1)
	vpHeight := a.height - 9
	if vpHeight < 3 {
		vpHeight = 3
	}
	// width: border(1 each side) + padding(1 each side) = 4
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
	// Input width: border(2) + padding(2) = 4 total overhead
	a.input.SetWidth(a.width - 6)

	a.flushViewport()
}

func (a *App) flushViewport() {
	if !a.ready {
		return
	}

	var blocks []string
	for _, m := range a.messages {
		blocks = append(blocks, m.rendered)
	}

	if a.isWaiting && a.streamBuf == "" {
		label := styleAssistantLabel.Render("assistant")
		thinking := lipgloss.JoinVertical(lipgloss.Left,
			"",
			label,
			styleHelp.Render("thinking…"),
		)
		blocks = append(blocks, thinking)
	} else if a.streamBuf != "" {
		label := styleAssistantLabel.Render("assistant")
		// Use lipgloss width-constrained style for wrapping
		content := lipgloss.NewStyle().Width(a.viewport.Width - 2).Render(a.streamBuf)
		streaming := lipgloss.JoinVertical(lipgloss.Left,
			"",
			label,
			content+"▌",
		)
		blocks = append(blocks, streaming)
	}

	a.viewport.SetContent(strings.Join(blocks, "\n"))
	a.viewport.GotoBottom()
}

func (a *App) renderMessage(role, content string, ts time.Time) renderMsg {
	tsStr := ""
	if !ts.IsZero() {
		tsStr = "  " + styleTimestamp.Render(ts.Format("15:04"))
	}

	// Use lipgloss Width to handle word-wrap automatically
	msgWidth := a.viewport.Width - 2
	if msgWidth < 10 {
		msgWidth = 10
	}
	wrapped := lipgloss.NewStyle().Width(msgWidth).Render(content)

	var label, rendered string
	switch role {
	case "user":
		label = styleUserLabel.Render("you") + tsStr
		rendered = lipgloss.JoinVertical(lipgloss.Left, "", label, styleMessageBody.Render(wrapped))
	case "assistant":
		label = styleAssistantLabel.Render("assistant") + tsStr
		body := styleMessageBody.Render(wrapped)
		rendered = lipgloss.JoinVertical(lipgloss.Left, "", label, body)
	default:
		return renderMsg{
			role:      role,
			content:   content,
			rendered:  styleSystemMsg.Render(content),
			timestamp: ts,
		}
	}

	return renderMsg{
		role:      role,
		content:   content,
		rendered:  rendered,
		timestamp: ts,
	}
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
