package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"presto/internal/agent"
)

func TestOpenAIChatSendsZeroTemperature(t *testing.T) {
	payloads := make(chan map[string]any, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		payloads <- payload
		writeChatResponse(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()

	llm := NewOpenAIChat(server.URL, "test-key", server.Client())
	_, err := llm.Chat(context.Background(), agent.ChatRequest{
		Model: "unit-model",
		Messages: []agent.Message{{
			Role:    agent.RoleUser,
			Content: "hello",
		}},
		Temperature: 0,
	})
	if err != nil {
		t.Fatal(err)
	}

	payload := <-payloads
	value, ok := payload["temperature"]
	if !ok {
		t.Fatal("temperature was omitted")
	}
	if value != float64(0) {
		t.Fatalf("temperature = %#v, want 0", value)
	}
}

func TestOpenAIChatSendsThinkingOptionsFromEnv(t *testing.T) {
	t.Setenv("PRESTO_THINKING", "enabled")
	t.Setenv("PRESTO_REASONING_EFFORT", "high")
	payloads := make(chan map[string]any, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		payloads <- payload
		writeChatResponse(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer server.Close()

	llm := NewOpenAIChat(server.URL, "test-key", server.Client())
	_, err := llm.Chat(context.Background(), agent.ChatRequest{
		Model: "unit-model",
		Messages: []agent.Message{{
			Role:    agent.RoleUser,
			Content: "hello",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	payload := <-payloads
	thinking, ok := payload["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v, want object", payload["thinking"])
	}
	if thinking["type"] != "enabled" {
		t.Fatalf("thinking.type = %#v, want enabled", thinking["type"])
	}
	if payload["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %#v, want high", payload["reasoning_effort"])
	}
}

func TestOpenAIChatStreamsContentDeltas(t *testing.T) {
	payloads := make(chan map[string]any, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		payloads <- payload
		writeSSE(w,
			`{"choices":[{"delta":{"role":"assistant"}}]}`,
			`{"choices":[{"delta":{"content":"hello"}}]}`,
			`{"choices":[{"delta":{"content":" world"}}]}`,
			`[DONE]`,
		)
	}))
	defer server.Close()

	llm := NewOpenAIChat(server.URL, "test-key", server.Client())
	var deltas []string
	response, err := llm.ChatStream(context.Background(), agent.ChatRequest{
		Model: "unit-model",
		Messages: []agent.Message{{
			Role:    agent.RoleUser,
			Content: "hello",
		}},
	}, func(delta agent.ChatStreamDelta) {
		if delta.Content != "" {
			deltas = append(deltas, delta.Content)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	payload := <-payloads
	if payload["stream"] != true {
		t.Fatalf("stream = %#v, want true", payload["stream"])
	}
	if response.Message.Content != "hello world" {
		t.Fatalf("content = %q, want hello world", response.Message.Content)
	}
	if got := len(deltas); got != 2 {
		t.Fatalf("deltas = %d, want 2: %#v", got, deltas)
	}
	if deltas[0] != "hello" || deltas[1] != " world" {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
}

func TestOpenAIChatStreamKeepsReasoningInternalByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w,
			`{"choices":[{"delta":{"reasoning_content":"private trace"}}]}`,
			`{"choices":[{"delta":{"content":"answer"}}]}`,
			`[DONE]`,
		)
	}))
	defer server.Close()

	llm := NewOpenAIChat(server.URL, "test-key", server.Client())
	var deltas []agent.ChatStreamDelta
	response, err := llm.ChatStream(context.Background(), agent.ChatRequest{
		Model: "unit-model",
		Messages: []agent.Message{{
			Role:    agent.RoleUser,
			Content: "hello",
		}},
	}, func(delta agent.ChatStreamDelta) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.ReasoningContent != "private trace" {
		t.Fatalf("reasoning_content = %q", response.Message.ReasoningContent)
	}
	if len(deltas) != 1 || deltas[0].Content != "answer" || deltas[0].ReasoningContent != "" {
		t.Fatalf("unexpected outward deltas: %#v", deltas)
	}
}

func writeChatResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func writeSSE(w http.ResponseWriter, frames ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, frame := range frames {
		_, _ = w.Write([]byte("data: " + frame + "\n\n"))
	}
}
