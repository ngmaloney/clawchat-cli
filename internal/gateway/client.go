// Package gateway implements an OpenClaw Gateway WebSocket client (Protocol v3).
package gateway

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Status represents the connection state.
type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusHandshaking  Status = "handshaking"
	StatusConnected    Status = "connected"
	StatusError        Status = "error"
)

// EventHandler is called when a gateway event arrives.
type EventHandler func(event string, payload map[string]any)

// StatusHandler is called when the connection status changes.
type StatusHandler func(Status)

// Options configures a Client.
type Options struct {
	URL             string
	Token           string
	OnStatus        StatusHandler
	OnEvent         EventHandler
	RequestTimeout  time.Duration
	MaxRetries      int
}

// Client is a Protocol v3 OpenClaw Gateway WebSocket client.
type Client struct {
	opts Options

	mu     sync.Mutex
	conn   *websocket.Conn
	status Status

	pendingMu sync.Mutex
	pending   map[string]chan response

	seq atomic.Int64

	done chan struct{}
	once sync.Once
}

type response struct {
	payload map[string]any
	err     error
}

// New creates a new Client. Call Connect() to establish the connection.
func New(opts Options) *Client {
	if opts.RequestTimeout == 0 {
		opts.RequestTimeout = 30 * time.Second
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = 10
	}
	return &Client{
		opts:    opts,
		status:  StatusDisconnected,
		pending: make(map[string]chan response),
		done:    make(chan struct{}),
	}
}

// Connect establishes the WebSocket connection and performs the handshake.
// It returns once the handshake is complete (status = connected) or fails.
func (c *Client) Connect() error {
	c.setStatus(StatusConnecting)

	u, err := url.Parse(c.opts.URL)
	if err != nil {
		return fmt.Errorf("invalid gateway URL: %w", err)
	}
	q := u.Query()
	q.Set("token", c.opts.Token)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		c.setStatus(StatusError)
		return fmt.Errorf("websocket dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.setStatus(StatusHandshaking)

	// Start read loop
	go c.readLoop()

	// Wait for connected (handshake driven by readLoop)
	deadline := time.After(c.opts.RequestTimeout)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			c.Close()
			return fmt.Errorf("handshake timed out")
		case <-tick.C:
			if c.Status() == StatusConnected {
				return nil
			}
			if c.Status() == StatusError {
				return fmt.Errorf("handshake failed")
			}
		case <-c.done:
			return fmt.Errorf("client closed during connect")
		}
	}
}

// Close shuts down the connection.
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.mu.Unlock()
		c.setStatus(StatusDisconnected)
		c.rejectAllPending("client closed")
	})
}

// Status returns the current connection status.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Call sends a request and waits for a response.
func (c *Client) Call(method string, params map[string]any) (map[string]any, error) {
	id := fmt.Sprintf("cc-%d", c.seq.Add(1))
	frame := map[string]any{
		"type":   "req",
		"id":     id,
		"method": method,
		"params": params,
	}

	ch := make(chan response, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.sendJSON(frame); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case r := <-ch:
		return r.payload, r.err
	case <-time.After(c.opts.RequestTimeout):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("request %q timed out", method)
	case <-c.done:
		return nil, fmt.Errorf("client closed")
	}
}

// sendHandshake sends the connect request (called from readLoop after challenge).
func (c *Client) sendHandshake(nonce string) error {
	id := fmt.Sprintf("cc-%d", c.seq.Add(1))

	params := map[string]any{
		"role":        "operator",
		"scopes":      []string{"operator.admin", "operator.approvals", "operator.pairing"},
		"auth":        map[string]any{"token": c.opts.Token},
		"client":      map[string]any{"id": "clawchat-cli", "version": "dev", "platform": "cli", "mode": "cli"},
		"minProtocol": 3,
		"maxProtocol": 3,
	}

	frame := map[string]any{
		"type":   "req",
		"id":     id,
		"method": "connect",
		"params": params,
	}

	ch := make(chan response, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.sendJSON(frame); err != nil {
		return err
	}

	// Wait for hello-ok
	select {
	case r := <-ch:
		if r.err != nil {
			c.setStatus(StatusError)
			return fmt.Errorf("handshake rejected: %w", r.err)
		}
		if t, _ := r.payload["type"].(string); t != "hello-ok" {
			c.setStatus(StatusError)
			return fmt.Errorf("unexpected handshake response: %v", r.payload)
		}
		c.setStatus(StatusConnected)
		return nil
	case <-time.After(c.opts.RequestTimeout):
		c.setStatus(StatusError)
		return fmt.Errorf("handshake timed out")
	case <-c.done:
		return fmt.Errorf("client closed during handshake")
	}
}

// readLoop reads frames from the WebSocket and dispatches them.
func (c *Client) readLoop() {
	defer func() {
		select {
		case <-c.done:
		default:
			c.setStatus(StatusError)
			c.rejectAllPending("read loop exited")
		}
	}()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-c.done:
			default:
				c.setStatus(StatusError)
				c.rejectAllPending(fmt.Sprintf("read error: %v", err))
			}
			return
		}

		var frame map[string]any
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}

		switch frame["type"] {
		case "event":
			c.handleEvent(frame)
		case "res":
			c.handleResponse(frame)
		}
	}
}

func (c *Client) handleEvent(frame map[string]any) {
	event, _ := frame["event"].(string)
	payload, _ := frame["payload"].(map[string]any)
	if payload == nil {
		payload = make(map[string]any)
	}

	if event == "connect.challenge" {
		nonce, _ := payload["nonce"].(string)
		go func() {
			if err := c.sendHandshake(nonce); err != nil {
				c.setStatus(StatusError)
			}
		}()
		return
	}

	if c.opts.OnEvent != nil {
		c.opts.OnEvent(event, payload)
	}
}

func (c *Client) handleResponse(frame map[string]any) {
	id, _ := frame["id"].(string)

	c.pendingMu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	if !ok {
		return
	}

	ok2, _ := frame["ok"].(bool)
	if ok2 {
		payload, _ := frame["payload"].(map[string]any)
		if payload == nil {
			payload = make(map[string]any)
		}
		ch <- response{payload: payload}
	} else {
		msg := "unknown error"
		if errObj, ok := frame["error"].(map[string]any); ok {
			if m, ok := errObj["message"].(string); ok {
				msg = m
			}
		}
		ch <- response{err: fmt.Errorf("%s", msg)}
	}
}

func (c *Client) sendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *Client) setStatus(s Status) {
	c.mu.Lock()
	changed := c.status != s
	c.status = s
	c.mu.Unlock()
	if changed && c.opts.OnStatus != nil {
		c.opts.OnStatus(s)
	}
}

func (c *Client) rejectAllPending(reason string) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, ch := range c.pending {
		ch <- response{err: fmt.Errorf(reason)}
		delete(c.pending, id)
	}
}
