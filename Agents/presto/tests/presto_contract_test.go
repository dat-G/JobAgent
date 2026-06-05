package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []toolSpec    `json:"tools,omitempty"`
	ToolChoice  interface{}   `json:"tool_choice,omitempty"`
	SessionID   string        `json:"session_id,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"`
	Name       string      `json:"name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCall  `json:"tool_calls,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolSpec struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Index        int         `json:"index,omitempty"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
}

type retryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

func TestPromptAssemblyPreservesMultiTurnContextAndTools(t *testing.T) {
	temperature := 0.0
	history := []chatMessage{
		{Role: "user", Content: "Find backend agent roles in Shanghai."},
		{Role: "assistant", Content: "I can search and rank matching openings."},
		{Role: "tool", ToolCallID: "call_jobs_1", Content: `{"count":2}`},
		{Role: "assistant", Content: "I found two matching roles."},
	}

	req := assemblePrompt(
		"Use concise answers and preserve session context.",
		history,
		"Which one has Go requirements?",
		[]toolSpec{jobSearchToolSpec()},
	)
	req.Temperature = &temperature

	assertRoles(t, req.Messages,
		"system",
		"user",
		"assistant",
		"tool",
		"assistant",
		"user",
	)
	if got := req.Messages[3].ToolCallID; got != "call_jobs_1" {
		t.Fatalf("tool result must stay linked to original call id, got %q", got)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("expected one tool declaration, got %d", len(req.Tools))
	}
	if req.Tools[0].Function.Name != "search_jobs" {
		t.Fatalf("unexpected tool name %q", req.Tools[0].Function.Name)
	}
	required, ok := req.Tools[0].Function.Parameters["required"].([]interface{})
	if !ok || len(required) != 2 {
		t.Fatalf("tool schema must declare required fields, got %#v", req.Tools[0].Function.Parameters["required"])
	}

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal prompt payload: %v", err)
	}
	var decoded chatRequest
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal prompt payload: %v", err)
	}
	assertRoles(t, decoded.Messages,
		"system",
		"user",
		"assistant",
		"tool",
		"assistant",
		"user",
	)
}

func TestRetryBehaviorRetriesTransientFailuresOnly(t *testing.T) {
	var transientAttempts int32
	transient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&transientAttempts, 1)
		if attempt < 3 {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer transient.Close()

	ctx := context.Background()
	resp, attempts, err := doJSONWithRetry(ctx, transient.Client(), http.MethodPost, transient.URL, []byte(`{}`), retryConfig{
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("retrying transient failure: %v", err)
	}
	defer resp.Body.Close()
	if attempts != 3 {
		t.Fatalf("expected success on third attempt, got %d attempts", attempts)
	}
	if got := resp.StatusCode; got != http.StatusOK {
		t.Fatalf("expected final status 200, got %d", got)
	}

	var badRequestAttempts int32
	badRequest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&badRequestAttempts, 1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer badRequest.Close()

	resp, attempts, err = doJSONWithRetry(ctx, badRequest.Client(), http.MethodPost, badRequest.URL, []byte(`{}`), retryConfig{
		MaxAttempts: 4,
		BaseDelay:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("non-retryable response should be returned for caller handling: %v", err)
	}
	defer resp.Body.Close()
	if attempts != 1 {
		t.Fatalf("400 response must not be retried, got %d attempts", attempts)
	}
	if got := atomic.LoadInt32(&badRequestAttempts); got != 1 {
		t.Fatalf("expected one bad-request call, got %d", got)
	}
}

func TestToolCallLoopSubmitsToolResultBeforeFinalAnswer(t *testing.T) {
	var mu sync.Mutex
	var seen []chatRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		seen = append(seen, req)
		callNumber := len(seen)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch callNumber {
		case 1:
			writeJSON(w, chatResponse{Choices: []chatChoice{{
				Message: chatMessage{
					Role: "assistant",
					ToolCalls: []toolCall{{
						ID:   "call_add_1",
						Type: "function",
						Function: functionCall{
							Name:      "add",
							Arguments: `{"a":2,"b":5}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}}})
		case 2:
			writeJSON(w, chatResponse{Choices: []chatChoice{{
				Message: chatMessage{Role: "assistant", Content: "2 + 5 = 7"},
			}}})
		default:
			http.Error(w, "unexpected extra model call", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	final, err := runToolLoop(context.Background(), server.Client(), server.URL, "What is 2 + 5?")
	if err != nil {
		t.Fatalf("tool loop failed: %v", err)
	}
	if final != "2 + 5 = 7" {
		t.Fatalf("unexpected final answer %q", final)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("expected two model calls, got %d", len(seen))
	}
	if len(seen[0].Tools) != 1 || seen[0].Tools[0].Function.Name != "add" {
		t.Fatalf("first request must advertise add tool, got %#v", seen[0].Tools)
	}
	second := seen[1]
	if len(second.Messages) < 4 {
		t.Fatalf("second request must include prior messages, assistant tool call, and tool result; got %d messages", len(second.Messages))
	}
	assistant := second.Messages[len(second.Messages)-2]
	toolResult := second.Messages[len(second.Messages)-1]
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_add_1" {
		t.Fatalf("second request missing assistant tool call transcript: %#v", assistant)
	}
	if toolResult.Role != "tool" || toolResult.ToolCallID != "call_add_1" || fmt.Sprint(toolResult.Content) != "7" {
		t.Fatalf("second request missing linked tool result: %#v", toolResult)
	}
}

func TestHTTPAPIMultiTurnSessionIfConfigured(t *testing.T) {
	baseURL := os.Getenv("PRESTO_HTTP_BASE_URL")
	if baseURL == "" {
		t.Skip("set PRESTO_HTTP_BASE_URL to run live HTTP API smoke validation")
	}

	sessionID := fmt.Sprintf("presto-contract-%d", time.Now().UnixNano())
	token := fmt.Sprintf("presto-session-token-%d", time.Now().UnixNano())
	endpoint := liveChatURL(baseURL, envDefault("PRESTO_CHAT_PATH", "/v1/chat/completions"))
	client := &http.Client{Timeout: 30 * time.Second}
	temperature := 0.0

	first := chatRequest{
		Model:       envDefault("PRESTO_MODEL", "presto-contract"),
		SessionID:   sessionID,
		Temperature: &temperature,
		Messages: []chatMessage{
			{Role: "system", Content: "You are a precise API smoke-test assistant."},
			{Role: "user", Content: fmt.Sprintf("Remember this exact token for this session: %s. Reply only OK.", token)},
		},
	}
	if _, err := postLiveChat(context.Background(), client, endpoint, sessionID, first); err != nil {
		t.Fatalf("first live chat request failed: %v", err)
	}

	second := chatRequest{
		Model:       first.Model,
		SessionID:   sessionID,
		Temperature: &temperature,
		Messages: []chatMessage{
			{Role: "user", Content: "What exact token were you asked to remember? Reply only with the token."},
		},
	}
	resp, err := postLiveChat(context.Background(), client, endpoint, sessionID, second)
	if err != nil {
		t.Fatalf("second live chat request failed: %v", err)
	}
	content := firstChoiceContent(resp)
	if !strings.Contains(content, token) {
		t.Fatalf("live API did not preserve session memory: expected token %q in %q", token, content)
	}
}

func BenchmarkPromptAssembly(b *testing.B) {
	history := []chatMessage{
		{Role: "user", Content: "Find roles."},
		{Role: "assistant", Content: "Searching."},
		{Role: "tool", ToolCallID: "call_jobs_1", Content: `{"count":2}`},
		{Role: "assistant", Content: "Found two roles."},
	}
	tools := []toolSpec{jobSearchToolSpec()}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := assemblePrompt("Be concise.", history, "Rank them.", tools)
		if _, err := json.Marshal(req); err != nil {
			b.Fatalf("marshal prompt payload: %v", err)
		}
	}
}

func assemblePrompt(system string, history []chatMessage, user string, tools []toolSpec) chatRequest {
	messages := make([]chatMessage, 0, len(history)+2)
	if system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	messages = append(messages, history...)
	messages = append(messages, chatMessage{Role: "user", Content: user})

	return chatRequest{
		Model:    "presto-contract",
		Messages: messages,
		Tools:    tools,
	}
}

func doJSONWithRetry(ctx context.Context, client *http.Client, method string, url string, body []byte, cfg retryConfig) (*http.Response, int, error) {
	if cfg.MaxAttempts <= 0 {
		return nil, 0, errors.New("max attempts must be positive")
	}
	if cfg.BaseDelay < 0 {
		return nil, 0, errors.New("base delay must not be negative")
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, attempt, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err == nil && !isTransientStatus(resp.StatusCode) {
			return resp, attempt, nil
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("transient HTTP status %d", resp.StatusCode)
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		if cfg.BaseDelay > 0 {
			timer := time.NewTimer(cfg.BaseDelay * time.Duration(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, attempt, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, cfg.MaxAttempts, lastErr
}

func isTransientStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func runToolLoop(ctx context.Context, client *http.Client, endpoint string, userPrompt string) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: "Call tools when arithmetic is required."},
		{Role: "user", Content: userPrompt},
	}

	for turn := 0; turn < 4; turn++ {
		req := chatRequest{
			Model:    "presto-contract",
			Messages: append([]chatMessage(nil), messages...),
			Tools:    []toolSpec{addToolSpec()},
		}
		resp, err := postChat(ctx, client, endpoint, req, nil)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", errors.New("model response had no choices")
		}

		assistant := resp.Choices[0].Message
		if len(assistant.ToolCalls) == 0 {
			return firstChoiceContent(resp), nil
		}
		messages = append(messages, assistant)
		for _, call := range assistant.ToolCalls {
			result, err := executeToolCall(call)
			if err != nil {
				return "", err
			}
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}
	}
	return "", errors.New("tool loop exceeded maximum turns")
}

func executeToolCall(call toolCall) (string, error) {
	switch call.Function.Name {
	case "add":
		var args struct {
			A float64 `json:"a"`
			B float64 `json:"b"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("decode add arguments: %w", err)
		}
		return fmt.Sprintf("%g", args.A+args.B), nil
	default:
		return "", fmt.Errorf("unknown tool %q", call.Function.Name)
	}
}

func postLiveChat(ctx context.Context, client *http.Client, endpoint string, sessionID string, req chatRequest) (chatResponse, error) {
	headers := map[string]string{}
	if apiKey := os.Getenv("PRESTO_API_KEY"); apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	if header := envDefault("PRESTO_SESSION_HEADER", "X-Session-ID"); header != "" {
		headers[header] = sessionID
	}

	payload, err := requestMap(req)
	if err != nil {
		return chatResponse{}, err
	}
	sessionField := envDefault("PRESTO_SESSION_FIELD", "session_id")
	if sessionField == "" {
		delete(payload, "session_id")
	} else if sessionField != "session_id" {
		delete(payload, "session_id")
		payload[sessionField] = sessionID
	}

	return postChatMap(ctx, client, endpoint, payload, headers)
}

func postChat(ctx context.Context, client *http.Client, endpoint string, req chatRequest, headers map[string]string) (chatResponse, error) {
	payload, err := requestMap(req)
	if err != nil {
		return chatResponse{}, err
	}
	return postChatMap(ctx, client, endpoint, payload, headers)
}

func postChatMap(ctx context.Context, client *http.Client, endpoint string, payload map[string]interface{}, headers map[string]string) (chatResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return chatResponse{}, fmt.Errorf("marshal chat request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return chatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return chatResponse{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return chatResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return chatResponse{}, fmt.Errorf("chat API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return chatResponse{}, fmt.Errorf("decode chat response: %w; body=%s", err, strings.TrimSpace(string(responseBody)))
	}
	return decoded, nil
}

func requestMap(req chatRequest) (map[string]interface{}, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode request map: %w", err)
	}
	return payload, nil
}

func firstChoiceContent(resp chatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	content := resp.Choices[0].Message.Content
	switch typed := content.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func writeJSON(w http.ResponseWriter, value interface{}) {
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func assertRoles(t *testing.T, messages []chatMessage, roles ...string) {
	t.Helper()
	if len(messages) != len(roles) {
		t.Fatalf("expected %d messages, got %d: %#v", len(roles), len(messages), messages)
	}
	for i, role := range roles {
		if messages[i].Role != role {
			t.Fatalf("message %d role: expected %q, got %q", i, role, messages[i].Role)
		}
	}
}

func envDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func liveChatURL(base string, path string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if path == "" {
		path = "/v1/chat/completions"
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func jobSearchToolSpec() toolSpec {
	return toolSpec{
		Type: "function",
		Function: toolFunction{
			Name:        "search_jobs",
			Description: "Search job postings using a query and location.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
					"city":  map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"query", "city"},
			},
		},
	}
}

func addToolSpec() toolSpec {
	return toolSpec{
		Type: "function",
		Function: toolFunction{
			Name:        "add",
			Description: "Add two numbers.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"a": map[string]interface{}{"type": "number"},
					"b": map[string]interface{}{"type": "number"},
				},
				"required": []interface{}{"a", "b"},
			},
		},
	}
}
