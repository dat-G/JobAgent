package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"presto/internal/agent"
)

type OpenAIChat struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewOpenAIChat(baseURL string, apiKey string, client *http.Client) *OpenAIChat {
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	return &OpenAIChat{
		endpoint: normalizeChatEndpoint(baseURL),
		apiKey:   apiKey,
		client:   client,
	}
}

func (p *OpenAIChat) Chat(ctx context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	if p.apiKey == "" {
		return agent.ChatResponse{}, errors.New("api key is required")
	}
	if req.Model == "" {
		return agent.ChatResponse{}, errors.New("model is required")
	}

	payload := chatPayload{
		Model:       req.Model,
		Messages:    convertMessages(req.Messages),
		Tools:       convertTools(req.Tools),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	if thinking := os.Getenv("PRESTO_THINKING"); thinking != "" {
		payload.Thinking = &chatThinking{Type: thinking}
	}
	if effort := os.Getenv("PRESTO_REASONING_EFFORT"); effort != "" {
		payload.ReasoningEffort = effort
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return agent.ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return agent.ChatResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return agent.ChatResponse{}, err
	}
	defer httpResp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(httpResp.Body, 8<<20))
	if err != nil {
		return agent.ChatResponse{}, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return agent.ChatResponse{}, fmt.Errorf("chat completion failed: status=%d body=%s", httpResp.StatusCode, string(data))
	}

	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return agent.ChatResponse{}, err
	}
	if len(parsed.Choices) == 0 {
		return agent.ChatResponse{}, errors.New("chat completion returned no choices")
	}

	message := agent.Message{
		Role:             parsed.Choices[0].Message.Role,
		Content:          parsed.Choices[0].Message.Content,
		ReasoningContent: parsed.Choices[0].Message.ReasoningContent,
	}
	for _, call := range parsed.Choices[0].Message.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, agent.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}

	return agent.ChatResponse{
		Message: message,
		Usage: agent.Usage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		},
		Raw: data,
	}, nil
}

func (p *OpenAIChat) ChatStream(ctx context.Context, req agent.ChatRequest, onDelta agent.ChatStreamHandler) (agent.ChatResponse, error) {
	if p.apiKey == "" {
		return agent.ChatResponse{}, errors.New("api key is required")
	}
	if req.Model == "" {
		return agent.ChatResponse{}, errors.New("model is required")
	}

	payload := chatPayload{
		Model:       req.Model,
		Messages:    convertMessages(req.Messages),
		Tools:       convertTools(req.Tools),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
	}
	if thinking := os.Getenv("PRESTO_THINKING"); thinking != "" {
		payload.Thinking = &chatThinking{Type: thinking}
	}
	if effort := os.Getenv("PRESTO_REASONING_EFFORT"); effort != "" {
		payload.ReasoningEffort = effort
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return agent.ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return agent.ChatResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return agent.ChatResponse{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(httpResp.Body, 8<<20))
		return agent.ChatResponse{}, fmt.Errorf("chat completion stream failed: status=%d body=%s", httpResp.StatusCode, string(data))
	}

	response, err := parseChatStream(httpResp.Body, onDelta, streamReasoningEnabled())
	if err != nil {
		return agent.ChatResponse{}, err
	}
	return response, nil
}

func parseChatStream(body io.Reader, onDelta agent.ChatStreamHandler, emitReasoning bool) (agent.ChatResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var dataLines []string
	var role string
	var content strings.Builder
	var reasoning strings.Builder
	var usage agent.Usage
	toolDeltas := make(map[int]*chatToolCall)
	received := false

	dispatch := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		rawText := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = nil
		if rawText == "" || rawText == "[DONE]" {
			return nil
		}
		raw := json.RawMessage(rawText)
		var chunk chatStreamChunk
		if err := json.Unmarshal(raw, &chunk); err != nil {
			return err
		}
		received = true
		if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 || chunk.Usage.TotalTokens != 0 {
			usage = agent.Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta
			if delta.Role != "" {
				role = delta.Role
			}
			if delta.ReasoningContent != "" {
				reasoning.WriteString(delta.ReasoningContent)
				if emitReasoning && onDelta != nil {
					onDelta(agent.ChatStreamDelta{ReasoningContent: delta.ReasoningContent, Raw: raw})
				}
			}
			if delta.Content != "" {
				content.WriteString(delta.Content)
				if onDelta != nil {
					onDelta(agent.ChatStreamDelta{Content: delta.Content, Raw: raw})
				}
			}
			mergeToolCallDeltas(toolDeltas, delta.ToolCalls)
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return agent.ChatResponse{}, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return agent.ChatResponse{}, err
	}
	if err := dispatch(); err != nil {
		return agent.ChatResponse{}, err
	}
	if !received {
		return agent.ChatResponse{}, errors.New("chat completion stream returned no chunks")
	}
	if role == "" {
		role = agent.RoleAssistant
	}

	message := agent.Message{
		Role:             role,
		Content:          content.String(),
		ReasoningContent: reasoning.String(),
		ToolCalls:        assembledToolCalls(toolDeltas),
	}
	raw, _ := json.Marshal(chatResponse{
		Choices: []struct {
			Message chatMessage `json:"message"`
		}{{
			Message: chatMessage{
				Role:             message.Role,
				Content:          message.Content,
				ReasoningContent: message.ReasoningContent,
				ToolCalls:        convertAgentToolCalls(message.ToolCalls),
			},
		}},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
		},
	})
	return agent.ChatResponse{Message: message, Usage: usage, Raw: raw}, nil
}

func mergeToolCallDeltas(toolDeltas map[int]*chatToolCall, deltas []chatToolCall) {
	for _, delta := range deltas {
		index := delta.Index
		current := toolDeltas[index]
		if current == nil {
			current = &chatToolCall{Index: index}
			toolDeltas[index] = current
		}
		if delta.ID != "" {
			current.ID = delta.ID
		}
		if delta.Type != "" {
			current.Type = delta.Type
		}
		if delta.Function.Name != "" {
			current.Function.Name += delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			current.Function.Arguments += delta.Function.Arguments
		}
	}
}

func assembledToolCalls(toolDeltas map[int]*chatToolCall) []agent.ToolCall {
	if len(toolDeltas) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(toolDeltas))
	for index := range toolDeltas {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	calls := make([]agent.ToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := toolDeltas[index]
		if call == nil {
			continue
		}
		calls = append(calls, agent.ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	return calls
}

func convertAgentToolCalls(calls []agent.ToolCall) []chatToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]chatToolCall, 0, len(calls))
	for index, call := range calls {
		out = append(out, chatToolCall{
			Index: index,
			ID:    call.ID,
			Type:  "function",
			Function: chatToolFunctionCall{
				Name:      call.Name,
				Arguments: string(call.Arguments),
			},
		})
	}
	return out
}

func streamReasoningEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PRESTO_STREAM_REASONING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeChatEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/chat/completions"
	}
	return baseURL + "/chat/completions"
}

func convertMessages(messages []agent.Message) []chatMessage {
	out := make([]chatMessage, 0, len(messages))
	for _, msg := range messages {
		converted := chatMessage{
			Role:             msg.Role,
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			Name:             msg.Name,
			ToolCallID:       msg.ToolCallID,
		}
		for _, call := range msg.ToolCalls {
			converted.ToolCalls = append(converted.ToolCalls, chatToolCall{
				ID:   call.ID,
				Type: "function",
				Function: chatToolFunctionCall{
					Name:      call.Name,
					Arguments: string(call.Arguments),
				},
			})
		}
		out = append(out, converted)
	}
	return out
}

func convertTools(specs []agent.ToolSpec) []chatTool {
	if len(specs) == 0 {
		return nil
	}
	out := make([]chatTool, 0, len(specs))
	for _, spec := range specs {
		params := spec.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`)
		}
		out = append(out, chatTool{
			Type: "function",
			Function: chatFunctionTool{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

type chatPayload struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Tools           []chatTool    `json:"tools,omitempty"`
	Temperature     float64       `json:"temperature"`
	MaxTokens       int           `json:"max_tokens,omitempty"`
	Thinking        *chatThinking `json:"thinking,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Stream          bool          `json:"stream,omitempty"`
}

type chatThinking struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role             string         `json:"role"`
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	Name             string         `json:"name,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatFunctionTool `json:"function"`
}

type chatFunctionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type chatStreamChunk struct {
	Choices []struct {
		Delta chatMessage `json:"delta"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type chatToolCall struct {
	Index    int                  `json:"index,omitempty"`
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolFunctionCall `json:"function"`
}

type chatToolFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
