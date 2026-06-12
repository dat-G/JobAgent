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
		s.streamChat(w, r, req)
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

func (s Server) streamChat(w http.ResponseWriter, r *http.Request, req ChatRequest) {
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

	emit := func(event string, payload any) bool {
		if writeChatSSE(w, event, payload) != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	emitError := func(err error) {
		if err == nil {
			err = errors.New("chat stream failed")
		}
		_ = emit("chat.error", map[string]any{
			"status": "failed",
			"error":  err.Error(),
		})
	}

	if writeChatSSE(w, "chat.status", map[string]any{
		"status":  "running",
		"message": "正在连接 Presto Chat workflow",
	}) != nil {
		return
	}
	flusher.Flush()

	client, err := s.newPrestoClient()
	if err != nil {
		emitError(err)
		return
	}
	prompt, err := buildLegatoChatPrompt(req)
	if err != nil {
		emitError(err)
		return
	}
	session, err := client.createSession(ctx, map[string]string{
		"app":      "legato",
		"workflow": "chat",
		"group":    "answer",
		"mode":     "stream",
	})
	if err != nil {
		emitError(err)
		return
	}
	run, err := client.createRun(ctx, session.ID, prompt)
	if err != nil {
		emitError(err)
		return
	}
	if !emit("chat.status", map[string]any{
		"status":     "running",
		"message":    "Presto run 已创建，等待模型输出",
		"session_id": session.ID,
		"run_id":     run.ID,
	}) {
		return
	}

	var output strings.Builder
	sentAnswer := ""
	streamErr := client.streamRunEvents(ctx, run.ID, func(event prestoEventResponse) bool {
		switch event.Type {
		case "run.retry":
			output.Reset()
			sentAnswer = ""
			if !emit("chat.reset", map[string]any{
				"status":  "retrying",
				"message": "Presto 自动重试中，已清空本次未完成输出",
			}) {
				return true
			}
			return false
		case "model.started":
			if !emit("chat.status", map[string]any{
				"status":  "reasoning",
				"message": "模型开始分析诊断上下文",
			}) {
				return true
			}
		case "model.delta":
			channel, delta := prestoChatDelta(event)
			if delta == "" {
				return false
			}
			if channel == "reasoning" || channel == "analysis" {
				if !emit("chat.reasoning", map[string]any{"delta": delta}) {
					return true
				}
				return false
			}
			if channel != "content" {
				return false
			}
			output.WriteString(delta)
			answerPrefix := chatAnswerPrefixFromJSONStream(output.String())
			if answerPrefix != "" && strings.HasPrefix(answerPrefix, sentAnswer) && len(answerPrefix) > len(sentAnswer) {
				chunk := answerPrefix[len(sentAnswer):]
				sentAnswer = answerPrefix
				if !emit("chat.chunk", map[string]any{"delta": chunk}) {
					return true
				}
			}
		case "model.done":
			if !emit("chat.status", map[string]any{
				"status":  "validating",
				"message": "模型输出完成，正在校验结构化结果",
			}) {
				return true
			}
		}
		return terminalPrestoEvent(event.Type)
	})
	if streamErr != nil {
		emitError(streamErr)
		return
	}
	finished, err := client.waitRun(ctx, run.ID)
	if err != nil {
		emitError(err)
		return
	}
	if finished.Error != "" {
		emitError(errors.New(finished.Error))
		return
	}
	if finished.Status != "completed" {
		emitError(fmt.Errorf("Presto run ended with status %s", finished.Status))
		return
	}
	response, err := buildChatResponseFromPrestoOutput(finished.Output, session.ID, run.ID)
	if err != nil {
		emitError(err)
		return
	}
	if strings.HasPrefix(response.Answer, sentAnswer) {
		if remaining := response.Answer[len(sentAnswer):]; remaining != "" {
			if !emit("chat.chunk", map[string]any{"delta": remaining}) {
				return
			}
		}
	} else if sentAnswer != response.Answer {
		if !emit("chat.reset", map[string]any{
			"status":  "validated",
			"message": "最终结构化结果已校验，正在同步完整回答",
		}) {
			return
		}
		if !emit("chat.chunk", map[string]any{"delta": response.Answer}) {
			return
		}
	}
	if !emit("chat.done", response.Payload) {
		return
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

func buildLegatoChatPrompt(req ChatRequest) (string, error) {
	common, err := readChatPromptFile("common.md")
	if err != nil {
		return "", err
	}
	template, err := readChatPromptFile("answer.md")
	if err != nil {
		return "", err
	}
	sourceContext := strings.TrimSpace(req.SourceContext)
	if sourceContext == "" {
		sourceContext = req.Question
	}
	replacements := map[string]string{
		"{{common}}":               strings.TrimSpace(common),
		"{{diagnosis_context}}":    compactChatJSON(req.Diagnosis, "{}"),
		"{{conversation_history}}": compactChatJSON(req.History, "[]"),
		"{{ui_schema_catalog}}":    compactChatJSON(req.UISchemaCatalog, "{}"),
		"{{source_context}}":       sourceContext,
		"{{question}}":             req.Question,
	}
	prompt := template
	for token, value := range replacements {
		prompt = strings.ReplaceAll(prompt, token, value)
	}
	return prompt, nil
}

func readChatPromptFile(name string) (string, error) {
	var candidates []string
	if legatoDir := strings.TrimSpace(os.Getenv("LEGATO_DIR")); legatoDir != "" {
		candidates = append(candidates, filepath.Join(legatoDir, "workflows", "chat", "prompts", name))
	}
	candidates = append(candidates,
		filepath.Join("..", "Agents", "legato", "workflows", "chat", "prompts", name),
		filepath.Join("Agents", "legato", "workflows", "chat", "prompts", name),
	)
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "..", "Agents", "legato", "workflows", "chat", "prompts", name),
			filepath.Join(wd, "Agents", "legato", "workflows", "chat", "prompts", name),
		)
	}
	for _, candidate := range candidates {
		raw, err := os.ReadFile(candidate)
		if err == nil {
			return string(raw), nil
		}
	}
	return "", fmt.Errorf("cannot locate Legato chat prompt %s", name)
}

func compactChatJSON(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	raw, err := marshalNoEscape(value)
	if err != nil {
		return fallback
	}
	return string(raw)
}

func buildChatResponseFromPrestoOutput(output string, sessionID string, runID string) (chatResponse, error) {
	chat, err := extractChatPayload(output)
	if err != nil {
		return chatResponse{}, err
	}
	answer := strings.TrimSpace(rawStringValue(chat["answer"]))
	if answer == "" {
		return chatResponse{}, errors.New("Presto chat returned empty answer")
	}
	chat = normalizePrestoChatPayload(chat, answer)
	payload := map[string]any{
		"answer":    answer,
		"chat":      chat,
		"formatter": "presto_chat_workflow_answer_stream",
		"warnings":  []any{},
		"debug": map[string]any{
			"presto_session_id": sessionID,
			"presto_run_id":     runID,
		},
	}
	return chatResponse{Answer: answer, Chat: chat, Payload: payload}, nil
}

func extractChatPayload(output string) (map[string]any, error) {
	raw, err := extractJSONObject(output)
	if err != nil {
		return nil, fmt.Errorf("Presto chat returned invalid JSON: %w", err)
	}
	if nested, ok := raw["chat"].(map[string]any); ok {
		return nested, nil
	}
	return raw, nil
}

func normalizePrestoChatPayload(chat map[string]any, answer string) map[string]any {
	out := make(map[string]any, len(chat)+4)
	for key, value := range chat {
		out[key] = value
	}
	out["answer"] = answer
	if _, ok := out["actions"].([]any); !ok {
		out["actions"] = []any{}
	}
	if _, ok := out["evidence_refs"].([]any); !ok {
		out["evidence_refs"] = []any{}
	}
	if _, ok := out["missing_evidence"].([]any); !ok {
		out["missing_evidence"] = []any{}
	}
	if _, ok := out["ui_intent"].(map[string]any); !ok {
		out["ui_intent"] = map[string]any{
			"mode":    "none",
			"target":  "none",
			"patches": []any{},
			"schema":  map[string]any{},
			"summary": "",
		}
	}
	if _, ok := out["confidence"]; !ok {
		out["confidence"] = 0.5
	}
	return out
}

func prestoChatDelta(event prestoEventResponse) (string, string) {
	if event.Type != "model.delta" || len(event.Data) == 0 {
		return "", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		return "", ""
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		data = payload
	}
	channel := strings.TrimSpace(rawStringValue(firstNonEmptyAny(data["channel"], payload["channel"])))
	text := rawStringValue(firstNonEmptyAny(data["text"], data["delta"], payload["text"], payload["delta"]))
	return channel, text
}

func chatAnswerPrefixFromJSONStream(text string) string {
	searchFrom := 0
	for {
		index := strings.Index(text[searchFrom:], `"answer"`)
		if index < 0 {
			return ""
		}
		index += searchFrom + len(`"answer"`)
		cursor := skipChatJSONSpaces(text, index)
		if cursor >= len(text) || text[cursor] != ':' {
			searchFrom = index
			continue
		}
		cursor = skipChatJSONSpaces(text, cursor+1)
		if cursor >= len(text) || text[cursor] != '"' {
			return ""
		}
		return partialJSONStringValue(text[cursor+1:])
	}
}

func skipChatJSONSpaces(text string, index int) int {
	for index < len(text) {
		switch text[index] {
		case ' ', '\n', '\r', '\t':
			index++
		default:
			return index
		}
	}
	return index
}

func partialJSONStringValue(text string) string {
	var out strings.Builder
	for len(text) > 0 {
		if text[0] == '"' {
			return out.String()
		}
		value, _, tail, err := strconv.UnquoteChar(text, '"')
		if err != nil {
			return out.String()
		}
		out.WriteRune(value)
		text = tail
	}
	return out.String()
}

func rawStringValue(value any) string {
	text, _ := value.(string)
	return text
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
