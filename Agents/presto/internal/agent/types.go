package agent

import (
	"context"
	"encoding/json"
	"time"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Name             string     `json:"name,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	CreatedAt        time.Time  `json:"created_at,omitempty"`
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ToolHandler func(context.Context, json.RawMessage) (any, error)

type Tool struct {
	Spec    ToolSpec
	Handler ToolHandler
	Retry   RetryPolicy
}

type Config struct {
	Name         string
	Model        string
	Instructions string
	Tools        []Tool
	MaxSteps     int
	Temperature  float64
	MaxTokens    int
	LLMRetry     RetryPolicy
	RunRetry     RetryPolicy
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolSpec
	Temperature float64
	MaxTokens   int
	Stream      bool
}

type ChatResponse struct {
	Message Message
	Usage   Usage
	Raw     json.RawMessage
}

type ChatStreamDelta struct {
	Content          string
	ReasoningContent string
	Raw              json.RawMessage
}

type ChatStreamHandler func(ChatStreamDelta)

type Provider interface {
	Chat(context.Context, ChatRequest) (ChatResponse, error)
}

type StreamingProvider interface {
	Provider
	ChatStream(context.Context, ChatRequest, ChatStreamHandler) (ChatResponse, error)
}

type EventType string

const (
	EventRunStarted   EventType = "run.started"
	EventModelStarted EventType = "model.started"
	EventModelDelta   EventType = "model.delta"
	EventModelDone    EventType = "model.done"
	EventToolStarted  EventType = "tool.started"
	EventToolDone     EventType = "tool.done"
	EventToolError    EventType = "tool.error"
	EventRunDone      EventType = "run.done"
	EventRunError     EventType = "run.error"
	EventRunRetry     EventType = "run.retry"
)

type Event struct {
	Type      EventType      `json:"type"`
	RunID     string         `json:"run_id"`
	SessionID string         `json:"session_id"`
	Step      int            `json:"step,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Message   string         `json:"message,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Time      time.Time      `json:"time"`
}

type RunInput struct {
	SessionID string
	UserInput string
	RunID     string
	Stream    bool
}

type RunResult struct {
	RunID     string    `json:"run_id"`
	SessionID string    `json:"session_id"`
	Output    string    `json:"output"`
	Messages  []Message `json:"messages"`
	Usage     Usage     `json:"usage"`
	Steps     int       `json:"steps"`
}
