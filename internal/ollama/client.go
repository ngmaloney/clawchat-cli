// Package ollama implements a direct Ollama HTTP chat client with streaming.
package ollama

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks directly to an Ollama server.
type Client struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// ChatMessage is a single message in the conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the request body for /api/chat.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatResponse is one streamed JSON line from /api/chat.
type chatResponse struct {
	Model     string      `json:"model"`
	Message   ChatMessage `json:"message"`
	Done      bool        `json:"done"`
	CreatedAt string      `json:"created_at"`
}

// StreamDelta is emitted for each chunk during streaming.
type StreamDelta struct {
	Content string
	Done    bool
}

// ModelInfo holds metadata about an available model.
type ModelInfo struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
}

// New creates a new Ollama client.
func New(baseURL, model string) *Client {
	return &Client{
		BaseURL: baseURL,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Minute, // LLM responses can be slow
		},
	}
}

// Ping checks if the Ollama server is reachable.
func (c *Client) Ping() error {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("cannot reach Ollama at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}
	return nil
}

// ListModels returns the available models on the server.
func (c *Client) ListModels() ([]ModelInfo, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []ModelInfo `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing models: %w", err)
	}
	return result.Models, nil
}

// ChatStream sends a chat request and streams deltas to the callback.
// The callback is called for each chunk; final chunk has Done=true.
// Returns the full accumulated response text.
func (c *Client) ChatStream(messages []ChatMessage, onDelta func(StreamDelta)) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return "", fmt.Errorf("marshalling request: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama returned %d: %s", resp.StatusCode, string(errBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	// Ollama can produce large lines for long responses
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var accumulated string
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk chatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue
		}

		accumulated += chunk.Message.Content
		if onDelta != nil {
			onDelta(StreamDelta{
				Content: accumulated,
				Done:    chunk.Done,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return accumulated, fmt.Errorf("reading stream: %w", err)
	}

	return accumulated, nil
}
