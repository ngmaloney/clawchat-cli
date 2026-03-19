package ollama

import (
	"os"
	"testing"
)

// These tests require a running Ollama server.
// Set OLLAMA_TEST_URL to enable (e.g. http://localhost:11434).

func ollamaURL(t *testing.T) string {
	url := os.Getenv("OLLAMA_TEST_URL")
	if url == "" {
		t.Skip("OLLAMA_TEST_URL not set, skipping integration test")
	}
	return url
}

func TestPing(t *testing.T) {
	url := ollamaURL(t)
	c := New(url, "qwen2.5:7b")
	if err := c.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestListModels(t *testing.T) {
	url := ollamaURL(t)
	c := New(url, "qwen2.5:7b")
	models, err := c.ListModels()
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	t.Logf("Found %d models:", len(models))
	for _, m := range models {
		t.Logf("  %s (%d MB)", m.Name, m.Size/(1024*1024))
	}
}

func TestChatStream(t *testing.T) {
	url := ollamaURL(t)
	c := New(url, "qwen2.5:7b")

	messages := []ChatMessage{
		{Role: "user", Content: "Say hello in exactly 3 words"},
	}

	var deltaCount int
	result, err := c.ChatStream(messages, func(delta StreamDelta) {
		deltaCount++
	})
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty response")
	}
	if deltaCount == 0 {
		t.Fatal("expected at least one delta callback")
	}
	t.Logf("Got %d deltas, final: %q", deltaCount, result)
}

func TestChatStreamMultiTurn(t *testing.T) {
	url := ollamaURL(t)
	c := New(url, "qwen2.5:7b")

	messages := []ChatMessage{
		{Role: "user", Content: "My name is TestBot"},
	}

	reply1, err := c.ChatStream(messages, nil)
	if err != nil {
		t.Fatalf("Turn 1 failed: %v", err)
	}
	t.Logf("Turn 1: %q", reply1)

	messages = append(messages,
		ChatMessage{Role: "assistant", Content: reply1},
		ChatMessage{Role: "user", Content: "What is my name?"},
	)

	reply2, err := c.ChatStream(messages, nil)
	if err != nil {
		t.Fatalf("Turn 2 failed: %v", err)
	}
	t.Logf("Turn 2: %q", reply2)
}
