package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxChatRequestBytes = 2 << 20

type ChatRequest struct {
	Question        string               `json:"question"`
	Diagnosis       map[string]any       `json:"diagnosis,omitempty"`
	History         []ChatHistoryMessage `json:"history,omitempty"`
	SourceContext   string               `json:"source_context,omitempty"`
	UISchemaCatalog map[string]any       `json:"ui_schema_catalog,omitempty"`
}

type ChatHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (s Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var req ChatRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChatRequestBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat request")
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "chat question is required")
		return
	}
	if len([]rune(req.Question)) > 1200 {
		writeError(w, http.StatusBadRequest, "chat question exceeds 1200 characters")
		return
	}
	req.History = normalizeChatHistory(req.History)
	if wantsChatStream(r) {
		streamChat(w, r, req)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout())
	defer cancel()
	envelope, err := runLegatoChat(ctx, req)
	if err != nil {
		writeError(w, chatErrorStatus(err), err.Error())
		return
	}
	response, err := buildChatResponse(envelope)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response.Payload)
}

type chatRunResult struct {
	envelope *LegatoEnvelope
	err      error
}

type chatResponse struct {
	Answer  string
	Chat    map[string]any
	Payload map[string]any
}

func wantsChatStream(r *http.Request) bool {
	if r.URL.Query().Get("stream") == "1" {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")
}

func streamChat(w http.ResponseWriter, r *http.Request, req ChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout())
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if writeChatSSE(w, "chat.status", map[string]any{
		"status":  "running",
		"message": "Legato Chat workflow 正在生成回答",
	}) != nil {
		return
	}
	flusher.Flush()

	resultCh := make(chan chatRunResult, 1)
	go func() {
		envelope, err := runLegatoChat(ctx, req)
		resultCh <- chatRunResult{envelope: envelope, err: err}
	}()

	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = writeChatSSE(w, "chat.error", map[string]any{
				"status": "failed",
				"error":  "chat stream canceled or timed out",
			})
			flusher.Flush()
			return
		case <-ticker.C:
			if writeChatSSE(w, "chat.status", map[string]any{
				"status":  "running",
				"message": "Legato Chat workflow 仍在生成回答",
			}) != nil {
				return
			}
			flusher.Flush()
		case result := <-resultCh:
			if result.err != nil {
				_ = writeChatSSE(w, "chat.error", map[string]any{
					"status": "failed",
					"error":  result.err.Error(),
				})
				flusher.Flush()
				return
			}
			response, err := buildChatResponse(result.envelope)
			if err != nil {
				_ = writeChatSSE(w, "chat.error", map[string]any{
					"status": "failed",
					"error":  err.Error(),
				})
				flusher.Flush()
				return
			}
			if writeChatSSE(w, "chat.status", map[string]any{
				"status":  "streaming",
				"message": "回答已生成，正在流式输出",
			}) != nil {
				return
			}
			flusher.Flush()
			for _, chunk := range splitChatAnswerChunks(response.Answer) {
				if writeChatSSE(w, "chat.chunk", map[string]any{"delta": chunk}) != nil {
					return
				}
				flusher.Flush()
				select {
				case <-ctx.Done():
					return
				case <-time.After(12 * time.Millisecond):
				}
			}
			if writeChatSSE(w, "chat.done", response.Payload) != nil {
				return
			}
			flusher.Flush()
			return
		}
	}
}

func buildChatResponse(envelope *LegatoEnvelope) (chatResponse, error) {
	chat, ok := envelope.Data["chat"].(map[string]any)
	if !ok {
		return chatResponse{}, errors.New("Legato chat returned invalid payload")
	}
	answer := strings.TrimSpace(stringValue(chat["answer"]))
	if answer == "" {
		return chatResponse{}, errors.New("Legato chat returned empty answer")
	}
	payload := map[string]any{
		"answer":     answer,
		"chat":       chat,
		"formatter":  envelope.Formatter,
		"elapsed_ms": envelope.ElapsedMS,
		"warnings":   envelope.Warnings,
		"debug":      envelope.Debug,
	}
	return chatResponse{Answer: answer, Chat: chat, Payload: payload}, nil
}

func writeChatSSE(w io.Writer, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"status":"failed","error":"chat event marshal failed"}`)
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func splitChatAnswerChunks(answer string) []string {
	runes := []rune(answer)
	if len(runes) == 0 {
		return nil
	}
	chunks := make([]string, 0, len(runes)/18+1)
	for start := 0; start < len(runes); {
		end := start + 18
		if end > len(runes) {
			end = len(runes)
		}
		for end < len(runes) && end-start < 28 && !isChatChunkBoundary(runes[end-1]) {
			end++
		}
		chunks = append(chunks, string(runes[start:end]))
		start = end
	}
	return chunks
}

func isChatChunkBoundary(value rune) bool {
	switch value {
	case '。', '，', '；', '：', '、', '.', ',', ';', ':', '!', '?', '！', '？', '\n':
		return true
	default:
		return false
	}
}

func runLegatoChat(ctx context.Context, req ChatRequest) (*LegatoEnvelope, error) {
	tempDir, err := os.MkdirTemp("", "jobagent-chat-*")
	if err != nil {
		return nil, errors.New("无法创建 chat 临时目录")
	}
	defer cleanupDiagnosisTempDir(tempDir)

	sourceContext := strings.TrimSpace(req.SourceContext)
	if sourceContext == "" {
		sourceContext = req.Question
	}
	sourcePath := filepath.Join(tempDir, "chat.md")
	if err := os.WriteFile(sourcePath, []byte(sourceContext+"\n"), 0o600); err != nil {
		return nil, errors.New("无法写入 chat source")
	}

	stageInput := map[string]any{
		"question":          req.Question,
		"diagnosis":         req.Diagnosis,
		"history":           req.History,
		"ui_schema_catalog": req.UISchemaCatalog,
	}
	inputPath := filepath.Join(tempDir, "chat-input.json")
	payload, err := json.Marshal(stageInput)
	if err != nil {
		return nil, errors.New("无法编码 chat input")
	}
	if err := os.WriteFile(inputPath, payload, 0o600); err != nil {
		return nil, errors.New("无法写入 chat input")
	}
	return runLegatoWithWorkflowStageInputTimeout(ctx, sourcePath, "chat", "answer", inputPath, chatLegatoTimeoutMS())
}

func normalizeChatHistory(history []ChatHistoryMessage) []ChatHistoryMessage {
	out := make([]ChatHistoryMessage, 0, minInt(len(history), 12))
	start := 0
	if len(history) > 12 {
		start = len(history) - 12
	}
	for _, item := range history[start:] {
		role := strings.TrimSpace(item.Role)
		content := strings.TrimSpace(item.Content)
		if role != "user" && role != "assistant" && role != "system" {
			continue
		}
		if content == "" {
			continue
		}
		if len([]rune(content)) > 2000 {
			content = string([]rune(content)[:2000])
		}
		out = append(out, ChatHistoryMessage{Role: role, Content: content})
	}
	return out
}

func chatTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("CHAT_TIMEOUT_SECONDS"))
	if value == "" {
		return 150 * time.Second
	}
	seconds, err := parsePositiveInt(value)
	if err != nil {
		return 150 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func chatLegatoTimeoutMS() string {
	value := strings.TrimSpace(os.Getenv("CHAT_LEGATO_TIMEOUT_MS"))
	if value != "" {
		return value
	}
	budget := chatTimeout() - 10*time.Second
	if budget < 60*time.Second {
		budget = 60 * time.Second
	}
	return strconv.FormatInt(budget.Milliseconds(), 10)
}

func parsePositiveInt(value string) (int, error) {
	parsed, err := time.ParseDuration(value + "s")
	if err == nil && parsed > 0 {
		return int(parsed / time.Second), nil
	}
	return 0, errors.New("invalid positive integer")
}

func chatErrorStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	message := err.Error()
	if strings.Contains(message, "cannot locate Agents/legato") || strings.Contains(message, "cannot reach Presto") {
		return http.StatusServiceUnavailable
	}
	if strings.Contains(message, "context deadline exceeded") {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}
