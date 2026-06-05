package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
		Role:    parsed.Choices[0].Message.Role,
		Content: parsed.Choices[0].Message.Content,
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
			Role:       msg.Role,
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
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
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
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

type chatToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolFunctionCall `json:"function"`
}

type chatToolFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
