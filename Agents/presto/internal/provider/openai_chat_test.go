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

func writeChatResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}
