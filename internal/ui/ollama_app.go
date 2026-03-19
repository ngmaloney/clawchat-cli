package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ngmaloney/clawchat-cli/internal/config"
	"github.com/ngmaloney/clawchat-cli/internal/ollama"
)

// ── Tea messages (Ollama-specific) ────────────────────────────────────────────

type ollamaConnectedMsg struct {
	models []ollama.ModelInfo
}
type ollamaConnectErrMsg struct{ err error }
type ollamaStreamDeltaMsg struct{ delta ollama.StreamDelta }
type ollamaStreamDoneMsg struct{ content string }
type ollamaStreamErrMsg struct{ err error }
type ollamaModelsMsg []ollama.ModelInfo

// ── OllamaApp ─────────────────────────────────────────────────────────────────

type OllamaApp struct {
	cfg    *config.Config
	client *ollama.Client
	state  appState
	err    error

	history  []ollama.ChatMessage // full conversation history sent to Ollama
	messages []renderMsg          // rendered UI messages

	streamBuf string
	isWaiting bool
	streamCh  chan ollama.StreamDelta // channel for streaming deltas

	// model picker
	models    []ollama.ModelInfo
	pickerIdx int

	viewport viewport.Model
	input    textarea.Model
	spin     spinner.Model

	width  int
	height int
	ready  bool
}

func NewOllama(cfg *config.Config) *OllamaApp {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleBadgeConnecting

	ti := textarea.New()
	ti.Placeholder = "Type a message…"
	ti.CharLimit = 4096
	ti.ShowLineNumbers = false
	ti.SetHeight(3)
	ti.Prompt = ""
	noBorder := lipgloss.NewStyle()
	ti.FocusedStyle.Base = noBorder
	ti.BlurredStyle.Base = noBorder
	ti.FocusedStyle.CursorLine = noBorder
	ti.BlurredStyle.CursorLine = noBorder
	ti.Focus()

	return &OllamaApp{
		cfg:      cfg,
		state:    stateConnecting,
		spin:     sp,
		input:    ti,
		streamCh: make(chan ollama.StreamDelta, 64),
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (a *OllamaApp) Init() tea.Cmd {
	return tea.Batch(a.spin.Tick, a.connectCmd())
}

func (a *OllamaApp) connectCmd() tea.Cmd {
	cfg := a.cfg
	return func() tea.Msg {
		client := ollama.New(cfg.Ollama.URL, cfg.Ollama.Model)
		if err := client.Ping(); err != nil {
			return ollamaConnectErrMsg{err}
		}
		models, _ := client.ListModels()
		return ollamaConnectedMsg{models: models}
	}
}

// waitForStreamDelta listens for the next delta from the stream channel.
func waitForStreamDelta(ch <-chan ollama.StreamDelta) tea.Cmd {
	return func() tea.Msg {
		delta, ok := <-ch
		if !ok {
			return nil
		}
		if delta.Done {
			return ollamaStreamDoneMsg{content: delta.Content}
		}
		return ollamaStreamDeltaMsg{delta: delta}
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (a *OllamaApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case stateSessionPicker: // reused as model picker
			if cmd := a.handlePickerKey(msg); cmd != nil {
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

	case ollamaConnectedMsg:
		a.client = ollama.New(a.cfg.Ollama.URL, a.cfg.Ollama.Model)
		a.models = msg.models
		a.state = stateChat
		a.rebuildLayout()

	case ollamaConnectErrMsg:
		a.err = msg.err
		a.state = stateError

	case ollamaStreamDeltaMsg:
		a.isWaiting = false
		a.streamBuf = msg.delta.Content
		a.flushViewport()
		cmds = append(cmds, waitForStreamDelta(a.streamCh))

	case ollamaStreamDoneMsg:
		a.isWaiting = false
		a.streamBuf = ""
		if msg.content != "" {
			a.history = append(a.history, ollama.ChatMessage{Role: "assistant", Content: msg.content})
			a.appendMsg(a.renderMessage("assistant", msg.content, time.Now()))
		}

	case ollamaStreamErrMsg:
		a.isWaiting = false
		a.streamBuf = ""
		a.appendMsg(renderMsg{
			rendered: styleError.Render("⚠ " + msg.err.Error()),
		})

	case ollamaModelsMsg:
		a.models = []ollama.ModelInfo(msg)
		a.pickerIdx = 0
		for i, m := range a.models {
			if m.Name == a.client.Model {
				a.pickerIdx = i
				break
			}
		}
		a.state = stateSessionPicker
	}

	if a.state == stateChat && a.ready {
		var vpCmd, tiCmd tea.Cmd
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
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

func (a *OllamaApp) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "ctrl+s":
		return a.openModelPickerCmd()
	case "enter":
		text := strings.TrimSpace(a.input.Value())
		if text == "" {
			return nil
		}
		if a.isWaiting || a.streamBuf != "" {
			return nil // don't allow sending while streaming
		}
		a.input.Reset()
		if strings.HasPrefix(text, "/") {
			return a.handleSlash(text)
		}
		a.isWaiting = true
		a.history = append(a.history, ollama.ChatMessage{Role: "user", Content: text})
		a.appendMsg(a.renderMessage("user", text, time.Now()))
		return a.sendCmd()
	}
	return nil
}

func (a *OllamaApp) handleSlash(cmd string) tea.Cmd {
	switch strings.Fields(cmd)[0] {
	case "/quit", "/exit", "/q":
		return tea.Quit
	case "/clear":
		a.messages = nil
		a.history = nil
		a.flushViewport()
		return nil
	case "/model", "/models":
		return a.openModelPickerCmd()
	case "/help":
		a.appendMsg(renderMsg{
			rendered: styleSystemMsg.Render(
				"/clear — reset conversation\n" +
					"/model — switch model\n" +
					"/quit  — exit\n" +
					"ctrl+s — model picker\n" +
					"↑↓ PgUp PgDn — scroll",
			),
		})
		return nil
	default:
		a.appendMsg(renderMsg{
			rendered: styleSystemMsg.Render("Unknown command: " + cmd + "  (try /help)"),
		})
		return nil
	}
}

func (a *OllamaApp) sendCmd() tea.Cmd {
	client := a.client
	history := make([]ollama.ChatMessage, len(a.history))
	copy(history, a.history)
	ch := a.streamCh

	// Start the stream in a goroutine, push deltas to the channel
	go func() {
		_, err := client.ChatStream(history, func(delta ollama.StreamDelta) {
			ch <- delta
		})
		if err != nil {
			// Send an error as a "done" with empty content; the error message
			// will be picked up separately
			ch <- ollama.StreamDelta{Content: "", Done: true}
		}
	}()

	// Return a cmd that starts listening for the first delta
	return waitForStreamDelta(ch)
}

func (a *OllamaApp) openModelPickerCmd() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		models, err := client.ListModels()
		if err != nil {
			return nil
		}
		return ollamaModelsMsg(models)
	}
}

func (a *OllamaApp) handlePickerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		a.state = stateChat
	case "up", "k":
		if a.pickerIdx > 0 {
			a.pickerIdx--
		}
	case "down", "j":
		if a.pickerIdx < len(a.models)-1 {
			a.pickerIdx++
		}
	case "enter":
		if len(a.models) > 0 {
			selected := a.models[a.pickerIdx]
			a.client.Model = selected.Name
			a.appendMsg(renderMsg{
				rendered: styleSystemMsg.Render(fmt.Sprintf("Switched to model: %s", selected.Name)),
			})
			a.state = stateChat
		}
	}
	return nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (a *OllamaApp) View() string {
	if a.width == 0 {
		return ""
	}
	switch a.state {
	case stateConnecting:
		return a.viewConnecting()
	case stateChat:
		return a.viewChat()
	case stateSessionPicker:
		return a.viewModelPicker()
	case stateError:
		return a.viewError()
	}
	return ""
}

func (a *OllamaApp) viewConnecting() string {
	statusLine := fmt.Sprintf("%s Connecting to Ollama at %s…", a.spin.View(), a.cfg.Ollama.URL)
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

func (a *OllamaApp) viewError() string {
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

func (a *OllamaApp) viewChat() string {
	if !a.ready {
		return ""
	}

	header := a.renderHeader()
	chatBox := styleChatBox.Width(a.width - 2).Render(a.viewport.View())
	inputBox := styleInputBoxFocused.Width(a.width - 2).Render(a.input.View())
	help := styleHelp.Padding(0, 1).Render("enter: send   ctrl+s: models   ctrl+c: quit   /help   ↑↓: scroll")

	return lipgloss.JoinVertical(lipgloss.Left, header, chatBox, inputBox, help)
}

func (a *OllamaApp) renderHeader() string {
	left := styleAppTitle.Render("🦀 ClawChat CLI")

	var badges []string
	badges = append(badges, styleBadgeSSH.Render(" ollama "))
	badges = append(badges, styleBadgeConnected.Render("● connected"))

	host := a.cfg.Ollama.URL
	model := a.client.Model

	right := lipgloss.JoinHorizontal(lipgloss.Center,
		styleSession.Render(host),
		"  ",
		styleSession.Render(model),
		"  ",
		strings.Join(badges, "  "),
	)

	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right

	return styleHeaderBar.Width(a.width).Render(line)
}

func (a *OllamaApp) viewModelPicker() string {
	if a.width == 0 {
		return ""
	}

	title := styleBadgeConnected.Render("  Models  ")
	var rows []string
	for i, m := range a.models {
		label := m.Name
		sizeMB := m.Size / (1024 * 1024)
		sizeStr := styleTimestamp.Render(fmt.Sprintf("  %dMB", sizeMB))

		if i == a.pickerIdx {
			line := styleBadgeConnected.Render("▶ ") + styleMessageBody.Render(label) + sizeStr
			rows = append(rows, line)
		} else {
			line := styleHelp.Render("  ") + styleHelp.Render(label) + sizeStr
			rows = append(rows, line)
		}
	}
	if len(rows) == 0 {
		rows = append(rows, styleHelp.Render("  No models available"))
	}

	hint := styleTimestamp.Render("↑↓: navigate   enter: select   esc: cancel")
	body := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(rows, "\n"),
		"",
		hint,
	)

	box := styleConnectBox.Width(min(60, a.width-8)).Render(body)
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, box)
}

// ── Layout helpers ────────────────────────────────────────────────────────────

func (a *OllamaApp) rebuildLayout() {
	if a.width == 0 || a.height == 0 {
		return
	}
	vpHeight := a.height - 9
	if vpHeight < 3 {
		vpHeight = 3
	}
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
	a.input.SetWidth(a.width - 6)
	a.flushViewport()
}

func (a *OllamaApp) flushViewport() {
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

func (a *OllamaApp) renderMessage(role, content string, ts time.Time) renderMsg {
	tsStr := ""
	if !ts.IsZero() {
		tsStr = "  " + styleTimestamp.Render(ts.Format("15:04"))
	}

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

func (a *OllamaApp) appendMsg(m renderMsg) {
	a.messages = append(a.messages, m)
	a.flushViewport()
}
