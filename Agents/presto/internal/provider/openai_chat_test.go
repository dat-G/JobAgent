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

func writeChatResponse(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}
