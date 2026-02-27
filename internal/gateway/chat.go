package gateway

import (
	"encoding/json"
	"fmt"
	"time"
)

// Session holds metadata about a gateway session.
type Session struct {
	Key     string
	Label   string
	Channel string
	Model   string
}

// Message is a chat message.
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// ChatEvent is a streaming chat event from the gateway.
type ChatEvent struct {
	RunID      string
	SessionKey string
	Seq        int
	State      string // "delta", "final", "error"
	Content    string // accumulated text
	ErrorMsg   string
}

// ListSessions returns the available sessions.
func (c *Client) ListSessions() ([]Session, error) {
	payload, err := c.Call("sessions.list", nil)
	if err != nil {
		return nil, fmt.Errorf("sessions.list: %w", err)
	}

	raw, _ := json.Marshal(payload)
	var result struct {
		Sessions []struct {
			Key     string `json:"key"`
			Label   string `json:"label"`
			Channel string `json:"channel"`
			Model   string `json:"model"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing sessions: %w", err)
	}

	sessions := make([]Session, len(result.Sessions))
	for i, s := range result.Sessions {
		sessions[i] = Session{
			Key:     s.Key,
			Label:   s.Label,
			Channel: s.Channel,
			Model:   s.Model,
		}
	}
	return sessions, nil
}

// GetHistory returns recent messages for a session.
func (c *Client) GetHistory(sessionKey string, limit int) ([]Message, error) {
	if limit == 0 {
		limit = 50
	}
	payload, err := c.Call("chat.history", map[string]any{
		"sessionKey": sessionKey,
		"limit":      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("chat.history: %w", err)
	}

	raw, _ := json.Marshal(payload)
	var result struct {
		Messages []struct {
			Role      string `json:"role"`
			Content   any    `json:"content"`
			Timestamp any    `json:"timestamp"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing history: %w", err)
	}

	messages := make([]Message, 0, len(result.Messages))
	for _, m := range result.Messages {
		// Only show user and assistant text messages — skip tool calls, results, system
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		content := extractContent(m.Content)
		if content == "" {
			continue
		}
		msg := Message{
			Role:    m.Role,
			Content: content,
		}
		switch ts := m.Timestamp.(type) {
		case float64:
			msg.Timestamp = time.UnixMilli(int64(ts))
		case string:
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				msg.Timestamp = t
			}
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// SendMessage sends a chat message to a session.
func (c *Client) SendMessage(sessionKey, text, idempotencyKey string) error {
	_, err := c.Call("chat.send", map[string]any{
		"sessionKey":     sessionKey,
		"message":        text,
		"idempotencyKey": idempotencyKey,
	})
	return err
}

// ParseChatEvent parses a raw "chat" event payload into a ChatEvent.
func ParseChatEvent(payload map[string]any) ChatEvent {
	ev := ChatEvent{
		RunID:      strField(payload, "runId"),
		SessionKey: strField(payload, "sessionKey"),
		State:      strField(payload, "state"),
		ErrorMsg:   strField(payload, "errorMessage"),
	}
	if seq, ok := payload["seq"].(float64); ok {
		ev.Seq = int(seq)
	}
	if msg, ok := payload["message"].(map[string]any); ok {
		ev.Content = extractContent(msg["content"])
	}
	return ev
}

// extractContent converts a content field (string or []ContentBlock) to a plain string.
func extractContent(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var out string
		for _, block := range c {
			if b, ok := block.(map[string]any); ok {
				if t, ok := b["text"].(string); ok {
					out += t
				}
			}
		}
		return out
	}
	return ""
}

func strField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
