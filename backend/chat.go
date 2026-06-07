package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxChatRequestBytes = 2 << 20

type ChatRequest struct {
	Question      string               `json:"question"`
	Diagnosis     map[string]any       `json:"diagnosis,omitempty"`
	History       []ChatHistoryMessage `json:"history,omitempty"`
	SourceContext string               `json:"source_context,omitempty"`
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

	ctx, cancel := context.WithTimeout(r.Context(), chatTimeout())
	defer cancel()
	envelope, err := runLegatoChat(ctx, req)
	if err != nil {
		writeError(w, chatErrorStatus(err), err.Error())
		return
	}
	chat, ok := envelope.Data["chat"].(map[string]any)
	if !ok {
		writeError(w, http.StatusBadGateway, "Legato chat returned invalid payload")
		return
	}
	answer := strings.TrimSpace(stringValue(chat["answer"]))
	if answer == "" {
		writeError(w, http.StatusBadGateway, "Legato chat returned empty answer")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"answer":     answer,
		"chat":       chat,
		"formatter":  envelope.Formatter,
		"elapsed_ms": envelope.ElapsedMS,
		"warnings":   envelope.Warnings,
		"debug":      envelope.Debug,
	})
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
		"question":  req.Question,
		"diagnosis": req.Diagnosis,
		"history":   req.History,
	}
	inputPath := filepath.Join(tempDir, "chat-input.json")
	payload, err := json.Marshal(stageInput)
	if err != nil {
		return nil, errors.New("无法编码 chat input")
	}
	if err := os.WriteFile(inputPath, payload, 0o600); err != nil {
		return nil, errors.New("无法写入 chat input")
	}
	return runLegatoWithWorkflowStageInput(ctx, sourcePath, "chat", "answer", inputPath)
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
		return 70 * time.Second
	}
	seconds, err := parsePositiveInt(value)
	if err != nil {
		return 70 * time.Second
	}
	return time.Duration(seconds) * time.Second
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
