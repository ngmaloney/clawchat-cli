package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ZeroClawClient is a WebSocket client for the ZeroClaw backend.
// Protocol: no handshake, no sessions — connection is ready immediately.
// Auth is passed as an Authorization header on the WebSocket upgrade.
type ZeroClawClient struct {
	url       string
	token     string
	sessionID string
	onEvent   EventHandler

	mu        sync.Mutex
	conn      *websocket.Conn
	status    Status
	streamBuf string // accumulated streaming tokens

	histOnce sync.Once
	histCh   chan []Message // receives history push on connect

	done chan struct{}
	once sync.Once
}

// ZeroClawOptions configures a ZeroClawClient.
type ZeroClawOptions struct {
	URL       string
	Token     string
	SessionID string // persistent session ID for history continuity
	OnEvent   EventHandler
}

// NewZeroClaw creates a new ZeroClawClient. Call Connect() to establish the connection.
func NewZeroClaw(opts ZeroClawOptions) *ZeroClawClient {
	return &ZeroClawClient{
		url:       opts.URL,
		token:     opts.Token,
		sessionID: opts.SessionID,
		onEvent:   opts.OnEvent,
		status:    StatusDisconnected,
		histCh:    make(chan []Message, 1),
		done:      make(chan struct{}),
	}
}

// Connect dials the ZeroClaw WebSocket endpoint and starts the read loop.
// Auth: the token is sent both as a ?token= query parameter and as an
// Authorization: Bearer header to support different ZeroClaw server builds.
func (z *ZeroClawClient) Connect() error {
	z.setStatus(StatusConnecting)

	// Build the URL with token and session_id as query parameters.
	u, err := url.Parse(z.url)
	if err != nil {
		z.setStatus(StatusError)
		return fmt.Errorf("zeroclaw: invalid URL %q: %w", z.url, err)
	}
	q := u.Query()
	q.Set("token", z.token)
	if z.sessionID != "" {
		q.Set("session_id", z.sessionID)
	}
	u.RawQuery = q.Encode()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+z.token)

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		z.setStatus(StatusError)
		if resp != nil {
			switch resp.StatusCode {
			case 401, 403:
				return fmt.Errorf("authentication failed (HTTP %d) — token missing or invalid. Pair first: POST /pair with header X-Pairing-Code, then set token in config", resp.StatusCode)
			case 404:
				return fmt.Errorf("gateway endpoint not found (HTTP 404) — check gateway_url (ZeroClaw: ws://<host>:42617/ws/chat, OpenClaw: ws://<host>:18789)")
			default:
				return fmt.Errorf("gateway returned HTTP %d — %w", resp.StatusCode, err)
			}
		}
		return fmt.Errorf("could not reach gateway at %s — is it running? (%w)", z.url, err)
	}

	z.mu.Lock()
	z.conn = conn
	z.mu.Unlock()

	z.setStatus(StatusConnected)

	go z.readLoop()

	return nil
}

// Close shuts down the connection.
func (z *ZeroClawClient) Close() {
	z.once.Do(func() {
		close(z.done)
		z.mu.Lock()
		if z.conn != nil {
			_ = z.conn.Close()
		}
		z.mu.Unlock()
		z.setStatus(StatusDisconnected)
	})
}

// Status returns the current connection status.
func (z *ZeroClawClient) Status() Status {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.status
}

// ListSessions returns a single synthetic session representing the ZeroClaw connection.
func (z *ZeroClawClient) ListSessions() ([]Session, error) {
	return []Session{{Key: "default", Label: "ZeroClaw"}}, nil
}

// GetHistory waits briefly for the history push that ZeroClaw sends on connect,
// then returns the messages. Returns empty slice if no history arrives in time.
func (z *ZeroClawClient) GetHistory(sessionKey string, limit int) ([]Message, error) {
	select {
	case msgs := <-z.histCh:
		if limit > 0 && len(msgs) > limit {
			msgs = msgs[len(msgs)-limit:]
		}
		return msgs, nil
	case <-time.After(500 * time.Millisecond):
		return nil, nil
	}
}

// SendMessage sends a chat message to ZeroClaw and returns a synthetic run ID.
func (z *ZeroClawClient) SendMessage(sessionKey, text, idempotencyKey string) (string, error) {
	msg := map[string]any{
		"type":    "message",
		"content": text,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("zeroclaw marshal: %w", err)
	}

	z.mu.Lock()
	conn := z.conn
	z.streamBuf = "" // reset accumulator for the new exchange
	z.mu.Unlock()

	if conn == nil {
		return "", fmt.Errorf("zeroclaw: not connected")
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return "", fmt.Errorf("zeroclaw send: %w", err)
	}
	return "zc-local", nil
}

// readLoop reads frames from the WebSocket and dispatches them as gateway events.
func (z *ZeroClawClient) readLoop() {
	defer func() {
		select {
		case <-z.done:
		default:
			z.setStatus(StatusError)
		}
	}()

	for {
		select {
		case <-z.done:
			return
		default:
		}

		z.mu.Lock()
		conn := z.conn
		z.mu.Unlock()
		if conn == nil {
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-z.done:
			default:
				z.setStatus(StatusError)
			}
			return
		}

		var frame map[string]any
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}

		z.dispatchFrame(frame)
	}
}

// dispatchFrame translates a ZeroClaw server frame into a gateway ChatEvent payload
// and calls onEvent("chat", ...) so the existing UI handles it.
//
// ZeroClaw `chunk` events carry only the new token; we accumulate them here so
// the UI sees the full streamed text on every delta (matching OpenClaw convention).
func (z *ZeroClawClient) dispatchFrame(frame map[string]any) {
	typ, _ := frame["type"].(string)

	switch typ {
	case "history":
		// Server sends this once on connect with past messages for the session.
		// Parse and deliver via histCh so GetHistory() can return them.
		var msgs []Message
		if raw, ok := frame["messages"].([]any); ok {
			for _, item := range raw {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				role, _ := m["role"].(string)
				content, _ := m["content"].(string)
				if (role == "user" || role == "assistant") && content != "" {
					msgs = append(msgs, Message{Role: role, Content: content})
				}
			}
		}
		// Non-blocking send — histCh is buffered(1); GetHistory drains it.
		z.histOnce.Do(func() {
			z.histCh <- msgs
		})

	case "chunk":
		// New token — accumulate and emit as delta with full text so far.
		token, _ := frame["content"].(string)
		z.mu.Lock()
		z.streamBuf += token
		accumulated := z.streamBuf
		z.mu.Unlock()

		if z.onEvent != nil {
			z.onEvent("chat", map[string]any{
				"state": "delta",
				"message": map[string]any{
					"content": accumulated,
				},
				"runId": "zc-local",
			})
		}

	case "done":
		// Full response — emit as final.
		fullResponse, _ := frame["full_response"].(string)
		z.mu.Lock()
		if fullResponse == "" {
			fullResponse = z.streamBuf
		}
		z.streamBuf = ""
		z.mu.Unlock()

		if z.onEvent != nil {
			z.onEvent("chat", map[string]any{
				"state": "final",
				"message": map[string]any{
					"content": fullResponse,
				},
				"runId": "zc-local",
			})
		}

	case "error":
		msg, _ := frame["message"].(string)
		z.mu.Lock()
		z.streamBuf = ""
		z.mu.Unlock()

		if z.onEvent != nil {
			z.onEvent("chat", map[string]any{
				"state":        "error",
				"errorMessage": msg,
				"runId":        "zc-local",
			})
		}

	case "tool_call":
		name, _ := frame["name"].(string)
		z.mu.Lock()
		note := fmt.Sprintf("[calling %s…]", name)
		z.streamBuf = note
		z.mu.Unlock()

		if z.onEvent != nil {
			z.onEvent("chat", map[string]any{
				"state": "delta",
				"message": map[string]any{
					"content": note,
				},
				"runId": "zc-local",
			})
		}

	case "tool_result":
		z.mu.Lock()
		z.streamBuf = ""
		z.mu.Unlock()
	}
}

func (z *ZeroClawClient) setStatus(s Status) {
	z.mu.Lock()
	z.status = s
	z.mu.Unlock()
}

// Compile-time assertion: *ZeroClawClient must satisfy Gateway.
var _ Gateway = (*ZeroClawClient)(nil)
