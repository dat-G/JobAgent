package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Runner struct {
	config   Config
	provider Provider
	store    SessionStore
	tools    *ToolRegistry
	prompt   PromptBuilder
}

func NewRunner(config Config, provider Provider, store SessionStore) (*Runner, error) {
	if provider == nil {
		return nil, errors.New("provider is required")
	}
	if store == nil {
		store = NewMemoryStore()
	}
	if config.Name == "" {
		config.Name = "PrestoAgent"
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = 8
	}
	if config.LLMRetry.MaxAttempts == 0 {
		config.LLMRetry = FastRetry()
	}
	if config.RunRetry.MaxAttempts == 0 {
		config.RunRetry = RunRetry()
	}

	registry, err := NewToolRegistry(config.Tools...)
	if err != nil {
		return nil, err
	}

	return &Runner{
		config:   config,
		provider: provider,
		store:    store,
		tools:    registry,
		prompt: PromptBuilder{
			System: CompactSystemPrompt(config.Name, config.Instructions),
		},
	}, nil
}

func (r *Runner) CreateSession(ctx context.Context) (Session, error) {
	return r.store.CreateSession(ctx)
}

func (r *Runner) Session(ctx context.Context, id string) (Session, error) {
	return r.store.GetSession(ctx, id)
}

func (r *Runner) Run(ctx context.Context, input RunInput) (RunResult, error) {
	events := make(chan Event, 1)
	defer close(events)
	return r.runWithRetry(ctx, input, events)
}

func (r *Runner) RunStream(ctx context.Context, input RunInput) (<-chan Event, <-chan RunResult) {
	events := make(chan Event, 64)
	results := make(chan RunResult, 1)

	go func() {
		defer close(events)
		defer close(results)
		result, err := r.runWithRetry(ctx, input, events)
		if err != nil {
			events <- Event{
				Type:      EventRunError,
				RunID:     input.RunID,
				SessionID: input.SessionID,
				Message:   err.Error(),
				Time:      time.Now().UTC(),
			}
			return
		}
		results <- result
	}()

	return events, results
}

func (r *Runner) runWithRetry(ctx context.Context, input RunInput, events chan<- Event) (RunResult, error) {
	if input.RunID == "" {
		input.RunID = NewID("run")
	}

	attempts := r.config.RunRetry.attempts()
	var baseline *Session
	if input.SessionID != "" {
		session, err := r.store.GetSession(ctx, input.SessionID)
		if err != nil {
			return RunResult{}, err
		}
		snapshot := cloneSession(session)
		baseline = &snapshot
	}

	var lastErr error
	attemptsUsed := 0
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptsUsed = attempt
		if attempt > 1 {
			if err := r.restoreRunSession(ctx, baseline); err != nil {
				return RunResult{}, err
			}
		}

		result, err := r.run(ctx, input, events)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == attempts {
			break
		}

		emit(events, Event{
			Type:      EventRunRetry,
			RunID:     input.RunID,
			SessionID: input.SessionID,
			Message:   err.Error(),
			Data: map[string]any{
				"attempt":      attempt,
				"next_attempt": attempt + 1,
				"max_attempts": attempts,
			},
			Time: time.Now().UTC(),
		})
		if err := sleepRetryDelay(ctx, r.config.RunRetry, attempt); err != nil {
			return RunResult{}, err
		}
	}

	if lastErr == nil {
		lastErr = errors.New("runner failed without result")
	}
	if attemptsUsed < attempts {
		return RunResult{}, fmt.Errorf("runner failed after %d/%d attempts: %w", attemptsUsed, attempts, lastErr)
	}
	return RunResult{}, fmt.Errorf("runner failed after %d attempts: %w", attempts, lastErr)
}

func (r *Runner) restoreRunSession(ctx context.Context, baseline *Session) error {
	if baseline == nil {
		return nil
	}
	return r.store.SaveSession(ctx, cloneSession(*baseline))
}

func sleepRetryDelay(ctx context.Context, policy RetryPolicy, failedAttempt int) error {
	delay := policy.delay(failedAttempt)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *Runner) run(ctx context.Context, input RunInput, events chan<- Event) (RunResult, error) {
	runID := input.RunID
	if runID == "" {
		runID = NewID("run")
	}

	session, err := r.loadSession(ctx, input.SessionID)
	if err != nil {
		return RunResult{}, err
	}
	sessionID := session.ID

	emit(events, Event{Type: EventRunStarted, RunID: runID, SessionID: sessionID})

	if input.UserInput != "" {
		session, err = r.appendMessages(ctx, sessionID, Message{
			Role:      RoleUser,
			Content:   input.UserInput,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			return RunResult{}, err
		}
	}

	var usage Usage
	for step := 1; step <= r.config.MaxSteps; step++ {
		session, err = r.store.GetSession(ctx, sessionID)
		if err != nil {
			return RunResult{}, err
		}
		emit(events, Event{Type: EventModelStarted, RunID: runID, SessionID: sessionID, Step: step})
		request := ChatRequest{
			Model:       r.config.Model,
			Messages:    r.prompt.Build(session.Messages),
			Tools:       r.tools.Specs(),
			Temperature: r.config.Temperature,
			MaxTokens:   r.config.MaxTokens,
			Stream:      input.Stream,
		}
		response, err := r.chat(ctx, request, runID, sessionID, step, events)
		if err != nil {
			return RunResult{}, err
		}
		usage.PromptTokens += response.Usage.PromptTokens
		usage.CompletionTokens += response.Usage.CompletionTokens
		usage.TotalTokens += response.Usage.TotalTokens

		response.Message.Role = RoleAssistant
		response.Message.CreatedAt = time.Now().UTC()
		session, err = r.appendMessages(ctx, sessionID, response.Message)
		if err != nil {
			return RunResult{}, err
		}
		emit(events, Event{
			Type:      EventModelDone,
			RunID:     runID,
			SessionID: sessionID,
			Step:      step,
			Data: map[string]any{
				"tool_calls": len(response.Message.ToolCalls),
				"usage":      response.Usage,
			},
		})

		if len(response.Message.ToolCalls) == 0 {
			emit(events, Event{Type: EventRunDone, RunID: runID, SessionID: sessionID, Step: step})
			return RunResult{
				RunID:     runID,
				SessionID: sessionID,
				Output:    response.Message.Content,
				Messages:  append([]Message(nil), session.Messages...),
				Usage:     usage,
				Steps:     step,
			}, nil
		}

		toolMessages := r.executeToolCalls(ctx, runID, sessionID, step, response.Message.ToolCalls, events)
		session, err = r.appendMessages(ctx, sessionID, toolMessages...)
		if err != nil {
			return RunResult{}, err
		}
	}

	return RunResult{}, fmt.Errorf("agent stopped after max steps: %d", r.config.MaxSteps)
}

func (r *Runner) chat(ctx context.Context, req ChatRequest, runID string, sessionID string, step int, events chan<- Event) (ChatResponse, error) {
	if req.Stream {
		if streaming, ok := r.provider.(StreamingProvider); ok {
			return streaming.ChatStream(ctx, req, func(delta ChatStreamDelta) {
				if delta.Content != "" {
					emit(events, Event{
						Type:      EventModelDelta,
						RunID:     runID,
						SessionID: sessionID,
						Step:      step,
						Data: map[string]any{
							"channel": "content",
							"text":    delta.Content,
						},
					})
				}
				if delta.ReasoningContent != "" {
					emit(events, Event{
						Type:      EventModelDelta,
						RunID:     runID,
						SessionID: sessionID,
						Step:      step,
						Data: map[string]any{
							"channel": "reasoning",
							"text":    delta.ReasoningContent,
						},
					})
				}
			})
		}
	}
	return withRetry(ctx, r.config.LLMRetry, func(ctx context.Context) (ChatResponse, error) {
		return r.provider.Chat(ctx, req)
	})
}

func (r *Runner) loadSession(ctx context.Context, id string) (Session, error) {
	if id == "" {
		return r.store.CreateSession(ctx)
	}
	return r.store.GetSession(ctx, id)
}

func (r *Runner) appendMessages(ctx context.Context, sessionID string, messages ...Message) (Session, error) {
	if len(messages) == 0 {
		return r.store.GetSession(ctx, sessionID)
	}
	if updater, ok := r.store.(SessionUpdater); ok {
		return updater.UpdateSession(ctx, sessionID, func(session Session) (Session, error) {
			session.Messages = append(session.Messages, messages...)
			return session, nil
		})
	}

	session, err := r.store.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	session.Messages = append(session.Messages, messages...)
	if err := r.store.SaveSession(ctx, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (r *Runner) executeToolCalls(ctx context.Context, runID string, sessionID string, step int, calls []ToolCall, events chan<- Event) []Message {
	type result struct {
		index   int
		message Message
	}

	results := make([]Message, len(calls))
	resultCh := make(chan result, len(calls))

	var wg sync.WaitGroup
	for index, call := range calls {
		wg.Add(1)
		go func(index int, call ToolCall) {
			defer wg.Done()
			emit(events, Event{
				Type:      EventToolStarted,
				RunID:     runID,
				SessionID: sessionID,
				Step:      step,
				ToolName:  call.Name,
			})
			value, err := r.tools.Execute(ctx, call)
			toolMessage := Message{
				Role:       RoleTool,
				ToolCallID: call.ID,
				Name:       call.Name,
				CreatedAt:  time.Now().UTC(),
			}
			if err != nil {
				toolMessage.Content = JSONContent(map[string]any{"error": err.Error()})
				emit(events, Event{
					Type:      EventToolError,
					RunID:     runID,
					SessionID: sessionID,
					Step:      step,
					ToolName:  call.Name,
					Message:   err.Error(),
				})
			} else {
				toolMessage.Content = JSONContent(value)
				emit(events, Event{
					Type:      EventToolDone,
					RunID:     runID,
					SessionID: sessionID,
					Step:      step,
					ToolName:  call.Name,
				})
			}
			resultCh <- result{index: index, message: toolMessage}
		}(index, call)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()
	for result := range resultCh {
		results[result.index] = result.message
	}
	return results
}

func emit(events chan<- Event, event Event) {
	event.Time = time.Now().UTC()
	select {
	case events <- event:
	default:
	}
}
