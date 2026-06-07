package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPromptBuilderPrependsSystemAndKeepsTurnOrder(t *testing.T) {
	history := []Message{
		{Role: RoleUser, Content: "remember Go roles"},
		{Role: RoleAssistant, Content: "noted"},
		{Role: RoleUser, Content: "what did I ask for?"},
	}

	messages := PromptBuilder{System: "  system instructions  "}.Build(history)
	assertMessageRoles(t, messages, RoleSystem, RoleUser, RoleAssistant, RoleUser)
	if messages[0].Content != "system instructions" {
		t.Fatalf("system prompt was not trimmed: %q", messages[0].Content)
	}
	if messages[1].Content != history[0].Content || messages[3].Content != history[2].Content {
		t.Fatalf("history order changed: %#v", messages)
	}

	messages = PromptBuilder{System: "   "}.Build(history)
	assertMessageRoles(t, messages, RoleUser, RoleAssistant, RoleUser)
}

func TestRunnerPersistsMultiTurnSessionAndBuildsPrompt(t *testing.T) {
	ctx := context.Background()
	provider := &recordingProvider{}
	provider.chat = func(ctx context.Context, req ChatRequest, call int) (ChatResponse, error) {
		switch call {
		case 1:
			return ChatResponse{Message: Message{Content: "remembered Go roles"}}, nil
		case 2:
			return ChatResponse{Message: Message{Content: "you prefer Go roles"}}, nil
		default:
			t.Fatalf("unexpected provider call %d", call)
			return ChatResponse{}, nil
		}
	}

	runner, err := NewRunner(Config{
		Name:         "UnitAgent",
		Model:        "unit-model",
		Instructions: "Track job-search preferences.",
		MaxSteps:     4,
		LLMRetry:     NoRetry(),
	}, provider, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}

	session, err := runner.CreateSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := runner.Run(ctx, RunInput{SessionID: session.ID, UserInput: "Remember I prefer Go roles."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Run(ctx, RunInput{SessionID: session.ID, UserInput: "What do I prefer?"})
	if err != nil {
		t.Fatal(err)
	}

	if first.SessionID != session.ID || second.SessionID != session.ID {
		t.Fatalf("results should keep session id %q, got %q and %q", session.ID, first.SessionID, second.SessionID)
	}
	if second.Output != "you prefer Go roles" {
		t.Fatalf("second output = %q", second.Output)
	}
	assertMessageRoles(t, second.Messages, RoleUser, RoleAssistant, RoleUser, RoleAssistant)

	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(requests))
	}
	if requests[0].Model != "unit-model" || requests[1].Model != "unit-model" {
		t.Fatalf("model was not passed through: %#v", requests)
	}
	assertMessageRoles(t, requests[0].Messages, RoleSystem, RoleUser)
	assertMessageRoles(t, requests[1].Messages, RoleSystem, RoleUser, RoleAssistant, RoleUser)
	if !strings.Contains(requests[1].Messages[0].Content, "You are UnitAgent.") {
		t.Fatalf("system prompt missing agent name: %q", requests[1].Messages[0].Content)
	}
	if !strings.Contains(requests[1].Messages[0].Content, "Track job-search preferences.") {
		t.Fatalf("system prompt missing custom instructions: %q", requests[1].Messages[0].Content)
	}
}

func TestRunnerRetriesProviderErrors(t *testing.T) {
	ctx := context.Background()
	provider := &recordingProvider{}
	provider.chat = func(ctx context.Context, req ChatRequest, call int) (ChatResponse, error) {
		if call < 3 {
			return ChatResponse{}, errors.New("temporary provider failure")
		}
		return ChatResponse{Message: Message{Content: "ok"}}, nil
	}

	runner, err := NewRunner(Config{
		Model:    "unit-model",
		MaxSteps: 2,
		LLMRetry: RetryPolicy{
			MaxAttempts: 3,
		},
	}, provider, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.Run(ctx, RunInput{UserInput: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "ok" {
		t.Fatalf("output = %q, want ok", result.Output)
	}
	if got := len(provider.Requests()); got != 3 {
		t.Fatalf("provider attempts = %d, want 3", got)
	}
}

func TestRunnerEmitsModelDeltaEventsWhenStreaming(t *testing.T) {
	ctx := context.Background()
	provider := &recordingProvider{}
	provider.stream = func(ctx context.Context, req ChatRequest, call int, onDelta ChatStreamHandler) (ChatResponse, error) {
		onDelta(ChatStreamDelta{Content: "hello"})
		onDelta(ChatStreamDelta{Content: " world"})
		return ChatResponse{Message: Message{Content: "hello world"}}, nil
	}

	runner, err := NewRunner(Config{
		Model:    "unit-model",
		MaxSteps: 2,
		LLMRetry: NoRetry(),
	}, provider, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}

	events, results := runner.RunStream(ctx, RunInput{UserInput: "hello", Stream: true})
	var deltas []string
	for event := range events {
		if event.Type != EventModelDelta {
			continue
		}
		if event.Data["channel"] != "content" {
			t.Fatalf("delta channel = %#v, want content", event.Data["channel"])
		}
		deltas = append(deltas, event.Data["text"].(string))
	}
	result := <-results
	if result.Output != "hello world" {
		t.Fatalf("output = %q, want hello world", result.Output)
	}
	if len(deltas) != 2 || deltas[0] != "hello" || deltas[1] != " world" {
		t.Fatalf("deltas = %#v", deltas)
	}
	requests := provider.Requests()
	if len(requests) != 1 || !requests[0].Stream {
		t.Fatalf("stream request not recorded: %#v", requests)
	}
}

func TestRunnerExecutesToolLoopAndSubmitsToolResult(t *testing.T) {
	ctx := context.Background()
	provider := &recordingProvider{}
	provider.chat = func(ctx context.Context, req ChatRequest, call int) (ChatResponse, error) {
		switch call {
		case 1:
			return ChatResponse{
				Message: Message{
					ToolCalls: []ToolCall{{
						ID:        "call_add_1",
						Name:      "add",
						Arguments: json.RawMessage(`{"a":2,"b":5}`),
					}},
				},
				Usage: Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
			}, nil
		case 2:
			return ChatResponse{
				Message: Message{Content: "7"},
				Usage:   Usage{PromptTokens: 5, CompletionTokens: 6, TotalTokens: 11},
			}, nil
		default:
			t.Fatalf("unexpected provider call %d", call)
			return ChatResponse{}, nil
		}
	}

	var toolAttempts int
	addTool := Tool{
		Spec: ToolSpec{
			Name:       "add",
			Parameters: json.RawMessage(`{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}},"required":["a","b"]}`),
		},
		Retry: RetryPolicy{MaxAttempts: 2},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			toolAttempts++
			if toolAttempts == 1 {
				return nil, errors.New("transient tool failure")
			}
			var args struct {
				A int `json:"a"`
				B int `json:"b"`
			}
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, err
			}
			return args.A + args.B, nil
		},
	}

	runner, err := NewRunner(Config{
		Model:    "unit-model",
		Tools:    []Tool{addTool},
		MaxSteps: 4,
		LLMRetry: NoRetry(),
	}, provider, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.Run(ctx, RunInput{UserInput: "Add 2 and 5."})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "7" {
		t.Fatalf("output = %q, want 7", result.Output)
	}
	if result.Steps != 2 {
		t.Fatalf("steps = %d, want 2", result.Steps)
	}
	if toolAttempts != 2 {
		t.Fatalf("tool attempts = %d, want 2", toolAttempts)
	}
	if result.Usage.TotalTokens != 18 {
		t.Fatalf("total usage = %d, want 18", result.Usage.TotalTokens)
	}

	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(requests))
	}
	if len(requests[0].Tools) != 1 || requests[0].Tools[0].Name != "add" {
		t.Fatalf("first request tools = %#v, want add tool", requests[0].Tools)
	}
	secondPrompt := requests[1].Messages
	assertMessageRoles(t, secondPrompt, RoleSystem, RoleUser, RoleAssistant, RoleTool)
	assistant := secondPrompt[len(secondPrompt)-2]
	toolResult := secondPrompt[len(secondPrompt)-1]
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID != "call_add_1" {
		t.Fatalf("assistant tool call transcript missing: %#v", assistant)
	}
	if toolResult.ToolCallID != "call_add_1" || toolResult.Name != "add" || toolResult.Content != "7" {
		t.Fatalf("tool result transcript mismatch: %#v", toolResult)
	}
}

func TestConcurrentRunsOnSameSessionDoNotDropMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var ready sync.WaitGroup
	ready.Add(2)
	release := make(chan struct{})
	provider := &recordingProvider{}
	provider.chat = func(ctx context.Context, req ChatRequest, call int) (ChatResponse, error) {
		ready.Done()
		select {
		case <-release:
		case <-ctx.Done():
			return ChatResponse{}, ctx.Err()
		}
		return ChatResponse{Message: Message{Content: "ok: " + lastUserContent(req.Messages)}}, nil
	}

	runner, err := NewRunner(Config{
		Model:    "unit-model",
		MaxSteps: 2,
		LLMRetry: NoRetry(),
	}, provider, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	session, err := runner.CreateSession(ctx)
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 2)
	for _, input := range []string{"first", "second"} {
		go func(input string) {
			_, err := runner.Run(ctx, RunInput{SessionID: session.ID, UserInput: input})
			errs <- err
		}(input)
	}
	ready.Wait()
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	stored, err := runner.Session(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	countByRole := map[string]int{}
	seen := map[string]bool{}
	for _, message := range stored.Messages {
		countByRole[message.Role]++
		seen[message.Content] = true
	}
	if countByRole[RoleUser] != 2 || countByRole[RoleAssistant] != 2 {
		t.Fatalf("stored messages = %#v", stored.Messages)
	}
	for _, want := range []string{"first", "second", "ok: first", "ok: second"} {
		if !seen[want] {
			t.Fatalf("stored messages missing %q: %#v", want, stored.Messages)
		}
	}
}

func TestRunnerExecutesMultipleToolCallsConcurrentlyAndPreservesOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	provider := &recordingProvider{}
	provider.chat = func(ctx context.Context, req ChatRequest, call int) (ChatResponse, error) {
		switch call {
		case 1:
			return ChatResponse{Message: Message{ToolCalls: []ToolCall{
				{ID: "call_slow", Name: "slow", Arguments: json.RawMessage(`{}`)},
				{ID: "call_fast", Name: "fast", Arguments: json.RawMessage(`{}`)},
			}}}, nil
		case 2:
			return ChatResponse{Message: Message{Content: "done"}}, nil
		default:
			t.Fatalf("unexpected provider call %d", call)
			return ChatResponse{}, nil
		}
	}

	started := make(chan string, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	go func() {
		<-started
		<-started
		releaseOnce.Do(func() { close(release) })
	}()
	defer releaseOnce.Do(func() { close(release) })

	makeTool := func(name string) Tool {
		return Tool{
			Spec: ToolSpec{Name: name},
			Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				started <- name
				select {
				case <-release:
					return name + "-result", nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}
	}

	runner, err := NewRunner(Config{
		Model:    "unit-model",
		Tools:    []Tool{makeTool("slow"), makeTool("fast")},
		MaxSteps: 3,
		LLMRetry: NoRetry(),
	}, provider, NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.Run(ctx, RunInput{UserInput: "call both tools"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "done" {
		t.Fatalf("output = %q", result.Output)
	}

	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(requests))
	}
	prompt := requests[1].Messages
	assertMessageRoles(t, prompt, RoleSystem, RoleUser, RoleAssistant, RoleTool, RoleTool)
	if prompt[len(prompt)-2].ToolCallID != "call_slow" || prompt[len(prompt)-1].ToolCallID != "call_fast" {
		t.Fatalf("tool result order changed: %#v", prompt[len(prompt)-2:])
	}
}

func TestToolSpecsAreSortedByName(t *testing.T) {
	registry, err := NewToolRegistry(
		Tool{Spec: ToolSpec{Name: "zeta"}, Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }},
		Tool{Spec: ToolSpec{Name: "alpha"}, Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }},
	)
	if err != nil {
		t.Fatal(err)
	}
	specs := registry.Specs()
	if len(specs) != 2 {
		t.Fatalf("specs = %d, want 2", len(specs))
	}
	if specs[0].Name != "alpha" || specs[1].Name != "zeta" {
		t.Fatalf("spec order = %#v", specs)
	}
}

type recordingProvider struct {
	mu       sync.Mutex
	requests []ChatRequest
	chat     func(context.Context, ChatRequest, int) (ChatResponse, error)
	stream   func(context.Context, ChatRequest, int, ChatStreamHandler) (ChatResponse, error)
}

func lastUserContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

func (p *recordingProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, cloneChatRequest(req))
	call := len(p.requests)
	chat := p.chat
	p.mu.Unlock()

	if chat == nil {
		return ChatResponse{}, errors.New("chat function is not configured")
	}
	return chat(ctx, req, call)
}

func (p *recordingProvider) ChatStream(ctx context.Context, req ChatRequest, onDelta ChatStreamHandler) (ChatResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, cloneChatRequest(req))
	call := len(p.requests)
	stream := p.stream
	chat := p.chat
	p.mu.Unlock()

	if stream == nil {
		if chat == nil {
			return ChatResponse{}, errors.New("stream function is not configured")
		}
		return chat(ctx, req, call)
	}
	return stream(ctx, req, call, onDelta)
}

func (p *recordingProvider) Requests() []ChatRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]ChatRequest, len(p.requests))
	for i, req := range p.requests {
		out[i] = cloneChatRequest(req)
	}
	return out
}

func cloneChatRequest(req ChatRequest) ChatRequest {
	clone := req
	clone.Messages = append([]Message(nil), req.Messages...)
	clone.Tools = append([]ToolSpec(nil), req.Tools...)
	return clone
}

func assertMessageRoles(t *testing.T, messages []Message, roles ...string) {
	t.Helper()
	if len(messages) != len(roles) {
		t.Fatalf("message count = %d, want %d: %#v", len(messages), len(roles), messages)
	}
	for i, role := range roles {
		if messages[i].Role != role {
			t.Fatalf("message %d role = %q, want %q", i, messages[i].Role, role)
		}
	}
}
